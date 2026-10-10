// Package registry owns the complete run-local public access lifecycle.
//
// Every public handle, receipt, continuation, observation cursor, retained
// byte charge, expiry timer and collector retirement belongs to one Registry.
// Callers serialize methods ending in Locked with the supplied owner lock.
// The registry never owns Core facts, authorization decisions or query
// snapshots; it only releases those finite resources through narrow ports.
package registry

import (
	"context"
	"sync"
	"time"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	accessobserve "ucupa/ucupa-core/internal/access/observe"
	federationcollect "ucupa/ucupa-core/internal/query/federation/collect"
	federationdenial "ucupa/ucupa-core/internal/query/federation/denial"
)

const contextBytes uint64 = 2048

// SnapshotOwner is the one query-snapshot cleanup operation needed when a
// public receipt retires. It must not reacquire the enclosing owner lock.
type SnapshotOwner interface {
	DiscardOwnedSnapshot(contract.CoreReadSnapshot)
}

// Ref is an opaque, unforgeable reference to one registry entry. It is
// comparable so an outer operation can prove that the same borrowed resource
// survived a lock-free interval, but it exposes no state or behavior.
type Ref struct{ entry *entry }

// Reservation is an opaque once-consumed transient byte reservation. It can
// either be released or transferred into one retained entry under the owner
// lock; neither path can refund it twice.
type Reservation struct {
	registry *Registry
	bytes    uint64
	active   bool
}

// Registry is protected by locker, which is the Core commit owner's write
// lock. entries is the sole public-context index and bytes the sole public
// capacity meter.
type Registry struct {
	authority values.FactAuthorityRef
	limits    values.PublicLimits
	now       func() time.Time
	locker    sync.Locker
	snapshots SnapshotOwner

	entries map[string]*entry
	bytes   uint64
	retired []retirement
	closed  bool
}

type entry struct {
	token    values.PublicToken
	caller   values.CallerContext
	boundary values.ElementID
	parent   *entry
	expires  time.Time
	bytes    uint64
	timer    *time.Timer

	read         *readState
	continuation *continuationState
	preparation  *preparationState
	cursor       *cursorState
	releases     []func(context.Context)
}

type readState struct {
	view         values.AuthorizationView
	requirements []values.Requirement
	cut          contract.CoreReadCut
	snapshot     *contract.CoreReadSnapshot
	operation    string
	conditions   string
	preparation  *entry
	basis        *entry
	source       *sourceState
	composite    *federationcollect.Read
	exact        *federationcollect.ExactBasis
	lifetime     context.Context
	cancel       context.CancelFunc
}

type continuationState struct {
	receipt    *entry
	last       values.ElementID
	conditions string
}

type preparationState struct {
	view         values.AuthorizationView
	requirements []values.Requirement
	cut          contract.CoreReadCut
	queue        *observationQueue
	receipt      *entry
	source       *entry
	initial      *entry
	controlKey   values.FacetAcceptanceKey
	ownsParent   bool
	ownsReceipt  bool
}

type cursorState struct {
	preparation *entry
	position    uint64
}

type retirement struct {
	read    *federationcollect.Read
	bytes   uint64
	release func(context.Context)
}

// New binds the registry to the commit owner's lock. A zero public policy
// disables the public surface and returns nil, matching the original model.
func New(authority values.FactAuthorityRef, limits values.PublicLimits, locker sync.Locker, snapshots SnapshotOwner) *Registry {
	if limits.ContextTTL == 0 {
		return nil
	}
	return &Registry{
		authority: authority,
		limits:    limits,
		now:       time.Now,
		locker:    locker,
		snapshots: snapshots,
		entries:   make(map[string]*entry),
	}
}

// ReserveLocked charges finite transient working capacity before allocation.
func (r *Registry) ReserveLocked(bytes uint64) (*Reservation, error) {
	if r == nil || r.closed {
		return nil, values.PublicUnavailable
	}
	r.expireLocked()
	if !r.canChargeLocked(bytes) {
		return nil, values.PublicResourceExhausted
	}
	r.bytes += bytes
	return &Reservation{registry: r, bytes: bytes, active: true}, nil
}

