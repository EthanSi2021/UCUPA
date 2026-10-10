package namespace

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-facet/contract"
)

// A Realm contact change that lands between an operation's authorization and
// its final read or commit boundary supersedes the approval's contact
// generation: the operation must stop as unavailable instead of committing or
// disclosing under a Realm contact the authorization never covered. The late
// move is exactly the shape of a maintenance change whose new Realm already
// refuses the actor.
func TestBoundaryStopsOnRealmContactMoveAfterAuthorization(t *testing.T) {
	profiles(t, func(t *testing.T, x *fixture) {
		ctx := context.Background()
		first := x.put(t, "initial", "one", contract.ExpectAbsent())
		// Each crossing runs while the actor's authorization already passed:
		// the Core evidence read happens after it and moves the Realm contact
		// to an address whose Realm refuses the actor from then on.
		crossesBoundary := func() {
			x.auth.deny.Store(false)
			x.source.mu.Lock()
			x.source.hook = func() {
				x.source.mu.Lock()
				x.source.hook = nil
				x.source.mu.Unlock()
				moveContact(x.f, "local:moved-realm")
				x.auth.deny.Store(true)
			}
			x.source.mu.Unlock()
		}
		crossesBoundary()
		if _, err := x.f.Put(ctx, testActor, x.key, x.typ, fixturePayload("two"), contract.ExpectRevision(first.Revision), "late-put"); err != contract.ErrUnavailable {
			t.Fatalf("Put after contact move = %v; want ErrUnavailable", err)
		}
		crossesBoundary()
		if _, err := x.f.Delete(ctx, testActor, x.key, x.typ, first.Revision, "late-delete"); err != contract.ErrUnavailable {
			t.Fatalf("Delete after contact move = %v; want ErrUnavailable", err)
		}
		crossesBoundary()
		if _, err := x.f.Get(ctx, testActor, x.key, x.typ); err != contract.ErrUnavailable {
			t.Fatalf("Get after contact move = %v; want ErrUnavailable", err)
		}
		// Under the moved contact a fresh authorization still works, and the
		// stopped boundaries left the record untouched.
		x.source.mu.Lock()
		x.source.hook = nil
		x.source.mu.Unlock()
		x.auth.deny.Store(false)
		read, err := x.f.Get(ctx, testActor, x.key, x.typ)
		if err != nil || fixtureText(read.Payload) != "one" || !read.Revision.Equal(first.Revision) {
			t.Fatalf("Get under moved contact = %q, %v; want untouched %q", fixtureText(read.Payload), err, "one")
		}
	})
}

// Preserve the real authorization decision, then schedule maintenance before
// the caller can acquire its final owner lock. The original contact adapter's
// own settle has already completed, so only the operation boundary can catch it.
type contactBoundaryAuthorizer struct {
	contract.Authorizer
	after func()
}

func (a contactBoundaryAuthorizer) AuthorizePrincipal(actor values.PrincipalRef, requirements ...values.Requirement) values.AuthorizationDecision {
	decision := a.Authorizer.AuthorizePrincipal(actor, requirements...)
	if decision == values.AuthorizationValid {
		a.after()
	}
	return decision
}

func (a contactBoundaryAuthorizer) AuthorizeView(ctx context.Context, caller values.CallerContext, requirements ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	decision, view := a.Authorizer.AuthorizeView(ctx, caller, requirements...)
	if decision == values.AuthorizationValid {
		a.after()
	}
	return decision, view
}

func changeContactAfterAuthorization(t *testing.T, x *fixture) func() {
	t.Helper()
	original := x.f.authorizer
	x.f.authorizer = contactBoundaryAuthorizer{original, func() {
		current, err := x.f.RealmContact()
		if err != nil {
			t.Fatal(err)
		}
		result, err := x.f.UpdateRealmContact(context.Background(), values.CallerContext{Subject: testActor}, contract.RealmContactUpdate{
			OperationID: "boundary-contact-move", Owner: current.Owner, Realm: current.Realm,
			Expected: current.Revision, NextHop: "local:boundary-moved",
		})
		if err != nil || result.Decision != contract.RealmContactCommitted {
			t.Fatalf("real contact maintenance = %+v, %v", result, err)
		}
		x.auth.deny.Store(true)
	}}
	return func() {
		x.f.authorizer = original
		x.auth.deny.Store(false)
	}
}

func TestRetainedMutationReceiptChecksFinalContact(t *testing.T) {
	for _, operation := range []string{"put", "delete", "accept", "retire"} {
		t.Run(operation, func(t *testing.T) {
			profiles(t, func(t *testing.T, x *fixture) {
				ctx := context.Background()
				var invoke func() (contract.MutationResult, error)
				switch operation {
				case "put":
					invoke = func() (contract.MutationResult, error) {
						return x.f.Put(ctx, testActor, x.key, x.typ, fixturePayload("one"), contract.ExpectAbsent(), "replay-put")
					}
				case "delete":
					written := x.put(t, "before-delete", "one", contract.ExpectAbsent())
					invoke = func() (contract.MutationResult, error) {
						return x.f.Delete(ctx, testActor, x.key, x.typ, written.Revision, "replay-delete")
					}
				case "accept":
					invoke = func() (contract.MutationResult, error) {
						return x.f.AcceptAttachment(ctx, testActor, x.key, "local:core", "accept")
					}
				case "retire":
					x.put(t, "before-retire", "one", contract.ExpectAbsent())
					x.setState(contract.CoreDetached)
					invoke = func() (contract.MutationResult, error) {
						return x.f.RetireContent(ctx, testActor, x.key, "replay-retire")
					}
				}
				original, err := invoke()
				if err != nil {
					t.Fatal(err)
				}
				cut, events := x.f.state.Cut, len(x.f.events)
				resume := changeContactAfterAuthorization(t, x)
				if _, err := invoke(); !errors.Is(err, contract.ErrUnavailable) {
					t.Fatalf("stale contact disclosed %s receipt: %v", operation, err)
				}
				resume()
				retry, err := invoke()
				if err != nil || !reflect.DeepEqual(retry, original) || x.f.state.Cut != cut || len(x.f.events) != events {
					t.Fatalf("current-contact retry changed original result or journal: %v", err)
				}
			})
		})
	}
}

