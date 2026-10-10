package registry

import (
	"context"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	federationcollect "ucupa/ucupa-core/internal/query/federation/collect"
)

// OwnAuthorizationReleaseLocked moves one finite view's cleanup into the
// existing receipt lifecycle. It is metadata, not a second evidence registry.
// Removal only queues cleanup; DrainRetired calls it after the fact lock exits.
func (r *Registry) OwnAuthorizationReleaseLocked(ref Ref, release func(context.Context)) bool {
	if !r.presentLocked(ref) {
		return false
	}
	if release != nil {
		ref.entry.releases = append(ref.entry.releases, release)
	}
	return true
}

// SpaceUse is the finite immutable result of resolving a space handle.
type SpaceUse struct {
	Ref      Ref
	Token    values.PublicToken
	Boundary values.ElementID
}

// ReadUse is a detached, finite view of one retained read receipt. Mutable
// source maps and registry links remain private; Ref values can only be passed
// back to Registry operations.
type ReadUse struct {
	Ref            Ref
	Token          values.PublicToken
	Caller         values.CallerContext
	Boundary       values.ElementID
	Parent         Ref
	View           values.AuthorizationView
	Requirements   []values.Requirement
	Cut            contract.CoreReadCut
	Snapshot       *contract.CoreReadSnapshot
	Conditions     string
	Preparation    Ref
	HasPreparation bool
	Basis          Ref
	HasBasis       bool
	Composite      *federationcollect.Read
	Exact          *federationcollect.ExactBasis
	Lifetime       context.Context
	Source         SourceUse
}

// ContinuationUse is the finite position bound to one read receipt.
type ContinuationUse struct {
	Ref        Ref
	Token      values.PublicToken
	Caller     values.CallerContext
	Boundary   values.ElementID
	Receipt    Ref
	Last       values.ElementID
	Conditions string
}

// IssueSpaceLocked creates one root public handle.
func (r *Registry) IssueSpaceLocked(caller values.CallerContext, boundary values.ElementID) (SpaceUse, error) {
	item, err := r.issueLocked(caller, "space-handle", boundary, nil)
	if err != nil {
		return SpaceUse{}, err
	}
	return spaceUse(item), nil
}

// ResolveSpaceLocked resolves one caller-bound handle and reclaims expiries.
func (r *Registry) ResolveSpaceLocked(caller values.CallerContext, token values.PublicToken) (SpaceUse, error) {
	item, err := r.resolveLocked(caller, token, "space-handle")
	if err != nil {
		return SpaceUse{}, err
	}
	return spaceUse(item), nil
}

// SpaceLocked revalidates an opaque handle after a lock-free interval.
func (r *Registry) SpaceLocked(ref Ref) (SpaceUse, bool) {
	if r == nil || !r.presentLocked(ref) || ref.entry.token.Kind != "space-handle" {
		return SpaceUse{}, false
	}
	return spaceUse(ref.entry), true
}

func spaceUse(item *entry) SpaceUse {
	return SpaceUse{Ref: Ref{item}, Token: item.token, Boundary: item.boundary}
}

// IssueLocalReadLocked creates a local read receipt and charges its canonical
// condition string at the original issuance point.
func (r *Registry) IssueLocalReadLocked(caller values.CallerContext, boundary values.ElementID, parent Ref, cut contract.CoreReadCut, view values.AuthorizationView, requirements []values.Requirement, operation, conditions string) (ReadUse, error) {
	item, err := r.issueReadLocked(caller, boundary, parent, uint64(len(conditions))*2)
	if err != nil {
		return ReadUse{}, err
	}
	item.read.cut = cut
	item.read.view = view
	item.read.requirements = cloneRequirements(requirements)
	item.read.operation = operation
	item.read.conditions = conditions
	return readUse(item), nil
}

