package query

import (
	"sync"
	"time"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	coretoken "ucupa/ucupa-core/internal/token"
)

type snapshotEntry struct {
	token   contract.CoreReadSnapshot
	view    *readView
	expires time.Time
}

type snapshotRegistry struct {
	mu       sync.Mutex
	run      [32]byte
	settings Settings
	now      func() time.Time
	closed   bool
	serial   uint64
	active   uint32
	bytes    uint64
	entries  map[uint64]snapshotEntry
	views    map[contract.CoreReadCut]*readView
	timer    *time.Timer
}

func newSnapshotRegistry(run [32]byte, settings Settings) *snapshotRegistry {
	return &snapshotRegistry{run: run, settings: settings, now: time.Now,
		entries: make(map[uint64]snapshotEntry), views: make(map[contract.CoreReadCut]*readView)}
}

// One timer per registry reclaims expired handles even when no new API call
// arrives. Active computations retain their charged view until finish.
func (registry *snapshotRegistry) armExpiryLocked() {
	if registry.timer != nil {
		registry.timer.Stop()
		registry.timer = nil
	}
	if registry.closed || len(registry.entries) == 0 {
		return
	}
	var next time.Time
	for _, entry := range registry.entries {
		if next.IsZero() || entry.expires.Before(next) {
			next = entry.expires
		}
	}
	delay := next.Sub(registry.now())
	if delay < 0 {
		delay = 0
	}
	registry.timer = time.AfterFunc(delay, func() {
		registry.mu.Lock()
		defer registry.mu.Unlock()
		if registry.closed {
			return
		}
		registry.expireLocked()
		registry.armExpiryLocked()
	})
}

func (registry *snapshotRegistry) expireLocked() {
	now := registry.now()
	for id, entry := range registry.entries {
		if !now.Before(entry.expires) {
			delete(registry.entries, id)
			entry.view.refs--
			registry.collectLocked(entry.view)
		}
	}
}

func (registry *snapshotRegistry) collectLocked(view *readView) {
	if view.refs == 0 && view.borrows == 0 {
		delete(registry.views, view.cut)
		registry.bytes -= view.bytes
	}
}

func (registry *snapshotRegistry) close() {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.closed = true
	if registry.timer != nil {
		registry.timer.Stop()
		registry.timer = nil
	}
	for id, entry := range registry.entries {
		delete(registry.entries, id)
		entry.view.refs--
		registry.collectLocked(entry.view)
	}
}

func (core *Operations) snapshotBinding(actor values.PrincipalRef, snapshot contract.CoreReadSnapshot) error {
	if core == nil || !coretoken.ValidCoreReadSnapshot(snapshot) || coretoken.SnapshotOwner(snapshot) != core.authority || coretoken.SnapshotActor(snapshot) != actor {
		return contract.ErrInvalid
	}
	return nil
}

func (core *Operations) Borrow(actor values.PrincipalRef, snapshot contract.CoreReadSnapshot) (*Lease, error) {
	if err := core.snapshotBinding(actor, snapshot); err != nil {
		return nil, err
	}
	core.owner.RLock()
	defer core.owner.RUnlock()
	r := core.reads
	r.mu.Lock()
	defer r.mu.Unlock()
	if core.owner.ClosedLocked() || r.closed || coretoken.SnapshotRun(snapshot) != r.run {
		return nil, contract.ErrReadCutExpired
	}
	if !core.owner.AvailableLocked() {
		return nil, contract.ErrStore
	}
	r.expireLocked()
	entry, exists := r.entries[coretoken.SnapshotID(snapshot)]
	if !exists {
		return nil, contract.ErrReadCutExpired
	}
	if entry.token != snapshot {
		return nil, contract.ErrInvalid
	}
	if !entry.view.ready || r.active >= r.settings.Concurrent {
		return nil, contract.ErrSnapshotCapacity
	}
	entry.view.borrows++
	r.active++
	return &Lease{core: core, view: entry.view, snapshot: &snapshot, mode: contract.QueryRetained, scope: coretoken.SnapshotScope(snapshot)}, nil
}

// CheckSnapshotLocked validates an existing retained basis without creating a
// second borrow or recursively acquiring the owner's lock. The caller keeps the
// owner lock through disclosure; registry state is inspected under its own lock.
func (core *Operations) CheckSnapshotLocked(actor values.PrincipalRef, snapshot contract.CoreReadSnapshot) error {
	if err := core.snapshotBinding(actor, snapshot); err != nil {
		return err
	}
	r := core.reads
	r.mu.Lock()
	defer r.mu.Unlock()
	if core.owner.ClosedLocked() || r.closed || coretoken.SnapshotRun(snapshot) != r.run {
		return contract.ErrReadCutExpired
	}
	if !core.owner.AvailableLocked() {
		return contract.ErrStore
	}
	r.expireLocked()
	entry, exists := r.entries[coretoken.SnapshotID(snapshot)]
	if !exists || entry.token != snapshot || !entry.view.ready {
		return contract.ErrReadCutExpired
	}
	return core.owner.CheckTransferReadLocked(entry.view.projection.Elements, entry.view.cut, coretoken.SnapshotScope(snapshot), "")
}

// ReleaseReadSnapshot retires only this caller's handle. It is idempotent even
// after expiry, release or run retirement, and does not require a query grant.
func (core *Operations) ReleaseReadSnapshot(actor values.PrincipalRef, snapshot contract.CoreReadSnapshot) error {
	if err := core.snapshotBinding(actor, snapshot); err != nil {
		return err
	}
	r := core.reads
	r.mu.Lock()
	defer r.mu.Unlock()
	if coretoken.SnapshotRun(snapshot) != r.run {
		return nil
	}
	entry, exists := r.entries[coretoken.SnapshotID(snapshot)]
	if !exists {
		return nil
	}
	if entry.token != snapshot {
		return contract.ErrInvalid
	}
	delete(r.entries, coretoken.SnapshotID(snapshot))
	entry.view.refs--
	r.collectLocked(entry.view)
	r.armExpiryLocked()
	return nil
}
