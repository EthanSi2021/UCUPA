// Package fact holds the deterministic Element records and rules shared by
// normal operations and replay. It does not own authorization, commits or I/O.
package fact

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
)

type Anchor struct {
	ID        values.ElementID
	Parent    values.ElementID
	LocalPose contract.Pose
	Range     contract.Range
	Revision  contract.Revision
}

func (anchor Anchor) Root() bool { return anchor.Parent == "" }

// Record returns the public value without exposing an alias to private facts.
func (anchor Anchor) Record() contract.ElementRecord {
	record := contract.AnchorRecord{ID: anchor.ID, Range: anchor.Range, Revision: anchor.Revision}
	if !anchor.Root() {
		parent, pose := anchor.Parent, anchor.LocalPose
		record.ParentAnchorID, record.LocalPose = &parent, &pose
	}
	return contract.ElementRecord{Kind: contract.ElementAnchor, Anchor: &record}
}

type Entity struct {
	ID        values.ElementID
	Parent    values.ElementID
	LocalPose contract.Pose
	Range     contract.BoundedSphere
	Revision  contract.Revision
}

// Record returns an independent public value, not the stored map entry.
func (entity Entity) Record() contract.ElementRecord {
	record := contract.EntityRecord{
		ID: entity.ID, ContainingAnchorID: entity.Parent, LocalPose: entity.LocalPose,
		Range: entity.Range, Revision: entity.Revision,
	}
	return contract.ElementRecord{Kind: contract.ElementEntity, Entity: &record}
}

type OperationKind uint8

const (
	OperationInvalid OperationKind = iota
	OperationCreateRoot
	OperationCreateAnchor
	OperationCreateEntity
	OperationUpdateAnchor
	OperationUpdateEntity
	OperationDeleteAnchor
	OperationDeleteEntity
)

type Rejection uint8

const (
	NoError Rejection = iota
	NotFound
	Conflict
	RevisionConflict
)