// IssueCompositeReadLocked creates the retained collector receipt, including
// the fixed recipe reservation and cancellation lifetime.
func (r *Registry) IssueCompositeReadLocked(caller values.CallerContext, boundary values.ElementID, parent Ref, operation, conditions string) (ReadUse, error) {
	cost := federationcollect.RecipeBytes + uint64(len(conditions))*2
	item, err := r.issueReadLocked(caller, boundary, parent, cost)
	if err != nil {
		return ReadUse{}, err
	}
	item.read.operation = operation
	item.read.conditions = conditions
	item.read.lifetime, item.read.cancel = context.WithCancel(context.Background())
	return readUse(item), nil
}

func (r *Registry) issueReadLocked(caller values.CallerContext, boundary values.ElementID, parent Ref, extra uint64) (*entry, error) {
	var parentEntry *entry
	if parent.entry != nil {
		if !r.presentLocked(parent) {
			return nil, values.PublicInvalidContext
		}
		parentEntry = parent.entry
	}
	item, err := r.issueLocked(caller, "read-receipt", boundary, parentEntry)
	if err != nil {
		return nil, err
	}
	item.read = &readState{}
	if !r.canChargeLocked(extra) {
		r.removeLocked(item)
		return nil, values.PublicResourceExhausted
	}
	r.bytes += extra
	item.bytes += extra
	return item, nil
}

// IssueDerivedReadLocked creates an exact read which borrows, but does not own,
// a prior basis. Its lifetime cannot exceed that basis.
func (r *Registry) IssueDerivedReadLocked(caller values.CallerContext, boundary values.ElementID, parent, basis Ref, view values.AuthorizationView, requirements []values.Requirement) (ReadUse, error) {
	if !r.presentLocked(basis) || basis.entry.read == nil {
		return ReadUse{}, values.PublicInvalidContext
	}
	item, err := r.issueReadLocked(caller, boundary, parent, 0)
	if err != nil {
		return ReadUse{}, err
	}
	item.read.operation = "read"
	item.read.basis = basis.entry
	item.read.view = view
	item.read.requirements = cloneRequirements(requirements)
	item.read.lifetime, item.read.cancel = context.WithCancel(context.Background())
	if basis.entry.expires.Before(item.expires) {
		item.expires = basis.entry.expires
		item.timer.Reset(item.expires.Sub(r.now()))
	}
	return readUse(item), nil
}

// ResolveReadLocked resolves one receipt with the original caller binding.
func (r *Registry) ResolveReadLocked(caller values.CallerContext, token values.PublicToken) (ReadUse, error) {
	item, err := r.resolveLocked(caller, token, "read-receipt")
	if err != nil {
		return ReadUse{}, err
	}
	if item.read == nil {
		return ReadUse{}, values.PublicInvalidContext
	}
	return readUse(item), nil
}

// ReadLocked revalidates one opaque receipt without repeating token parsing.
func (r *Registry) ReadLocked(ref Ref) (ReadUse, bool) {
	if r == nil || !r.presentLocked(ref) || ref.entry.read == nil {
		return ReadUse{}, false
	}
	return readUse(ref.entry), true
}

func readUse(item *entry) ReadUse {
	state := item.read
	result := ReadUse{
		Ref:          Ref{item},
		Token:        item.token,
		Caller:       item.caller,
		Boundary:     item.boundary,
		View:         state.view,
		Requirements: cloneRequirements(state.requirements),
		Cut:          state.cut,
		Conditions:   state.conditions,
		Composite:    state.composite,
		Exact:        state.exact,
		Lifetime:     state.lifetime,
	}
	if item.parent != nil {
		result.Parent = Ref{item.parent}
	}
	if state.snapshot != nil {
		copy := *state.snapshot
		result.Snapshot = &copy
	}
	if state.preparation != nil {
		result.Preparation = Ref{state.preparation}
		result.HasPreparation = true
	}
	if state.basis != nil {
		result.Basis = Ref{state.basis}
		result.HasBasis = true
	}
	if state.source != nil {
		result.Source = sourceUse(state.source)
	}
	return result
}

