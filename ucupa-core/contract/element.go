package contract

import "ucupa/shared/values"

// OperationID identifies one exact mutation within one Core lifetime or Store.
// It is opaque and caller supplied; its canonical byte length is 1..128.
type OperationID string

type ElementKind uint8

const (
	ElementAnchor ElementKind = iota + 1
	ElementEntity
)

type MutationStatus uint8

const (
	MutationCommitted MutationStatus = iota + 1
	MutationUnchanged
	MutationRejected
)

type SpatialTuple struct {
	Parent    values.ElementID
	LocalPose Pose
	Range     BoundedSphere
}

type AnchorRecord struct {
	ID             values.ElementID
	ParentAnchorID *values.ElementID
	LocalPose      *Pose
	Range          Range
	Revision       Revision
}

type EntityRecord struct {
	ID                 values.ElementID
	ContainingAnchorID values.ElementID
	LocalPose          Pose
	Range              BoundedSphere
	Revision           Revision
}

type ElementRecord struct {
	Kind   ElementKind
	Anchor *AnchorRecord
	Entity *EntityRecord
}

type MutationResult struct {
	Status            MutationStatus
	Record            *ElementRecord
	TombstoneRevision Revision
	OutcomeCut        CoreReadCut
}

// Element permissions are shared by local operations and their consumers.
const (
	PermissionCreateRoot     values.PermissionID = "ucupa.core.anchor.create-root"
	PermissionCreateAnchor   values.PermissionID = "ucupa.core.anchor.create"
	PermissionCreateEntity   values.PermissionID = "ucupa.core.entity.create"
	PermissionReadElement    values.PermissionID = "ucupa.core.element.read"
	PermissionObserveElement values.PermissionID = "ucupa.core.element.observe"
	PermissionListDirect     values.PermissionID = "ucupa.core.anchor.list-direct"
	PermissionUpdateElement  values.PermissionID = "ucupa.core.element.update"
	PermissionPlaceElement   values.PermissionID = "ucupa.core.element.place"
	PermissionDeleteAnchor   values.PermissionID = "ucupa.core.anchor.delete"
	PermissionDeleteEntity   values.PermissionID = "ucupa.core.entity.delete"
)
