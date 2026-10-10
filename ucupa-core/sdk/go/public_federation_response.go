package coresdk

import (
	"bytes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/reflect/protoreflect"
	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
)

// The authenticated entry attests a federated read. Its context tokens remain
// entry-owned; a disclosed record's revisions remain fact-owner-qualified.
// This exception applies to disclosed read records and observation groups, never mutation outcomes,
// service grants, source-part messages, handles, or continuation tokens.
func validatePublicReadOwners(m protoreflect.Message, entry values.FactAuthorityRef) (bool, error) {
	bad := func() error { return status.Error(codes.DataLoss, "incoherent read source") }
	owners := map[values.FactAuthorityRef]bool{entry: true}
	closure := func(raw []byte) error {
		if raw != nil && (len(raw) != 32 || bytes.Equal(raw, make([]byte, 32))) {
			return bad()
		}
		return nil
	}
	tokens := func(list ...*netpb.Token) error {
		for _, token := range list {
			if token != nil {
				if err := validateTokenOwners(token.ProtoReflect(), entry); err != nil {
					return err
				}
			}
		}
		return nil
	}
	record := func(row *pb.Record) error {
		if row == nil || row.Revision == nil {
			return bad()
		}
		source := values.FactAuthorityRef(row.Revision.GetOwner())
		if !source.Valid() || !owners[source] {
			return bad()
		}
		return validateTokenOwners(row.ProtoReflect(), source)
	}
	switch result := m.Interface().(type) {
	case *pb.PollChangesResult:
		if err := tokens(result.NextCursor); err != nil {
			return true, err
		}
		for _, group := range result.Groups {
			if group == nil || !values.FactAuthorityRef(group.GetOwner()).Valid() {
				return true, bad()
			}
			terminalCategory := false
			for _, category := range group.Category {
				if category == pb.Category_TERMINAL {
					terminalCategory = true
				}
			}
			if terminal := group.GetTerminal(); terminalCategory && (len(group.Category) != 1 || terminal == nil) ||
				!terminalCategory && terminal != nil ||
				terminal != nil && (len(terminal.GetOperationId()) == 0 || len(terminal.GetOperationId()) > 128 ||
					!values.FactAuthorityRef(terminal.GetSource()).Valid() || !values.FactAuthorityRef(terminal.GetTarget()).Valid() ||
					!values.NextHop(terminal.GetNextHop()).Valid()) {
				return true, bad()
			}
			if err := validateTokenOwners(group.ProtoReflect(), values.FactAuthorityRef(group.GetOwner())); err != nil {
				return true, err
			}
		}
		return true, nil
	case *pb.ReadElementResult:
		var err error
		owners, err = validatePublicSourceGraph(result.SourceGraph, result.Closure, entry)
		if err != nil {
			return true, err
		}
		if err := closure(result.Closure); err != nil {
			return true, err
		}
		if err := tokens(result.Receipt, result.Preparation); err != nil {
			return true, err
		}
		if result.Record == nil {
			return true, nil
		}
		if err := record(result.Record); err != nil {
			return true, err
		}
		source := values.FactAuthorityRef(result.Record.Revision.GetOwner())
		if result.Attachment == nil {
			return true, bad()
		}
		if key := result.Attachment.Key; key != nil && key.GetElement() != result.Record.GetElement() {
			return true, bad()
		}
		return true, validateTokenOwners(result.Attachment.ProtoReflect(), source)
	case *pb.ListDirectResult:
		var err error
		owners, err = validatePublicSourceGraph(result.SourceGraph, result.Closure, entry)
		if err != nil || result.SourceGraph != nil && result.SourceGraph.GetOperation() != "list" {
			return true, bad()
		}
		if err := closure(result.Closure); err != nil {
			return true, err
		}
		if err := tokens(result.Receipt, result.Continuation, result.Preparation); err != nil {
			return true, err
		}
		for _, row := range result.Records {
			if err := record(row); err != nil {
				return true, err
			}
		}
		return true, nil
	case *pb.QueryRegionResult:
		var err error
		owners, err = validatePublicSourceGraph(result.SourceGraph, result.Closure, entry)
		if err != nil || result.SourceGraph != nil && result.SourceGraph.GetOperation() != "query" {
			return true, bad()
		}
		if err := validatePublicQueryEvidence(result); err != nil {
			return true, err
		}
		if err := closure(result.Closure); err != nil {
			return true, err
		}
		if err := tokens(result.Receipt, result.Continuation, result.Preparation); err != nil {
			return true, err
		}
		for _, match := range result.Matches {
			if match == nil || match.Revision == nil || !owners[values.FactAuthorityRef(match.Revision.GetOwner())] {
				return true, bad()
			}
			if err := validateTokenOwners(match.ProtoReflect(), values.FactAuthorityRef(match.Revision.GetOwner())); err != nil {
				return true, err
			}
		}
		return true, nil
	}
	return false, nil
}
