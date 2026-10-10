// Package record encodes and validates finite persisted Core values. It owns
// neither a Store nor live aggregate state; loading fills only a caller-owned
// element builder, used by both durable replay and bounded evidence decoding.
package record

import (
	"encoding/hex"
	"errors"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	"ucupa/ucupa-core/internal/geometry"
	storebytes "ucupa/ucupa-core/internal/store"
	"ucupa/ucupa-core/internal/token"
)

type StoredElement struct {
	Kind                string                  `json:"kind"`
	RevisionSerial      uint64                  `json:"revision"`
	Parent              string                  `json:"parent,omitempty"`
	Pose                *StoredPose             `json:"pose,omitempty"`
	RangeKind           string                  `json:"rangeKind,omitempty"`
	RadiusBits          string                  `json:"radiusBits,omitempty"`
	Departure           []byte                  `json:"departure,omitempty"`
	DepartureController values.FactAuthorityRef `json:"departureController,omitempty"`
	RootDeparture       bool                    `json:"rootDeparture,omitempty"`
}

type StoredPose struct {
	Translation [3]string `json:"translation"`
	Pivot       uint8     `json:"pivot"`
	Ratios      [3]string `json:"ratios"`
}

type StoredTuple struct {
	Parent     string     `json:"parent"`
	Pose       StoredPose `json:"pose"`
	RadiusBits string     `json:"radiusBits"`
}

type StoredRevision struct {
	Scope   string `json:"scope"`
	Element string `json:"element"`
	Serial  uint64 `json:"serial"`
}

func (stored StoredElement) childValues() (values.ElementID, contract.Pose, contract.BoundedSphere, error) {
	parent := values.ElementID(stored.Parent)
	if stored.RangeKind != "bounded-sphere" || !parent.Valid() || stored.Pose == nil || stored.RadiusBits == "" {
		return "", contract.Pose{}, contract.BoundedSphere{}, errors.New("bounded Element record shape is invalid")
	}
	pose, err := stored.Pose.pose()
	if err != nil {
		return "", contract.Pose{}, contract.BoundedSphere{}, err
	}
	radius, err := storebytes.DecodeFloatBits(stored.RadiusBits)
	if err != nil {
		return "", contract.Pose{}, contract.BoundedSphere{}, err
	}
	sphere := geometry.SphereFromStored(radius)
	if !geometry.ValidSphere(sphere) {
		return "", contract.Pose{}, contract.BoundedSphere{}, errors.New("bounded Range is invalid")
	}
	return parent, pose, sphere, nil
}

func StoreAnchor(anchor elementfact.Anchor) StoredElement {
	if anchor.Root() {
		return StoredElement{Kind: "anchor", RevisionSerial: token.RevisionSerial(anchor.Revision), RangeKind: "unbounded-root"}
	}
	sphere, _ := anchor.Range.BoundedSphere()
	pose := StorePose(anchor.LocalPose)
	return StoredElement{Kind: "anchor", RevisionSerial: token.RevisionSerial(anchor.Revision), Parent: string(anchor.Parent), Pose: &pose, RangeKind: "bounded-sphere", RadiusBits: storebytes.EncodeFloatBits(sphere.Radius())}
}

func StoreEntity(entity elementfact.Entity) StoredElement {
	pose := StorePose(entity.LocalPose)
	return StoredElement{Kind: "entity", RevisionSerial: token.RevisionSerial(entity.Revision), Parent: string(entity.Parent), Pose: &pose, RangeKind: "bounded-sphere", RadiusBits: storebytes.EncodeFloatBits(entity.Range.Radius())}
}

func StoreTombstone(revision contract.Revision) StoredElement {
	return StoredElement{Kind: "tombstone", RevisionSerial: token.RevisionSerial(revision)}
}

func StoreDeparture(departure elementfact.Departure) StoredElement {
	return StoredElement{Kind: "departure", RevisionSerial: token.RevisionSerial(departure.Revision), Departure: []byte(departure.Operation), DepartureController: departure.Controller, RootDeparture: departure.Root}
}

func StorePose(pose contract.Pose) StoredPose {
	r := pose.Rotation.Ratios()
	return StoredPose{
		Translation: [3]string{storebytes.EncodeFloatBits(pose.Translation.X), storebytes.EncodeFloatBits(pose.Translation.Y), storebytes.EncodeFloatBits(pose.Translation.Z)},
		Pivot:       uint8(pose.Rotation.Pivot()),
		Ratios:      [3]string{storebytes.EncodeFloatBits(r[0]), storebytes.EncodeFloatBits(r[1]), storebytes.EncodeFloatBits(r[2])},
	}
}

func (stored StoredPose) pose() (contract.Pose, error) {
	translation := [3]float64{}
	ratios := [3]float64{}
	for index := range translation {
		value, err := storebytes.DecodeFloatBits(stored.Translation[index])
		if err != nil || !geometry.CanonicalFinite(value) {
			return contract.Pose{}, errors.New("Pose translation is invalid")
		}
		translation[index] = value
	}
	for index := range ratios {
		value, err := storebytes.DecodeFloatBits(stored.Ratios[index])
		if err != nil {
			return contract.Pose{}, errors.New("Rotation ratio is invalid")
		}
		ratios[index] = value
	}
	pose := contract.Pose{Translation: contract.Vec3{X: translation[0], Y: translation[1], Z: translation[2]}, Rotation: geometry.RotationFromComponents(geometry.RotationPivot(stored.Pivot), ratios)}
	if !geometry.CanonicalPose(pose) {
		return contract.Pose{}, errors.New("Pose is not canonical")
	}
	return pose, nil
}

func StoreTuple(tuple contract.SpatialTuple) StoredTuple {
	return StoredTuple{Parent: string(tuple.Parent), Pose: StorePose(tuple.LocalPose), RadiusBits: storebytes.EncodeFloatBits(tuple.Range.Radius())}
}

func (stored StoredTuple) Tuple() (contract.SpatialTuple, error) {
	parent := values.ElementID(stored.Parent)
	if !parent.Valid() {
		return contract.SpatialTuple{}, errors.New("final parent is invalid")
	}
	pose, err := stored.Pose.pose()
	if err != nil {
		return contract.SpatialTuple{}, err
	}
	radius, err := storebytes.DecodeFloatBits(stored.RadiusBits)
	if err != nil {
		return contract.SpatialTuple{}, err
	}
	sphere := geometry.SphereFromStored(radius)
	if !geometry.ValidSphere(sphere) {
		return contract.SpatialTuple{}, errors.New("final Range is invalid")
	}
	return contract.SpatialTuple{Parent: parent, LocalPose: pose, Range: sphere}, nil
}

func StoreRevision(revision contract.Revision) StoredRevision {
	return StoredRevision{Scope: hex.EncodeToString(token.RevisionScopeBytes(revision)), Element: string(token.RevisionElement(revision)), Serial: token.RevisionSerial(revision)}
}

func (stored StoredRevision) Revision() (contract.Revision, error) {
	decoded, err := hex.DecodeString(stored.Scope)
	if err != nil || len(decoded) != 32 || hex.EncodeToString(decoded) != stored.Scope {
		return contract.Revision{}, errors.New("Revision scope is invalid")
	}
	var scope [32]byte
	copy(scope[:], decoded)
	revision := token.RevisionFromParts(scope, values.ElementID(stored.Element), stored.Serial)
	if !token.ValidRevision(revision) {
		return contract.Revision{}, errors.New("Revision is invalid")
	}
	return revision, nil
}