// ReleaseLocked refunds a reservation exactly once. It is safe to call with a
// nil or already-consumed reservation.
func (r *Registry) ReleaseLocked(reservation *Reservation) {
	if reservation == nil || !reservation.active || reservation.registry != r {
		return
	}
	reservation.active = false
	r.bytes -= reservation.bytes
}

// Release refunds a transient reservation while acquiring the shared owner
// lock. It is intended for defers after a lock-free computation.
func (r *Registry) Release(reservation *Reservation) {
	if r == nil || reservation == nil {
		return
	}
	r.locker.Lock()
	r.ReleaseLocked(reservation)
	r.locker.Unlock()
}

func (r *Registry) transferLocked(reservation *Reservation, target *entry) bool {
	if reservation == nil || !reservation.active || reservation.registry != r || target == nil {
		return false
	}
	reservation.active = false
	target.bytes += reservation.bytes
	return true
}

func (r *Registry) canChargeLocked(bytes uint64) bool {
	return bytes <= r.limits.Bytes && r.bytes <= r.limits.Bytes-bytes
}

func (r *Registry) issueLocked(caller values.CallerContext, kind string, boundary values.ElementID, parent *entry) (*entry, error) {
	if r == nil || r.closed {
		return nil, values.PublicUnavailable
	}
	r.expireLocked()
	if parent != nil && r.entries[parent.token.Value] != parent {
		return nil, values.PublicInvalidContext
	}
	perCaller := uint64(0)
	for _, current := range r.entries {
		if current.caller.Subject == caller.Subject {
			perCaller++
		}
	}
	if uint64(len(r.entries)) >= r.limits.Contexts || perCaller >= r.limits.PerCaller || !r.canChargeLocked(contextBytes) {
		return nil, values.PublicResourceExhausted
	}
	token, err := values.NewPublicToken(r.authority, kind)
	if err != nil {
		return nil, err
	}
	current := r.now()
	item := &entry{
		token:    token,
		caller:   caller,
		boundary: boundary,
		parent:   parent,
		expires:  current.Add(r.limits.ContextTTL),
		bytes:    contextBytes,
	}
	if parent != nil && parent.expires.Before(item.expires) {
		item.expires = parent.expires
	}
	r.entries[token.Value] = item
	r.bytes += item.bytes
	item.timer = time.AfterFunc(item.expires.Sub(current), func() {
		r.locker.Lock()
		r.expireLocked()
		r.locker.Unlock()
		r.DrainRetired()
	})
	return item, nil
}

func (r *Registry) shortenObservationLocked(item *entry) {
	deadline := r.now().Add(r.limits.ObservationTTL)
	if deadline.Before(item.expires) {
		item.expires = deadline
		item.timer.Reset(deadline.Sub(r.now()))
	}
}

func (r *Registry) resolveLocked(caller values.CallerContext, token values.PublicToken, kind string) (*entry, error) {
	if r == nil || r.closed {
		return nil, values.PublicUnavailable
	}
	if !token.Valid() || token.Kind != kind || token.Owner != r.authority {
		return nil, values.PublicInvalid
	}
	r.expireLocked()
	item := r.entries[token.Value]
	if item == nil {
		if kind == "observation-preparation" || kind == "observation-cursor" {
			return nil, values.PublicGap
		}
		return nil, values.PublicInvalidContext
	}
	if item.token != token || item.caller != caller {
		return nil, values.PublicDenied
	}
	return item, nil
}

func (r *Registry) presentLocked(ref Ref) bool {
	return ref.entry != nil && r.entries[ref.entry.token.Value] == ref.entry
}

// RemoveLocked immediately retires one context and every parent/basis-owned
// descendant. Snapshot release stays under the owner lock. Collector I/O is
// queued for DrainRetired and never runs while this lock is held.
func (r *Registry) RemoveLocked(ref Ref) {
	if r == nil || ref.entry == nil {
		return
	}
	r.removeLocked(ref.entry)
}

