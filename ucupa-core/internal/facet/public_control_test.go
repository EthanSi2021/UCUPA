package facet_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/access"
	"ucupa/ucupa-core/internal/commit"
	"ucupa/ucupa-core/internal/element"
	"ucupa/ucupa-core/internal/facet"
)

var publicCaller = values.CallerContext{Subject: facetActor, Credential: [16]byte{1}, Establishment: [32]byte{2}, Proof: [32]byte{3}}

func publicFixture(t *testing.T, durable bool, admission *access.Admission) fixture {
	t.Helper()
	f := newFixture(t, durable)
	f.auth.caller = publicCaller
	if admission != nil {
		f.admission = admission
		f.facets = facet.New(f.owner, f.auth, admission, nil)
	}
	return f
}

func publicPropose(t *testing.T, f fixture, id contract.OperationID) contract.PublicFacetInspection {
	t.Helper()
	initial, err := f.facets.InspectFacetControl(context.Background(), publicCaller, facetEntity)
	if err != nil || initial.State != contract.FacetNone || !initial.Revision.Valid() {
		t.Fatalf("initial = %#v, %v", initial, err)
	}
	target, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	status, pending, err := f.facets.ApplyFacetControl(context.Background(), publicCaller, id, contract.PublicFacetCommand{
		Kind: "propose", Element: facetEntity, Expected: initial.Revision, Target: target, NextHop: "local:facet",
	})
	if err != nil || status != contract.MutationCommitted || pending.PendingTarget != target || !pending.PendingProposal.Valid() {
		t.Fatalf("public propose = %#v, %v", pending, err)
	}
	return pending
}

func TestPublicFacetLifecycleAndDurableReplay(t *testing.T) {
	for _, durable := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory", true: "durable"}[durable], func(t *testing.T) {
			f := publicFixture(t, durable, nil)
			pending := publicPropose(t, f, "public-propose")
			_, repaired, err := f.facets.ApplyFacetControl(context.Background(), publicCaller, "public-contact", contract.PublicFacetCommand{
				Kind: "contact", Element: facetEntity, Expected: pending.ContactRevision, NextHop: "local:repaired",
			})
			if err != nil || repaired.NextHop != "local:repaired" || repaired.Revision != pending.Revision || repaired.PendingProposal != pending.PendingProposal {
				t.Fatalf("public contact = %#v, %v", repaired, err)
			}
			if err = f.facets.SetFacetAcceptanceSource(acceptanceSource(func(_ context.Context, hop values.NextHop, key values.FacetAcceptanceKey) (contract.FacetAcceptanceEvidence, error) {
				if hop != "local:repaired" {
					t.Fatalf("acceptance used %q", hop)
				}
				return accepted(key), nil
			})); err != nil {
				t.Fatal(err)
			}
			command := contract.PublicFacetCommand{Kind: "activate", Element: facetEntity, Expected: repaired.Revision}
			status, active, err := f.facets.ApplyFacetControl(context.Background(), publicCaller, "public-activate", command)
			if err != nil || status != contract.MutationCommitted || active.State != contract.FacetActive || active.PendingTarget != "" {
				t.Fatalf("public activate = %#v, %v", active, err)
			}
			before := facts(t, f.owner, facetEntity)
			if durable {
				f.owner.Lock()
				f.owner.BeginCloseLocked()
				f.owner.Unlock()
				if err = f.owner.CloseStore(); err != nil {
					t.Fatal(err)
				}
				owner, openErr := commit.OpenDurable(f.path, nil)
				if openErr != nil {
					t.Fatal(openErr)
				}
				defer owner.CloseStore()
				f.owner = owner
				f.facets = facet.New(owner, f.auth, access.New(f.auth, owner, publicLimits(8)), nil)
			}
			replayStatus, replay, replayErr := f.facets.ApplyFacetControl(context.Background(), publicCaller, "public-activate", command)
			if replayErr != nil || replayStatus != contract.MutationCommitted || replay != active || facts(t, f.owner, facetEntity) != before {
				t.Fatalf("public replay = %#v, %v", replay, replayErr)
			}
			_, detached, err := f.facets.ApplyFacetControl(context.Background(), publicCaller, "public-detach", contract.PublicFacetCommand{
				Kind: "detach", Element: facetEntity, Expected: active.Revision,
			})
			if err != nil || detached.State != contract.FacetDetached {
				t.Fatalf("public detach = %#v, %v", detached, err)
			}
			_, _, err = f.facets.ApplyFacetControl(context.Background(), publicCaller, "stale-propose", contract.PublicFacetCommand{
				Kind: "propose", Element: facetEntity, Expected: pending.Revision, Target: pending.PendingTarget, NextHop: "local:facet",
			})
			if !errors.Is(err, values.PublicConflict) {
				t.Fatalf("stale proposal = %v", err)
			}
		})
	}
}

