package element_test

import (
	"errors"
	"reflect"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/commit"
	"ucupa/ucupa-core/internal/element"
)

const (
	readRoot   values.ElementID = "31000000-0000-4000-8000-000000000001"
	readAnchor values.ElementID = "31000000-0000-4000-8000-000000000002"
	readEntity values.ElementID = "31000000-0000-4000-8000-000000000003"
	readLater  values.ElementID = "31000000-0000-4000-8000-000000000004"
	readAbsent values.ElementID = "31000000-0000-4000-8000-000000000099"
)

type readFixture struct {
	owner      *commit.Owner
	operations *element.Operations
	authority  values.FactAuthorityRef
}

func newReadFixture(t *testing.T, authorizer contract.Authorizer) readFixture {
	return newReadFixtureWithScope(t, authorizer, [32]byte{31})
}

func newReadFixtureWithScope(t *testing.T, authorizer contract.Authorizer, scope [32]byte) readFixture {
	t.Helper()
	authority, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	owner, err := commit.New(authority, scope, contract.RealmInitialization{Realm: "fixture-realm", NextHop: "local:fixture"})
	if err != nil {
		t.Fatal(err)
	}
	if authorizer == nil {
		authorizer = decide(func(actor values.PrincipalRef, _ ...values.Requirement) bool {
			return actor == localActor
		})
	}
	return readFixture{owner: owner, operations: element.New(owner, authorizer), authority: authority}
}

