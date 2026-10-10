package fact

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/token"
)

// CoreInstallation names the original target installation of exactly one Core
// transfer profile. It is evidence obtained from that authenticated target, not
// a caller credential or a new decision shared with another fact owner.
type CoreInstallation struct {
	Source  TargetInstallation
	Carrier CarrierInstallation
}

func (i CoreInstallation) Valid() bool {
	if i.Source == (TargetInstallation{}) {
		return i.Carrier.Valid()
	}
	if i.Carrier != (CarrierInstallation{}) {
		return false
	}
	a, d := i.Source.Decision.Acceptance, i.Source.Decision
	f := EdgeFence{Source: a.Fence, Mount: a.Fence.Input.Boundary}
	return f.Valid() && token.Valid(a.At) && a.Revision != 0 && token.Scope(a.At) != token.Scope(a.Fence.FinalCut) &&
		token.Valid(d.At) && d.Revision != 0 && token.Scope(d.At) == token.Scope(a.Fence.FinalCut) && token.Serial(d.At) > token.Serial(a.Fence.FinalCut) &&
		token.Valid(i.Source.At) && token.Scope(i.Source.At) == token.Scope(a.At) && token.Serial(i.Source.At) > token.Serial(a.At) && i.Source.Revision > a.Revision
}

func (i CoreInstallation) Fence(mount values.ElementID) EdgeFence {
	if i.Source != (TargetInstallation{}) {
		return EdgeFence{Source: i.Source.Decision.Acceptance.Fence, Mount: mount}
	}
	return EdgeFence{Carrier: i.Carrier.Binding.Acceptance.Fence, Mount: mount}
}

func (i CoreInstallation) Cut() contract.CoreReadCut {
	if i.Source != (TargetInstallation{}) {
		return i.Source.At
	}
	return i.Carrier.At
}

func (i CoreInstallation) Revision() uint64 {
	if i.Source != (TargetInstallation{}) {
		return i.Source.Revision
	}
	return i.Carrier.Revision
}

func (i CoreInstallation) Key() TransferKey {
	return i.Fence("").Key().Transfer
}

func (i CoreInstallation) Target() values.FactAuthorityRef {
	_, target, _ := i.Fence("").Owners()
	return target
}

func (i CoreInstallation) Address() values.NextHop {
	if i.Source != (TargetInstallation{}) {
		return i.Source.Decision.Acceptance.Fence.Input.TargetAddress
	}
	return i.Carrier.Binding.Acceptance.Fence.Permit.Reservation.Input.TargetAddress
}

// Delegation is derived from the installed target cut, not the old parent's
// revision or an invented ordinary MountPrepare. The UUID and carrier stay the
// same; only the parent owner and its newly issued relation change.
func (i CoreInstallation) Delegation(c EdgeClosure) (contract.MountAdmissionKey, error) {
	if !i.Valid() || i.Fence(c.Fence.Mount) != c.Fence || !c.Valid(c.Previous.Key.MountKey.CarrierAuthority) || token.Scope(i.Cut()) == token.Scope(c.At) {
		return contract.MountAdmissionKey{}, contract.ErrConflict
	}
	key := contract.MountKey{ParentAuthority: i.Target(), MountAnchorUUID: c.Fence.Mount, CarrierAuthority: c.Previous.Key.MountKey.CarrierAuthority,
		ProposalRevision: token.MountRevisionFromParts(token.Scope(i.Cut()), c.Fence.Mount, token.Serial(i.Cut()))}
	return contract.MountAdmissionKey{MountKey: key, OperationID: i.Key().Operation}, nil
}

// EdgeInstallation is the carrier's durable matching reparent result. It
// preserves that carrier's content and binds the target's actual installation.
type EdgeInstallation struct {
	Parent   CoreInstallation
	Closure  EdgeClosure
	At       contract.CoreReadCut
	Revision uint64
}

func (e EdgeInstallation) Valid() bool {
	_, err := e.Parent.Delegation(e.Closure)
	return err == nil && token.Valid(e.At) && token.Scope(e.At) == token.Scope(e.Closure.At) &&
		token.Serial(e.At) > token.Serial(e.Closure.At) && e.Revision > e.Closure.Revision
}

// CoreCompletion proves all retained child obligations were fulfilled. With no
// external edges, the original installation is already complete (same cut and
// revision); otherwise the target issues a later durable completion result.
type CoreCompletion struct {
	Installed CoreInstallation
	At        contract.CoreReadCut
	Revision  uint64
}

func (c CoreCompletion) Valid() bool {
	if !c.Installed.Valid() || !token.Valid(c.At) || token.Scope(c.At) != token.Scope(c.Installed.Cut()) {
		return false
	}
	return c.At == c.Installed.Cut() && c.Revision == c.Installed.Revision() ||
		token.Serial(c.At) > token.Serial(c.Installed.Cut()) && c.Revision > c.Installed.Revision()
}
