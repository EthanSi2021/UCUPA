package commit_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/commit"
)

func TestAttachmentFaultsAndExactRetry(t *testing.T) {
	for _, after := range []bool{false, true} {
		t.Run(map[bool]string{false: "rollback", true: "response-loss"}[after], func(t *testing.T) {
			c, id, _ := attachmentOwner(t, true)
			read, _ := c.elements.ReadElement(attachmentActor, id, nil)
			target, _ := values.NewFactAuthorityRef()
			commit.InjectWriteFailureForTest(c.owner, after, errors.New("injected attachment write failure"))
			_, err := c.facets.ProposeFacet(attachmentActor, id, target, "local:facet", read.Attachment.Revision, "fault-propose")
			want := corecontract.ErrUnavailable
			if after {
				want = corecontract.ErrAttachmentIndeterminate
			}
			if !errors.Is(err, want) {
				t.Fatalf("fault: %v", err)
			}
			current, _ := c.facets.ReadFacetProposal(attachmentActor, id)
			if current.Pending.Valid() != after {
				t.Fatal("recovery chose wrong side of commit")
			}
			result, err := c.facets.ProposeFacet(attachmentActor, id, target, "local:facet", read.Attachment.Revision, "fault-propose")
			events, _ := commit.AttachmentHistoryForTest(c.owner, "fault-propose")
			if err != nil || result.Status != corecontract.MutationCommitted || events != 1 {
				t.Fatalf("retry: %#v %v", result, err)
			}
			if err := commit.ValidateCompleteStateForTest(c.owner); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAttachmentAuthorizationAndUnavailableDoNotClaimIntent(t *testing.T) {
	c, id, _ := attachmentOwner(t, true)
	p := attachmentProposal(t, c, id, "proposal")
	var called atomic.Int32
	var unavailable atomic.Bool
	unavailable.Store(true)
	if err := c.facets.SetFacetAcceptanceSource(attachmentSourceFunc(func(_ context.Context, _ values.NextHop, k values.FacetAcceptanceKey) (corecontract.FacetAcceptanceEvidence, error) {
		called.Add(1)
		if unavailable.Load() {
			return corecontract.FacetAcceptanceEvidence{}, corecontract.ErrUnavailable
		}
		return acceptedAttachment(k), nil
	})); err != nil {
		t.Fatal(err)
	}
	c.policy.denied.Store(true)
	_, err := c.facets.ActivateFacet(context.Background(), attachmentActor, id, p.Attachment.Revision, "activate")
	_, claimed := commit.AttachmentHistoryForTest(c.owner, "activate")
	if !errors.Is(err, corecontract.ErrDenied) || called.Load() != 0 || claimed {
		t.Fatal("denied intent reached peer or was retained")
	}
	c.policy.denied.Store(false)
	_, err = c.facets.ActivateFacet(context.Background(), attachmentActor, id, p.Attachment.Revision, "activate")
	_, claimed = commit.AttachmentHistoryForTest(c.owner, "activate")
	if !errors.Is(err, corecontract.ErrUnavailable) || claimed {
		t.Fatal("unavailable peer claimed intent")
	}
	unavailable.Store(false)
	active, err := c.facets.ActivateFacet(context.Background(), attachmentActor, id, p.Attachment.Revision, "activate")
	if err != nil || active.Attachment.State != corecontract.FacetActive {
		t.Fatal(err)
	}
	c.policy.denied.Store(true)
	_, err = c.facets.ActivateFacet(context.Background(), attachmentActor, id, p.Attachment.Revision, "activate")
	if !errors.Is(err, corecontract.ErrDenied) || called.Load() != 2 {
		t.Fatal("retained result bypassed authorization")
	}
}

func TestAttachmentInFlightAddressUpdatePreservesNewContact(t *testing.T) {
	c, id, _ := attachmentOwner(t, true)
	p := attachmentProposal(t, c, id, "proposal")
	if err := c.facets.SetFacetAcceptanceSource(attachmentSourceFunc(func(_ context.Context, hop values.NextHop, k values.FacetAcceptanceKey) (corecontract.FacetAcceptanceEvidence, error) {
		if hop != "local:facet" {
			t.Fatal("did not use captured current contact")
		}
		if _, err := c.facets.UpdateFacetNextHop(attachmentActor, "repair", k, p.NextHopRevision, "local:new"); err != nil {
			t.Fatal(err)
		}
		return acceptedAttachment(k), nil
	})); err != nil {
		t.Fatal(err)
	}
	active, err := c.facets.ActivateFacet(context.Background(), attachmentActor, id, p.Attachment.Revision, "activate")
	if err != nil || active.Attachment.NextHop != "local:new" {
		t.Fatalf("old response reverted contact: %#v %v", active, err)
	}
	read, _ := c.elements.ReadElement(attachmentActor, id, nil)
	if _, err := c.elements.DeleteEntity(attachmentActor, id, read.Record.Entity.Revision, "delete"); err != nil {
		t.Fatal(err)
	}
	missing, err := c.facets.ReadFacetAttachment(attachmentActor, id)
	if err != nil || missing.State != corecontract.ExistenceNotFound || missing.Attachment != (corecontract.FacetAttachment{}) {
		t.Fatal("deleted Element retained effective attachment")
	}
	if err := commit.ValidateCompleteStateForTest(c.owner); err != nil {
		t.Fatal(err)
	}
}

func TestAttachmentCorruptOutcomeAndCommitRejected(t *testing.T) {
	for _, part := range []string{"outcome", "before-cut", "accepted-key", "contact", "commit"} {
		t.Run(part, func(t *testing.T) {
			c, id, path := attachmentOwner(t, true)
			p := attachmentProposal(t, c, id, "proposal")
			if err := c.facets.SetFacetAcceptanceSource(attachmentSourceFunc(func(_ context.Context, _ values.NextHop, k values.FacetAcceptanceKey) (corecontract.FacetAcceptanceEvidence, error) {
				return acceptedAttachment(k), nil
			})); err != nil {
				t.Fatal(err)
			}
			if _, err := c.facets.ActivateFacet(context.Background(), attachmentActor, id, p.Attachment.Revision, "activate"); err != nil {
				t.Fatal(err)
			}
			if err := c.owner.CloseStore(); err != nil {
				t.Fatal(err)
			}
			if err := commit.CorruptAttachmentStoreForTest(path, part); err != nil {
				t.Fatal(err)
			}
			opened, err := commit.OpenDurable(path, nil)
			if opened != nil {
				opened.CloseStore()
				t.Fatal("corrupt attachment Store opened")
			}
			if !errors.Is(err, corecontract.ErrStore) {
				t.Fatal(err)
			}
		})
	}
}
