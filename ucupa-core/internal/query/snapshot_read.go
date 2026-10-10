package query

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementread "ucupa/ucupa-core/internal/element/read"
	queryscan "ucupa/ucupa-core/internal/query/scan"
	coretoken "ucupa/ucupa-core/internal/token"
)

func (core *Operations) ReadElementSnapshot(actor values.PrincipalRef, id values.ElementID, snapshot contract.CoreReadSnapshot) (contract.ElementRead, error) {
	if core == nil || !id.Valid() || !coretoken.ValidCoreReadSnapshot(snapshot) {
		return contract.ElementRead{}, contract.ErrInvalid
	}
	permission := core.requirement(contract.PermissionReadElement, id)
	approval, err := core.authorize(actor, permission)
	if err != nil {
		return contract.ElementRead{}, err
	}
	borrow, decision := core.Borrow(actor, snapshot)
	var result contract.ElementRead
	if decision == nil {
		defer borrow.Finish()
		decision = validateReadView(borrow.view)
		if decision == nil {
			closure, err := queryscan.BoundaryClosure(borrow.view.projection.Elements, coretoken.SnapshotScope(snapshot), nil)
			if err != nil {
				decision = contract.ErrStore
			} else {
				projection := borrow.view.projection
				if _, exists := projection.Elements.Anchor(id); exists {
					if _, inside := closure[id]; inside {
						result, decision = elementread.Element(projection.Authority, projection.Elements, projection.Attachments, projection.Transfers, id, coretoken.SnapshotCut(snapshot))
					}
				} else if e, exists := projection.Elements.Entity(id); exists {
					if _, inside := closure[e.Parent]; inside {
						result, decision = elementread.Element(projection.Authority, projection.Elements, projection.Attachments, projection.Transfers, id, coretoken.SnapshotCut(snapshot))
					}
				}
				if result.Record == nil && decision == nil {
					decision = contract.ErrInvalid
				}
			}
		}
	}
	approval, err = core.authorize(actor, permission)
	if err != nil {
		return contract.ElementRead{}, err
	}
	if borrow != nil {
		core.owner.RLock()
		defer core.owner.RUnlock()
		if !core.settled(approval) {
			return contract.ElementRead{}, contract.ErrUnavailable
		}
		if err := core.CheckLocked(borrow, id); err != nil {
			return contract.ElementRead{}, err
		}
	}
	if decision != nil {
		return contract.ElementRead{}, decision
	}
	return result, nil
}
