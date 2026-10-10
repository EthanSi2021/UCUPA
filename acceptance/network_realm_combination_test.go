package acceptance_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	realm "ucupa/ucupa-realm/contract"
	realminstance "ucupa/ucupa-realm/instance"
)

// All serving owners are real products, including both authorization sources.
// A shared Realm remains one process/Store, not two writable copies.
func TestRealmCombinationIndependentProducts(t *testing.T) {
	runFacetRelationshipRealmProducts(t, true, true, false)
}

func TestRealmCombinationSharedProducts(t *testing.T) {
	runFacetRelationshipRealmProducts(t, true, true, true)
}

func verifyCombinationStartup(t *testing.T, dir string, configuration, endpoint map[string]any) {
	t.Helper()
	realmID := configuration["realm_id"]
	configuration["realm_id"] = "wrong-realm"
	writeServiceConfig(t, dir, "ucupa-facet", configuration)
	rejectServiceProductStartup(t, dir, "ucupa-facet")
	configuration["realm_id"] = realmID
	pin := endpoint["spki_sha256"]
	endpoint["spki_sha256"] = values.EncodePublicBytes([]byte("00000000000000000000000000000000"))
	writeServiceConfig(t, dir, "ucupa-facet", configuration)
	rejectServiceProductStartup(t, dir, "ucupa-facet")
	endpoint["spki_sha256"] = pin
	writeServiceConfig(t, dir, "ucupa-facet", configuration)
}

func prepareCombinationCoreRealm(t *testing.T, c *networkFixture, facetDir, facetStore string, facetEndpoint map[string]any, shared bool) (map[string]any, func() *serviceProductProcess) {
	t.Helper()
	endpoint, configuration, host := prepareOwnerRealmProduct(t, c.dir, filepath.Join(c.dir, "realm.db"), c.actor.Realm, c.c.AuthorityRef())
	var start func() *serviceProductProcess
	if shared {
		// Distinct service certificates bind the two actual owner identities.
		// Only serving address/pin/roots are shared; service access is not.
		for _, field := range []string{"next_hop", "ca_file", "spki_sha256"} {
			endpoint[field] = facetEndpoint[field]
		}
		path := filepath.Join(facetDir, "realm-host", "service.json")
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var sharedConfiguration map[string]any
		if err := json.Unmarshal(raw, &sharedConfiguration); err != nil {
			t.Fatal(err)
		}
		sharedConfiguration["realm_store"] = facetStore
		sharedConfiguration["admitted_services"] = append(sharedConfiguration["admitted_services"].([]any), configuration["admitted_services"].([]any)...)
		roots, err := os.ReadFile(filepath.Join(facetDir, "realm-service-ca.pem"))
		if err != nil {
			t.Fatal(err)
		}
		coreRoots, err := os.ReadFile(filepath.Join(c.dir, "realm-service-ca.pem"))
		if err != nil {
			t.Fatal(err)
		}
		writeNetworkFile(t, filepath.Join(facetDir, "realm-service-ca.pem"), append(roots, coreRoots...))
		writeServiceConfig(t, filepath.Join(facetDir, "realm-host"), "ucupa-realm", sharedConfiguration)
	} else {
		start = prepareServiceProductControl(t, host, "ucupa-realm", configuration)
	}
	for _, p := range []values.PermissionID{corecontract.PermissionRealmContactRead, corecontract.PermissionRealmContactUpdate} {
		c.grant(t, p, values.AuthorityScope(c.c.AuthorityRef()), false)
	}
	caller, err := c.realmLinks.TrustLocalCaller(c.realmAddress, c.actor.Realm, c.actor)
	if err != nil {
		t.Fatal(err)
	}
	contact, ok := c.c.CurrentRealmContact()
	if !ok {
		t.Fatal("missing Core contact")
	}
	moved, err := c.c.RealmContact().Update(context.Background(), caller, corecontract.RealmContactUpdate{
		OperationID: "combination-core-network", Owner: contact.Owner, Realm: contact.Realm, Expected: contact.Revision, NextHop: endpoint["next_hop"].(values.NextHop)})
	if err != nil || moved.Decision != corecontract.RealmContactCommitted {
		t.Fatalf("Core network contact: %+v %v", moved, err)
	}
	return map[string]any{"realm_profile": "network", "realm_endpoints": []any{endpoint}}, start
}

// Re-establishment is explicit test orchestration. It never renews the old
// session or retries a business mutation after an uncertain result.
func reconnectCombinationCore(t *testing.T, c *networkFixture) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		err := c.client.Establish(context.Background(), c.actor.Realm, c.id, c.secret)
		if err == nil {
			return
		}
		if status.Code(err) != codes.Unavailable || !time.Now().Before(deadline) {
			t.Fatalf("fresh Core session: %v", err)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func changeCombinationBinding(t *testing.T, store string, actor values.PrincipalRef, p values.PermissionID, scope values.OperationScope, remove bool, operation string) {
	t.Helper()
	r, err := realminstance.OpenDurable(store)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	binding := realm.PermissionBinding{Subject: actor, Permission: p, Scope: scope}
	if remove {
		_, err = r.DeleteBinding(actor, binding, r.BindingSetRevision(), realm.OperationID(operation))
	} else {
		_, err = r.PutBinding(actor, binding, r.BindingSetRevision(), realm.OperationID(operation))
	}
	if err != nil {
		t.Fatal(err)
	}
}
