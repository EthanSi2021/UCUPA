package composition

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"testing"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	facetcontract "ucupa/ucupa-facet/contract"
	facet "ucupa/ucupa-facet/instance"
	realm "ucupa/ucupa-realm/contract"
	realminstance "ucupa/ucupa-realm/instance"
)

var replacementGrantAdmin = values.PrincipalRef{Realm: "facet-endpoint-test", Principal: "replacement-grant-admin"}

type stubCoreSource struct{}

func (stubCoreSource) ReadProposal(context.Context, values.NextHop, values.FacetAcceptanceKey) (facetcontract.CoreEvidence, error) {
	return facetcontract.CoreEvidence{}, facetcontract.ErrUnavailable
}
func (stubCoreSource) ReadAttachment(context.Context, values.NextHop, values.FacetAcceptanceKey) (facetcontract.CoreEvidence, error) {
	return facetcontract.CoreEvidence{}, facetcontract.ErrUnavailable
}
func (stubCoreSource) ReadSuccessor(context.Context, values.NextHop, values.FactAuthorityRef, values.ElementID) (facetcontract.CoreSuccessorRead, error) {
	return facetcontract.CoreSuccessorRead{}, facetcontract.ErrUnavailable
}

func replacementTestRealm(t *testing.T) *realminstance.Realm {
	t.Helper()
	policy, err := realminstance.New(realm.Seed{
		ID:              replacementGrantAdmin.Realm,
		LocalPrincipals: []values.PrincipalID{replacementGrantAdmin.Principal, endpointActor.Principal},
		Bindings: []realm.PermissionBinding{
			{Subject: replacementGrantAdmin, Permission: realm.PermissionPutBinding, Scope: values.RealmScope(replacementGrantAdmin.Realm)},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

func replacementCaller(t *testing.T, policy *realminstance.Realm) values.CallerContext {
	t.Helper()
	caller, err := policy.TrustLocalCaller(endpointActor)
	if err != nil {
		t.Fatal(err)
	}
	return caller
}

func replacementTestFacet(t *testing.T, policy *realminstance.Realm) *facet.Instance {
	t.Helper()
	connections := &LocalRealmConnections{}
	if err := connections.BindEndpoint("local:replacement-realm", policy); err != nil {
		t.Fatal(err)
	}
	initial := facetcontract.RealmInitialization{Realm: replacementGrantAdmin.Realm, NextHop: "local:replacement-realm"}
	instance, err := facet.New(connections, stubCoreSource{}, initial, facet.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return instance
}

func replacementTestGrant(t *testing.T, policy *realminstance.Realm, grant realm.PermissionBinding) {
	t.Helper()
	if _, err := policy.PutBinding(replacementGrantAdmin, grant, policy.BindingSetRevision(), realm.OperationID("replacement-peers-grant")); err != nil {
		t.Fatal(err)
	}
}

func replacementTestKey(t *testing.T, facetAuthority values.FactAuthorityRef) values.FacetAcceptanceKey {
	t.Helper()
	var raw [76]byte
	raw[0] = 1
	copy(raw[32:68], endpointID)
	binary.BigEndian.PutUint64(raw[68:], 1)
	proposal, err := values.NewAttachmentRevision(raw[:])
	if err != nil {
		t.Fatal(err)
	}
	return values.FacetAcceptanceKey{Core: endpointIdentity(t), Element: endpointID, Facet: facetAuthority, Proposal: proposal}
}

func replacementTestType(t *testing.T) values.FacetTypeRef {
	t.Helper()
	digest := sha256.Sum256([]byte("composition-replacement-peers-test"))
	typ, err := values.NewFacetTypeRef(digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return typ
}

func replacementTestManifest(t *testing.T) []byte {
	t.Helper()
	manifest, err := facetcontract.EncodeNamespaceManifest(facetcontract.NamespaceManifest{
		Element: endpointID, Source: endpointIdentity(t), FinalCut: 3,
		Entries: []facetcontract.NamespaceManifestEntry{{Type: replacementTestType(t), Live: true, Length: 4, Digest: [32]byte{5}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}

func TestForReplacementRequiresBoundConnectionsAndServiceActor(t *testing.T) {
	var nilConnections *LocalFacetConnections
	if _, err := nilConnections.ForReplacement(values.CallerContext{Subject: endpointActor, Proof: [32]byte{1}}); !errors.Is(err, corecontract.ErrInvalid) {
		t.Fatalf("nil connections: got %v", err)
	}
	if _, err := (&LocalFacetConnections{}).ForReplacement(values.CallerContext{}); !errors.Is(err, corecontract.ErrInvalid) {
		t.Fatalf("invalid actor: got %v", err)
	}
	peers, err := (&LocalFacetConnections{}).ForReplacement(values.CallerContext{Subject: endpointActor, Proof: [32]byte{1}})
	if err != nil || peers.Source == nil || peers.Target == nil {
		t.Fatalf("peers = %#v, err=%v", peers, err)
	}
}

func TestReplacementPeersConnectOnlyTheExactFacetAddressAndIdentity(t *testing.T) {
	policy := replacementTestRealm(t)
	instance := replacementTestFacet(t, policy)
	connections := &LocalFacetConnections{}
	if err := connections.BindEndpoint(endpointAddr, instance); err != nil {
		t.Fatal(err)
	}
	peers, err := connections.ForReplacement(replacementCaller(t, policy))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	foreign := replacementTestKey(t, endpointIdentity(t))
	target, typ, manifest := endpointIdentity(t), replacementTestType(t), replacementTestManifest(t)
	if _, err = peers.Source.CheckSourceExport(ctx, endpointAddr, foreign, "peers-op"); !errors.Is(err, corecontract.ErrUnavailable) {
		t.Fatalf("foreign source check: got %v", err)
	}
	if _, err = peers.Source.FenceSource(ctx, endpointAddr, foreign, "peers-op", target); !errors.Is(err, corecontract.ErrUnavailable) {
		t.Fatalf("foreign fence: got %v", err)
	}
	if _, err = peers.Source.ReadSourceEntry(ctx, endpointAddr, foreign, "peers-op", typ); !errors.Is(err, corecontract.ErrUnavailable) {
		t.Fatalf("foreign read: got %v", err)
	}
	if err = peers.Source.SettleSource(ctx, endpointAddr, foreign, "peers-op", target); !errors.Is(err, corecontract.ErrUnavailable) {
		t.Fatalf("foreign settle: got %v", err)
	}
	if _, err = peers.Target.CheckTargetImport(ctx, endpointAddr, foreign, "peers-op", manifest, "local:core"); !errors.Is(err, corecontract.ErrUnavailable) {
		t.Fatalf("foreign import check: got %v", err)
	}
	if err = peers.Target.StageTargetEntry(ctx, endpointAddr, foreign, "peers-op", typ, []byte("data")); !errors.Is(err, corecontract.ErrUnavailable) {
		t.Fatalf("foreign stage: got %v", err)
	}
	if _, err = peers.Target.AcceptTarget(ctx, endpointAddr, foreign, "peers-op", "local:core"); !errors.Is(err, corecontract.ErrUnavailable) {
		t.Fatalf("foreign accept: got %v", err)
	}
	if err = peers.Target.InstallTarget(ctx, endpointAddr, foreign, "peers-op", target, 3); !errors.Is(err, corecontract.ErrUnavailable) {
		t.Fatalf("foreign install: got %v", err)
	}
	if _, err = peers.Target.ReadTargetImport(ctx, endpointAddr, foreign, "peers-op"); !errors.Is(err, corecontract.ErrUnavailable) {
		t.Fatalf("foreign import read: got %v", err)
	}
	exact := replacementTestKey(t, instance.AuthorityRef())
	if _, err = peers.Source.CheckSourceExport(ctx, "local:missing", exact, "peers-op"); !errors.Is(err, corecontract.ErrUnavailable) {
		t.Fatalf("unbound address: got %v", err)
	}
	if _, err = peers.Target.ReadTargetImport(ctx, "remote:facet", exact, "peers-op"); !errors.Is(err, corecontract.ErrUnavailable) {
		t.Fatalf("non-local address: got %v", err)
	}
}

func TestReplacementSourcePeerUsesOnlyTheFixedServiceIdentity(t *testing.T) {
	policy := replacementTestRealm(t)
	instance := replacementTestFacet(t, policy)
	connections := &LocalFacetConnections{}
	if err := connections.BindEndpoint(endpointAddr, instance); err != nil {
		t.Fatal(err)
	}
	peers, err := connections.ForReplacement(replacementCaller(t, policy))
	if err != nil {
		t.Fatal(err)
	}
	ctx, exact := context.Background(), replacementTestKey(t, instance.AuthorityRef())
	if _, err = peers.Source.CheckSourceExport(ctx, endpointAddr, exact, "peers-op"); !errors.Is(err, corecontract.ErrUnavailable) {
		t.Fatalf("service actor without export grant: got %v", err)
	}
	replacementTestGrant(t, policy, realm.PermissionBinding{
		Subject: endpointActor, Permission: facetcontract.PermissionNamespaceExport,
		Scope: values.FacetElementScope(instance.AuthorityRef(), endpointID),
	})
	state, err := peers.Source.CheckSourceExport(ctx, endpointAddr, exact, "peers-op")
	if err != nil || state.Found || state.Fenced || state.Refused || state.Settled || state.FinalCut != 0 || state.ManifestDigest != [32]byte{} || state.Manifest != nil {
		t.Fatalf("granted check = %#v, err=%v", state, err)
	}
}

func TestCheckTargetImportRejectsNonJournalManifestBytes(t *testing.T) {
	policy := replacementTestRealm(t)
	instance := replacementTestFacet(t, policy)
	connections := &LocalFacetConnections{}
	if err := connections.BindEndpoint(endpointAddr, instance); err != nil {
		t.Fatal(err)
	}
	peers, err := connections.ForReplacement(replacementCaller(t, policy))
	if err != nil {
		t.Fatal(err)
	}
	exact := replacementTestKey(t, instance.AuthorityRef())
	if _, err = peers.Target.CheckTargetImport(context.Background(), endpointAddr, exact, "peers-op", []byte("not-a-journal-manifest"), "local:core"); !errors.Is(err, corecontract.ErrConflict) {
		t.Fatalf("corrupt manifest bytes: got %v", err)
	}
}

func TestReplacementPeersRespectCancelledContext(t *testing.T) {
	policy := replacementTestRealm(t)
	instance := replacementTestFacet(t, policy)
	connections := &LocalFacetConnections{}
	if err := connections.BindEndpoint(endpointAddr, instance); err != nil {
		t.Fatal(err)
	}
	peers, err := connections.ForReplacement(replacementCaller(t, policy))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	exact := replacementTestKey(t, instance.AuthorityRef())
	if _, err = peers.Source.CheckSourceExport(ctx, endpointAddr, exact, "peers-op"); !errors.Is(err, corecontract.ErrUnavailable) {
		t.Fatalf("cancelled source check: got %v", err)
	}
	if _, err = peers.Target.ReadTargetImport(ctx, endpointAddr, exact, "peers-op"); !errors.Is(err, corecontract.ErrUnavailable) {
		t.Fatalf("cancelled import read: got %v", err)
	}
}
