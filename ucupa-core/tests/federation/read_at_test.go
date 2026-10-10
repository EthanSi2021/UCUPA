package federation_test

import (
	"errors"
	"testing"
	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
)

func TestFederationPublicReadAtUsesOriginalComposite(t *testing.T) {
	for _, retained := range []bool{false, true} {
		t.Run(map[bool]string{false: "current", true: "retained"}[retained], func(t *testing.T) {
			f, connections, caller := vectorFixture(t, retained)
			leaf, n := storeTestID(9791), storeTestID(9720)
			created, err := f.owners[2].Elements().CreateEntity(storeTestActor, leaf, storeTestTuple(t, n, 1, 1), "read-at-leaf")
			if err != nil {
				t.Fatal(err)
			}
			handle := federatedTestHandle(t, f, caller, connections, "query")
			query, err := federatedTestCall(t, f.owners[0], caller, connections, map[string]any{"op": "query", "handle": handle, "start": pathJSON(f.root, nil), "basis": pathJSON(f.root, nil), "sphere": map[string]any{"center": []any{0., 0., 0.}, "radius": 100.}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.owners[2].Elements().UpdateEntity(storeTestActor, leaf, storeTestTuple(t, n, 3, 1), created.Record.Entity.Revision, "move-read-at-leaf"); err != nil {
				t.Fatal(err)
			}
			request := map[string]any{"op": "read", "handle": handle, "basis": query["receipt"], "address": pathJSON(f.root, []values.ElementID{storeTestID(9710), n, leaf})}
			old, err := federatedTestCall(t, f.owners[0], caller, connections, request)
			if !retained {
				if !errors.Is(err, values.PublicInvalidContext) {
					t.Fatal("current read-at recaptured latest", old, err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				record := old["record"].(map[string]any)
				if record["pose"].(map[string]any)["translation"].([]any)[0] != 1. || old["readMode"] != "RETAINED" {
					t.Fatal("wrong read cut", old)
				}
				request["basis"] = old["receipt"]
				again, err := federatedTestCall(t, f.owners[0], caller, connections, request)
				if err != nil || again["record"].(map[string]any)["pose"].(map[string]any)["translation"].([]any)[0] != 1. {
					t.Fatal("derived receipt lost original cut", again, err)
				}
				if connections.opened != 2 {
					t.Fatal("read-at opened a new source", connections.opened)
				}
			}
			if _, err := federatedTestCall(t, f.owners[0], caller, connections, map[string]any{"op": "close", "handle": handle}); err != nil {
				t.Fatal(err)
			}
			connections.requireReleased(t)
		})
	}
}

func TestFederationPublicReadAtCannotPromoteQueryGrant(t *testing.T) {
	auth := &readAtAuthorizer{denyRead: true}
	f, connections, caller := vectorFixtureWithPolicy(t, true, []federationPolicy{nil, nil, auth})
	leaf, n := storeTestID(9792), storeTestID(9720)
	if _, err := f.owners[2].Elements().CreateEntity(storeTestActor, leaf, storeTestTuple(t, n, 1, 1), "query-only-leaf"); err != nil {
		t.Fatal(err)
	}
	handle := federatedTestHandle(t, f, caller, connections, "query")
	query, err := federatedTestCall(t, f.owners[0], caller, connections, map[string]any{"op": "query", "handle": handle, "start": pathJSON(f.root, nil), "basis": pathJSON(f.root, nil), "sphere": map[string]any{"center": []any{0., 0., 0.}, "radius": 100.}})
	if err != nil {
		t.Fatal(err)
	}
	request := map[string]any{"op": "read", "handle": handle, "basis": query["receipt"], "address": pathJSON(f.root, []values.ElementID{storeTestID(9710), n, leaf})}
	if _, err := federatedTestCall(t, f.owners[0], caller, connections, request); !errors.Is(err, values.PublicDenied) {
		t.Fatal("Query promoted to Read", err)
	}
	auth.denyRead = false
	read, err := federatedTestCall(t, f.owners[0], caller, connections, request)
	if err != nil {
		t.Fatal(err)
	}
	request["basis"] = read["receipt"]
	auth.changeRead = true
	if _, err := federatedTestCall(t, f.owners[0], caller, connections, request); !errors.Is(err, values.PublicInvalidContext) {
		t.Fatal("derived receipt renewed changed Read view", err)
	}
}

type readAtAuthorizer struct {
	publicTestAuth
	denyRead   bool
	changeRead bool
}

func (a *readAtAuthorizer) AuthorizeView(c values.CallerContext, rs ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	for _, r := range rs {
		if r.Permission == corecontract.PermissionReadElement && a.denyRead {
			return values.AuthorizationDenied, values.AuthorizationView{}
		}
	}
	return a.publicTestAuth.AuthorizeView(c, rs...)
}

func (a *readAtAuthorizer) ValidateView(c values.CallerContext, view values.AuthorizationView, rs ...values.Requirement) values.AuthorizationDecision {
	for _, r := range rs {
		if r.Permission == corecontract.PermissionReadElement && a.changeRead {
			return values.AuthorizationChanged
		}
	}
	return a.publicTestAuth.ValidateView(c, view, rs...)
}
