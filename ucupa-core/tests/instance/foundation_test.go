package instance_test

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
)

func TestFoundationAuthorityRefIsStableAndOwnerLocal(t *testing.T) {
	var absent *core.Core
	if absent.AuthorityRef().Valid() {
		t.Fatal("nil Core acquired an identity")
	}
	first, second := newFoundationCore(t), newFoundationCore(t)
	if !first.AuthorityRef().Valid() || !second.AuthorityRef().Valid() || first.AuthorityRef() == second.AuthorityRef() {
		t.Fatal("independent Cores must receive distinct canonical identities")
	}
	identity := first.AuthorityRef()
	foundationCreateRoot(t, first, foundationID(8000), "identity-root")
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if first.AuthorityRef() != identity {
		t.Fatal("Close changed immutable Core identity")
	}
	defer second.Close()
}

func TestFoundationAuthorityRefSurvivesEmptyAndNonemptyReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.db")
	created, err := core.CreateDurable(instancePolicyContact{foundationAllowAuthorizer{}}, instanceContact, path, corecontract.Config{})
	if err != nil {
		t.Fatal(err)
	}
	identity := created.AuthorityRef()
	if !identity.Valid() {
		t.Fatalf("new durable identity: %q", identity)
	}
	if err := created.Close(); err != nil {
		t.Fatal(err)
	}
	empty, err := core.OpenDurable(instancePolicyContact{foundationAllowAuthorizer{}}, path, corecontract.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if empty.AuthorityRef() != identity {
		t.Fatal("empty Store acquired a new identity on reopen")
	}
	root := foundationCreateRoot(t, empty, foundationID(8001), "durable-identity-root")
	if err := empty.Close(); err != nil {
		t.Fatal(err)
	}
	if empty.AuthorityRef() != identity {
		t.Fatal("Close changed durable Core identity")
	}
	if _, err := empty.Elements().ReadElement(foundationActor, root.Anchor.ID, nil); !errors.Is(err, corecontract.ErrStore) {
		t.Fatalf("identity availability must not imply fact availability: %v", err)
	}
	nonempty, err := core.OpenDurable(instancePolicyContact{foundationAllowAuthorizer{}}, path, corecontract.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer nonempty.Close()
	if nonempty.AuthorityRef() != identity {
		t.Fatal("nonempty Store acquired a new identity on reopen")
	}
	replay, err := nonempty.Elements().CreateRoot(foundationActor, root.Anchor.ID, "durable-identity-root")
	if err != nil || replay.Status != corecontract.MutationCommitted || replay.Record == nil || !replay.Record.Anchor.Revision.Equal(root.Anchor.Revision) {
		t.Fatalf("identity-preserving replay: %#v err=%v", replay, err)
	}
}

type foundationOwnerAuthorizer struct {
	owner values.FactAuthorityRef
}

func (policy *foundationOwnerAuthorizer) Authorize(_ values.PrincipalRef, requirements ...values.Requirement) bool {
	if len(requirements) == 0 {
		return false
	}
	for _, requirement := range requirements {
		owner, _, ok := requirement.Scope.CoreElementTarget()
		if !requirement.Valid() || !ok || owner != policy.owner {
			return false
		}
	}
	return true
}

func TestFoundationSameElementIDDoesNotShareOwnerAuthorization(t *testing.T) {
	policy := &foundationOwnerAuthorizer{}
	first, err := core.New(instancePolicyContact{policy}, instanceContact, corecontract.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := core.New(instancePolicyContact{policy}, instanceContact, corecontract.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	policy.owner = first.AuthorityRef()
	rootID := foundationID(8002)
	foundationCreateRoot(t, first, rootID, "same-root-id")
	result, err := second.Elements().CreateRoot(foundationActor, rootID, "same-root-id")
	if !errors.Is(err, corecontract.ErrDenied) || result != (corecontract.MutationResult{}) {
		t.Fatalf("first owner's grant authorized a second owner: %#v err=%v", result, err)
	}
	read, err := second.Elements().ReadElement(foundationActor, rootID, nil)
	if !errors.Is(err, corecontract.ErrDenied) || read != (corecontract.ElementRead{}) {
		t.Fatalf("first owner's grant disclosed a second owner: %#v err=%v", read, err)
	}
	policy.owner = second.AuthorityRef()
	foundationCreateRoot(t, second, rootID, "same-root-id")
	read, err = first.Elements().ReadElement(foundationActor, rootID, nil)
	if !errors.Is(err, corecontract.ErrDenied) || read != (corecontract.ElementRead{}) {
		t.Fatalf("second owner's grant disclosed the first owner: %#v err=%v", read, err)
	}
}

var foundationActor = values.PrincipalRef{Realm: "test-realm", Principal: "builder"}

type foundationAllowAuthorizer struct{}

func (foundationAllowAuthorizer) Authorize(values.PrincipalRef, ...values.Requirement) bool {
	return true
}
func newFoundationCore(t *testing.T) *core.Core {
	t.Helper()
	c, err := core.New(instancePolicyContact{foundationAllowAuthorizer{}}, instanceContact, corecontract.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func foundationID(n uint64) values.ElementID {
	return values.ElementID(fmt.Sprintf("00000000-0000-4000-8000-%012x", n))
}
func foundationCreateRoot(t *testing.T, c *core.Core, id values.ElementID, op corecontract.OperationID) corecontract.ElementRecord {
	t.Helper()
	r, err := c.Elements().CreateRoot(foundationActor, id, op)
	if err != nil || r.Status != corecontract.MutationCommitted || r.Record == nil {
		t.Fatalf("create root: %#v %v", r, err)
	}
	return *r.Record
}
