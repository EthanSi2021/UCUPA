package fact

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/geometry"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	"ucupa/ucupa-core/internal/token"
)

// EdgeFence identifies one outgoing boundary in an authenticated, complete
// source manifest. Exactly one source profile is present; it is not a permit
// minted by the destination or a second transfer of the carrier's content.
type EdgeFence struct {
	Source  SourceFence
	Carrier CarrierFence
	Mount   values.ElementID
}

type EdgeKey struct {
	Transfer TransferKey
	Mount    values.ElementID
}

func (f EdgeFence) Valid() bool {
	local := f.Source.Input.Valid() && token.Valid(f.Source.FinalCut) &&
		token.Scope(f.Source.FinalCut) == token.RevisionScope(f.Source.Input.Expected) && f.Source.ManifestDigest != ([32]byte{})
	return f.Mount.Valid() && ((local && f.Carrier == (CarrierFence{})) ||
		(f.Source == (SourceFence{}) && f.Carrier.Valid()))
}

func (f EdgeFence) Key() EdgeKey {
	if f.Source != (SourceFence{}) {
		return EdgeKey{Transfer: f.Source.Key(), Mount: f.Mount}
	}
	return EdgeKey{Transfer: f.Carrier.Permit.Reservation.Input.Key(), Mount: f.Mount}
}

func (f EdgeFence) Owners() (source, target values.FactAuthorityRef, actor values.PrincipalRef) {
	if f.Source != (SourceFence{}) {
		i := f.Source.Input
		return i.Source, i.Target, i.Actor
	}
	i := f.Carrier.Permit.Reservation.Input
	return i.Source, i.Target, i.Actor
}

// EdgeClosure is the carrier's durable hold. Its content remains at the same
// owner, but the old parent cannot reopen its write or read gate. A later exact
// reparent result, not a timeout or an ordinary mount retry, releases this hold.
type EdgeClosure struct {
	Fence    EdgeFence
	Previous mountfact.Admission
	Range    contract.BoundedSphere
	At       contract.CoreReadCut
	Revision uint64
}

func (c EdgeClosure) Valid(owner values.FactAuthorityRef) bool {
	source, target, _ := c.Fence.Owners()
	key := c.Previous.Key.MountKey
	return c.Fence.Valid() && owner.Valid() && target != owner &&
		mountfact.ValidMountAdmissionKey(c.Previous.Key) && key.ParentAuthority == source &&
		key.CarrierAuthority == owner && key.MountAnchorUUID == c.Fence.Mount &&
		geometry.ValidSphere(c.Previous.RangeValue) && geometry.ValidSphere(c.Range) &&
		token.Valid(c.Previous.DecisionCut) && token.Scope(c.Previous.DecisionCut) == token.MountRevisionScope(key.ProposalRevision) &&
		token.Valid(c.At) && token.Scope(c.At) != token.Scope(c.Previous.DecisionCut) && c.Revision != 0
}

// EdgeRange uses the caller's explicit final range only when the transferred
// boundary is this very mount. The old carrier must validate its whole content
// against it; changing the surrounding pose never silently rewrites that content.
func (f EdgeFence) EdgeRange(member Member) (contract.BoundedSphere, error) {
	if !f.Valid() || member.Kind != contract.ElementAnchor || member.ID() != f.Mount || member.Mount.State != contract.MountActive {
		return contract.BoundedSphere{}, contract.ErrConflict
	}
	r, bounded := member.Anchor.Range.BoundedSphere()
	if !bounded {
		return contract.BoundedSphere{}, contract.ErrConflict
	}
	if f.Source.Input.Profile == SliceTransfer && f.Source.Input.Boundary == f.Mount {
		r = f.Source.Input.Final.Range
	}
	return r, nil
}
