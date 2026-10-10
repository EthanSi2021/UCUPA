// Package commit owns the Core aggregate's serialized state transition and
// durable publication boundary.  The surrounding operation packages decide
// authorization and protocol details; they enter this owner only with a
// complete, already-bound mutation.
package commit

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	token "ucupa/ucupa-core/internal/token"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/access/observe"
	commitbinding "ucupa/ucupa-core/internal/commit/binding"
)

// PublicationSink receives the immutable before/after fact views produced by
// one committed state transition.  It does not receive the aggregate, Store,
// journal, or a key and must be called synchronously while Owner's write lock
// is held.
type PublicationSink interface {
	Publish(before, after observe.View)
}

// Owner is the one Core aggregate owner.  state and store are deliberately
// private: callers can obtain a lock and ask for the small operations below,
// but cannot retain a mutable snapshot or install a candidate independently.
type Owner struct {
	mu     sync.RWMutex
	closed bool
	fenced bool

	authority values.FactAuthorityRef
	state     *coreState
	store     *localStore
	sink      PublicationSink
	// contact is the lock-free projection of the current Realm attachment for
	// the authorization adapter. It is refreshed only under the write lock at
	// the same state-install boundary that persists the contact.
	contact atomic.Pointer[contract.RealmAttachment]
}

// New creates the original in-memory aggregate with its one durable Realm
// contact bound in the initial state. Publication is bound once by instance
// assembly before the owner is made available to callers.
func New(authority values.FactAuthorityRef, scope [32]byte, initial contract.RealmInitialization) (*Owner, error) {
	contact, err := newRealmContact(initial)
	if err != nil {
		return nil, err
	}
	state := newCoreState(authority, scope)
	state.realmContact = contact
	owner := &Owner{authority: authority, state: state}
	owner.refreshContactLocked()
	return owner, nil
}

func (owner *Owner) contactAttachment() contract.RealmAttachment {
	if owner == nil || owner.state == nil {
		return contract.RealmAttachment{}
	}
	return owner.state.realmContact.attachment(owner.authority, owner.state.scope)
}

func (owner *Owner) refreshContactLocked() {
	if owner == nil || owner.state == nil {
		return
	}
	attachment := owner.contactAttachment()
	owner.contact.Store(&attachment)
}

// CurrentContact is the lock-free current Realm attachment. The zero value
// reports an unusable authorization dependency, never a silent fallback.
func (owner *Owner) CurrentContact() (contract.RealmAttachment, bool) {
	if owner == nil {
		return contract.RealmAttachment{}, false
	}
	if attachment := owner.contact.Load(); attachment != nil && attachment.Revision != (contract.RealmContactRevision{}) {
		return *attachment, true
	}
	return contract.RealmAttachment{}, false
}

// Lock enters the aggregate's write lock.  All methods whose name ends in
// Locked require this lock to be held by the caller and do not lock again.
func (owner *Owner) Lock() {
	owner.mu.Lock()
}

// Unlock leaves the aggregate's write lock.
func (owner *Owner) Unlock() {
	owner.mu.Unlock()
}

// RLock enters the aggregate's read lock.
func (owner *Owner) RLock() {
	owner.mu.RLock()
}

// RUnlock leaves the aggregate's read lock.
func (owner *Owner) RUnlock() {
	owner.mu.RUnlock()
}

// AuthorityRef returns the stable fact-owner identity.  The identity is
// immutable after construction, so this accessor is safe without a lock; a
// nil Owner reports the zero reference rather than panicking.
func (owner *Owner) AuthorityRef() values.FactAuthorityRef {
	if owner == nil {
		return ""
	}
	return owner.authority
}

// AvailableLocked reports whether the aggregate still has a usable state.
// An unreconcilable Store response or a durable writer fence makes facts
// unavailable. Query and observation resources remain outside this owner.
func (owner *Owner) AvailableLocked() bool {
	return owner != nil && owner.state != nil && !owner.fenced
}

// CurrentCutLocked returns the current aggregate cut, or the zero cut while
// the owner has no state.
func (owner *Owner) CurrentCutLocked() contract.CoreReadCut {
	if owner == nil || owner.state == nil {
		return contract.CoreReadCut{}
	}
	return owner.state.currentCut()
}

// SetPublicationSinkLocked binds the one synchronous publication sink during
// instance assembly.  A non-nil sink cannot be replaced or rebound.  Passing
// nil is a no-op so an optional sink can remain absent.
func (owner *Owner) SetPublicationSinkLocked(sink PublicationSink) error {
	if owner == nil {
		return contract.ErrStore
	}
	if sink == nil {
		return nil
	}
	if owner.sink != nil {
		return contract.ErrConflict
	}
	owner.sink = sink
	return nil
}

