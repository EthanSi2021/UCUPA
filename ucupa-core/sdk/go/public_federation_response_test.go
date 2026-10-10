package coresdk

import (
	"google.golang.org/protobuf/proto"
	"testing"
	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
)

func TestFederationPublicResponseKeepsEntryAndFactOwnersSeparate(t *testing.T) {
	entry, source := fedOwner, fedActor
	token := func(owner values.FactAuthorityRef, kind netpb.TokenKind) *netpb.Token {
		return &netpb.Token{Owner: proto.String(string(owner)), Kind: kind.Enum(), Value: make([]byte, 32)}
	}
	result := &pb.ReadElementResult{Receipt: token(entry, netpb.TokenKind_READ_RECEIPT),
		Closure: publicTestStamp(), SourceGraph: publicTestGraph("read"),
		Record:     &pb.Record{Element: proto.String(fedElement), Revision: token(source, netpb.TokenKind_RECORD_REVISION)},
		Attachment: &pb.Attachment{State: pb.AttachmentState_NONE.Enum(), Revision: token(source, netpb.TokenKind_ATTACHMENT_REVISION)}}
	if err := validateTokenOwners(result.ProtoReflect(), entry); err != nil {
		t.Fatal(err)
	}
	wrong := proto.Clone(result).(*pb.ReadElementResult)
	wrong.Receipt.Owner = proto.String(string(source))
	if validateTokenOwners(wrong.ProtoReflect(), entry) == nil {
		t.Fatal("accepted remote aggregate receipt")
	}
	wrong = proto.Clone(result).(*pb.ReadElementResult)
	wrong.Attachment.Revision.Owner = proto.String(string(entry))
	if validateTokenOwners(wrong.ProtoReflect(), entry) == nil {
		t.Fatal("mixed record and attachment owners")
	}
	mutation := &pb.MutationResult{Outcome: &pb.MutationResult_Record{Record: result.Record}}
	if validateTokenOwners(mutation.ProtoReflect(), entry) == nil {
		t.Fatal("read exception widened mutation owner")
	}
	query := &pb.QueryRegionResult{Receipt: result.Receipt, Matches: []*pb.Match{{Revision: result.Record.Revision, Element: proto.String(fedElement), Kind: pb.ElementKind_ANCHOR.Enum(), Address: &netpb.Address{Origin: proto.String(fedElement), Down: &netpb.ElementPath{}}}}}
	query.Closure, query.SourceGraph = publicTestStamp(), publicTestGraph("query")
	if err := validateTokenOwners(query.ProtoReflect(), entry); err != nil {
		t.Fatal(err)
	}
	query.Continuation = token(source, netpb.TokenKind_QUERY_CONTINUATION)
	if validateTokenOwners(query.ProtoReflect(), entry) == nil {
		t.Fatal("accepted remote public continuation")
	}
	poll := &pb.PollChangesResult{NextCursor: token(entry, netpb.TokenKind_OBSERVATION_CURSOR), Groups: []*pb.ChangeGroup{{Owner: proto.String(string(source)), ChangeId: token(source, netpb.TokenKind_CHANGE_ID), Basis: token(source, netpb.TokenKind_CHANGE_BASIS)}}}
	if err := validateTokenOwners(poll.ProtoReflect(), entry); err != nil {
		t.Fatal(err)
	}
	poll.Groups[0].Basis.Owner = proto.String(string(entry))
	if validateTokenOwners(poll.ProtoReflect(), entry) == nil {
		t.Fatal("mixed observation owners")
	}
	poll.Groups[0].Basis.Owner = proto.String(string(source))
	poll.NextCursor.Owner = proto.String(string(source))
	if validateTokenOwners(poll.ProtoReflect(), entry) == nil {
		t.Fatal("source cursor disclosed")
	}
}

func TestFederationPublicClosureShape(t *testing.T) {
	for _, raw := range [][]byte{{}, {1}, make([]byte, 32), make([]byte, 33)} {
		result := &pb.QueryRegionResult{Closure: raw}
		if _, err := validatePublicReadOwners(result.ProtoReflect(), fedOwner); err == nil {
			t.Fatal("accepted invalid closure", len(raw))
		}
	}
	raw := make([]byte, 32)
	raw[0] = 1
	for _, result := range []proto.Message{&pb.QueryRegionResult{Closure: raw, SourceGraph: publicTestGraph("query")}, &pb.ListDirectResult{Closure: raw, SourceGraph: publicTestGraph("list")}, &pb.ReadElementResult{Closure: raw, SourceGraph: publicTestGraph("read")}} {
		if _, err := validatePublicReadOwners(result.ProtoReflect(), fedOwner); err != nil {
			t.Fatal(err)
		}
	}
}