func childTuple(t *testing.T, parent values.ElementID) contract.SpatialTuple {
	t.Helper()
	rotation, err := contract.NewRotation(1, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	pose, err := contract.NewPose(contract.Vec3{}, rotation)
	if err != nil {
		t.Fatal(err)
	}
	radius, err := contract.NewBoundedSphere(0)
	if err != nil {
		t.Fatal(err)
	}
	return contract.SpatialTuple{Parent: parent, LocalPose: pose, Range: radius}
}

func seedReadFixture(t *testing.T, f readFixture) (contract.MutationResult, contract.MutationResult, contract.MutationResult) {
	t.Helper()
	root, err := f.operations.CreateRoot(localActor, readRoot, "read-root")
	if err != nil {
		t.Fatal(err)
	}
	anchor, err := f.operations.CreateAnchor(localActor, readAnchor, childTuple(t, readRoot), "read-anchor")
	if err != nil {
		t.Fatal(err)
	}
	entity, err := f.operations.CreateEntity(localActor, readEntity, childTuple(t, readRoot), "read-entity")
	if err != nil {
		t.Fatal(err)
	}
	return root, anchor, entity
}

func recordID(record contract.ElementRecord) values.ElementID {
	if record.Anchor != nil {
		return record.Anchor.ID
	}
	return record.Entity.ID
}

func TestElementReadUsesOwnerIdentityCurrentCutAndExactScope(t *testing.T) {
	var requirements []values.Requirement
	f := newReadFixture(t, decide(func(actor values.PrincipalRef, got ...values.Requirement) bool {
		if actor != localActor {
			return false
		}
		requirements = append(requirements, got...)
		return true
	}))
	seedReadFixture(t, f)
	requirements = nil

	var cut contract.CoreReadCut
	for _, id := range []values.ElementID{readRoot, readAnchor, readEntity} {
		read, err := f.operations.ReadElement(localActor, id, nil)
		if err != nil {
			t.Fatal(err)
		}
		if read.Authority != f.authority || read.Element != id || read.State != contract.ExistencePresent ||
			read.Record == nil || recordID(*read.Record) != id || read.Attachment.State != contract.FacetNone {
			t.Fatalf("read %s = %#v", id, read)
		}
		if cut == (contract.CoreReadCut{}) {
			cut = read.Cut
		} else if read.Cut != cut {
			t.Fatalf("current reads used different cuts: %#v %#v", cut, read.Cut)
		}
	}
	want := []values.Requirement{
		{Permission: contract.PermissionReadElement, Scope: values.CoreElementScope(f.authority, readRoot)},
		{Permission: contract.PermissionReadElement, Scope: values.CoreElementScope(f.authority, readAnchor)},
		{Permission: contract.PermissionReadElement, Scope: values.CoreElementScope(f.authority, readEntity)},
	}
	if !reflect.DeepEqual(requirements, want) {
		t.Fatalf("read requirements = %#v, want %#v", requirements, want)
	}
	missing, err := f.operations.ReadElement(localActor, readAbsent, &cut)
	if err != nil || missing.Authority != f.authority || missing.Element != readAbsent ||
		missing.State != contract.ExistenceNotFound || missing.Record != nil || missing.Cut != cut {
		t.Fatalf("missing read = %#v, %v", missing, err)
	}
	if f.operations.AuthorityRef() != f.authority {
		t.Fatal("operation authority differs from owner authority")
	}
}

func TestElementListDirectOrdersPagesAndExpiresContinuation(t *testing.T) {
	f := newReadFixture(t, nil)
	_, _, entity := seedReadFixture(t, f)
	first, err := f.operations.ListDirect(localActor, readRoot, 1, nil)
	if err != nil || len(first.Records) != 1 || recordID(first.Records[0]) != readAnchor || first.Continuation == nil {
		t.Fatalf("first page = %#v, %v", first, err)
	}
	second, err := f.operations.ListDirect(localActor, readRoot, 1, first.Continuation)
	if err != nil || len(second.Records) != 1 || recordID(second.Records[0]) != readEntity ||
		second.Continuation != nil || second.Cut != first.Cut {
		t.Fatalf("second page = %#v, %v", second, err)
	}
	if _, err := f.operations.ListDirect(localActor, readEntity, 1, nil); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("entity parent error = %v", err)
	}
	if _, err := f.operations.ListDirect(localActor, readAbsent, 1, nil); !errors.Is(err, contract.ErrNotFound) {
		t.Fatalf("unknown parent error = %v", err)
	}
	if _, err := f.operations.DeleteEntity(localActor, readEntity, entity.Record.Entity.Revision, "read-delete"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.operations.ListDirect(localActor, readRoot, 1, first.Continuation); !errors.Is(err, contract.ErrReadCutExpired) {
		t.Fatalf("old continuation error = %v", err)
	}
}

func TestElementReadRejectsForeignCut(t *testing.T) {
	local := newReadFixture(t, nil)
	seedReadFixture(t, local)
	foreign := newReadFixtureWithScope(t, nil, [32]byte{32})
	if _, err := foreign.operations.CreateRoot(localActor, readLater, "foreign-root"); err != nil {
		t.Fatal(err)
	}
	foreignRead, err := foreign.operations.ReadElement(localActor, readLater, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := local.operations.ReadElement(localActor, readRoot, &foreignRead.Cut); !errors.Is(err, contract.ErrInvalid) {
		t.Fatalf("foreign cut error = %v", err)
	}
}

func TestElementReadAuthorizationPrecedesStateAndClose(t *testing.T) {
	f := newReadFixture(t, nil)
	seedReadFixture(t, f)
	denied := element.New(f.owner, decide(func(values.PrincipalRef, ...values.Requirement) bool { return false }))
	if _, err := denied.ReadElement(localActor, readAbsent, nil); !errors.Is(err, contract.ErrDenied) {
		t.Fatalf("missing state leaked before authorization: %v", err)
	}
	if _, err := denied.ListDirect(localActor, readAbsent, 1, nil); !errors.Is(err, contract.ErrDenied) {
		t.Fatalf("parent state leaked before authorization: %v", err)
	}
	f.owner.Lock()
	f.owner.BeginCloseLocked()
	f.owner.Unlock()
	if _, err := denied.ReadElement(localActor, readRoot, nil); !errors.Is(err, contract.ErrDenied) {
		t.Fatalf("close leaked before authorization: %v", err)
	}
	if _, err := f.operations.ReadElement(localActor, readRoot, nil); !errors.Is(err, contract.ErrStore) {
		t.Fatalf("authorized closed read error = %v", err)
	}
	if _, err := f.operations.ListDirect(localActor, readRoot, 1, nil); !errors.Is(err, contract.ErrStore) {
		t.Fatalf("authorized closed list error = %v", err)
	}
}

func TestNilElementReadEntryReturnsInvalid(t *testing.T) {
	var absent *element.Operations
	read, err := absent.ReadElement(localActor, readRoot, nil)
	if !errors.Is(err, contract.ErrInvalid) || !reflect.DeepEqual(read, contract.ElementRead{}) {
		t.Fatalf("nil read = %#v, %v", read, err)
	}
	page, err := absent.ListDirect(localActor, readRoot, 1, nil)
	if !errors.Is(err, contract.ErrInvalid) || !reflect.DeepEqual(page, contract.DirectPage{}) {
		t.Fatalf("nil list = %#v, %v", page, err)
	}
	if absent.AuthorityRef() != "" {
		t.Fatal("nil operation exposed an authority")
	}
}
