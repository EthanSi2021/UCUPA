package commit_test

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/commit"
	"ucupa/ucupa-core/internal/element"
	queryoracle "ucupa/ucupa-core/internal/query/oracle"
)

var linearOracleActor = values.PrincipalRef{Realm: "oracle-realm", Principal: "oracle"}

type linearOraclePolicy struct{}

func (linearOraclePolicy) AuthorizePrincipal(values.PrincipalRef, ...values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationValid
}

func (linearOraclePolicy) AuthorizeView(ctx context.Context, caller values.CallerContext, requirements ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	return values.AuthorizationValid, authorizationViewDigest(caller.Subject, requirements)
}

func (linearOraclePolicy) ValidateView(ctx context.Context, caller values.CallerContext, view values.AuthorizationView, requirements ...values.Requirement) values.AuthorizationDecision {
	if authorizationViewDigest(caller.Subject, requirements) != view {
		return values.AuthorizationChanged
	}
	return values.AuthorizationValid
}

func (linearOraclePolicy) CaptureDenialView(ctx context.Context, caller values.CallerContext, requirement values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	return values.AuthorizationValid, values.AuthorizationDenialView{}
}

func (linearOraclePolicy) ValidateDenialView(ctx context.Context, caller values.CallerContext, view values.AuthorizationDenialView, requirement values.Requirement) values.AuthorizationDecision {
	if view == (values.AuthorizationDenialView{}) {
		return values.AuthorizationDenied
	}
	if _, current := (linearOraclePolicy{}).CaptureDenialView(ctx, caller, requirement); current != view {
		return values.AuthorizationChanged
	}
	return values.AuthorizationValid
}

func linearOracleID(serial uint64) values.ElementID {
	return values.ElementID(fmt.Sprintf("10000000-0000-4000-8000-%012x", serial))
}

func linearOracleOwner(t *testing.T) (*commit.Owner, *element.Operations) {
	t.Helper()
	authority, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	var scope [32]byte
	if _, err := rand.Read(scope[:]); err != nil {
		t.Fatal(err)
	}
	owner, err := commit.New(authority, scope, contract.RealmInitialization{Realm: "fixture-realm", NextHop: "local:fixture"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.CloseStore() })
	return owner, element.New(owner, linearOraclePolicy{})
}

func linearOracleTuple(t *testing.T, parent values.ElementID, radius float64) contract.SpatialTuple {
	t.Helper()
	rotation, err := contract.NewRotation(1, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	pose, err := contract.NewPose(contract.Vec3{}, rotation)
	if err != nil {
		t.Fatal(err)
	}
	sphere, err := contract.NewBoundedSphere(radius)
	if err != nil {
		t.Fatal(err)
	}
	return contract.SpatialTuple{Parent: parent, LocalPose: pose, Range: sphere}
}

func TestLinearOracleCorruptStateFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		fault    string
		selector queryoracle.Selector
	}{
		{"missing parent", queryoracle.Both},
		{"cycle", queryoracle.Both},
		{"root record", queryoracle.Entities},
		{"filtered Anchor record", queryoracle.Entities},
		{"filtered Entity record", queryoracle.Anchors},
	} {
		t.Run(tc.fault, func(t *testing.T) {
			owner, elements := linearOracleOwner(t)
			root, first, second, entity := linearOracleID(50), linearOracleID(51), linearOracleID(52), linearOracleID(53)
			created, err := elements.CreateRoot(linearOracleActor, root, "corrupt-root")
			if err != nil || created.Record == nil {
				t.Fatalf("root %#v err=%v", created, err)
			}
			created, err = elements.CreateAnchor(linearOracleActor, first, linearOracleTuple(t, root, 10), "corrupt-first")
			if err != nil || created.Record == nil {
				t.Fatalf("anchor %#v err=%v", created, err)
			}
			created, err = elements.CreateAnchor(linearOracleActor, second, linearOracleTuple(t, root, 10), "corrupt-second")
			if err != nil || created.Record == nil {
				t.Fatalf("anchor %#v err=%v", created, err)
			}
			created, err = elements.CreateEntity(linearOracleActor, entity, linearOracleTuple(t, first, 1), "corrupt-entity")
			if err != nil || created.Record == nil {
				t.Fatalf("entity %#v err=%v", created, err)
			}
			if err := commit.InjectOracleCorruptionForTest(owner, tc.fault); err != nil {
				t.Fatal(err)
			}
			result, scanErr := queryoracle.Scan(owner, root, root, nil, contract.QuerySphere{Radius: 1}, tc.selector)
			if !errors.Is(scanErr, contract.ErrStore) || len(result.Records) != 0 || !result.Cut.Equal(contract.CoreReadCut{}) {
				t.Fatalf("corrupt state returned partial facts: %#v err=%v", result, scanErr)
			}
		})
	}
}

func TestLinearOracleExactEdgeLimitAndFirstOverflow(t *testing.T) {
	owner, elements := linearOracleOwner(t)
	root := linearOracleID(1000)
	created, err := elements.CreateRoot(linearOracleActor, root, "edge-root")
	if err != nil || created.Record == nil {
		t.Fatalf("root %#v err=%v", created, err)
	}
	parent, atLimitEntity := commit.BuildOracleEdgeFixtureForTest(t, owner)
	atLimit, err := queryoracle.Scan(owner, root, root, nil, contract.QuerySphere{}, queryoracle.Entities)
	if err != nil || len(atLimit.Records) != 1 || atLimit.Records[0].Entity == nil || atLimit.Records[0].Entity.ID != atLimitEntity {
		t.Fatalf("oracle at edge limit: %#v err=%v", atLimit, err)
	}
	overLimitEntity := commit.AddOracleOverflowForTest(t, owner)
	overLimit, err := queryoracle.Scan(owner, root, root, nil, contract.QuerySphere{}, queryoracle.Entities)
	if !errors.Is(err, contract.ErrUnrepresentable) || len(overLimit.Records) != 0 || !overLimit.Cut.Equal(contract.CoreReadCut{}) {
		t.Fatalf("oracle over edge limit returned partial facts: %#v err=%v", overLimit, err)
	}
	read, err := elements.ReadElement(linearOracleActor, overLimitEntity, nil)
	if err != nil || read.State != contract.ExistencePresent || read.Record == nil {
		t.Fatalf("edge-limit scan truncated deepest truth: %#v err=%v", read, err)
	}
	children, err := elements.ListDirect(linearOracleActor, parent, 1, nil)
	if err != nil || len(children.Records) != 1 || children.Records[0].Entity == nil || children.Records[0].Entity.ID != overLimitEntity {
		t.Fatalf("edge-limit scan damaged Structure: %#v err=%v", children, err)
	}
}
