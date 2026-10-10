package commit

import (
	"fmt"

	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/token"
)

func validateTransferParentFacts(f mountSnapshotFacts, slot contract.OperationID, op MountOperation) error {
	c, h := op.Parent, op.Parent.Hold
	i := h.Input
	if !c.Valid() || op != (MountOperation{Parent: c}) || slot != i.Slot() || i.Parent != f.authority ||
		!validLocalMountCutValues(f.scope, f.cutSerial, h.At) || token.RevisionSerial(i.ExpectedElement) > f.revisionSerial {
		return fmt.Errorf("invalid transfer parent hold")
	}
	r := f.mount.Record(i.Mount)
	if c.ReleasedAt != (contract.CoreReadCut{}) {
		if !validLocalMountCutValues(f.scope, f.cutSerial, c.ReleasedAt) || r.Slot == slot {
			return fmt.Errorf("released parent hold still owns slot")
		}
		return nil
	}
	a, exists := f.elements.Anchor(i.Mount)
	if !exists || a.Root() || a.Revision != i.ExpectedElement {
		return fmt.Errorf("transfer parent hold lost anchor")
	}
	rangeValue, bounded := a.Range.BoundedSphere()
	if !bounded || h.Tuple != (contract.SpatialTuple{Parent: a.Parent, LocalPose: a.LocalPose, Range: rangeValue}) ||
		r.Admission != h.Admission || r.Slot != slot || r.Revision != token.MountRevisionFromParts(f.scope, i.Mount, token.Serial(h.At)) {
		return fmt.Errorf("transfer parent hold lost relation or placement")
	}
	return nil
}
