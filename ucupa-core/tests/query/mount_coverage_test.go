package query_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"ucupa/shared/values"
	core "ucupa/ucupa-core/instance"
)

func coveragePublicCall(t *testing.T, owner *core.Core, request map[string]any) (map[string]any, error) {
	t.Helper()
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	v, err := values.ParsePublicJSON(raw, values.PublicJSONLimits())
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = values.CanonicalJSONValue(v)
	out, err := owner.Access().PublicCall(context.Background(), values.CallerContext{Subject: coverageReader}, raw)
	if err != nil {
		return nil, err
	}
	v, err = values.ParseCanonicalJSON(out, values.PublicJSONLimits())
	if err != nil {
		t.Fatal(err)
	}
	return v.(map[string]any), nil
}

func TestPublicQueryNeverClaimsUnresolvedMountComplete(t *testing.T) {
	for _, active := range []bool{false, true} {
		t.Run(map[bool]string{false: "establishing", true: "active"}[active], func(t *testing.T) {
			c, root, mount := newCoverageMount(t, active)
			ref := func(id values.ElementID) map[string]any {
				return map[string]any{"boundary": map[string]any{"core": string(c.AuthorityRef()), "anchor": string(id)}, "nextHop": "local:core"}
			}
			if _, err := coveragePublicCall(t, c, map[string]any{"op": "enter", "reference": ref(mount), "intent": "query"}); !errors.Is(err, values.PublicUnavailable) {
				t.Fatal(err)
			}
			h, err := coveragePublicCall(t, c, map[string]any{"op": "enter", "reference": ref(root), "intent": "query"})
			if err != nil {
				t.Fatal(err)
			}
			address := map[string]any{"down": []any{}}
			request := map[string]any{"op": "query", "handle": h["handle"], "start": address, "basis": address, "selector": "entities", "sphere": map[string]any{"center": []any{0., 0., 0.}, "radius": 100.}}
			r, err := coveragePublicCall(t, c, request)
			if err != nil || r["status"] != "PARTIAL" || r["gap"] != "UNRESOLVED_COVERAGE" {
				t.Fatal(r, err)
			}
			request["sphere"] = map[string]any{"center": []any{1e6, 0., 0.}, "radius": 1.}
			r, err = coveragePublicCall(t, c, request)
			if err != nil || r["status"] != "COMPLETE" {
				t.Fatal(r, err)
			}
		})
	}
}
