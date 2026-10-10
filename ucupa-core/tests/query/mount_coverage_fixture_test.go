package query_test

import (
	"context"
	"testing"
	"time"

	"ucupa/composition"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
)

var coverageWriter = values.PrincipalRef{Realm: "store-realm", Principal: "store-writer"}
var coverageReader = values.PrincipalRef{Realm: "oracle-realm", Principal: "oracle"}

// This fixed trusted policy isolates coverage, not Realm credential handling.
// Its view evidence stays valid exactly as it was before the contact port
// existed; address identity is not a variable in these tests.
type coveragePolicy struct{}

func (coveragePolicy) Check(context.Context, values.NextHop, values.RealmID) error { return nil }
func (coveragePolicy) AuthorizePrincipal(values.NextHop, values.RealmID, values.PrincipalRef, ...values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationValid
}
func (coveragePolicy) AuthorizeView(context.Context, values.NextHop, values.RealmID, values.CallerContext, ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	return values.AuthorizationValid, values.AuthorizationView{1}
}
func (coveragePolicy) ValidateView(context.Context, values.NextHop, values.RealmID, values.CallerContext, values.AuthorizationView, ...values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationValid
}
func (coveragePolicy) CaptureDenialView(context.Context, values.NextHop, values.RealmID, values.CallerContext, values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	return values.AuthorizationUnavailable, values.AuthorizationDenialView{}
}
func (coveragePolicy) ValidateDenialView(context.Context, values.NextHop, values.RealmID, values.CallerContext, values.AuthorizationDenialView, values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationUnavailable
}

// Drop the reply only after the real carrier has prepared. This leaves the same
// establishing truth as the old deterministic fixture, without changing tables.
type coverageLostPrepare struct{ contract.MountPeerPort }

func (peer coverageLostPrepare) Prepare(ctx context.Context, source values.FactAuthorityRef, request contract.MountPrepareRequest) (contract.MountPrepared, error) {
	prepared, err := peer.MountPeerPort.Prepare(ctx, source, request)
	if err != nil || !prepared.Prepared {
		return prepared, err
	}
	return contract.MountPrepared{}, contract.ErrUnavailable
}

func newCoverageMount(t *testing.T, active bool) (*core.Core, values.ElementID, values.ElementID) {
	t.Helper()
	parent, err := core.New(coveragePolicy{}, queryContact, contract.Config{PublicAccessPolicy: &values.PublicAccessPolicy{ContextTTL: time.Hour, ObservationTTL: time.Hour}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = parent.Close() })
	carrier, err := core.New(coveragePolicy{}, queryContact, contract.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = carrier.Close() })
	root := values.ElementID("20000000-0000-4000-8000-00000000238c")  // 9100
	mount := values.ElementID("20000000-0000-4000-8000-00000000238d") // 9101
	if _, err := parent.Elements().CreateRoot(coverageWriter, root, "guard-root"); err != nil {
		t.Fatal(err)
	}
	anchor, err := parent.Elements().CreateAnchor(coverageWriter, mount, regionTuple(t, root, regionPose(t, 0, 0, 0), 10), "guard-mount")
	if err != nil || anchor.Record == nil || anchor.Record.Anchor == nil {
		t.Fatal(anchor, err)
	}
	connections := &composition.LocalMountConnections{}
	var peer contract.MountPeerPort = carrier.Mounts().MountPeer()
	if !active {
		peer = coverageLostPrepare{MountPeerPort: peer}
	}
	if err := connections.BindEndpoint("local:guard-b", peer); err != nil {
		t.Fatal(err)
	}
	if err := connections.BindEndpoint("local:guard-a", parent.Mounts().MountPeer()); err != nil {
		t.Fatal(err)
	}
	for _, owner := range []*core.Core{parent, carrier} {
		transport, err := connections.TransportFor(owner)
		if err != nil {
			t.Fatal(err)
		}
		if err := owner.Mounts().ConfigureMountTransport(transport); err != nil {
			t.Fatal(err)
		}
	}
	command := contract.MountCommand{Kind: contract.MountEstablish, MountAnchorUUID: mount,
		ExpectedElementRevision: anchor.Record.Anchor.Revision, CarrierAuthority: carrier.AuthorityRef(),
		CarrierNextHop: "local:guard-b", ParentNextHop: "local:guard-a"}
	callers := contract.LocalCallers{parent.AuthorityRef(): coverageWriter, carrier.AuthorityRef(): coverageWriter}
	result, err := parent.Mounts().ApplyMount(context.Background(), callers, "guard-establish", command)
	if err != nil {
		t.Fatal(err)
	}
	wantState := contract.MountEstablishing
	if active {
		wantState = contract.MountActive
		if result.Decision != contract.MountCommitted || !result.Settled {
			t.Fatalf("active fixture: %#v", result)
		}
	} else if result.Decision != contract.MountPending || result.Settled || result.BlockedBy != contract.MountBlockedUnavailable {
		t.Fatalf("establishing fixture: %#v", result)
	}
	for _, owner := range []*core.Core{parent, carrier} {
		read, err := owner.Mounts().ReadMount(coverageWriter, mount)
		if err != nil || read.State != wantState {
			t.Fatalf("fixture mount state: %#v %v", read, err)
		}
	}
	return parent, root, mount
}
