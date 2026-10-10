package acceptance_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"ucupa/composition"
	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
	realm "ucupa/ucupa-realm/contract"
)

// This adapter isolates the authenticated owner boundary. It is explicitly
// local, not evidence that mount control has been wired over gRPC.
type authenticatedPrepareCapture struct {
	source     values.FactAuthorityRef
	b          *core.Core
	caller     values.CallerContext
	request    corecontract.MountPrepareRequest
	view       values.AuthorizationView
	commitment [32]byte
}

func (p *authenticatedPrepareCapture) Prepare(ctx context.Context, _ values.NextHop, _ values.FactAuthorityRef, request corecontract.MountPrepareRequest) (corecontract.MountPrepared, error) {
	p.request = request
	return p.b.Mounts().PrepareApprovedMountControl(ctx, p.caller, p.source, request, p.view, p.commitment)
}
func (p *authenticatedPrepareCapture) Finish(context.Context, values.NextHop, values.FactAuthorityRef, corecontract.MountFinishRequest) (corecontract.MountClosed, error) {
	return corecontract.MountClosed{}, corecontract.ErrUnavailable
}
func (p *authenticatedPrepareCapture) ReadDecision(context.Context, values.NextHop, values.FactAuthorityRef, corecontract.MountAttemptKey) (corecontract.MountDecisionEvidence, error) {
	return corecontract.MountDecisionEvidence{}, corecontract.ErrUnavailable
}

func TestMountPrepareUsesRealmCredentialNotPrincipalName(t *testing.T) {
	f := newNetworkFixture(t, false)
	b, err := core.CreateDurable(f.realmLinks, corecontract.RealmInitialization{Realm: f.actor.Realm, NextHop: f.realmAddress}, filepath.Join(t.TempDir(), "carrier.db"), corecontract.Config{PublicAccessPolicy: &values.PublicAccessPolicy{ContextTTL: time.Minute, ObservationTTL: time.Minute}})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	mount := values.ElementID(networkEntity)
	f.grant(t, corecontract.PermissionMountControl, values.CoreElementScope(f.c.AuthorityRef(), mount), false)
	f.grant(t, corecontract.PermissionCreateAnchor, values.CoreElementScope(f.c.AuthorityRef(), networkRoot), false)
	f.grant(t, corecontract.PermissionMountAccept, values.CoreElementScope(b.AuthorityRef(), mount), false)
	rangeValue, _ := corecontract.NewBoundedSphere(10)
	rotation, _ := corecontract.NewRotation(1, 0, 0, 0)
	r, err := f.c.Elements().CreateAnchor(f.actor, mount, corecontract.SpatialTuple{Parent: networkRoot, LocalPose: corecontract.Pose{Rotation: rotation}, Range: rangeValue}, "mount-anchor")
	if err != nil {
		t.Fatal(err)
	}
	caller, err := (composition.RealmNetworkAuthentication{Connections: f.realmLinks, Owner: f.c.Access()}).Establish(context.Background(), f.actor.Realm, f.id, f.secret)
	if err != nil {
		t.Fatal(err)
	}
	p := &authenticatedPrepareCapture{source: f.c.AuthorityRef(), b: b, caller: caller}
	if err := f.c.Mounts().ConfigureMountTransport(p); err != nil {
		t.Fatal(err)
	}
	command := corecontract.MountCommand{Kind: corecontract.MountEstablish, MountAnchorUUID: mount, ExpectedElementRevision: r.Record.Anchor.Revision, CarrierAuthority: b.AuthorityRef(), CarrierNextHop: "local:carrier", ParentNextHop: "local:parent"}
	p.view, err = b.Mounts().AuthorizeMountAcceptance(context.Background(), caller, mount)
	if err != nil {
		t.Fatal(err)
	}
	p.commitment, err = f.c.Mounts().CommandCommitment(f.c.AuthorityRef(), b.AuthorityRef(), "credential-mount", command)
	if err != nil {
		t.Fatal(err)
	}
	result, err := f.c.Mounts().ApplyMount(context.Background(), corecontract.LocalCallers{f.c.AuthorityRef(): f.actor, b.AuthorityRef(): f.actor}, "credential-mount", command)
	if err != nil || result.Decision != corecontract.MountCommitted || result.Settled {
		t.Fatalf("prepare/held finish: %#v %v", result, err)
	}
	if !p.request.Key.Valid() {
		t.Fatal("no real prepare request")
	}
	_, err = b.Mounts().PrepareMountControl(context.Background(), values.CallerContext{Subject: caller.Subject}, p.source, p.request)
	if !errors.Is(err, values.PublicDenied) {
		t.Fatalf("bare subject accepted: %v", err)
	}
	_, err = f.r.RevokeCredential(f.admin, realm.CredentialID(f.id), f.r.CredentialSetRevision(), "revoke-mount-requester")
	if err != nil {
		t.Fatal(err)
	}
	_, err = b.Mounts().PrepareMountControl(context.Background(), caller, p.source, p.request)
	if !errors.Is(err, values.PublicDenied) {
		t.Fatalf("revoked credential replay: %v", err)
	}
	// The parent already durably decided before the user credential was
	// revoked. Service recovery closes that exact decision without asking the
	// revoked user to prepare again or manufacturing a replacement identity.
	decision, err := f.c.Mounts().MountPeer().ReadDecision(context.Background(), b.AuthorityRef(), corecontract.MountAttemptKey{ParentAuthority: f.c.AuthorityRef(), OperationID: "credential-mount"})
	if err != nil || decision.Decision != corecontract.MountCommitted {
		t.Fatalf("retained decision: %#v %v", decision, err)
	}
	closed, err := b.Mounts().MountPeer().Finish(context.Background(), f.c.AuthorityRef(), corecontract.MountFinishRequest{Evidence: decision})
	if err != nil || closed.Evidence != decision {
		t.Fatalf("revocation stranded durable decision: %#v %v", closed, err)
	}
}