func TestPublicFacetRejectionsDoNotClaimOperation(t *testing.T) {
	for _, mode := range []string{"owner", "kind", "forgery", "extra", "denied", "aba", "cancelled", "missing", "capacity"} {
		t.Run(mode, func(t *testing.T) {
			f := publicFixture(t, false, nil)
			initial, err := f.facets.InspectFacetControl(context.Background(), publicCaller, facetEntity)
			if err != nil {
				t.Fatal(err)
			}
			target := endpointAuthority(t)
			valid := contract.PublicFacetCommand{Kind: "propose", Element: facetEntity, Expected: initial.Revision, Target: target, NextHop: "local:facet"}
			command := valid
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			want := error(values.PublicInvalid)
			switch mode {
			case "owner":
				command.Expected.Owner = target
			case "kind":
				command.Expected.Kind = "record-revision"
			case "forgery":
				command.Expected, _ = values.NewPublicToken(f.owner.AuthorityRef(), "attachment-revision")
				want = values.PublicConflict
			case "extra":
				command.Kind = "cancel"
			case "denied":
				f.auth.decision = values.AuthorizationDenied
				want = values.PublicDenied
			case "aba":
				f.auth.validate = values.AuthorizationChanged
				want = values.PublicInvalidContext
			case "cancelled":
				cancel()
				want = values.PublicUnavailable
			case "missing":
				command.Element = "32000000-0000-4000-8000-000000000099"
				want = values.PublicNotFound
			case "capacity":
				f.admission = access.New(f.auth, f.owner, publicLimits(1))
				f.facets = facet.New(f.owner, f.auth, f.admission, nil)
				if err = f.admission.Acquire(); err != nil {
					t.Fatal(err)
				}
				want = values.PublicResourceExhausted
			}
			before := facts(t, f.owner, facetEntity)
			_, _, err = f.facets.ApplyFacetControl(ctx, publicCaller, "must-not-claim", command)
			if !errors.Is(err, want) || facts(t, f.owner, facetEntity) != before {
				t.Fatalf("rejection = %v, want %v without fact change", err, want)
			}
			f.auth.decision, f.auth.validate = values.AuthorizationValid, values.AuthorizationValid
			if mode == "capacity" {
				f.admission.Release()
			}
			status, recovered, retryErr := f.facets.ApplyFacetControl(context.Background(), publicCaller, "must-not-claim", valid)
			if retryErr != nil || status != contract.MutationCommitted || recovered.PendingTarget != target {
				t.Fatalf("operation was claimed: %#v, %v", recovered, retryErr)
			}
		})
	}
}

func TestPublicFacetOldContactCannotChangeNewProposal(t *testing.T) {
	f := publicFixture(t, false, nil)
	first := publicPropose(t, f, "first-proposal")
	_, cancelled, err := f.facets.ApplyFacetControl(context.Background(), publicCaller, "cancel", contract.PublicFacetCommand{
		Kind: "cancel", Element: facetEntity, Expected: first.Revision,
	})
	if err != nil || cancelled.PendingTarget != "" {
		t.Fatalf("cancel = %#v, %v", cancelled, err)
	}
	target := endpointAuthority(t)
	_, second, err := f.facets.ApplyFacetControl(context.Background(), publicCaller, "second-proposal", contract.PublicFacetCommand{
		Kind: "propose", Element: facetEntity, Expected: cancelled.Revision, Target: target, NextHop: "local:second",
	})
	if err != nil || second.PendingProposal == first.PendingProposal {
		t.Fatalf("second proposal = %#v, %v", second, err)
	}
	before := facts(t, f.owner, facetEntity)
	_, _, err = f.facets.ApplyFacetControl(context.Background(), publicCaller, "contact-operation", contract.PublicFacetCommand{
		Kind: "contact", Element: facetEntity, Expected: first.ContactRevision, NextHop: "local:wrong",
	})
	if !errors.Is(err, values.PublicConflict) || facts(t, f.owner, facetEntity) != before {
		t.Fatalf("old contact = %v", err)
	}
	_, repaired, err := f.facets.ApplyFacetControl(context.Background(), publicCaller, "contact-operation", contract.PublicFacetCommand{
		Kind: "contact", Element: facetEntity, Expected: second.ContactRevision, NextHop: "local:repaired",
	})
	if err != nil || repaired.NextHop != "local:repaired" {
		t.Fatalf("old contact claimed operation: %#v, %v", repaired, err)
	}
}

