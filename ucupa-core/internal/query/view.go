package query

import (
	"math"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	commitpkg "ucupa/ucupa-core/internal/commit"
	"ucupa/ucupa-core/internal/token"
)

// readView owns only spatial records and the bounded current mount evidence
// required to interpret them. Published record maps are immutable; sharing those
// maps does not retain the publishing coreState or its unbounded history.
type readView struct {
	projection *commitpkg.ReadView
	candidate  *commitpkg.ReadCandidate
	cut        contract.CoreReadCut
	run        [32]byte
	bytes      uint64
	refs       uint32
	borrows    uint32
	ready      bool
}

type Lease struct {
	core     *Operations
	view     *readView
	snapshot *contract.CoreReadSnapshot
	mode     contract.QueryReadMode
	scope    values.ElementID
	done     bool
}

func validateReadView(view *readView) error {
	if view == nil || view.projection == nil || !token.Valid(view.cut) || view.projection.CurrentCut() != view.cut ||
		!view.projection.Authority.Valid() || view.projection.Scope == ([32]byte{}) || !view.projection.RevisionsValid() ||
		commitpkg.ValidateMountSnapshotProjection(*view.projection) != nil {
		return contract.ErrStore
	}
	if err := view.projection.Elements.Validate(view.projection.Scope, view.projection.RevisionSerial, commitpkg.ElementExternalParentForView(view.projection.Mount)); err != nil {
		return contract.ErrStore
	}
	return nil
}

// The accounting unit includes map entries, UUIDs, scalar geometry and exact
// attachment projections at 1536 logical bytes per live record, plus 64 KiB per current mount
// record/evidence. It is a declared admission measure, not an exact RSS bound.
func readViewBytes(projection commitpkg.ReadView) (uint64, bool) {
	n := uint64(projection.Elements.Counts().Anchors) + uint64(projection.Elements.Counts().Entities)
	m := uint64(projection.Mount.OutgoingCount()) + 1
	if n > (math.MaxUint64-1024)/1536 || m > (math.MaxUint64-1024-n*1536)/(64<<10) {
		return 0, false
	}
	base := 1024 + n*1536 + m*(64<<10)
	inactive, fences, parents := projection.Transfers.Counts()
	if uint64(inactive) > (math.MaxUint64-base)/128 {
		return 0, false
	}
	base += uint64(inactive) * 128
	if uint64(fences) > (math.MaxUint64-base)/512 {
		return 0, false
	}
	base += uint64(fences) * 512
	// Exact held relationships contain a bounded contact and no journal.
	if uint64(parents) > (math.MaxUint64-base)/(64<<10) {
		return 0, false
	}
	return base + uint64(parents)*(64<<10), true
}

