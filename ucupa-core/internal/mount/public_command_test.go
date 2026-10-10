package mount

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/access"
	coretoken "ucupa/ucupa-core/internal/token"
)

func TestPublicMountComparisonsAndRetainedRetry(t *testing.T) {
	f := newMountPublicFixture(t, false)
	inspection, err := f.a.mounts.InspectMountControl(context.Background(), f.caller, f.anchor)
	if err != nil || !inspection.ElementRevision.Valid() || inspection.MountRevision != (values.PublicToken{}) || inspection.State != corecontract.MountNone {
		t.Fatalf("inspect: %+v %v", inspection, err)
	}
	input := f.publicCommand(t)
	if input.ExpectedElement != inspection.ElementRevision {
		t.Fatal("different element token domains")
	}
	before := mountProbe(f.a.owner, "opaque-establish", f.anchor)
	resolved, view, release, err := f.a.mounts.ResolveMountControl(context.Background(), f.caller, "opaque-establish", input)
	if err != nil || resolved != f.command || !reflectEqual(before, mountProbe(f.a.owner, "opaque-establish", f.anchor)) {
		t.Fatalf("resolve wrote facts: %v", err)
	}
	defer release()
	result, err := f.a.mounts.ApplyMountControl(context.Background(), f.caller, view, "opaque-establish", resolved, f.transport)
	if err != nil || result.Decision != corecontract.MountCommitted || !result.Settled {
		t.Fatalf("apply: %+v %v", result, err)
	}
	resolved, _, release, err = f.a.mounts.ResolveMountControl(context.Background(), f.caller, "opaque-establish", input)
	if err != nil || resolved != f.command {
		t.Fatalf("retained comparison: %v", err)
	}
	defer release()
	inspection, err = f.a.mounts.InspectMountControl(context.Background(), f.caller, f.anchor)
	if err != nil || !inspection.MountRevision.Valid() || inspection.Carrier != f.b.owner.AuthorityRef() || inspection.State != corecontract.MountActive {
		t.Fatalf("active: %+v %v", inspection, err)
	}
	update := corecontract.PublicMountCommand{Kind: corecontract.MountUpdate, Element: f.anchor, ExpectedElement: inspection.ElementRevision, ExpectedMount: inspection.MountRevision, Final: mountTestTuple(t, coretoken.RevisionElement(f.command.ExpectedElementRevision), 0, 5)}
	if _, _, release, err := f.a.mounts.ResolveMountControl(context.Background(), f.caller, "opaque-update", update); err != nil {
		t.Fatal(err)
	} else {
		release()
	}
	revoke := corecontract.PublicMountCommand{Kind: corecontract.MountRevoke, Element: f.anchor, ExpectedMount: inspection.MountRevision}
	resolved, view, release, err = f.a.mounts.ResolveMountControl(context.Background(), f.caller, "opaque-revoke", revoke)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	result, err = f.a.mounts.ApplyMountControl(context.Background(), f.caller, view, "opaque-revoke", resolved, f.transport)
	if err != nil || result.Decision != corecontract.MountCommitted || !result.Settled {
		t.Fatalf("revoke: %+v %v", result, err)
	}
	if _, _, release, err := f.a.mounts.ResolveMountControl(context.Background(), f.caller, "opaque-revoke", revoke); err != nil {
		t.Fatal(err)
	} else {
		release()
	}
	if _, _, _, err := f.a.mounts.ResolveMountControl(context.Background(), f.caller, "new-revoke", revoke); !errors.Is(err, values.PublicConflict) {
		t.Fatalf("stale: %v", err)
	}
}

func TestPublicMountCommandRejectsForgeryWithoutReservation(t *testing.T) {
	for _, scenario := range []string{"owner", "kind", "value", "extra-mount", "missing-element", "extra-final", "denied", "aba", "unknown-anchor"} {
		t.Run(scenario, func(t *testing.T) {
			f := newMountPublicFixture(t, false)
			input := f.publicCommand(t)
			want := error(values.PublicInvalid)
			switch scenario {
			case "owner":
				input.ExpectedElement.Owner = f.b.owner.AuthorityRef()
			case "kind":
				input.ExpectedElement.Kind = "mount-revision"
			case "value":
				input.ExpectedElement, _ = values.NewPublicToken(f.a.owner.AuthorityRef(), "record-revision")
				want = values.PublicConflict
			case "extra-mount":
				input.ExpectedMount, _ = values.NewPublicToken(f.a.owner.AuthorityRef(), "mount-revision")
			case "missing-element":
				input.ExpectedElement = values.PublicToken{}
			case "extra-final":
				input.Final = mountTestTuple(t, mountTestID(30), 0, 5)
			case "denied":
				f.a.auth.decision = values.AuthorizationDenied
				want = values.PublicDenied
			case "aba":
				f.a.auth.validate = values.AuthorizationChanged
				want = values.PublicInvalidContext
			case "unknown-anchor":
				input.Element = mountTestID(99)
				want = values.PublicNotFound
			}
			before := mountProbe(f.a.owner, "opaque-operation", f.anchor)
			_, _, _, err := f.a.mounts.ResolveMountControl(context.Background(), f.caller, "opaque-operation", input)
			if !errors.Is(err, want) || !reflectEqual(before, mountProbe(f.a.owner, "opaque-operation", f.anchor)) {
				t.Fatalf("%v want %v", err, want)
			}
		})
	}
}

