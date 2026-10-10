package acceptance_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net"
	"path/filepath"
	"testing"

	"ucupa/composition"
	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	facetcontract "ucupa/ucupa-facet/contract"
	facet "ucupa/ucupa-facet/instance"
	realm "ucupa/ucupa-realm/contract"
	realminstance "ucupa/ucupa-realm/instance"
)

// Only provisioning uses owner APIs. All owners are closed before the two
// actual product hosts start; the independent client receives public material.
func interopProducts(t *testing.T) map[string]any {
	t.Helper()
	c := newNetworkFixture(t, false)
	c.config.Certificate, c.clientConfig.Roots, c.clientConfig.SPKISHA256, c.certPEM, c.keyPEM = interopCertificate(t)
	dir := t.TempDir()
	actor := values.PrincipalRef{Realm: "interop-facet", Principal: "operator"}
	credential := realm.CredentialEvidence{Audience: actor.Realm}
	rand.Read(credential.ID[:])
	rand.Read(credential.Secret[:])
	r, err := realminstance.CreateDurable(realm.Seed{ID: actor.Realm, LocalPrincipals: []values.PrincipalID{actor.Principal}, Credentials: []realm.CredentialBootstrap{{Subject: actor.Principal, Evidence: credential}}, Bindings: []realm.PermissionBinding{{Subject: actor, Permission: realm.PermissionPutBinding, Scope: values.RealmScope(actor.Realm)}}}, filepath.Join(dir, "realm.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	links := &composition.LocalFacetCoreConnections{}
	if err = links.BindEndpoint("local:interop-core", composition.LocalFacetCoreEndpoint{Read: c.c.Facets(), Control: c.c.Facets(), Access: c.c.Access()}); err != nil {
		t.Fatal(err)
	}
	caller, err := c.r.EstablishCaller(realm.CredentialEvidence{Audience: c.actor.Realm, ID: realm.CredentialID(c.id), Secret: realm.CredentialSecret(c.secret)})
	if err != nil {
		t.Fatal(err)
	}
	source, err := links.ForFacetCaller(caller)
	if err != nil {
		t.Fatal(err)
	}
	realmLinks := &composition.LocalRealmConnections{}
	if err = realmLinks.BindEndpoint("local:interop-realm", r); err != nil {
		t.Fatal(err)
	}
	f, err := facet.CreateDurable(realmLinks, source, facetcontract.RealmInitialization{Realm: actor.Realm, NextHop: "local:interop-realm"}, filepath.Join(dir, "facet.db"), facet.Config{PublicAccessPolicy: publicPolicy()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	typ := fixtureType("independent-string")
	grant := func(p values.PermissionID, scope values.OperationScope) {
		t.Helper()
		if _, e := r.PutBinding(actor, realm.PermissionBinding{Subject: actor, Permission: p, Scope: scope}, r.BindingSetRevision(), realm.OperationID(fmt.Sprint(p))); e != nil {
			t.Fatal(e)
		}
	}
	for _, p := range []values.PermissionID{facetcontract.PermissionAccept, facetcontract.PermissionReadAcceptance, facetcontract.PermissionCoreContactUpdate, facetcontract.PermissionObserveElement, facetcontract.PermissionListTypes} {
		grant(p, values.FacetElementScope(f.AuthorityRef(), networkRoot))
	}
	for _, p := range []values.PermissionID{facetcontract.PermissionGet, facetcontract.PermissionPut, facetcontract.PermissionDelete, facetcontract.PermissionObserveKey} {
		grant(p, values.FacetKeyScope(f.AuthorityRef(), networkRoot, typ))
	}
	for _, p := range []values.PermissionID{corecontract.PermissionFacetPropose, corecontract.PermissionFacetActivate, corecontract.PermissionFacetContactUpdate, corecontract.PermissionFacetProposalRead, corecontract.PermissionFacetAttachmentRead, corecontract.PermissionObserveAttachment} {
		c.grant(t, p, values.CoreElementScope(c.c.AuthorityRef(), networkRoot), false)
	}
	facetLinks := &composition.LocalFacetConnections{}
	if err = facetLinks.BindEndpoint("local:interop-facet", f); err != nil {
		t.Fatal(err)
	}
	acceptance, err := facetLinks.ForCore(actor)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.c.Facets().SetFacetAcceptanceSource(acceptance); err != nil {
		t.Fatal(err)
	}
	before, err := c.c.Facets().ReadFacetAttachment(c.actor, networkRoot)
	if err != nil {
		t.Fatal(err)
	}
	p, err := c.c.Facets().ProposeFacet(c.actor, networkRoot, f.AuthorityRef(), "local:interop-facet", before.Attachment.Revision, "interop-propose")
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := f.Namespace.AcceptAttachment(context.Background(), actor, p.Pending, "local:interop-core", "interop-accept")
	if err != nil {
		t.Fatal(err)
	}
	active, err := c.c.Facets().ActivateFacet(context.Background(), c.actor, networkRoot, p.Attachment.Revision, "interop-activate")
	if err != nil {
		t.Fatal(err)
	}
	_, _, pin, certPEM, keyPEM := interopCertificate(t)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := l.Addr().String()
	l.Close()
	hop := values.NextHop("https://" + address)
	if _, err = c.c.Facets().UpdateFacetNextHop(c.actor, "interop-facet-address", p.Pending, active.NextHopRevision, hop); err != nil {
		t.Fatal(err)
	}
	if _, err = f.Namespace.UpdateCoreNextHop(actor, "interop-core-address", p.Pending, accepted.NextHopRevision, c.config.NextHop); err != nil {
		t.Fatal(err)
	}
	owner := f.AuthorityRef()
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	if err = r.Close(); err != nil {
		t.Fatal(err)
	}
	startCore := prepareNetworkProduct(t, c, map[string]any{"facet_services": []any{map[string]any{"realm": c.actor.Realm, "principal": c.actor.Principal, "facet": owner}}})
	startCore()
	writeNetworkFile(t, filepath.Join(dir, "tls.pem"), certPEM)
	writeNetworkFile(t, filepath.Join(dir, "tls.key"), keyPEM)
	writeNetworkFile(t, filepath.Join(dir, "core-ca.pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: c.config.Certificate.Certificate[1]}))
	serviceCredential, _ := json.Marshal(map[string]any{"audience": c.actor.Realm, "credential_id": values.EncodePublicBytes(c.id[:]), "secret": values.EncodePublicBytes(c.secret[:])})
	writeNetworkFile(t, filepath.Join(dir, "core-credential.json"), serviceCredential)
	corePin := c.clientConfig.SPKISHA256
	config := map[string]any{
		"listen": address, "next_hop": hop, "expected_facet": owner, "realm_id": actor.Realm, "realm_next_hop": "local:interop-realm",
		"facet_store": filepath.Join(dir, "facet.db"), "realm_store": filepath.Join(dir, "realm.db"), "certificate_file": filepath.Join(dir, "tls.pem"), "private_key_file": filepath.Join(dir, "tls.key"),
		"session_ttl": "30s", "rpc_deadline": "3s", "handshake_timeout": "1s", "connection_lifetime": "1m", "shutdown_timeout": "1s",
		"max_sessions": 8, "max_sessions_per_caller": 4, "max_connections": 8, "max_concurrent_rpcs": 8, "max_streams": 8, "max_metadata_bytes": 8192, "max_response_bytes": 4 << 20, "context_ttl": "1m", "observation_ttl": "1m",
		"core_connections": []any{map[string]any{"next_hop": c.config.NextHop, "expected_core": c.config.Expected, "ca_file": filepath.Join(dir, "core-ca.pem"), "spki_sha256": values.EncodePublicBytes(corePin[:]), "credential_file": filepath.Join(dir, "core-credential.json")}},
	}
	prepareServiceProduct(t, dir, "ucupa-facet", config)()
	return map[string]any{
		"root": networkRoot, "type_ref": values.EncodePublicBytes(typ.Bytes()),
		"core":  map[string]any{"address": c.config.NextHop, "owner": c.config.Expected, "cert": filepath.Join(c.dir, "tls.pem"), "key": filepath.Join(c.dir, "tls.key"), "pin": values.EncodePublicBytes(corePin[:]), "audience": c.actor.Realm, "id": values.EncodePublicBytes(c.id[:]), "secret": values.EncodePublicBytes(c.secret[:])},
		"facet": map[string]any{"address": hop, "owner": owner, "cert": filepath.Join(dir, "tls.pem"), "key": filepath.Join(dir, "tls.key"), "pin": values.EncodePublicBytes(pin[:]), "audience": actor.Realm, "id": values.EncodePublicBytes(credential.ID[:]), "secret": values.EncodePublicBytes(credential.Secret[:])},
	}
}