// BindBasisLocked makes an already-issued local exact receipt borrow a prior
// read basis and clamps its cut and expiry to that basis.
func (r *Registry) BindBasisLocked(read, basis Ref) (ReadUse, bool) {
	if !r.presentLocked(read) || read.entry.read == nil || !r.presentLocked(basis) || basis.entry.read == nil {
		return ReadUse{}, false
	}
	read.entry.read.cut = basis.entry.read.cut
	read.entry.read.basis = basis.entry
	if basis.entry.expires.Before(read.entry.expires) {
		read.entry.expires = basis.entry.expires
		read.entry.timer.Reset(read.entry.expires.Sub(r.now()))
	}
	return readUse(read.entry), true
}

// InstallSnapshotLocked transfers ownership of a newly captured snapshot to a
// still-live receipt. The caller discards the snapshot itself if this fails.
func (r *Registry) InstallSnapshotLocked(ref Ref, snapshot *contract.CoreReadSnapshot) bool {
	if !r.presentLocked(ref) || ref.entry.read == nil {
		return false
	}
	if snapshot == nil {
		ref.entry.read.snapshot = nil
		return true
	}
	copy := *snapshot
	ref.entry.read.snapshot = &copy
	return true
}

// InstallCompositeLocked binds the one sealed collector to its receipt.
func (r *Registry) InstallCompositeLocked(ref Ref, read *federationcollect.Read) bool {
	if !r.presentLocked(ref) || ref.entry.read == nil || ref.entry.read.composite != nil || read == nil {
		return false
	}
	ref.entry.read.composite = read
	return true
}

// InstallExactLocked binds the opaque exact-read basis to a derived receipt.
func (r *Registry) InstallExactLocked(ref Ref, exact *federationcollect.ExactBasis) bool {
	if !r.presentLocked(ref) || ref.entry.read == nil || exact == nil {
		return false
	}
	ref.entry.read.exact = exact
	return true
}

// LinkPreparationLocked associates a separately parented local preparation
// with its read receipt.
func (r *Registry) LinkPreparationLocked(read, preparation Ref) bool {
	if !r.presentLocked(read) || read.entry.read == nil || !r.presentLocked(preparation) || preparation.entry.preparation == nil {
		return false
	}
	read.entry.read.preparation = preparation.entry
	return true
}

// IssueContinuationLocked creates one finite position owned by a receipt.
func (r *Registry) IssueContinuationLocked(read Ref, last values.ElementID, conditions string) (ContinuationUse, error) {
	if !r.presentLocked(read) || read.entry.read == nil {
		return ContinuationUse{}, values.PublicInvalidContext
	}
	item, err := r.issueLocked(read.entry.caller, "query-continuation", read.entry.boundary, read.entry)
	if err != nil {
		return ContinuationUse{}, err
	}
	item.continuation = &continuationState{receipt: read.entry, last: last, conditions: conditions}
	return continuationUse(item), nil
}

// ResolveContinuationLocked resolves a continuation and its retained receipt
// identity without exposing the registry table.
func (r *Registry) ResolveContinuationLocked(caller values.CallerContext, token values.PublicToken) (ContinuationUse, error) {
	item, err := r.resolveLocked(caller, token, "query-continuation")
	if err != nil {
		return ContinuationUse{}, err
	}
	if item.continuation == nil || item.continuation.receipt == nil {
		return ContinuationUse{}, values.PublicInvalidContext
	}
	return continuationUse(item), nil
}

func continuationUse(item *entry) ContinuationUse {
	state := item.continuation
	return ContinuationUse{
		Ref: Ref{item}, Token: item.token, Caller: item.caller, Boundary: item.boundary,
		Receipt: Ref{state.receipt}, Last: state.last, Conditions: state.conditions,
	}
}

func cloneRequirements(input []values.Requirement) []values.Requirement {
	return append([]values.Requirement(nil), input...)
}
