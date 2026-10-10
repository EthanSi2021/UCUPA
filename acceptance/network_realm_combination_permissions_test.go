package acceptance_test

import (
	"context"
	"google.golang.org/protobuf/proto"
	"path/filepath"
	"testing"
	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	corepb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
	facetpb "ucupa/ucupa-facet/bindings/go/protobuf"
	facetcontract "ucupa/ucupa-facet/contract"
	facetsdk "ucupa/ucupa-facet/sdk/go"
)

// Administration is a cold operation on the original Realm Store, not an
// added product RPC. Restarts are explicit and old evidence is never renewed.
func verifyCombinationPermissions(t *testing.T, c *networkFixture, store string, actor values.PrincipalRef, owner values.FactAuthorityRef, client *facetsdk.Client, access *facetpb.Access, typ values.FacetTypeRef, written *facetpb.PutResponse, startFacetRealm func() *serviceProductProcess, facetRealm **serviceProductProcess, startCoreRealm func() *serviceProductProcess, coreRealm **serviceProductProcess, shared bool, restartOwners func()) {
	t.Helper()
	ctx := context.Background()
	request := &facetpb.PutRequest{Access: access, TypeRef: typ.Bytes(), Expected: &facetpb.Expected{Condition: &facetpb.Expected_Absent{Absent: true}}, OperationId: []byte("relation-content"), Payload: []byte(`"network"`)}
	recoverRequest := &facetpb.RecoverOutcomeRequest{Access: access, OperationId: request.OperationId, Request: &facetpb.MutationCommand{Command: &facetpb.MutationCommand_Put{Put: request}}}
	scope := values.FacetKeyScope(owner, networkRoot, typ)
	(*facetRealm).gracefulStop()
	changeCombinationBinding(t, store, actor, facetcontract.PermissionPut, scope, true, "combination-deny-facet-put")
	*facetRealm = startFacetRealm()
	restartOwners()
	// Core permissions do not grant Facet writes, nor disclose old receipts.
	coreRead, err := c.client.Core().ReadElement(ctx, &corepb.ReadElementRequest{Handle: c.enter(t, corepb.Intent_READ), Address: &netpb.Address{}})
	if err != nil || coreRead.GetResult() == nil {
		t.Fatalf("Facet denial revoked unrelated Core permission: %v %v", coreRead, err)
	}
	denied, err := client.Facet().RecoverOutcome(ctx, recoverRequest)
	if err != nil || denied.GetFailure() != netpb.Failure_DENIED {
		t.Fatalf("current denial disclosed old Facet receipt: %v %v", denied, err)
	}
	get, err := client.Facet().Get(ctx, &facetpb.GetRequest{Access: access, TypeRef: typ.Bytes()})
	if err != nil || get.GetResult() == nil {
		t.Fatalf("Put denial removed separately granted Get: %v %v", get, err)
	}
	(*facetRealm).gracefulStop()
	changeCombinationBinding(t, store, actor, facetcontract.PermissionPut, scope, false, "combination-restore-facet-put")
	*facetRealm = startFacetRealm()
	restartOwners()
	recovered, err := client.Facet().RecoverOutcome(ctx, recoverRequest)
	if err != nil || !proto.Equal(written.GetResult(), recovered.GetResult()) {
		t.Fatalf("restored permission changed original receipt: %v %v", recovered, err)
	}

	// Conversely, Facet content permission cannot supply a Core permission.
	coreStore := filepath.Join(c.dir, "realm.db")
	stopCoreRealm := *coreRealm
	start := startCoreRealm
	if shared {
		stopCoreRealm, start = *facetRealm, startFacetRealm
	}
	stopCoreRealm.gracefulStop()
	changeCombinationBinding(t, coreStore, c.actor, corecontract.PermissionReadElement, values.AnchorClosureScope(c.config.Expected, networkRoot), true, "combination-deny-core-read")
	process := start()
	if shared {
		*facetRealm = process
	} else {
		*coreRealm = process
	}
	restartOwners()
	enter, err := c.client.Core().Enter(ctx, &corepb.EnterRequest{Reference: c.reference(), Intent: corepb.Intent_READ.Enum()})
	if err != nil || enter.GetFailure() != netpb.Failure_DENIED {
		t.Fatalf("Facet grant supplied Core read permission: %v %v", enter, err)
	}
	get, err = client.Facet().Get(ctx, &facetpb.GetRequest{Access: access, TypeRef: typ.Bytes()})
	if err != nil || get.GetResult() == nil {
		t.Fatalf("Core user read denial removed Facet content grant: %v %v", get, err)
	}
	process.gracefulStop()
	changeCombinationBinding(t, coreStore, c.actor, corecontract.PermissionReadElement, values.AnchorClosureScope(c.config.Expected, networkRoot), false, "combination-restore-core-read")
	process = start()
	if shared {
		*facetRealm = process
	} else {
		*coreRealm = process
	}
	restartOwners()
}