// ApplyElementLocked executes one already-bound Element mutation through the
// aggregate's original deduplication, conflict, candidate, and result rules.
// The caller must hold Owner's write lock.
func (owner *Owner) ApplyElementLocked(
	operationID contract.OperationID,
	binding commitbinding.Binding,
) (contract.MutationResult, error) {
	if owner == nil || owner.closed || owner.fenced || owner.state == nil || owner.state.scope == ([32]byte{}) {
		return contract.MutationResult{}, contract.ErrStore
	}
	next, outcome, err := owner.state.prepareMutation(operationID, binding)
	if err != nil {
		return contract.MutationResult{}, err
	}
	if next != nil {
		if err := owner.publishStateLocked(next); err != nil {
			return contract.MutationResult{}, err
		}
	}
	return outcome.Outcome.Result(outcome.cut)
}

// publishStateLocked is the sole state-install boundary.  It persists first;
// on an uncertain Store response it reloads and accepts only a complete
// snapshot matching the previous or candidate side.  The chosen before/after
// views are published synchronously before installing the chosen state, while
// the caller still holds the same Owner lock.
func (owner *Owner) publishStateLocked(next *coreState) error {
	if owner.closed || owner.fenced {
		return contract.ErrStore
	}
	previous := owner.state
	if owner.store != nil {
		if err := owner.store.persist(next); err != nil {
			recovered, recoveryErr := owner.store.reload()
			recoveryErr = errors.Join(recoveryErr, owner.store.checkWriter())
			if recoveryErr != nil ||
				(!sameCompleteCoreState(recovered, previous) && !sameCompleteCoreState(recovered, next)) {
				if recoveryErr == nil {
					recoveryErr = errors.New("reloaded Store state matches neither side of the attempted commit")
				}
				// The durable authority can no longer be reconciled with this
				// instance.  Poison it so no later mutation can overwrite an
				// uncertain committed state from the stale memory snapshot.
				owner.state = nil
				return errors.Join(err, fmt.Errorf("%w: reconcile failed persist: %w", contract.ErrStore, recoveryErr))
			}
			// A failed commit response can mean either rollback or commit.  The
			// Store is authoritative: publish whichever complete valid side it
			// exposes, while preserving ErrStore for this uncertain response.
			owner.publishLocked(publicationView(previous), publicationView(recovered))
			owner.state = recovered
			owner.refreshContactLocked()
			return err
		}
	}
	owner.publishLocked(publicationView(previous), publicationView(next))
	owner.state = next
	owner.refreshContactLocked()
	return nil
}

func (owner *Owner) publishLocked(before, after observe.View) {
	if owner.sink != nil {
		owner.sink.Publish(before, after)
	}
}

// CloseStore closes only the owner-private durable database.  Core lifecycle,
// read registries, and public observation resources are deliberately managed
// by the surrounding instance; the caller serializes this tail with its
// lifecycle transition as the original Core.Close did.
func (owner *Owner) CloseStore() error {
	if owner == nil || owner.store == nil || owner.store.db == nil {
		return nil
	}
	if err := owner.store.db.Close(); err != nil {
		return WrapStoreError("close", err)
	}
	return nil
}

// ClosedLocked is the shared run-retirement flag, guarded by the one Owner lock.
// It is distinct from a poisoned Store so callers retain their original error ordering.
func (owner *Owner) ClosedLocked() bool { return owner.closed || owner.fenced }

// BeginCloseLocked retires admission once; resource draining remains in assembly
// and is completed before the Store close, exactly as in the original Core.Close.
func (owner *Owner) BeginCloseLocked() bool {
	if owner.closed {
		return false
	}
	owner.closed = true
	return true
}

// SelectCurrentCutLocked preserves the exact local current-cut check. Callers
// validate input shape and owner availability first, under this same read lock.
func (owner *Owner) SelectCurrentCutLocked(at *contract.CoreReadCut) (contract.CoreReadCut, error) {
	current := owner.CurrentCutLocked()
	if at == nil {
		return current, nil
	}
	if token.Scope(*at) != owner.FactsLocked().Scope {
		return contract.CoreReadCut{}, contract.ErrInvalid
	}
	if *at != current {
		return contract.CoreReadCut{}, contract.ErrReadCutExpired
	}
	return current, nil
}