func (r *Registry) removeLocked(item *entry) {
	if item == nil || r.entries[item.token.Value] != item {
		return
	}
	delete(r.entries, item.token.Value)
	for _, release := range item.releases {
		r.retired = append(r.retired, retirement{release: release})
	}
	item.releases = nil
	if item.read != nil && item.read.cancel != nil {
		item.read.cancel()
	}
	if item.timer != nil {
		item.timer.Stop()
	}
	r.bytes -= item.bytes
	if item.read != nil && item.read.composite != nil {
		// Disclosure retires immediately. The full receipt reservation remains
		// charged until the collector borrower exits and Retire completes.
		r.bytes += item.bytes
		r.retired = append(r.retired, retirement{read: item.read.composite, bytes: item.bytes})
		item.read.composite = nil
	}
	if item.preparation != nil && item.preparation.queue != nil {
		queue := item.preparation.queue
		r.bytes -= uint64(len(queue.groups)) * 512
		r.bytes -= uint64(len(queue.pendingIDs)) * 128
		queue.groups = nil
		queue.pendingIDs = nil
		queue.gap = true
	}
	if item.read != nil && item.read.snapshot != nil && r.snapshots != nil {
		r.snapshots.DiscardOwnedSnapshot(*item.read.snapshot)
	}
	for _, child := range r.entries {
		if child.parent == item || child.read != nil && child.read.basis == item {
			r.removeLocked(child)
		}
	}
	if item.preparation != nil {
		if item.preparation.ownsParent {
			r.removeLocked(item.parent)
		}
		if item.preparation.ownsReceipt {
			r.removeLocked(item.preparation.receipt)
		}
	}
}

// ExpireLocked reclaims every context whose absolute deadline has passed.
func (r *Registry) ExpireLocked() { r.expireLocked() }

func (r *Registry) expireLocked() {
	if r == nil {
		return
	}
	now := r.now()
	for _, item := range r.entries {
		if !now.Before(item.expires) {
			r.removeLocked(item)
		}
	}
}

// CloseLocked retires all registry state once. Collector I/O remains queued
// until DrainRetired runs after the shared owner lock is released.
func (r *Registry) CloseLocked() {
	if r == nil || r.closed {
		return
	}
	r.closed = true
	for _, item := range r.entries {
		r.removeLocked(item)
	}
}

// DrainRetired performs collector/source shutdown strictly outside the owner
// lock, then reacquires it only to refund the delayed retained bytes.
func (r *Registry) DrainRetired() {
	if r == nil {
		return
	}
	r.locker.Lock()
	items := r.retired
	r.retired = nil
	r.locker.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, item := range items {
		if item.release != nil {
			if ctx.Err() == nil {
				item.release(ctx)
			}
			continue
		}
		_ = item.read.Retire(ctx)
		r.locker.Lock()
		r.bytes -= item.bytes
		r.locker.Unlock()
	}
}

// DenialDeadlineLocked returns the same absolute ContextTTL deadline used by
// source receipts without exposing policy internals.
func (r *Registry) DenialDeadlineLocked() time.Time {
	return r.now().Add(r.limits.ContextTTL)
}

// DenialExpiredLocked applies this registry's clock to retained denial proof.
func (r *Registry) DenialExpiredLocked(evidence federationdenial.Evidence) bool {
	return federationdenial.Expired(evidence, r.now())
}

