package element

import (
	"context"
	elementread "ucupa/ucupa-core/internal/element/read"
	mountfact "ucupa/ucupa-core/internal/mount/fact"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/token"
)

func (core *Operations) ReadElement(
	actor values.PrincipalRef,
	id values.ElementID,
	at *contract.CoreReadCut,
) (contract.ElementRead, error) {
	if core == nil || !id.Valid() || !token.ValidOptionalCut(at) {
		return contract.ElementRead{}, contract.ErrInvalid
	}
	approval, err := core.authorize(actor, core.requirement(contract.PermissionReadElement, id))
	if err != nil {
		return contract.ElementRead{}, err
	}
	core.owner.RLock()
	defer core.owner.RUnlock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		return contract.ElementRead{}, contract.ErrStore
	}
	if !core.settled(approval) {
		return contract.ElementRead{}, contract.ErrUnavailable
	}
	cut, err := core.owner.SelectCurrentCutLocked(at)
	if err != nil {
		return contract.ElementRead{}, err
	}
	return elementread.Element(core.owner.AuthorityRef(), core.owner.FactsLocked().Elements, core.owner.FactsLocked().Attachments, core.owner.SourceVisibilityLocked(), id, cut)
}

func (core *Operations) ListDirect(
	actor values.PrincipalRef,
	parent values.ElementID,
	limit uint16,
	continuation *contract.ListContinuation,
) (contract.DirectPage, error) {
	if core == nil || !parent.Valid() || limit < 1 || limit > 256 ||
		(continuation != nil && !token.ValidListContinuation(*(continuation))) {
		return contract.DirectPage{}, contract.ErrInvalid
	}
	approval, err := core.authorize(actor, core.requirement(contract.PermissionListDirect, parent))
	if err != nil {
		return contract.DirectPage{}, err
	}
	core.owner.RLock()
	defer core.owner.RUnlock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		return contract.DirectPage{}, contract.ErrStore
	}
	if !core.settled(approval) {
		return contract.DirectPage{}, contract.ErrUnavailable
	}
	cut := core.owner.CurrentCutLocked()
	transfers := core.owner.SourceVisibilityLocked()
	if transfers.Inactive(parent) {
		return contract.DirectPage{}, contract.ErrUnavailable
	}
	for fence := range transfers.Fences() {
		if fence.Origin.Parent == parent {
			return contract.DirectPage{}, contract.ErrUnavailable
		}
	}
	var after values.ElementID
	if continuation != nil {
		if token.ListScope(*(continuation)) != core.owner.FactsLocked().Scope || token.ListParent(*(continuation)) != parent {
			return contract.DirectPage{}, contract.ErrInvalid
		}
		if token.ListCut(*(continuation)) != cut {
			return contract.DirectPage{}, contract.ErrReadCutExpired
		}
		after = token.ListLast(*(continuation))
	}
	if err := mountfact.GuardLocalList(core.owner.FactsLocked().Mount, parent); err != nil {
		return contract.DirectPage{}, err
	}
	if _, exists := core.owner.FactsLocked().Elements.Anchor(parent); !exists {
		if core.owner.FactsLocked().Elements.Kind(parent) != 0 {
			return contract.DirectPage{}, contract.ErrConflict
		}
		return contract.DirectPage{}, contract.ErrNotFound
	}

	adjacency, err := elementread.DirectAdjacency(context.Background(), core.owner.FactsLocked().Elements, map[values.ElementID]struct{}{parent: {}})
	if err != nil {
		return contract.DirectPage{}, contract.ErrStore
	}
	ids := adjacency[parent]
	start, end := elementread.PageBounds(ids, after, limit)
	records := make([]contract.ElementRecord, 0, end-start)
	for _, id := range ids[start:end] {
		if anchor, exists := core.owner.FactsLocked().Elements.Anchor(id); exists {
			records = append(records, anchor.Record())
		} else if entity, exists := core.owner.FactsLocked().Elements.Entity(id); exists {
			records = append(records, entity.Record())
		} else {
			return contract.DirectPage{}, contract.ErrStore
		}
	}
	page := contract.DirectPage{Records: records, Cut: cut}
	if end < len(ids) {
		next := token.ListContinuationFromParts(core.owner.FactsLocked().Scope, parent, cut, ids[end-1])
		page.Continuation = &next
	}
	return page, nil
}
