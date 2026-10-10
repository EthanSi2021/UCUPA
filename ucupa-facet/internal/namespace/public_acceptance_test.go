package namespace

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"ucupa/shared/values"
	facetcontract "ucupa/ucupa-facet/contract"
)

func TestPublicAcceptanceLifecycleReopenAndOfflineRepair(t *testing.T) {
	for _, durable := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory", true: "durable"}[durable], func(t *testing.T) {
			x := newPublicAcceptanceFixture(t, durable)
			initial, err := x.f.InspectAcceptanceControl(context.Background(), x.caller, x.ref)
			if err != nil || initial.Found || initial.Ref != x.ref {
				t.Fatalf("initial: %+v %v", initial, err)
			}
			accepted := x.accept(t, "accept-public")
			if accepted.Status != facetcontract.Committed || !accepted.Inspection.Found || accepted.Inspection.CoreNextHop != "local:core" || !accepted.Inspection.ContactRevision.Valid() {
				t.Fatalf("accept: %+v", accepted)
			}
			x.source.err = facetcontract.ErrUnavailable
			read, err := x.f.InspectAcceptanceControl(context.Background(), x.caller, x.ref)
			if err != nil || read != accepted.Inspection {
				t.Fatalf("retained read: %+v %v", read, err)
			}
			private, err := x.f.ReadAcceptanceControl(context.Background(), x.caller, x.key)
			if err != nil || !private.Found || private.Acceptance.Key != x.key {
				t.Fatalf("service read: %+v %v", private, err)
			}
			repaired, err := x.f.UpdateCoreContactControl(context.Background(), x.caller, "repair", x.ref, read.ContactRevision, "local:repaired")
			if err != nil || repaired.Status != facetcontract.Committed || repaired.Inspection.CoreNextHop != "local:repaired" {
				t.Fatalf("repair: %+v %v", repaired, err)
			}
			if x.source.reads.Load() != 1 {
				t.Fatal("offline operations called Core")
			}
			if durable {
				if err = Close(x.f); err != nil {
					t.Fatal(err)
				}
				x.f, err = OpenDurable(x.auth, x.source, x.path, x.config, nil)
				if err != nil {
					t.Fatal(err)
				}
			}
			before, events := x.f.state.clone(), len(x.f.events)
			replay := x.accept(t, "accept-public")
			if replay != accepted || !reflect.DeepEqual(before, x.f.state) || len(x.f.events) != events {
				t.Fatalf("original accept replay changed contact: %+v", replay)
			}
			replay, err = x.f.UpdateCoreContactControl(context.Background(), x.caller, "repair", x.ref, read.ContactRevision, "local:repaired")
			if err != nil || replay != repaired || len(x.f.events) != events {
				t.Fatalf("contact replay: %+v %v", replay, err)
			}
			_, err = x.f.UpdateCoreContactControl(context.Background(), x.caller, "stale", x.ref, read.ContactRevision, "local:stale")
			if !errors.Is(err, values.PublicConflict) || len(x.f.events) != events {
				t.Fatalf("stale CAS reserved operation: %v", err)
			}
			duplicate, err := x.f.AcceptAttachmentControl(context.Background(), x.caller, x.ref, "local:ignored", "duplicate")
			if err != nil || duplicate.Status != facetcontract.Unchanged || duplicate.Inspection != repaired.Inspection || x.source.reads.Load() != 1 {
				t.Fatalf("duplicate reset initial contact: %+v %v", duplicate, err)
			}
			_, err = x.f.AcceptAttachmentControl(context.Background(), x.caller, x.ref, "local:changed", "accept-public")
			if !errors.Is(err, values.PublicConflict) {
				t.Fatalf("adopted original request: %v", err)
			}
			other := x.caller
			other.Subject.Principal = "someone-else"
			_, err = x.f.AcceptAttachmentControl(context.Background(), other, x.ref, "local:core", "accept-public")
			if !errors.Is(err, values.PublicConflict) {
				t.Fatalf("adopted original actor: %v", err)
			}
		})
	}
}