// Publish appends one atomic change group to every matching local/source
// observation queue. The commit owner calls it synchronously while holding the
// shared owner lock and supplies exactly its immutable before/after views.
func (r *Registry) Publish(before, after accessobserve.View) {
	if r == nil || r.closed || !before.Present || !after.Present || before.CutSerial == after.CutSerial {
		return
	}
	r.expireLocked()
	for _, item := range r.entries {
		if item.preparation == nil || item.preparation.queue == nil {
			continue
		}
		queue := item.preparation.queue
		if queue.gap || queue.terminal {
			continue
		}
		selection := cloneSelection(queue.selection)
		if accessobserve.SourceLost(before, after, selection) {
			// A parent-side gate closure is never this owner's provable
			// decision: it ends in GAP immediately. A settled decision proves
			// the whole recorded element loss ended: the queue retains that
			// terminal evidence once instead of an immediate GAP. A loss still
			// under this owner's own unsettled transfer decision stays pending
			// until a later publication proves or loses it; every other loss
			// ends in GAP now.
			if accessobserve.GateLost(before, after) {
				r.gapLocked(item)
				continue
			}
			lost := accessobserve.LostDepartures(before, after, selection)
			proof := queue.pendingIDs
			if len(lost) > 0 {
				proof = make([]values.ElementID, 0, len(queue.pendingIDs)+len(lost))
				proof = append(proof, queue.pendingIDs...)
				proof = append(proof, lost...)
			}
			if terminal, ok := accessobserve.SettledSuccessor(proof, after); ok &&
				uint64(len(queue.groups)) < r.limits.Groups && r.canChargeLocked(512) &&
				r.retainTerminalLocked(queue, terminal) {
				continue
			} else if accessobserve.AllPendingDecisions(lost, after) && r.canChargeLocked(uint64(len(lost))*128) {
				queue.pending = true
				queue.pendingIDs = append(queue.pendingIDs, lost...)
				r.bytes += uint64(len(lost)) * 128
				continue
			}
			r.gapLocked(item)
			continue
		}
		if queue.pending {
			// No new loss arrived: proving the recorded pending decision settled
			// discloses its retained terminal evidence now, and a settled decision
			// whose evidence cannot be retained ends in GAP exactly like the
			// new-loss publication path. A decision still unsettled holds the
			// queue pending: no ordinary change may enter it and it never GAPs
			// here; a stuck decision loses its retention window and ends in GAP.
			if terminal, ok := accessobserve.SettledSuccessor(queue.pendingIDs, after); ok {
				if uint64(len(queue.groups)) < r.limits.Groups && r.canChargeLocked(512) &&
					r.retainTerminalLocked(queue, terminal) {
					continue
				}
				r.gapLocked(item)
				continue
			}
			continue
		}
		spatial, attachment := accessobserve.Changes(before, after, selection)
		if !spatial && !attachment {
			continue
		}
		if uint64(len(queue.groups)) >= r.limits.Groups || !r.canChargeLocked(512) {
			r.gapLocked(item)
			continue
		}
		categories := make([]string, 0, 2)
		if spatial {
			categories = append(categories, "spatial")
		}
		if attachment {
			categories = append(categories, "attachment")
		}
		id, idErr := values.NewPublicToken(r.authority, "change-id")
		basis, basisErr := values.NewPublicToken(r.authority, "change-basis")
		if idErr != nil || basisErr != nil {
			r.gapLocked(item)
			continue
		}
		queue.groups = append(queue.groups, ChangeGroup{ID: id, Basis: basis, Categories: categories})
		r.bytes += 512
	}
}

// retainTerminalLocked discloses one settled successor decision as the queue's
// single retained terminal group and clears its pending proof obligations.
// Callers pre-check the group limit and charge.
func (r *Registry) retainTerminalLocked(queue *observationQueue, terminal contract.TransitionTerminal) bool {
	id, idErr := values.NewPublicToken(r.authority, "change-id")
	basis, basisErr := values.NewPublicToken(r.authority, "change-basis")
	if idErr != nil || basisErr != nil {
		return false
	}
	queue.groups = append(queue.groups, ChangeGroup{ID: id, Basis: basis, Categories: []string{"terminal"}, Terminal: &terminal})
	queue.terminal = true
	queue.pending = false
	r.bytes += 512
	r.bytes -= uint64(len(queue.pendingIDs)) * 128
	queue.pendingIDs = nil
	return true
}
