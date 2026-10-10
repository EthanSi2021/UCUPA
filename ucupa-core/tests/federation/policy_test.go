package federation_test

import (
	federationcontract "ucupa/ucupa-core/contract/federation"

	"context"
	"testing"
	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
)

func TestFederationPublicMixedStartupPoliciesRecollectCurrent(t *testing.T) {
	f, connections, caller := newFederationFixture(t, []corecontract.Config{federationConfig(true), federationConfig(false)}, nil)
	f.establish(t, 0, 1, storeTestID(9710), f.root, 20, "mixed-startup-ab")
	handle := federatedTestHandle(t, f, caller, connections, "query")
	result, err := federatedTestCall(t, f.owners[0], caller, connections, map[string]any{"op": "query", "handle": handle, "start": pathJSON(f.root, nil), "basis": pathJSON(f.root, nil), "sphere": map[string]any{"center": []any{0., 0., 0.}, "radius": 100.}})
	if err != nil || result["readMode"] != "CURRENT_VERIFIED" || result["status"] != "COMPLETE" {
		t.Fatalf("mixed startup: %+v %v", result, err)
	}
	if connections.opened != 2 {
		t.Fatal("mixed source set was not recollected", connections.opened)
	}
	if _, err := federatedTestCall(t, f.owners[0], caller, connections, map[string]any{"op": "close", "handle": handle}); err != nil {
		t.Fatal(err)
	}
	connections.requireReleased(t)
}

type failedProjectionConnections struct {
	base federationcontract.FederationConnections
}

func (f failedProjectionConnections) OpenFederationRead(ctx context.Context, caller values.CallerContext, hop values.NextHop, owner values.FactAuthorityRef, request federationcontract.FederationReadRequest) (federationcontract.FederationReadConnection, error) {
	part, err := f.base.OpenFederationRead(ctx, caller, hop, owner, request)
	if err != nil {
		return nil, err
	}
	return failedProjectionPart{part}, nil
}

type failedProjectionPart struct {
	federationcontract.FederationReadConnection
}

func (p failedProjectionPart) Project(context.Context, federationcontract.FederationProjectionRequest, *values.PublicToken, uint16) (federationcontract.FederationProjectionPage, error) {
	return federationcontract.FederationProjectionPage{}, values.PublicUnavailable
}

func TestFederationPublicSourceLostAfterCaptureDoesNotEraseRootResult(t *testing.T) {
	f, connections, caller := vectorFixture(t, true)
	failed := failedProjectionConnections{connections}
	handle := federatedTestHandle(t, f, caller, failed, "query")
	result, err := federatedTestCall(t, f.owners[0], caller, failed, map[string]any{"op": "query", "handle": handle, "start": pathJSON(f.root, nil), "basis": pathJSON(f.root, nil), "sphere": map[string]any{"center": []any{0., 0., 0.}, "radius": 100.}})
	if err != nil || result["status"] != "PARTIAL" || result["gap"] != "UNRESOLVED_COVERAGE" || len(result["matches"].([]any)) != 2 {
		t.Fatalf("lost descendant erased defined root: %+v %v", result, err)
	}
	connections.requireReleased(t)
}