// captureReadView is private owner plumbing. Its caller has already checked
// the relevant authorization. CurrentVerified may be selected even when this
// owner supports retention (a mixed-owner connected query uses that policy).
func (core *Operations) Capture(actor values.PrincipalRef, scope values.ElementID, at *contract.CoreReadCut, mode contract.QueryReadMode) (*Lease, error) {
	if !actor.Valid() || !scope.Valid() || !token.ValidOptionalCut(at) ||
		(mode != contract.QueryCurrentVerified && mode != contract.QueryRetained) {
		return nil, contract.ErrInvalid
	}
	core.owner.RLock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		core.owner.RUnlock()
		return nil, contract.ErrStore
	}
	cut := core.owner.CurrentCutLocked()
	if at != nil && *at != cut {
		core.owner.RUnlock()
		return nil, contract.ErrReadCutExpired
	}
	candidate, valid := core.owner.CaptureReadLocked()
	if !valid {
		core.owner.RUnlock()
		return nil, contract.ErrStore
	}
	bytes, valid := readViewBytes(candidate.Projection)
	r := core.reads
	r.mu.Lock()
	r.expireLocked()
	if r.closed {
		r.mu.Unlock()
		core.owner.RUnlock()
		return nil, contract.ErrReadCutExpired
	}
	if mode == contract.QueryRetained && r.settings.Retention <= 0 {
		r.mu.Unlock()
		core.owner.RUnlock()
		return nil, contract.ErrInvalid
	}
	shared := r.views[cut]
	// An in-flight capture reserves this cut's construction slot. No caller
	// can borrow its partially validated projection or queue without a bound.
	if shared != nil && !shared.ready {
		r.mu.Unlock()
		core.owner.RUnlock()
		return nil, contract.ErrSnapshotCapacity
	}
	if !valid || r.active >= r.settings.Concurrent || (mode == contract.QueryRetained && uint32(len(r.entries)) >= r.settings.Handles) ||
		(shared == nil && (bytes > r.settings.Bytes || r.bytes > r.settings.Bytes-bytes)) {
		r.mu.Unlock()
		core.owner.RUnlock()
		return nil, contract.ErrSnapshotCapacity
	}
	if shared == nil {
		shared = &readView{candidate: candidate, cut: cut, run: r.run, bytes: bytes}
		r.views[cut] = shared
		r.bytes += bytes
	}
	shared.borrows++
	r.active++
	borrow := &Lease{core: core, view: shared, mode: mode, scope: scope}
	if mode == contract.QueryRetained {
		if r.serial == math.MaxUint64 {
			shared.borrows--
			r.active--
			r.collectLocked(shared)
			r.mu.Unlock()
			core.owner.RUnlock()
			return nil, contract.ErrSnapshotCapacity
		}
		r.serial++
		snapshot := token.CoreReadSnapshotFromParts(core.authority, r.run, r.serial, actor, scope, cut)
		r.entries[token.SnapshotID(snapshot)] = snapshotEntry{token: snapshot, view: shared, expires: r.now().Add(r.settings.Retention)}
		shared.refs++
		borrow.snapshot = &snapshot
		r.armExpiryLocked()
	}
	r.mu.Unlock()
	core.owner.RUnlock()
	// Validate current facts outside Operations locks, before any data is disclosed.
	// A shared view is already immutable; only the new candidate is modified.
	if candidate.Validate() != nil {
		if borrow.snapshot != nil {
			_ = core.ReleaseReadSnapshot(actor, *borrow.snapshot)
		}
		borrow.Finish()
		return nil, contract.ErrStore
	}
	r.mu.Lock()
	if shared.candidate == candidate {
		projection := candidate.Seal()
		shared.projection = &projection
		shared.candidate = nil
	}
	shared.ready = true
	r.mu.Unlock()
	return borrow, nil
}

func (core *Operations) Check(borrow *Lease) error {
	core.owner.RLock()
	defer core.owner.RUnlock()
	return core.CheckLocked(borrow, "")
}

// CheckLocked preserves owner -> registry lock order at the last disclosure
// gate. A nonempty exact ID limits source effect to an already-validated point
// read; empty means the complete scope selected by the original lease.
func (core *Operations) CheckLocked(borrow *Lease, exact values.ElementID) error {
	r := core.reads
	r.mu.Lock()
	defer r.mu.Unlock()
	if borrow == nil || borrow.core != core || borrow.done {
		return contract.ErrReadCutExpired
	}
	if core.owner.ClosedLocked() || r.closed || borrow.view.run != r.run {
		return contract.ErrReadCutExpired
	}
	if !core.owner.AvailableLocked() {
		return contract.ErrStore
	}
	if borrow.mode == contract.QueryCurrentVerified {
		if core.owner.CurrentCutLocked() != borrow.view.cut {
			return contract.ErrReadCutExpired
		}
	} else {
		r.expireLocked()
		entry, exists := r.entries[token.SnapshotID(*(borrow.snapshot))]
		if !exists || entry.token != *borrow.snapshot {
			return contract.ErrReadCutExpired
		}
	}
	return core.owner.CheckTransferReadLocked(borrow.view.projection.Elements, borrow.view.cut, borrow.scope, exact)
}

func (borrow *Lease) Finish() {
	if borrow == nil {
		return
	}
	r := borrow.core.reads
	r.mu.Lock()
	defer r.mu.Unlock()
	if borrow.done {
		return
	}
	borrow.done = true
	borrow.view.borrows--
	r.active--
	r.collectLocked(borrow.view)
}

// Projection returns only the immutable live read set held by this lease.
// It does not expose the owner, journals, write maps or registry.
func (borrow *Lease) Projection() commitpkg.ReadView { return *borrow.view.projection }
func (borrow *Lease) Cut() contract.CoreReadCut      { return borrow.view.cut }
func (borrow *Lease) Run() [32]byte                  { return borrow.view.run }
func (borrow *Lease) Mode() contract.QueryReadMode   { return borrow.mode }
func (borrow *Lease) Snapshot() *contract.CoreReadSnapshot {
	if borrow.snapshot == nil {
		return nil
	}
	copy := *borrow.snapshot
	return &copy
}
func (borrow *Lease) Validate() error { return validateReadView(borrow.view) }
