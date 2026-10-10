package federation_test

import (
	"context"
	"testing"
	"ucupa/shared/values"
	federationcontract "ucupa/ucupa-core/contract/federation"
	core "ucupa/ucupa-core/instance"
)

func federatedTestCall(t *testing.T, owner *core.Core, caller values.CallerContext, connections federationcontract.FederationConnections, request map[string]any) (map[string]any, error) {
	t.Helper()
	encoded, err := values.CanonicalJSONValue(request)
	if err != nil {
		t.Fatal(err)
	}
	result, err := owner.Access().PublicCallWithFederation(context.Background(), caller, encoded, connections)
	if err != nil {
		return nil, err
	}
	parsed, err := values.ParseCanonicalJSON(result, values.PublicJSONLimits())
	if err != nil {
		t.Fatal(err)
	}
	return parsed.(map[string]any), nil
}

func federatedTestHandle(t *testing.T, f *federationFixture, caller values.CallerContext, connections federationcontract.FederationConnections, intent string) any {
	t.Helper()
	result, err := federatedTestCall(t, f.owners[0], caller, connections, map[string]any{"op": "enter", "intent": intent,
		"reference": map[string]any{"boundary": map[string]any{"core": string(f.owners[0].AuthorityRef()), "anchor": string(f.root)}, "nextHop": string(nestedMountAddress(0))}})
	if err != nil {
		t.Fatal(err)
	}
	return result["handle"]
}
