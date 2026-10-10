package commit

import (
	"fmt"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	commitbinding "ucupa/ucupa-core/internal/commit/binding"
	elementfact "ucupa/ucupa-core/internal/element/fact"
)

var foundationCandidateActor = values.PrincipalRef{Realm: "oracle-realm", Principal: "oracle"}
var foundationCandidateMountActor = values.PrincipalRef{Realm: "store-realm", Principal: "store-writer"}

type foundationCandidateElementFixture struct {
	owner        *Owner
	root, entity values.ElementID
}

type foundationCandidateMountFixture struct {
	owner       *Owner
	root, mount values.ElementID
	carrier     values.FactAuthorityRef
	command     contract.MountCommand
	callers     contract.LocalCallers
}

func foundationCandidateID(n uint64) values.ElementID {
	return values.ElementID(fmt.Sprintf("10000000-0000-4000-8000-%012x", n))
}

func foundationCandidateMountID(n uint64) values.ElementID {
	return values.ElementID(fmt.Sprintf("20000000-0000-4000-8000-%012x", n))
}

func foundationCandidateTuple(t *testing.T, parent values.ElementID, x, y, z, radius float64) contract.SpatialTuple {
	t.Helper()
	rotation, err := contract.NewRotation(1, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	pose, err := contract.NewPose(contract.Vec3{X: x, Y: y, Z: z}, rotation)
	if err != nil {
		t.Fatal(err)
	}
	sphere, err := contract.NewBoundedSphere(radius)
	if err != nil {
		t.Fatal(err)
	}
	return contract.SpatialTuple{Parent: parent, LocalPose: pose, Range: sphere}
}

func foundationCandidateApplyElement(t *testing.T, owner *Owner, operation contract.OperationID, binding commitbinding.Binding) contract.MutationResult {
	t.Helper()
	owner.Lock()
	result, err := owner.ApplyElementLocked(operation, binding)
	owner.Unlock()
	if err != nil || result.Status != contract.MutationCommitted || result.Record == nil {
		t.Fatalf("seed Element %q: %#v %v", operation, result, err)
	}
	return result
}

func newFoundationCandidateElementFixture(t *testing.T) foundationCandidateElementFixture {
	t.Helper()
	authority := values.FactAuthorityRef(foundationCandidateID(899000))
	owner, err := New(authority, [32]byte{1}, contract.RealmInitialization{Realm: "fixture-realm", NextHop: "local:fixture"})
	if err != nil {
		t.Fatal(err)
	}
	f := foundationCandidateElementFixture{owner: owner, root: foundationCandidateID(811000), entity: foundationCandidateID(811001)}
	foundationCandidateApplyElement(t, owner, "attach-root", commitbinding.Binding{
		Actor: foundationCandidateActor, Kind: elementfact.OperationCreateRoot, Element: f.root,
	})
	foundationCandidateApplyElement(t, owner, "attach-entity", commitbinding.Binding{
		Actor: foundationCandidateActor, Kind: elementfact.OperationCreateEntity, Element: f.entity,
		Final: foundationCandidateTuple(t, f.root, 0, 0, 0, 1),
	})
	return f
}

func newFoundationCandidateMountFixture(t *testing.T) foundationCandidateMountFixture {
	t.Helper()
	authority := values.FactAuthorityRef(foundationCandidateMountID(9298))
	carrier := values.FactAuthorityRef(foundationCandidateMountID(9299))
	owner, err := New(authority, [32]byte{2}, contract.RealmInitialization{Realm: "fixture-realm", NextHop: "local:fixture"})
	if err != nil {
		t.Fatal(err)
	}
	f := foundationCandidateMountFixture{owner: owner, root: foundationCandidateMountID(9200), mount: foundationCandidateMountID(9201), carrier: carrier}
	foundationCandidateApplyElement(t, owner, "control-root", commitbinding.Binding{
		Actor: foundationCandidateMountActor, Kind: elementfact.OperationCreateRoot, Element: f.root,
	})
	mount := foundationCandidateApplyElement(t, owner, "control-mount", commitbinding.Binding{
		Actor: foundationCandidateMountActor, Kind: elementfact.OperationCreateAnchor, Element: f.mount,
		Final: foundationCandidateTuple(t, f.root, 0, 0, 0, 10),
	})
	f.command = contract.MountCommand{
		Kind: contract.MountEstablish, MountAnchorUUID: f.mount,
		ExpectedElementRevision: mount.Record.Anchor.Revision,
		CarrierAuthority:        carrier, CarrierNextHop: "local:b", ParentNextHop: "local:a",
	}
	f.callers = contract.LocalCallers{authority: foundationCandidateMountActor, carrier: foundationCandidateMountActor}
	return f
}

func foundationCandidateOtherAuthority(t *testing.T, current values.FactAuthorityRef) values.FactAuthorityRef {
	t.Helper()
	ref := values.FactAuthorityRef(foundationCandidateID(899099))
	if !ref.Valid() || ref == current {
		t.Fatalf("other authority: %q", ref)
	}
	return ref
}
