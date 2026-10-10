package oracle_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/commit"
	"ucupa/ucupa-core/internal/element"
)

type oracleAuthorizer struct{}

func oracleAuthorizationViewDigest(subject values.PrincipalRef, requirements []values.Requirement) values.AuthorizationView {
	return values.AuthorizationView(sha256.Sum256([]byte(fmt.Sprintf("view:%v:%v", subject, requirements))))
}

func oracleAuthorizationDenialDigest(subject values.PrincipalRef, requirement values.Requirement) values.AuthorizationDenialView {
	return values.AuthorizationDenialView(sha256.Sum256([]byte(fmt.Sprintf("denial:%v:%v", subject, requirement))))
}

func (oracleAuthorizer) AuthorizePrincipal(values.PrincipalRef, ...values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationValid
}

func (oracleAuthorizer) AuthorizeView(ctx context.Context, caller values.CallerContext, requirements ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	return values.AuthorizationValid, oracleAuthorizationViewDigest(caller.Subject, requirements)
}

func (oracleAuthorizer) ValidateView(ctx context.Context, caller values.CallerContext, view values.AuthorizationView, requirements ...values.Requirement) values.AuthorizationDecision {
	if oracleAuthorizationViewDigest(caller.Subject, requirements) != view {
		return values.AuthorizationChanged
	}
	return values.AuthorizationValid
}

func (oracleAuthorizer) CaptureDenialView(ctx context.Context, caller values.CallerContext, requirement values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	return values.AuthorizationValid, values.AuthorizationDenialView{}
}

func (oracleAuthorizer) ValidateDenialView(ctx context.Context, caller values.CallerContext, view values.AuthorizationDenialView, requirement values.Requirement) values.AuthorizationDecision {
	if view == (values.AuthorizationDenialView{}) {
		return values.AuthorizationDenied
	}
	if _, current := (oracleAuthorizer{}).CaptureDenialView(ctx, caller, requirement); current != view {
		return values.AuthorizationChanged
	}
	return values.AuthorizationValid
}

var oracleActor = values.PrincipalRef{Realm: "oracle-realm", Principal: "oracle"}

type oracleFixture struct {
	owner    *commit.Owner
	elements *element.Operations
}

// The original cases create their actual facts through Element operations.
// Only instance assembly unrelated to the scan is omitted from this fixture.
func newOracleFixture() (*oracleFixture, error) {
	authority, err := values.NewFactAuthorityRef()
	if err != nil {
		return nil, err
	}
	var scope [32]byte
	if _, err := rand.Read(scope[:]); err != nil {
		return nil, err
	}
	owner, err := commit.New(authority, scope, contract.RealmInitialization{Realm: "fixture-realm", NextHop: "local:fixture"})
	if err != nil {
		return nil, err
	}
	return &oracleFixture{owner: owner, elements: element.New(owner, oracleAuthorizer{})}, nil
}

func (fixture *oracleFixture) Elements() *element.Operations { return fixture.elements }

func oracleID(n uint64) values.ElementID {
	return values.ElementID(fmt.Sprintf("10000000-0000-4000-8000-%012x", n))
}

func oracleTuple(t *testing.T, parent values.ElementID, x, y, z, radius float64) contract.SpatialTuple {
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

func oracleCreateRoot(t *testing.T, c *oracleFixture, id values.ElementID, op contract.OperationID) values.ElementID {
	t.Helper()
	result, err := c.Elements().CreateRoot(oracleActor, id, op)
	if err != nil || result.Record == nil {
		t.Fatalf("root %#v err=%v", result, err)
	}
	return result.Record.Anchor.ID
}

func oracleCreateAnchor(t *testing.T, c *oracleFixture, id values.ElementID, tuple contract.SpatialTuple, op contract.OperationID) values.ElementID {
	t.Helper()
	result, err := c.Elements().CreateAnchor(oracleActor, id, tuple, op)
	if err != nil || result.Record == nil {
		t.Fatalf("anchor %#v err=%v", result, err)
	}
	return result.Record.Anchor.ID
}

func oracleCreateEntity(t *testing.T, c *oracleFixture, id values.ElementID, tuple contract.SpatialTuple, op contract.OperationID) values.ElementID {
	t.Helper()
	result, err := c.Elements().CreateEntity(oracleActor, id, tuple, op)
	if err != nil || result.Record == nil {
		t.Fatalf("entity %#v err=%v", result, err)
	}
	return result.Record.Entity.ID
}

func oracleHas(records []contract.ElementRecord, id values.ElementID) bool {
	for _, record := range records {
		if record.Kind == contract.ElementAnchor && record.Anchor != nil && record.Anchor.ID == id {
			return true
		}
		if record.Kind == contract.ElementEntity && record.Entity != nil && record.Entity.ID == id {
			return true
		}
	}
	return false
}
