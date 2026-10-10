package query

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	commitpkg "ucupa/ucupa-core/internal/commit"
	"ucupa/ucupa-core/internal/token"
)

// RetainedPlan preserves the original resource-lock interval for private
// connected replanning. The caller holds the commit read lock and must Finish
// before releasing it. No I/O or authorization may run in this interval.
type RetainedPlan struct {
	registry   *snapshotRegistry
	projection commitpkg.ReadView
	done       bool
}

func (plan *RetainedPlan) Projection() commitpkg.ReadView { return plan.projection }
func (plan *RetainedPlan) Finish() {
	if plan != nil && !plan.done {
		plan.done = true
		plan.registry.mu.Unlock()
	}
}
func (core *Operations) PlanRetained(actor values.PrincipalRef, snapshot *contract.CoreReadSnapshot) (*RetainedPlan, error) {
	r := core.reads
	r.mu.Lock()
	if snapshot == nil || !token.ValidCoreReadSnapshot(*snapshot) || token.SnapshotOwner(*snapshot) != core.authority || token.SnapshotActor(*snapshot) != actor {
		r.mu.Unlock()
		return nil, contract.ErrInvalid
	}
	entry, exists := r.entries[token.SnapshotID(*snapshot)]
	if !exists || entry.token != *snapshot || !r.now().Before(entry.expires) {
		r.mu.Unlock()
		return nil, contract.ErrReadCutExpired
	}
	if entry.view.projection == nil {
		r.mu.Unlock()
		return nil, contract.ErrStore
	}
	return &RetainedPlan{registry: r, projection: *entry.view.projection}, nil
}

// CheckRetainedFailure checks the original failure disclosure basis without
// adding a new capture or changing the active-read capacity decision.
func (core *Operations) CheckRetainedFailure(snapshot *contract.CoreReadSnapshot) error {
	r := core.reads
	r.mu.Lock()
	defer r.mu.Unlock()
	if snapshot == nil || token.SnapshotRun(*snapshot) != r.run {
		return contract.ErrReadCutExpired
	}
	entry, exists := r.entries[token.SnapshotID(*snapshot)]
	if !exists || entry.token != *snapshot || !r.now().Before(entry.expires) {
		return contract.ErrReadCutExpired
	}
	return nil
}

// SnapshotPresent is the original public return-time check under the commit
// lock. It reclaims expired pins; it does not authorize disclosure.
func (core *Operations) SnapshotPresent(snapshot *contract.CoreReadSnapshot) bool {
	r := core.reads
	r.mu.Lock()
	defer r.mu.Unlock()
	r.expireLocked()
	if snapshot == nil || r.closed {
		return false
	}
	entry, exists := r.entries[token.SnapshotID(*snapshot)]
	return exists && entry.token == *snapshot
}

// DiscardOwnedSnapshot is trusted local context cleanup, not caller release.
// The context already owns this handle; preserve original ID retirement and
// accounting without reacquiring the enclosing commit lock.
func (core *Operations) DiscardOwnedSnapshot(snapshot contract.CoreReadSnapshot) {
	r := core.reads
	r.mu.Lock()
	defer r.mu.Unlock()
	if current, ok := r.entries[token.SnapshotID(snapshot)]; ok {
		delete(r.entries, token.SnapshotID(snapshot))
		current.view.refs--
		r.collectLocked(current.view)
	}
}