func TestPublicFacetRetainedOperationRejectsOtherCallerAndCommand(t *testing.T) {
	f := publicFixture(t, false, nil)
	pending := publicPropose(t, f, "proposal")
	if err := f.facets.SetFacetAcceptanceSource(acceptanceSource(func(_ context.Context, _ values.NextHop, key values.FacetAcceptanceKey) (contract.FacetAcceptanceEvidence, error) {
		return accepted(key), nil
	})); err != nil {
		t.Fatal(err)
	}
	command := contract.PublicFacetCommand{Kind: "activate", Element: facetEntity, Expected: pending.Revision}
	_, active, err := f.facets.ApplyFacetControl(context.Background(), publicCaller, "activate", command)
	if err != nil {
		t.Fatal(err)
	}
	before := facts(t, f.owner, facetEntity)
	f.auth.caller = values.CallerContext{}
	other := publicCaller
	other.Subject.Principal = "other"
	_, _, err = f.facets.ApplyFacetControl(context.Background(), other, "activate", command)
	if !errors.Is(err, values.PublicConflict) || facts(t, f.owner, facetEntity) != before {
		t.Fatalf("other caller adopted operation: %v", err)
	}
	_, _, err = f.facets.ApplyFacetControl(context.Background(), publicCaller, "activate", contract.PublicFacetCommand{
		Kind: "detach", Element: facetEntity, Expected: active.Revision,
	})
	if !errors.Is(err, values.PublicConflict) || facts(t, f.owner, facetEntity) != before {
		t.Fatalf("other command adopted operation: %v", err)
	}
}

func endpointAuthority(t *testing.T) values.FactAuthorityRef {
	t.Helper()
	ref, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

func TestPublicFacetOriginalViewAcrossAcceptanceRoundTrip(t *testing.T) {
	for _, mode := range []string{"denied", "aba", "unavailable", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			f := publicFixture(t, false, nil)
			pending := publicPropose(t, f, "proposal")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if err := f.facets.SetFacetAcceptanceSource(acceptanceSource(func(_ context.Context, _ values.NextHop, key values.FacetAcceptanceKey) (contract.FacetAcceptanceEvidence, error) {
				switch mode {
				case "denied":
					f.auth.validate = values.AuthorizationDenied
				case "aba":
					f.auth.validate = values.AuthorizationChanged
				case "unavailable":
					f.auth.validate = values.AuthorizationUnavailable
				case "cancel":
					cancel()
				}
				return accepted(key), nil
			})); err != nil {
				t.Fatal(err)
			}
			before := facts(t, f.owner, facetEntity)
			_, _, err := f.facets.ApplyFacetControl(ctx, publicCaller, "activate", contract.PublicFacetCommand{
				Kind: "activate", Element: facetEntity, Expected: pending.Revision,
			})
			want := error(values.PublicUnavailable)
			switch mode {
			case "denied":
				want = values.PublicDenied
			case "aba":
				want = values.PublicInvalidContext
			case "cancel":
				want = context.Canceled
			}
			if !errors.Is(err, want) || facts(t, f.owner, facetEntity) != before {
				t.Fatalf("round trip = %v, want %v without fact change", err, want)
			}
		})
	}
}

func TestPublicFacetKeepsOriginalCallerViewAndPermitReauthorization(t *testing.T) {
	f := publicFixture(t, false, nil)
	pending := publicPropose(t, f, "proposal")
	seenSource := false
	if err := f.facets.SetFacetAcceptanceSource(acceptanceSource(func(_ context.Context, _ values.NextHop, key values.FacetAcceptanceKey) (contract.FacetAcceptanceEvidence, error) {
		seenSource = true
		f.auth.validate = values.AuthorizationDenied
		return accepted(key), nil
	})); err != nil {
		t.Fatal(err)
	}
	before := facts(t, f.owner, facetEntity)
	_, _, err := f.facets.ApplyFacetControl(context.Background(), publicCaller, "activate", contract.PublicFacetCommand{
		Kind: "activate", Element: facetEntity, Expected: pending.Revision,
	})
	if !seenSource || !errors.Is(err, values.PublicDenied) || facts(t, f.owner, facetEntity) != before {
		t.Fatalf("secondary authorization = %v, source=%v", err, seenSource)
	}

	f.auth.validate = values.AuthorizationValid
	other := publicCaller
	other.Subject.Principal = "other"
	f.auth.before = func() {
		if other == publicCaller {
			t.Fatal("test caller did not differ")
		}
	}
	_, _, err = f.facets.ApplyFacetControl(context.Background(), other, "other-proposal", contract.PublicFacetCommand{
		Kind: "cancel", Element: facetEntity, Expected: pending.Revision,
	})
	if err == nil {
		t.Fatal("different caller adopted the original operation basis")
	}
}