func TestPublicMountRetryCannotChangeActorOrIntent(t *testing.T) {
	f := newMountPublicFixture(t, false)
	input := f.publicCommand(t)
	if _, err := f.a.mounts.ApplyMountControl(context.Background(), f.caller, f.view, "bound-original", f.command, f.transport); err != nil {
		t.Fatal(err)
	}
	before := mountProbe(f.a.owner, "bound-original", f.anchor)
	other := f.caller
	other.Subject.Principal = "other"
	if _, _, _, err := f.a.mounts.ResolveMountControl(context.Background(), other, "bound-original", input); !errors.Is(err, values.PublicConflict) {
		t.Fatal(err)
	}
	input.ParentNextHop = "local:replacement"
	if _, _, _, err := f.a.mounts.ResolveMountControl(context.Background(), f.caller, "bound-original", input); !errors.Is(err, values.PublicConflict) {
		t.Fatal(err)
	}
	if !reflectEqual(before, mountProbe(f.a.owner, "bound-original", f.anchor)) {
		t.Fatal("retry changed facts")
	}
}

func TestPublicMountTokensSurviveDurableReopen(t *testing.T) {
	f := newMountPublicFixture(t, true)
	input := f.publicCommand(t)
	if _, err := f.a.mounts.ApplyMountControl(context.Background(), f.caller, f.view, "durable-original", f.command, f.transport); err != nil {
		t.Fatal(err)
	}
	before, err := f.a.mounts.InspectMountControl(context.Background(), f.caller, f.anchor)
	if err != nil {
		t.Fatal(err)
	}
	f.a.reopen(t)
	after, err := f.a.mounts.InspectMountControl(context.Background(), f.caller, f.anchor)
	if err != nil || before != after {
		t.Fatalf("tokens changed: %+v %+v %v", before, after, err)
	}
	resolved, _, release, err := f.a.mounts.ResolveMountControl(context.Background(), f.caller, "durable-original", input)
	if err != nil || resolved != f.command {
		t.Fatalf("retained command: %v", err)
	}
	release()
}

func reflectEqual(a, b any) bool { return reflect.DeepEqual(a, b) }

type mountEvidenceContact struct{}

func (mountEvidenceContact) CurrentContact() (corecontract.RealmAttachment, bool) {
	return corecontract.RealmAttachment{Realm: "fixture-realm", NextHop: "https://realm.example:443"}, true
}

// Model the remote Realm's destructive ReleaseHandle, unlike local no-op cleanup.
type mountEvidenceAuthorizer struct {
	*mountTestAuthorizer
	released map[[32]byte]bool
}

func (a *mountEvidenceAuthorizer) ReleaseRealmEvidence(_ context.Context, _ corecontract.RealmAttachment, evidence [32]byte) {
	a.released[evidence] = true
}

func (a *mountEvidenceAuthorizer) ValidateView(ctx context.Context, caller values.CallerContext, view values.AuthorizationView, requirements ...values.Requirement) values.AuthorizationDecision {
	if a.released[[32]byte(view)] {
		return values.AuthorizationDenied
	}
	return a.mountTestAuthorizer.ValidateView(ctx, caller, view, requirements...)
}

func TestResolveMountRetainsRemoteEvidenceUntilApply(t *testing.T) {
	f := newMountPublicFixture(t, false)
	auth := &mountEvidenceAuthorizer{mountTestAuthorizer: f.a.auth, released: make(map[[32]byte]bool)}
	f.a.mounts.admission = access.New(auth, mountEvidenceContact{}, values.PublicLimits{ContextTTL: time.Minute, ConcurrentReads: 4})
	ctx := context.Background()
	command, view, release, err := f.a.mounts.ResolveMountControl(ctx, f.caller, "remote-evidence", f.publicCommand(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		release()
		if !auth.released[[32]byte(view)] {
			t.Fatal("adapter cleanup leaked the original view")
		}
	}()
	result, err := f.a.mounts.ApplyMountControl(ctx, f.caller, view, "remote-evidence", command, f.transport)
	if err != nil || result.Decision != corecontract.MountCommitted {
		t.Fatalf("released=%v: unchanged authorized apply failed: %+v %v", auth.released[[32]byte(view)], result, err)
	}
	if auth.released[[32]byte(view)] {
		t.Fatal("apply prematurely released the original view")
	}
}

func TestResolveMountFailureAndDescriptionReleaseRemoteEvidence(t *testing.T) {
	for _, describe := range []bool{false, true} {
		f := newMountPublicFixture(t, false)
		auth := &mountEvidenceAuthorizer{mountTestAuthorizer: f.a.auth, released: make(map[[32]byte]bool)}
		f.a.mounts.admission = access.New(auth, mountEvidenceContact{}, values.PublicLimits{ContextTTL: time.Minute, ConcurrentReads: 4})
		input := f.publicCommand(t)
		if describe {
			if _, _, _, err := f.a.mounts.DescribeMountApproval(context.Background(), f.caller, "remote-describe", input); err != nil {
				t.Fatal(err)
			}
		} else {
			input.ExpectedElement, _ = values.NewPublicToken(f.a.owner.AuthorityRef(), "record-revision")
			if _, _, release, err := f.a.mounts.ResolveMountControl(context.Background(), f.caller, "remote-stale", input); !errors.Is(err, values.PublicConflict) || release != nil {
				t.Fatalf("stale resolution: cleanup=%v error=%v", release != nil, err)
			}
		}
		if len(auth.released) != 1 || !auth.released[[32]byte{1}] {
			t.Fatalf("description=%v: one-shot evidence leaked: %v", describe, auth.released)
		}
	}
}
