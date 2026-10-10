package facet_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
)

func TestFacetServiceCurrentProposalAndExactResolution(t *testing.T) {
	for _, durable := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory", true: "durable"}[durable], func(t *testing.T) {
			f := newServiceFixture(t, durable, serviceLimits())
			pending := proposal(t, f.fixture, "service-propose")
			key := pending.Pending
			token := serviceProposalToken(t, f)
			before := facts(t, f.owner, facetEntity)
			f.resetCalls()
			read, err := f.facets.ReadProposalControl(context.Background(), publicCaller, key)
			if err != nil || read.Pending != key || read.Authority != key.Core || read.Element != facetEntity || read.State != contract.ExistencePresent || read.NextHop != "local:facet" || read.Cut != before.cut {
				t.Fatalf("proposal read = %#v, %v", read, err)
			}
			f.expectCalls(t, true, contract.PermissionFacetProposalRead)
			f.resetCalls()
			resolved, err := f.facets.ResolveProposalControl(context.Background(), publicCaller, key.Facet, facetEntity, token)
			if err != nil || resolved != read {
				t.Fatalf("resolve = %#v, %v; read = %#v", resolved, err, read)
			}
			f.expectCalls(t, true, contract.PermissionFacetProposalRead)
			other := newFixture(t, false).owner.AuthorityRef()
			if _, err := f.facets.ResolveProposalControl(context.Background(), publicCaller, other, facetEntity, token); !errors.Is(err, values.PublicConflict) {
				t.Fatalf("wrong service = %v", err)
			}
			missing := key
			missing.Element = "32000000-0000-4000-8000-000000000099"
			if _, err := f.facets.ReadProposalControl(context.Background(), publicCaller, missing); !errors.Is(err, values.PublicInvalid) {
				t.Fatalf("proposal key with changed element = %v", err)
			}
			if _, err := f.facets.ResolveProposalControl(context.Background(), publicCaller, key.Facet, missing.Element, token); !errors.Is(err, values.PublicNotFound) {
				t.Fatalf("missing resolve = %v", err)
			}
			if facts(t, f.owner, facetEntity) != before {
				t.Fatal("service reads changed facts")
			}
			if _, err := f.facets.CancelFacetProposal(facetActor, facetEntity, pending.Attachment.Revision, "cancel-service-proposal"); err != nil {
				t.Fatal(err)
			}
			newer := proposal(t, f.fixture, "new-service-proposal")
			if _, err := f.facets.ResolveProposalControl(context.Background(), publicCaller, key.Facet, facetEntity, token); !errors.Is(err, values.PublicConflict) {
				t.Fatalf("old proposal resolved from history or to newer key %v: %v", newer.Pending, err)
			}
		})
	}
}