func TestPublicAcceptanceOriginalViewAcrossProposal(t *testing.T) {
	for _, mode := range []string{"denied", "aba", "unavailable", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			x := newPublicAcceptanceFixture(t, true)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			want := error(values.PublicUnavailable)
			x.source.hook = func() {
				switch mode {
				case "denied":
					x.auth.decision.Store(uint32(values.AuthorizationDenied))
					want = values.PublicDenied
				case "aba":
					x.auth.version.Add(1)
					want = values.PublicInvalidContext
				case "unavailable":
					x.auth.decision.Store(uint32(values.AuthorizationUnavailable))
				case "cancel":
					cancel()
				}
			}
			before := x.f.state.clone()
			result, err := x.f.AcceptAttachmentControl(ctx, x.caller, x.ref, "local:core", "must-not-commit")
			if !errors.Is(err, want) || result != (facetcontract.PublicAcceptanceOutcome{}) || !reflect.DeepEqual(before, x.f.state) || len(x.f.events) != 0 {
				t.Fatalf("%+v %v want %v; altered=%v", result, err, want, !reflect.DeepEqual(before, x.f.state))
			}
		})
	}
}

func TestPublicAcceptanceRejectsInvalidResolutionWithoutClaims(t *testing.T) {
	for _, mode := range []string{"facet", "core", "element", "token", "pending", "cut", "active", "denied", "capacity"} {
		t.Run(mode, func(t *testing.T) {
			x := newPublicAcceptanceFixture(t, false)
			want := error(values.PublicUnavailable)
			switch mode {
			case "facet":
				x.source.resolved.Key.Facet = testCore
			case "core":
				x.source.resolved.Key.Core = x.f.authority
			case "element":
				x.source.resolved.Key.Element = "00000000-0000-4000-8000-00000000fa02"
			case "token":
				x.source.resolved.Proposal, _ = values.NewPublicToken(testCore, "attachment-revision")
			case "pending":
				x.source.resolved.Evidence.Pending = values.FacetAcceptanceKey{}
			case "cut":
				x.source.resolved.Evidence.Cut = values.CoreCutEvidence{}
			case "active":
				x.source.resolved.Evidence = testProof(x.key, facetcontract.CoreActive, 3)
			case "denied":
				x.auth.decision.Store(uint32(values.AuthorizationDenied))
				want = values.PublicDenied
			case "capacity":
				for n := uint64(0); n < x.f.public.Limits().ConcurrentReads; n++ {
					if !x.f.public.Enter() {
						t.Fatal("failed to reserve expected public slot")
					}
					defer x.f.public.Leave()
				}
				want = values.PublicResourceExhausted
			}
			_, err := x.f.AcceptAttachmentControl(context.Background(), x.caller, x.ref, "local:core", "rejected")
			if !errors.Is(err, want) || len(x.f.events) != 0 || len(x.f.state.Acceptances) != 0 {
				t.Fatalf("got %v; want %v; events=%d", err, want, len(x.f.events))
			}
			if (mode == "denied" || mode == "capacity") && x.source.reads.Load() != 0 {
				t.Fatal("rejected caller acquired proposal")
			}
		})
	}
}

func TestPublicAcceptanceCommitSurvivesHiddenResponse(t *testing.T) {
	x := newPublicAcceptanceFixture(t, true)
	x.auth.hook = func() {
		if x.auth.checks.Load() == 3 {
			x.auth.decision.Store(uint32(values.AuthorizationDenied))
		}
	}
	hidden, err := x.f.AcceptAttachmentControl(context.Background(), x.caller, x.ref, "local:core", "accepted")
	if !errors.Is(err, values.PublicDenied) || hidden != (facetcontract.PublicAcceptanceOutcome{}) || len(x.f.state.Acceptances) != 1 {
		t.Fatalf("commit/response: %+v %v", hidden, err)
	}
	x.auth.hook = nil
	x.auth.decision.Store(uint32(values.AuthorizationValid))
	x.source.err = facetcontract.ErrUnavailable
	if err := Close(x.f); err != nil {
		t.Fatal(err)
	}
	x.f, err = OpenDurable(x.auth, x.source, x.path, x.config, nil)
	if err != nil {
		t.Fatal(err)
	}
	result := x.accept(t, "accepted")
	if result.Status != facetcontract.Committed || x.source.reads.Load() != 1 || len(x.f.events) != 1 {
		t.Fatalf("did not recover accepted exact request: %+v", result)
	}
}
