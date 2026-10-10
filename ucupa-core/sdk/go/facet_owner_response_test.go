package coresdk

import (
	"testing"

	"google.golang.org/protobuf/proto"
	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
)

func predecessorToken(owner values.FactAuthorityRef, kind netpb.TokenKind) *netpb.Token {
	return &netpb.Token{Owner: proto.String(string(owner)), Kind: kind.Enum(), Value: make([]byte, 32)}
}

func TestFacetFormerOwnerIsOnlyDetachedHistoricalKey(t *testing.T) {
	current, former := fedOwner, fedActor
	base := &pb.Attachment{State: pb.AttachmentState_DETACHED.Enum(), Revision: predecessorToken(current, netpb.TokenKind_ATTACHMENT_REVISION), Key: &netpb.AttachmentKey{
		Core: proto.String(string(former)), Facet: proto.String(string(current)), Element: proto.String(fedElement), Proposal: predecessorToken(former, netpb.TokenKind_ATTACHMENT_REVISION),
	}}
	check := func(m proto.Message) error {
		if err := validateResponse(m.ProtoReflect(), 0); err != nil {
			return err
		}
		return validateTokenOwners(m.ProtoReflect(), current)
	}
	if err := check(base); err != nil {
		t.Fatal("valid predecessor", err)
	}
	for name, mutate := range map[string]func(*pb.Attachment){
		"active-foreign-owner": func(a *pb.Attachment) {
			a.State = pb.AttachmentState_ACTIVE.Enum()
			a.NextHop = proto.String("local:facet")
			a.ContactRevision = predecessorToken(current, netpb.TokenKind_CONTACT_REVISION)
		},
		"foreign-current-revision": func(a *pb.Attachment) { a.Revision.Owner = proto.String(string(former)) },
		"reminted-proposal":        func(a *pb.Attachment) { a.Key.Proposal.Owner = proto.String(string(current)) },
		"wrong-purpose":            func(a *pb.Attachment) { a.Key.Proposal.Kind = netpb.TokenKind_RECORD_REVISION.Enum() },
		"missing-key":              func(a *pb.Attachment) { a.Key = nil },
		"detached-address":         func(a *pb.Attachment) { a.NextHop = proto.String("local:facet") },
		"detached-contact": func(a *pb.Attachment) {
			a.ContactRevision = predecessorToken(current, netpb.TokenKind_CONTACT_REVISION)
		},
		"none-with-key": func(a *pb.Attachment) { a.State = pb.AttachmentState_NONE.Enum() },
	} {
		t.Run(name, func(t *testing.T) {
			a := proto.Clone(base).(*pb.Attachment)
			mutate(a)
			if check(a) == nil {
				t.Fatal("accepted invalid predecessor")
			}
		})
	}
	inspection := &pb.PublicFacetInspection{Element: proto.String(fedElement), State: base.State, Revision: base.Revision, Target: base.Key.Facet, KeyCore: base.Key.Core, Proposal: base.Key.Proposal}
	if err := check(inspection); err != nil {
		t.Fatal("control precursor", err)
	}
	for name, mutate := range map[string]func(*pb.PublicFacetInspection){
		"missing-key-core":    func(a *pb.PublicFacetInspection) { a.KeyCore = nil },
		"active-former-owner": func(a *pb.PublicFacetInspection) { a.State = pb.AttachmentState_ACTIVE.Enum() },
		"detached-pending":    func(a *pb.PublicFacetInspection) { a.PendingTarget = a.Target; a.PendingProposal = a.Proposal },
		"old-contact": func(a *pb.PublicFacetInspection) {
			a.NextHop = proto.String("local:old")
			a.ContactRevision = predecessorToken(former, netpb.TokenKind_CONTACT_REVISION)
		},
	} {
		t.Run(name, func(t *testing.T) {
			a := proto.Clone(inspection).(*pb.PublicFacetInspection)
			mutate(a)
			if check(a) == nil {
				t.Fatal("accepted invalid control precursor")
			}
		})
	}
	// Current pending and ACTIVE still work; the historical exception must not
	// allow a foreign pending proposal or turn former-owner history into a grant.
	pending := &pb.PublicFacetInspection{Element: proto.String(fedElement), State: pb.AttachmentState_NONE.Enum(), Revision: base.Revision, PendingTarget: base.Key.Facet, PendingProposal: base.Revision, NextHop: proto.String("local:facet"), ContactRevision: predecessorToken(current, netpb.TokenKind_CONTACT_REVISION)}
	if err := check(pending); err != nil {
		t.Fatal(err)
	}
	pending.PendingProposal = base.Key.Proposal
	if check(pending) == nil {
		t.Fatal("allowed old key to become pending")
	}
	active := proto.Clone(base).(*pb.Attachment)
	active.State = pb.AttachmentState_ACTIVE.Enum()
	active.Key.Core = proto.String(string(current))
	active.Key.Proposal = base.Revision
	active.NextHop = proto.String("local:facet")
	active.ContactRevision = predecessorToken(current, netpb.TokenKind_CONTACT_REVISION)
	if err := check(active); err != nil {
		t.Fatal(err)
	}
}