func TestPublicFacetCommitSurvivesFinalViewRevocation(t *testing.T) {
	f := publicFixture(t, true, nil)
	pending := publicPropose(t, f, "propose")
	checks := 0
	if err := f.facets.SetFacetAcceptanceSource(acceptanceSource(func(_ context.Context, _ values.NextHop, key values.FacetAcceptanceKey) (contract.FacetAcceptanceEvidence, error) {
		f.auth.before = func() {
			checks++
			if checks == 2 {
				f.auth.validate = values.AuthorizationDenied
			}
		}
		return accepted(key), nil
	})); err != nil {
		t.Fatal(err)
	}
	command := contract.PublicFacetCommand{Kind: "activate", Element: facetEntity, Expected: pending.Revision}
	_, hidden, err := f.facets.ApplyFacetControl(context.Background(), publicCaller, "activate", command)
	if !errors.Is(err, values.PublicDenied) || hidden != (contract.PublicFacetInspection{}) {
		t.Fatalf("hidden response = %#v, %v", hidden, err)
	}
	if got := facts(t, f.owner, facetEntity); got.attachment.State != contract.FacetActive {
		t.Fatalf("final revocation rolled back facts: %#v", got)
	}
	f.auth.before, f.auth.validate = nil, values.AuthorizationValid
	before := facts(t, f.owner, facetEntity)
	status, recovered, err := f.facets.ApplyFacetControl(context.Background(), publicCaller, "activate", command)
	if err != nil || status != contract.MutationCommitted || recovered.State != contract.FacetActive || facts(t, f.owner, facetEntity) != before {
		t.Fatalf("exact recovery = %#v, %v", recovered, err)
	}
}

func TestPublicFacetUsesSharedAdmissionAndDisabledFailsFirst(t *testing.T) {
	f := newFixture(t, false)
	shared := access.New(f.auth, f.owner, publicLimits(1))
	f.facets = facet.New(f.owner, f.auth, shared, nil)
	if err := shared.Acquire(); err != nil {
		t.Fatal(err)
	}
	_, _, err := f.facets.ApplyFacetControl(context.Background(), publicCaller, "capacity", contract.PublicFacetCommand{})
	if !errors.Is(err, values.PublicInvalid) {
		t.Fatalf("input validation must precede admission: %v", err)
	}
	initial, err := f.facets.InspectFacetControl(context.Background(), publicCaller, facetEntity)
	if err != nil {
		t.Fatal(err)
	}
	target, _ := values.NewFactAuthorityRef()
	_, _, err = f.facets.ApplyFacetControl(context.Background(), publicCaller, "capacity", contract.PublicFacetCommand{
		Kind: "propose", Element: facetEntity, Expected: initial.Revision, Target: target, NextHop: "local:facet",
	})
	shared.Release()
	if !errors.Is(err, values.PublicResourceExhausted) {
		t.Fatalf("shared admission = %v", err)
	}

	disabled := facet.New(f.owner, f.auth, access.New(f.auth, f.owner, values.PublicLimits{}), nil)
	if _, err = disabled.InspectFacetControl(context.Background(), publicCaller, "invalid"); !errors.Is(err, values.PublicUnavailable) {
		t.Fatalf("disabled inspect ordering = %v", err)
	}
	if _, _, err = disabled.ApplyFacetControl(nil, publicCaller, "", contract.PublicFacetCommand{}); !errors.Is(err, values.PublicUnavailable) {
		t.Fatalf("disabled apply ordering = %v", err)
	}
}

func TestPublicFixtureCanAssembleOpenedOwnerWithoutRoot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "opened.db")
	authority, _ := values.NewFactAuthorityRef()
	owner, err := commit.CreateDurable(path, authority, [32]byte{33}, contract.RealmInitialization{Realm: "fixture-realm", NextHop: "local:fixture"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	auth := &testAuthorizer{local: true, decision: values.AuthorizationValid, validate: values.AuthorizationValid, view: values.AuthorizationView{1}}
	elements := element.New(owner, auth)
	if _, err = elements.CreateRoot(facetActor, facetRoot, "root"); err != nil {
		t.Fatal(err)
	}
	owner.Lock()
	owner.BeginCloseLocked()
	owner.Unlock()
	if err = owner.CloseStore(); err != nil {
		t.Fatal(err)
	}
	opened, err := commit.OpenDurable(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.CloseStore()
	operations := facet.New(opened, auth, access.New(auth, opened, publicLimits(1)), nil)
	if operations.AuthorityRef() != authority {
		t.Fatal("opened operations changed owner identity")
	}
}
