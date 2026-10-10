package commit

import (
	"fmt"

	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/token"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// Current-cut validation needs only the occupied slot and its finite proof.
// Complete retained history is separately validated by the original replay.
func validateCarrierMountOperationFacts(f mountSnapshotFacts, id contract.OperationID, op MountOperation) error {
	c := op.Carrier
	r := c.Reservation
	i := r.Input
	b := op.CarrierBinding
	settled := op.CarrierSettlement
	if !c.Valid() || op != (MountOperation{Carrier: c, CarrierBinding: b, CarrierSettlement: settled}) || i.Operation != id || i.Parent != f.authority ||
		!validLocalMountCutValues(f.scope, f.cutSerial, r.At) || token.RevisionSerial(i.ExpectedElement) > f.revisionSerial {
		return fmt.Errorf("invalid carrier reservation")
	}
	for _, at := range []contract.CoreReadCut{c.PermitAt, c.CancelledAt} {
		if at != (contract.CoreReadCut{}) && !validLocalMountCutValues(f.scope, f.cutSerial, at) {
			return fmt.Errorf("invalid carrier control cut")
		}
	}
	if c.CancelledAt != (contract.CoreReadCut{}) {
		if b != (transferfact.CarrierBinding{}) || settled != (transferfact.CarrierSettlement{}) {
			return fmt.Errorf("cancelled carrier has binding")
		}
		return nil
	}
	if b != (transferfact.CarrierBinding{}) && (!b.Valid() || b.Acceptance.Fence.Permit != c || !validLocalMountCutValues(f.scope, f.cutSerial, b.At)) {
		return fmt.Errorf("invalid carrier binding decision")
	}
	if settled != (transferfact.CarrierSettlement{}) {
		if !settled.Valid(f.authority) || settled.Installation.Binding != b || !validLocalMountCutValues(f.scope, f.cutSerial, settled.At) || settled.Revision <= b.Revision {
			return fmt.Errorf("invalid carrier settlement")
		}
		// Terminal evidence survives later range updates, revocation and further
		// replacement. Replay, not today's relation, proves its original slot.
		if f.mount.Record(i.Mount).Slot == id {
			return fmt.Errorf("settled carrier still owns control slot")
		}
		return nil
	}
	a, ok := f.elements.Anchor(i.Mount)
	if !ok || a.Root() || a.Revision != i.ExpectedElement {
		return fmt.Errorf("carrier reservation lost mount anchor")
	}
	sphere, ok := a.Range.BoundedSphere()
	if !ok || r.Tuple != (contract.SpatialTuple{Parent: a.Parent, LocalPose: a.LocalPose, Range: sphere}) {
		return fmt.Errorf("carrier reservation lost spatial tuple")
	}
	m := f.mount.Record(i.Mount)
	if b != (transferfact.CarrierBinding{}) {
		if m.State != contract.MountActive || m.Key != r.NextKey || m.Admission != (contract.MountAdmissionKey{MountKey: r.NextKey, OperationID: id}) || m.Slot != id || m.Revision != token.MountRevisionFromParts(f.scope, i.Mount, token.Serial(b.At)) {
			return fmt.Errorf("carrier binding lost committed relation")
		}
		return nil
	}
	if m.State != contract.MountActive || m.Key != r.Previous.MountKey || m.Admission != r.Previous || m.Slot != id || m.Revision != r.NextKey.ProposalRevision {
		return fmt.Errorf("carrier replacement lost parent control slot")
	}
	return nil
}