func TestFacetServiceSharesAdmissionAndPreservesValidationOrder(t *testing.T) {
	f := newServiceFixture(t, false, serviceLimits())
	key := proposal(t, f.fixture, "admission-propose").Pending
	token := serviceProposalToken(t, f)
	prepared, err := f.facets.ReadAttachmentControl(context.Background(), publicCaller, key, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.admission.Acquire(); err != nil {
		t.Fatal(err)
	}
	defer f.admission.Release()
	f.resetCalls()
	if _, err := f.facets.ReadAttachmentControl(context.Background(), publicCaller, key, false); !errors.Is(err, values.PublicResourceExhausted) {
		t.Fatalf("attachment bypassed shared admission: %v", err)
	}
	if _, err := f.facets.ReadProposalControl(context.Background(), publicCaller, key); !errors.Is(err, values.PublicResourceExhausted) {
		t.Fatalf("proposal bypassed shared admission: %v", err)
	}
	invalid := values.FacetAcceptanceKey{}
	if _, err := f.facets.ReadAttachmentControl(context.Background(), publicCaller, invalid, true); !errors.Is(err, values.PublicInvalid) {
		t.Fatalf("attachment shape priority = %v", err)
	}
	if _, err := f.facets.ReadProposalControl(context.Background(), publicCaller, invalid); !errors.Is(err, values.PublicInvalid) {
		t.Fatalf("proposal shape priority = %v", err)
	}
	if len(f.policy.authorized) != 0 || len(f.policy.validated) != 0 {
		t.Fatal("rejected admission reached authorization")
	}
	// The original resolve/check ports do not acquire a second read slot.
	if _, err := f.facets.ResolveProposalControl(context.Background(), publicCaller, key.Facet, facetEntity, token); err != nil {
		t.Fatalf("resolve added read admission: %v", err)
	}
	if err := f.facets.CheckAttachmentControl(context.Background(), publicCaller, key, prepared.Preparation); err != nil {
		t.Fatalf("check added read admission: %v", err)
	}
}

func TestFacetServiceDeletedElementReturnsProvenAbsence(t *testing.T) {
	f := newServiceFixture(t, true, serviceLimits())
	key := proposal(t, f.fixture, "deleted-propose").Pending
	token := serviceProposalToken(t, f)
	deleted, err := f.elements.DeleteEntity(facetActor, facetEntity, f.entity.Record.Entity.Revision, "service-delete")
	if err != nil || deleted.Status != contract.MutationCommitted {
		t.Fatalf("delete = %v, %v", deleted.Status, err)
	}
	f.owner.RLock()
	cut := f.owner.CurrentCutLocked()
	f.owner.RUnlock()
	read, err := f.facets.ReadProposalControl(context.Background(), publicCaller, key)
	if err != nil || read.State != contract.ExistenceNotFound || read.Authority != key.Core || read.Element != key.Element || read.Cut != cut || read.Pending.Valid() {
		t.Fatalf("deleted proposal state = %v, %v", read.State, err)
	}
	attachment, err := f.facets.ReadAttachmentControl(context.Background(), publicCaller, key, true)
	if err != nil || attachment.Read.State != contract.ExistenceNotFound || attachment.Read.Cut != cut || !attachment.Preparation.Valid() {
		t.Fatalf("deleted attachment state = %v, %v", attachment.Read.State, err)
	}
	if _, err := f.facets.ResolveProposalControl(context.Background(), publicCaller, key.Facet, facetEntity, token); !errors.Is(err, values.PublicNotFound) {
		t.Fatalf("deleted proposal resolved from history: %v", err)
	}
}

func TestFacetServicePreparationUsesSameCommitPublication(t *testing.T) {
	f := newServiceFixture(t, true, serviceLimits())
	pending := proposal(t, f.fixture, "publication-propose")
	key := pending.Pending
	f.resetCalls()
	read, err := f.facets.ReadAttachmentControl(context.Background(), publicCaller, key, false)
	if err != nil || read.Preparation.Valid() || read.Proposal.Valid() || read.Read.Attachment.State != contract.FacetNone {
		t.Fatalf("unprepared pending attachment = %#v, %v", read, err)
	}
	f.expectCalls(t, true, contract.PermissionFacetAttachmentRead)
	f.resetCalls()
	prepared, err := f.facets.ReadAttachmentControl(context.Background(), publicCaller, key, true)
	if err != nil || !prepared.Preparation.Valid() || prepared.Read != read.Read {
		t.Fatalf("prepare = %#v, %v", prepared, err)
	}
	f.expectCalls(t, true, contract.PermissionFacetAttachmentRead, contract.PermissionObserveAttachment)
	if err := f.facets.SetFacetAcceptanceSource(acceptanceSource(func(_ context.Context, _ values.NextHop, got values.FacetAcceptanceKey) (contract.FacetAcceptanceEvidence, error) {
		if got != key {
			t.Fatalf("activation key = %#v", got)
		}
		return accepted(got), nil
	})); err != nil {
		t.Fatal(err)
	}
	active, err := f.facets.ActivateFacet(context.Background(), facetActor, facetEntity, pending.Attachment.Revision, "service-activate")
	if err != nil || active.Status != contract.MutationCommitted {
		t.Fatalf("activate = %#v, %v", active, err)
	}
	f.resetCalls()
	if err := f.facets.CheckAttachmentControl(context.Background(), publicCaller, key, prepared.Preparation); err != nil {
		t.Fatal(err)
	}
	f.expectGuardedCalls(t, contract.PermissionFacetAttachmentRead, contract.PermissionObserveAttachment)
	cursor := f.begin(t, prepared.Preparation)
	groups, next := f.poll(t, cursor)
	if len(groups) != 1 || !reflect.DeepEqual(groups[0].Categories, []string{"attachment"}) || !groups[0].ID.Valid() || !groups[0].Basis.Valid() || !next.Valid() {
		t.Fatalf("actual commit publication = %#v, %v", groups, next)
	}
	if groups, _ := f.poll(t, next); len(groups) != 0 {
		t.Fatalf("acknowledged group repeated: %#v", groups)
	}
	current, err := f.facets.ReadAttachmentControl(context.Background(), publicCaller, key, false)
	if err != nil || current.Read.Attachment != active.Attachment || current.Read.Cut != active.OutcomeCut || !current.Proposal.Valid() || current.Read.Cut == prepared.Read.Cut {
		t.Fatalf("current attachment mixes cuts = %#v, %v", current, err)
	}
}

func TestFacetServicePreparationPermissionContinuity(t *testing.T) {
	for _, decision := range []values.AuthorizationDecision{values.AuthorizationUnavailable, values.AuthorizationDenied, values.AuthorizationChanged} {
		t.Run(map[values.AuthorizationDecision]string{values.AuthorizationUnavailable: "unavailable", values.AuthorizationDenied: "denied", values.AuthorizationChanged: "changed"}[decision], func(t *testing.T) {
			f := newServiceFixture(t, false, serviceLimits())
			key := proposal(t, f.fixture, "permission-propose").Pending
			prepared, err := f.facets.ReadAttachmentControl(context.Background(), publicCaller, key, true)
			if err != nil {
				t.Fatal(err)
			}
			f.auth.validate = decision
			want := map[values.AuthorizationDecision]error{values.AuthorizationUnavailable: values.PublicUnavailable, values.AuthorizationDenied: values.PublicDenied, values.AuthorizationChanged: values.PublicGap}[decision]
			if err := f.facets.CheckAttachmentControl(context.Background(), publicCaller, key, prepared.Preparation); !errors.Is(err, want) {
				t.Fatalf("permission changed: %v, want %v", err, want)
			}
			f.auth.validate = values.AuthorizationValid
			f.resetCalls()
			err = f.facets.CheckAttachmentControl(context.Background(), publicCaller, key, prepared.Preparation)
			if decision == values.AuthorizationUnavailable {
				if err != nil {
					t.Fatalf("temporary unavailable made permanent gap: %v", err)
				}
			} else if !errors.Is(err, values.PublicGap) || len(f.policy.validated) != 0 {
				t.Fatalf("regrant reopened old preparation: %v, calls %v", err, f.policy.validated)
			}
		})
	}
}

func TestFacetServicePreparationBindsCallerKeyAndLifecycle(t *testing.T) {
	f := newServiceFixture(t, false, serviceLimits())
	key := proposal(t, f.fixture, "binding-propose").Pending
	prepared, err := f.facets.ReadAttachmentControl(context.Background(), publicCaller, key, true)
	if err != nil {
		t.Fatal(err)
	}
	otherCaller := publicCaller
	otherCaller.Proof[0]++
	if err := f.facets.CheckAttachmentControl(context.Background(), otherCaller, key, prepared.Preparation); !errors.Is(err, values.PublicDenied) {
		t.Fatalf("another establishment used preparation: %v", err)
	}
	otherKey := key
	otherKey.Facet = key.Core
	if err := f.facets.CheckAttachmentControl(context.Background(), publicCaller, otherKey, prepared.Preparation); !errors.Is(err, values.PublicInvalid) {
		t.Fatalf("different key used preparation: %v", err)
	}
	for _, invalid := range []values.PublicToken{{}, serviceToken(t, key.Core, "read-receipt"), serviceToken(t, key.Facet, "observation-preparation")} {
		if err := f.facets.CheckAttachmentControl(context.Background(), publicCaller, key, invalid); !errors.Is(err, values.PublicInvalid) {
			t.Fatalf("invalid preparation %#v: %v", invalid, err)
		}
	}
	if err := f.facets.CheckAttachmentControl(context.Background(), publicCaller, key, prepared.Preparation); err != nil {
		t.Fatalf("foreign/invalid attempts damaged valid preparation: %v", err)
	}
	f.removePreparation(t, prepared.Preparation)
	if err := f.facets.CheckAttachmentControl(context.Background(), publicCaller, key, prepared.Preparation); !errors.Is(err, values.PublicGap) {
		t.Fatalf("retired preparation = %v", err)
	}
	f.close()
	if _, err := f.facets.ReadAttachmentControl(context.Background(), publicCaller, key, false); !errors.Is(err, values.PublicUnavailable) {
		t.Fatalf("closed read = %v", err)
	}
	if err := f.facets.CheckAttachmentControl(context.Background(), publicCaller, key, prepared.Preparation); !errors.Is(err, values.PublicUnavailable) {
		t.Fatalf("closed preparation = %v", err)
	}
}

func TestFacetServicePreparationFailureRefundsInvisibleParent(t *testing.T) {
	limits := serviceLimits()
	limits.Contexts, limits.PerCaller = 1, 1
	f := newServiceFixture(t, false, limits)
	key := proposal(t, f.fixture, "capacity-propose").Pending
	for attempt := 0; attempt < 3; attempt++ {
		if _, err := f.facets.ReadAttachmentControl(context.Background(), publicCaller, key, true); !errors.Is(err, values.PublicResourceExhausted) {
			t.Fatalf("prepare attempt %d = %v", attempt, err)
		}
		if err := f.probeSingleContext(); err != nil {
			t.Fatalf("failed preparation leaked its parent: %v", err)
		}
	}
	if _, err := f.facets.ReadAttachmentControl(context.Background(), publicCaller, key, false); err != nil {
		t.Fatalf("failed prepare leaked admission: %v", err)
	}
}

func TestFacetServiceDisabledAndAuthorizationPriority(t *testing.T) {
	disabled := newFixture(t, false)
	key := proposal(t, disabled, "disabled-propose").Pending
	if _, err := disabled.facets.ReadAttachmentControl(context.Background(), publicCaller, key, true); !errors.Is(err, values.PublicUnavailable) {
		t.Fatalf("disabled read = %v", err)
	}
	if _, err := disabled.facets.ReadProposalControl(context.Background(), publicCaller, key); !errors.Is(err, values.PublicUnavailable) {
		t.Fatalf("disabled proposal read = %v", err)
	}
	if _, err := disabled.facets.ResolveProposalControl(context.Background(), publicCaller, key.Facet, key.Element, values.PublicToken{}); !errors.Is(err, values.PublicUnavailable) {
		t.Fatalf("disabled resolve priority = %v", err)
	}
	if err := disabled.facets.CheckAttachmentControl(context.Background(), publicCaller, key, values.PublicToken{}); !errors.Is(err, values.PublicUnavailable) {
		t.Fatalf("disabled check priority = %v", err)
	}
	f := newServiceFixture(t, false, serviceLimits())
	key = proposal(t, f.fixture, "priority-propose").Pending
	token := serviceProposalToken(t, f)
	f.close()
	f.auth.decision = values.AuthorizationDenied
	for name, call := range map[string]func() error{
		"attachment": func() error {
			_, err := f.facets.ReadAttachmentControl(context.Background(), publicCaller, key, false)
			return err
		},
		"proposal": func() error {
			_, err := f.facets.ReadProposalControl(context.Background(), publicCaller, key)
			return err
		},
		"resolve": func() error {
			_, err := f.facets.ResolveProposalControl(context.Background(), publicCaller, key.Facet, facetEntity, token)
			return err
		},
	} {
		if err := call(); !errors.Is(err, values.PublicDenied) {
			t.Fatalf("%s changed pre-lock authorization priority: %v", name, err)
		}
	}
}