// A retired slot discloses exactly the disposed lineage key under the
// releasing owner, with no live contact left to revive.
func TestFacetRetiredStateDisclosesDisposedLineage(t *testing.T) {
	current := fedOwner
	base := &pb.Attachment{State: pb.AttachmentState_RETIRED.Enum(), Revision: predecessorToken(current, netpb.TokenKind_ATTACHMENT_REVISION), Key: &netpb.AttachmentKey{
		Core: proto.String(string(current)), Facet: proto.String(string(fedActor)), Element: proto.String(fedElement), Proposal: predecessorToken(current, netpb.TokenKind_ATTACHMENT_REVISION),
	}}
	check := func(m proto.Message) error {
		if err := validateResponse(m.ProtoReflect(), 0); err != nil {
			return err
		}
		return validateTokenOwners(m.ProtoReflect(), current)
	}
	if err := check(base); err != nil {
		t.Fatal("valid retired disclosure", err)
	}
	for name, mutate := range map[string]func(*pb.Attachment){
		"missing-key":       func(a *pb.Attachment) { a.Key = nil },
		"reminted-proposal": func(a *pb.Attachment) { a.Key.Proposal.Owner = proto.String(string(fedActor)) },
		"retired-address":   func(a *pb.Attachment) { a.NextHop = proto.String("local:facet") },
		"retired-contact": func(a *pb.Attachment) {
			a.ContactRevision = predecessorToken(current, netpb.TokenKind_CONTACT_REVISION)
		},
	} {
		t.Run(name, func(t *testing.T) {
			a := proto.Clone(base).(*pb.Attachment)
			mutate(a)
			if check(a) == nil {
				t.Fatal("accepted invalid retired disclosure")
			}
		})
	}
	inspection := &pb.PublicFacetInspection{Element: proto.String(fedElement), State: base.State, Revision: base.Revision, Target: base.Key.Facet, KeyCore: base.Key.Core, Proposal: base.Key.Proposal}
	if err := check(inspection); err != nil {
		t.Fatal("retired control projection", err)
	}
	inspection.KeyCore = nil
	if check(inspection) == nil {
		t.Fatal("accepted retired projection without its releasing owner")
	}
}

func TestFederatedReadKeepsThirdOwnerPredecessorOutsideSourceGraph(t *testing.T) {
	entry, source := fedOwner, fedActor
	former := values.FactAuthorityRef("00000000-0000-4000-8000-000000000777")
	r := &pb.ReadElementResult{Receipt: predecessorToken(entry, netpb.TokenKind_READ_RECEIPT), Closure: publicTestStamp(), SourceGraph: publicTestGraph("read"),
		Record:     &pb.Record{Element: proto.String(fedElement), Revision: predecessorToken(source, netpb.TokenKind_RECORD_REVISION)},
		Attachment: &pb.Attachment{State: pb.AttachmentState_DETACHED.Enum(), Revision: predecessorToken(source, netpb.TokenKind_ATTACHMENT_REVISION), Key: &netpb.AttachmentKey{Element: proto.String(fedElement), Core: proto.String(string(former)), Facet: proto.String(string(entry)), Proposal: predecessorToken(former, netpb.TokenKind_ATTACHMENT_REVISION)}}}
	if err := validateTokenOwners(r.ProtoReflect(), entry); err != nil {
		t.Fatal(err)
	}
	r.Record.Revision.Owner = proto.String(string(former))
	if validateTokenOwners(r.ProtoReflect(), entry) == nil {
		t.Fatal("historical key enlarged authorized fact source graph")
	}
}
