package fact

import (
	"crypto/sha256"
	"encoding/hex"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/geometry"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	"ucupa/ucupa-core/internal/token"
)

type ParentRole uint8

const (
	SourceParent ParentRole = iota + 1
	TargetParent
)

// ParentIntent binds an existing source intent and one upstream relationship.
// It preserves that relationship, not its carrier's content. The operation must
// obtain Initial from the authenticated source and independently authorize the
// original actor at this parent; a well-formed value is not that authorization.
type ParentIntent struct {
	Initial         IntentRecord
	Role            ParentRole
	Parent          values.FactAuthorityRef
	Mount           values.ElementID
	ExpectedElement contract.Revision
	ExpectedMount   contract.MountRevision
	ParentAddress   values.NextHop
}

func (i ParentIntent) Carrier() values.FactAuthorityRef {
	if i.Role == SourceParent {
		return i.Initial.Input.Source
	}
	return i.Initial.Input.Target
}

func (i ParentIntent) CarrierAddress() values.NextHop {
	if i.Role == SourceParent {
		return i.Initial.Input.SourceAddress
	}
	return i.Initial.Input.TargetAddress
}

func (i ParentIntent) Valid() bool {
	r := i.Initial
	return r.Input.Valid() && r.Input.Profile == SliceTransfer && r.Progress == IntentPending && r.Revision > 0 &&
		token.Valid(r.Admission) && token.Scope(r.Admission) == token.RevisionScope(r.Input.Expected) &&
		r.FinalCut == (contract.CoreReadCut{}) && r.FenceRevision == 0 && r.DecisionRevision == 0 && r.Origin == (contract.SpatialTuple{}) &&
		(i.Role == SourceParent || i.Role == TargetParent) && i.Parent.Valid() && i.Parent != i.Carrier() && i.Mount.Valid() &&
		token.ValidRevision(i.ExpectedElement) && token.RevisionElement(i.ExpectedElement) == i.Mount &&
		token.ValidMountRevision(i.ExpectedMount) && token.MountRevisionElement(i.ExpectedMount) == i.Mount &&
		token.MountRevisionScope(i.ExpectedMount) == token.RevisionScope(i.ExpectedElement) && i.ParentAddress.Valid()
}

// Slot is the local index of a qualified transfer/role/mount, not a new public
// operation or permission. Exact input changes retain this key and conflict.
func (i ParentIntent) Slot() contract.OperationID {
	h := sha256.New()
	h.Write([]byte("ucupa-transfer-parent\x00"))
	h.Write([]byte{byte(i.Role)})
	h.Write([]byte(i.Initial.Input.Source)) // fixed canonical AuthorityRef
	h.Write([]byte(i.Mount))                // fixed canonical UUID
	h.Write([]byte(i.Initial.Input.Operation))
	return contract.OperationID("transfer-parent:" + hex.EncodeToString(h.Sum(nil)))
}

type ParentHold struct {
	Input     ParentIntent
	Admission contract.MountAdmissionKey
	Tuple     contract.SpatialTuple
	Contact   contract.NextHopRevision
	At        contract.CoreReadCut
	Revision  uint64
}

func (h ParentHold) Valid() bool {
	i, k := h.Input, h.Admission.MountKey
	return i.Valid() && mountfact.ValidMountAdmissionKey(h.Admission) && k.ParentAuthority == i.Parent &&
		k.CarrierAuthority == i.Carrier() && k.MountAnchorUUID == i.Mount && h.Tuple.Parent.Valid() && h.Tuple.Parent != i.Mount &&
		geometry.CanonicalPose(h.Tuple.LocalPose) && geometry.ValidSphere(h.Tuple.Range) && h.Revision > 0 &&
		token.Valid(h.At) && token.Scope(h.At) == token.MountRevisionScope(i.ExpectedMount) &&
		token.MountRevisionScope(k.ProposalRevision) == token.Scope(h.At) && token.MountRevisionSerial(k.ProposalRevision) <= token.MountRevisionSerial(i.ExpectedMount) &&
		token.Serial(h.At) > token.MountRevisionSerial(i.ExpectedMount) &&
		token.ValidNextHopRevision(h.Contact) && !token.NextHopIncoming(h.Contact) && token.NextHopKey(h.Contact) == k &&
		token.NextHopScope(h.Contact) == token.Scope(h.At) && token.NextHopSerial(h.Contact) < token.Serial(h.At)
}

// ParentRelease uses source-authoritative terminal evidence. Silence, a target
// acceptance, or a not-yet-settled source is never enough to drop the hold.
// The two alternatives are exclusive and never permit another transfer input.
type ParentRelease struct {
	Cancelled IntentRecord
	Settled   SourceSettlement
}

func (r ParentRelease) ValidFor(h ParentHold) bool {
	if !h.Valid() {
		return false
	}
	initial := h.Input.Initial
	if r.Settled == (SourceSettlement{}) {
		c := r.Cancelled
		return c.Input == initial.Input && c.Progress == IntentCancelled && c.Admission == initial.Admission &&
			c.Revision > initial.Revision && c.FinalCut == (contract.CoreReadCut{}) && c.FenceRevision == 0 &&
			c.DecisionRevision == 0 && c.Origin == (contract.SpatialTuple{})
	}
	s := r.Settled
	d := s.Installation.Decision
	return r.Cancelled == (IntentRecord{}) && (CoreInstallation{Source: s.Installation}).Valid() &&
		d.Acceptance.Fence.Input == initial.Input && token.Scope(d.Acceptance.Fence.FinalCut) == token.Scope(initial.Admission) &&
		token.Serial(d.Acceptance.Fence.FinalCut) >= token.Serial(initial.Admission) && d.Revision > initial.Revision &&
		token.Valid(s.At) && token.Scope(s.At) == token.Scope(initial.Admission) && token.Serial(s.At) > token.Serial(d.At) && s.Revision > d.Revision
}

type ParentControl struct {
	Hold       ParentHold
	Release    ParentRelease
	ReleasedAt contract.CoreReadCut
	Revision   uint64
}

func (c ParentControl) Valid() bool {
	if !c.Hold.Valid() {
		return false
	}
	if c.ReleasedAt == (contract.CoreReadCut{}) {
		return c.Release == (ParentRelease{}) && c.Revision == c.Hold.Revision
	}
	return c.Release.ValidFor(c.Hold) && token.Valid(c.ReleasedAt) && token.Scope(c.ReleasedAt) == token.Scope(c.Hold.At) &&
		token.Serial(c.ReleasedAt) > token.Serial(c.Hold.At) && c.Revision > c.Hold.Revision
}
