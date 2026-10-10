package namespace

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	"ucupa/shared/values"
	facetcontract "ucupa/ucupa-facet/contract"
)

func TestPublicAcceptanceConcurrentOriginalRequestsConverge(t *testing.T) {
	x := newPublicAcceptanceFixture(t, true)
	arrived := make(chan struct{}, 2)
	release := make(chan struct{})
	x.source.hook = func() { arrived <- struct{}{}; <-release }
	var wg sync.WaitGroup
	results := make(chan facetcontract.PublicAcceptanceOutcome, 2)
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := x.f.AcceptAttachmentControl(context.Background(), x.caller, x.ref, "local:core", "concurrent")
			results <- r
			errs <- err
		}()
	}
	<-arrived
	<-arrived
	close(release)
	wg.Wait()
	if a, b := <-results, <-results; a != b || a.Status != facetcontract.Committed {
		t.Fatalf("did not converge: %+v %+v", a, b)
	}
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	if len(x.f.events) != 1 || len(x.f.state.Acceptances) != 1 {
		t.Fatal("duplicate commit")
	}
}

func TestPublicAcceptanceNewProposalDoesNotReuseOldContact(t *testing.T) {
	x := newPublicAcceptanceFixture(t, true)
	old := x.accept(t, "old")
	oldKey, oldRef := x.key, x.ref
	x.key = testKey(x.f.authority, testCore, testElement, 4)
	x.ref.Proposal, _ = values.NewPublicToken(testCore, "attachment-revision")
	e := testProof(x.key, facetcontract.CoreNone, 4)
	e.Pending = x.key
	x.source.ref, x.source.resolved = x.ref, facetcontract.CoreProposalRead{Key: x.key, Evidence: e, Proposal: x.ref.Proposal}
	fresh := x.accept(t, "new")
	if fresh.Inspection.ContactRevision == old.Inspection.ContactRevision || len(x.f.state.Acceptances) != 2 {
		t.Fatal("proposal reused relation")
	}
	before := x.f.state.clone()
	_, err := x.f.UpdateCoreContactControl(context.Background(), x.caller, "wrong-relation", x.ref, old.Inspection.ContactRevision, "local:wrong")
	if !errors.Is(err, values.PublicConflict) || !reflect.DeepEqual(before, x.f.state) {
		t.Fatalf("cross-proposal CAS: %v", err)
	}
	repaired, err := x.f.UpdateCoreContactControl(context.Background(), x.caller, "old-contact", oldRef, old.Inspection.ContactRevision, "local:old")
	if err != nil || repaired.Inspection.Ref != oldRef || x.f.state.Contacts[packKey(oldKey).id()].Address != "local:old" || x.f.state.Contacts[packKey(x.key).id()].Address != "local:core" {
		t.Fatalf("historical relation contact: %+v %v", repaired, err)
	}
}

func TestPublicAcceptanceHistoryRejectsForgedContactClaims(t *testing.T) {
	x := newPublicAcceptanceFixture(t, true)
	a := x.accept(t, "accept")
	_, err := x.f.UpdateCoreContactControl(context.Background(), x.caller, "repair", x.ref, a.Inspection.ContactRevision, "local:repaired")
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"contact-token", "proposal", "first-evidence", "missing-public-claim"} {
		t.Run(mode, func(t *testing.T) {
			events := append([]event(nil), x.f.events...)
			switch mode {
			case "contact-token":
				token, _ := values.NewPublicToken(x.f.authority, "contact-revision")
				events[1].Input.PublicRequest = publicAcceptanceRequest("contact", x.ref, "local:repaired", "repair", token)
			case "proposal":
				ref := x.ref
				ref.Proposal, _ = values.NewPublicToken(testCore, "attachment-revision")
				events[1].Input.PublicRequest = publicAcceptanceRequest("contact", ref, "local:repaired", "repair", a.Inspection.ContactRevision)
			case "first-evidence":
				events[0].Core = proof{}
			case "missing-public-claim":
				events[0].Input.PublicRequest = ""
			}
			if err := validateHistory(x.f.authority, x.f.scope, x.f.state, events); !errors.Is(err, facetcontract.ErrStore) {
				t.Fatalf("accepted damaged history: %v", err)
			}
		})
	}
}

func TestPublicAcceptanceReadsAndRepairRespectCurrentAuthorization(t *testing.T) {
	x := newPublicAcceptanceFixture(t, false)
	a := x.accept(t, "accept")
	before, count := x.f.state.clone(), len(x.f.events)
	x.auth.decision.Store(uint32(values.AuthorizationDenied))
	if _, err := x.f.ReadAcceptanceControl(context.Background(), x.caller, x.key); !errors.Is(err, values.PublicDenied) {
		t.Fatalf("service read bypassed authorization: %v", err)
	}
	if _, err := x.f.InspectAcceptanceControl(context.Background(), x.caller, x.ref); !errors.Is(err, values.PublicDenied) {
		t.Fatalf("public read bypassed authorization: %v", err)
	}
	if _, err := x.f.UpdateCoreContactControl(context.Background(), x.caller, "repair", x.ref, a.Inspection.ContactRevision, "local:changed"); !errors.Is(err, values.PublicDenied) {
		t.Fatalf("contact bypassed authorization: %v", err)
	}
	if _, err := x.f.AcceptAttachmentControl(context.Background(), x.caller, x.ref, "local:core", "accept"); !errors.Is(err, values.PublicDenied) {
		t.Fatalf("retained operation bypassed authorization: %v", err)
	}
	if len(x.f.events) != count || !reflect.DeepEqual(before, x.f.state) {
		t.Fatal("denial changed state")
	}
}
