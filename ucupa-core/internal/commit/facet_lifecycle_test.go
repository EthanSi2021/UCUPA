package commit_test

import (
	"context"
	"errors"
	"testing"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/commit"
)

func TestAttachmentControlLifecycleAndDurableReplay(t *testing.T) {
	for _, durable := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory", true: "durable"}[durable], func(t *testing.T) {
			c, id, path := attachmentOwner(t, durable)
			initial, err := c.elements.ReadElement(attachmentActor, id, nil)
			if err != nil {
				t.Fatal(err)
			}
			if initial.Attachment.State != corecontract.FacetNone || !initial.Attachment.Revision.Valid() || initial.Authority != c.owner.AuthorityRef() || initial.Element != id {
				t.Fatal("missing atomic NONE")
			}
			proposal := attachmentProposal(t, c, id, "propose")
			read, _ := c.elements.ReadElement(attachmentActor, id, nil)
			if read.Record.Entity.Revision != initial.Record.Entity.Revision || read.Attachment.Key.Valid() || read.Attachment.State != corecontract.FacetNone {
				t.Fatal("pending leaked or spatial revision changed")
			}
			acceptance := attachmentSourceFunc(func(_ context.Context, hop values.NextHop, key values.FacetAcceptanceKey) (corecontract.FacetAcceptanceEvidence, error) {
				if hop != "local:repaired" || !key.Valid() {
					t.Fatal("wrong address or key")
				}
				return acceptedAttachment(key), nil
			})
			if err := c.facets.SetFacetAcceptanceSource(acceptance); err != nil {
				t.Fatal(err)
			}
			updated, err := c.facets.UpdateFacetNextHop(attachmentActor, "address", proposal.Pending, proposal.NextHopRevision, "local:repaired")
			if err != nil || updated.Attachment.Revision != proposal.Attachment.Revision || updated.NextHopRevision == proposal.NextHopRevision {
				t.Fatalf("address: %v", err)
			}
			active, err := c.facets.ActivateFacet(context.Background(), attachmentActor, id, proposal.Attachment.Revision, "activate")
			if err != nil || active.Attachment.State != corecontract.FacetActive || active.Attachment.NextHop != "local:repaired" {
				t.Fatalf("activate: %#v %v", active, err)
			}
			detached, err := c.facets.DetachFacet(attachmentActor, id, active.Attachment.Revision, "detach")
			if err != nil || detached.Attachment.State != corecontract.FacetDetached || detached.Attachment.Key != proposal.Pending || detached.NextHop != "" {
				t.Fatalf("detach: %#v %v", detached, err)
			}
			if durable {
				if err := c.owner.CloseStore(); err != nil {
					t.Fatal(err)
				}
				c, err = reopenAttachmentOwner(path)
				if err != nil {
					t.Fatal(err)
				}
				defer c.owner.CloseStore()
				if err := c.facets.SetFacetAcceptanceSource(acceptance); err != nil {
					t.Fatal(err)
				}
			}
			before := commit.CaptureCompleteStateForTest(c.owner)
			replay, err := c.facets.ActivateFacet(context.Background(), attachmentActor, id, proposal.Attachment.Revision, "activate")
			if err != nil || replay != active || !commit.MatchesCompleteStateForTest(c.owner, before) {
				t.Fatalf("offline retained activation: %#v %v", replay, err)
			}
			_, err = c.facets.UpdateFacetNextHop(attachmentActor, "after-detach", proposal.Pending, updated.NextHopRevision, "local:wrong")
			if !errors.Is(err, corecontract.ErrAttachmentChanged) {
				t.Fatalf("detached address allowed: %v", err)
			}
			// The locally detached record keeps its revoked key of the same
			// Core: only that Facet may be re-proposed, and the open recovery
			// obligation survives ordinary cancellation and Element deletion
			// because the Facet may already hold the new acceptance.
			recovery, err := c.facets.ProposeFacet(attachmentActor, id, proposal.Pending.Facet, "local:repaired", detached.Attachment.Revision, "reattach")
			if err != nil || recovery.Status != corecontract.MutationCommitted || !recovery.Pending.Valid() ||
				recovery.Pending.Core != proposal.Pending.Core || recovery.Pending.Facet != proposal.Pending.Facet {
				t.Fatalf("recovery proposal rejected: %#v %v", recovery, err)
			}
			if _, err = c.facets.ProposeFacet(attachmentActor, id, proposal.Pending.Facet, "local:repaired", recovery.Attachment.Revision, "reattach-duplicate"); !errors.Is(err, corecontract.ErrAttachmentStateConflict) {
				t.Fatalf("duplicate recovery proposal admitted: %v", err)
			}
			if _, err = c.facets.CancelFacetProposal(attachmentActor, id, recovery.Attachment.Revision, "reattach-cancel"); !errors.Is(err, corecontract.ErrAttachmentStateConflict) {
				t.Fatalf("recovery proposal cancelled: %v", err)
			}
			deleted, err := c.elements.ReadElement(attachmentActor, id, nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = c.elements.DeleteEntity(attachmentActor, id, deleted.Record.Entity.Revision, "reattach-delete"); !errors.Is(err, corecontract.ErrConflict) {
				t.Fatalf("recovery obligation deleted: %v", err)
			}
			revived, err := c.facets.ActivateFacet(context.Background(), attachmentActor, id, recovery.Attachment.Revision, "reattach-activate")
			if err != nil || revived.Attachment.State != corecontract.FacetActive || revived.Attachment.Key != recovery.Pending ||
				revived.Attachment.NextHop != "local:repaired" {
				t.Fatalf("recovery activation: %#v %v", revived, err)
			}
			if err := commit.ValidateCompleteStateForTest(c.owner); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAttachmentLateAcceptanceCannotReviveCanceledProposal(t *testing.T) {
	c, id, _ := attachmentOwner(t, true)
	p := attachmentProposal(t, c, id, "proposal")
	if err := c.facets.SetFacetAcceptanceSource(attachmentSourceFunc(func(_ context.Context, _ values.NextHop, key values.FacetAcceptanceKey) (corecontract.FacetAcceptanceEvidence, error) {
		// A callback into Core would deadlock if the outgoing call held its lock.
		if _, err := c.facets.CancelFacetProposal(attachmentActor, id, p.Attachment.Revision, "cancel-during-acceptance"); err != nil {
			t.Fatal(err)
		}
		return acceptedAttachment(key), nil
	})); err != nil {
		t.Fatal(err)
	}
	_, err := c.facets.ActivateFacet(context.Background(), attachmentActor, id, p.Attachment.Revision, "late-activate")
	if !errors.Is(err, corecontract.ErrAttachmentRevisionConflict) {
		t.Fatalf("late activation: %v", err)
	}
	read, err := c.facets.ReadFacetProposal(attachmentActor, id)
	if err != nil {
		t.Fatal(err)
	}
	if read.Pending.Valid() || read.Attachment.State != corecontract.FacetNone || read.NextHop != "" {
		t.Fatal("cancel revived")
	}
	if err := commit.ValidateCompleteStateForTest(c.owner); err != nil {
		t.Fatal(err)
	}
}