func TestTransferReceiptConflictChecksFinalContact(t *testing.T) {
	for _, step := range []string{"fence", "settle", "accept-namespace", "install"} {
		t.Run(step, func(t *testing.T) {
			profiles(t, func(t *testing.T, x *fixture) {
				// A real content commit occupies the step's ID. Even disclosing
				// this retained conflict must obey the current contact boundary.
				x.put(t, contract.OperationID(stepID("transfer", step)), "one", contract.ExpectAbsent())
				target, err := values.NewFactAuthorityRef()
				if err != nil {
					t.Fatal(err)
				}
				invoke := func() error {
					ctx := context.Background()
					switch step {
					case "fence":
						_, err := x.f.FenceNamespace(ctx, values.CallerContext{Subject: testActor}, x.key, "transfer", target)
						return err
					case "settle":
						return x.f.SettleNamespace(ctx, values.CallerContext{Subject: testActor}, x.key, "transfer", target)
					case "accept-namespace":
						_, err := x.f.AcceptNamespace(ctx, values.CallerContext{Subject: testActor}, x.key, "transfer", "local:core")
						return err
					default:
						return x.f.InstallNamespace(ctx, values.CallerContext{Subject: testActor}, x.key, "transfer", target, 1)
					}
				}
				if err := invoke(); !errors.Is(err, contract.ErrOperationConflict) {
					t.Fatalf("retained step conflict = %v", err)
				}
				cut, events := x.f.state.Cut, len(x.f.events)
				resume := changeContactAfterAuthorization(t, x)
				if err := invoke(); !errors.Is(err, contract.ErrUnavailable) {
					t.Fatalf("stale contact disclosed %s conflict: %v", step, err)
				}
				resume()
				if err := invoke(); !errors.Is(err, contract.ErrOperationConflict) || x.f.state.Cut != cut || len(x.f.events) != events {
					t.Fatalf("retry changed retained conflict or journal: %v", err)
				}
			})
		})
	}
}

type contactImportSource struct {
	*testSource
	plan contract.CoreReplacementRead
}

func (s contactImportSource) ReadReplacement(ctx context.Context, address values.NextHop, core values.FactAuthorityRef, element values.ElementID, op contract.OperationID) (contract.CoreReplacementRead, error) {
	if ctx.Err() != nil || address != s.address || core != s.plan.Authority || element != s.plan.Element || op != s.plan.Operation {
		return contract.CoreReplacementRead{}, contract.ErrUnavailable
	}
	return s.plan, nil
}

func TestExistingImportStateChecksFinalContact(t *testing.T) {
	for _, pending := range []bool{false, true} {
		name := "staged"
		if pending {
			name = "pending"
		}
		t.Run(name, func(t *testing.T) {
			profiles(t, func(t *testing.T, x *fixture) {
				source, err := values.NewFactAuthorityRef()
				if err != nil {
					t.Fatal(err)
				}
				manifest := contract.NamespaceManifest{Element: x.key.Element, Source: source,
					FinalCut: 1, Entries: []contract.NamespaceManifestEntry{}, Digest: manifestDigest(nil)}
				x.f.cores = contactImportSource{x.source, contract.CoreReplacementRead{
					Authority: x.key.Core, Element: x.key.Element, Operation: "import",
					Source: source, Target: x.key.Facet, Proposal: x.key.Proposal,
					FinalCut: manifest.FinalCut, ManifestDigest: manifest.Digest, Permitted: true, Fenced: true,
				}}
				invoke := func() (contract.NamespaceImportState, error) {
					return x.f.CheckNamespaceImport(context.Background(), values.CallerContext{Subject: testActor}, x.key, "import", manifest, "local:core")
				}
				if _, err := invoke(); err != nil {
					t.Fatal(err)
				}
				if pending {
					if _, err := x.f.AcceptNamespace(context.Background(), values.CallerContext{Subject: testActor}, x.key, "import", "local:core"); err != nil {
						t.Fatal(err)
					}
				}
				original, err := invoke()
				if err != nil || original.Accepted != pending {
					t.Fatalf("initial import state = %+v, %v", original, err)
				}
				cut, events, staged := x.f.state.Cut, len(x.f.events), len(x.f.staged)
				resume := changeContactAfterAuthorization(t, x)
				if _, err := invoke(); !errors.Is(err, contract.ErrUnavailable) {
					t.Fatalf("stale contact disclosed %s import state: %v", name, err)
				}
				resume()
				retry, err := invoke()
				if err != nil || !reflect.DeepEqual(retry, original) || x.f.state.Cut != cut || len(x.f.events) != events || len(x.f.staged) != staged {
					t.Fatalf("retry changed import state or journal: %v", err)
				}
			})
		})
	}
}
