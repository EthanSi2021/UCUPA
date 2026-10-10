package facetgrpc

import (
	"context"
	"encoding/binary"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"ucupa/shared/values"
	pb "ucupa/ucupa-facet/bindings/go/protobuf"
	facetcontract "ucupa/ucupa-facet/contract"
)

type acceptanceControlOwner interface {
	ReadAcceptanceControl(context.Context, values.CallerContext, values.FacetAcceptanceKey) (facetcontract.AcceptanceRead, error)
}

type dispositionControlOwner interface {
	ReadDispositionControl(context.Context, values.CallerContext, values.ElementID) (facetcontract.DispositionRead, error)
}

func decodeServiceKey(w *pb.ServiceAttachmentKey) (values.FacetAcceptanceKey, error) {
	if w == nil || w.Proposal == nil || len(w.Proposal.Incarnation) != 32 || w.Proposal.GetSequence() == 0 || !values.ElementID(w.GetElement()).Valid() {
		return values.FacetAcceptanceKey{}, status.Error(codes.InvalidArgument, "invalid service relationship")
	}
	var raw [76]byte
	copy(raw[:32], w.Proposal.Incarnation)
	copy(raw[32:68], w.GetElement())
	binary.BigEndian.PutUint64(raw[68:], w.Proposal.GetSequence())
	r, err := values.NewAttachmentRevision(raw[:])
	k := values.FacetAcceptanceKey{Core: values.FactAuthorityRef(w.GetCore()), Facet: values.FactAuthorityRef(w.GetFacet()), Element: values.ElementID(w.GetElement()), Proposal: r}
	if err != nil || !k.Valid() {
		return values.FacetAcceptanceKey{}, status.Error(codes.InvalidArgument, "invalid service relationship")
	}
	return k, nil
}

func encodeFacetCut(c values.FacetReadCut) *pb.ServiceFacetCut {
	if !c.Valid() {
		return nil
	}
	b := c.Bytes()
	return &pb.ServiceFacetCut{Incarnation: b[:32], Sequence: proto.Uint64(binary.BigEndian.Uint64(b[32:]))}
}

func encodeServiceKey(k values.FacetAcceptanceKey) *pb.ServiceAttachmentKey {
	if !k.Valid() {
		return nil
	}
	b := k.Proposal.Bytes()
	return &pb.ServiceAttachmentKey{Core: proto.String(string(k.Core)), Facet: proto.String(string(k.Facet)), Element: proto.String(string(k.Element)), Proposal: &pb.ServiceRevision{Incarnation: b[:32], Sequence: proto.Uint64(binary.BigEndian.Uint64(b[68:]))}}
}

// ReadAcceptance has a separate fixed-service audience. Its session proves a
// Realm caller; configured binding limits which Core's keys it may request.
// The owner still checks its acceptance-read permission for the exact Element.
func (s *Server) ReadAcceptance(ctx context.Context, in *pb.ReadAcceptanceRequest) (*pb.ReadAcceptanceResponse, error) {
	caller, err := s.Caller(ctx)
	core := s.config.CoreServices[caller.Subject]
	if err != nil || !core.Valid() {
		return nil, status.Error(codes.PermissionDenied, "fixed Core service required")
	}
	key, err := decodeServiceKey(in.GetKey())
	if err != nil || key.Core != core || key.Facet != s.config.Expected {
		return nil, status.Error(codes.InvalidArgument, "invalid service relationship")
	}
	o, ok := s.owner.(acceptanceControlOwner)
	if !ok {
		return nil, status.Error(codes.Unimplemented, "acceptance control unavailable")
	}
	r, err := o.ReadAcceptanceControl(ctx, caller, key)
	if err != nil {
		f, e := relationshipFailure(err)
		return &pb.ReadAcceptanceResponse{Outcome: &pb.ReadAcceptanceResponse_Failure{Failure: f}}, e
	}
	if !r.Cut.Valid() || (r.Found && (r.Acceptance.Key != key || !r.Acceptance.AcceptedAt.Valid())) {
		return nil, status.Error(codes.Internal, "acceptance result unavailable")
	}
	w := &pb.ServiceAcceptanceRead{Key: proto.Clone(in.Key).(*pb.ServiceAttachmentKey), Found: proto.Bool(r.Found), Cut: encodeFacetCut(r.Cut)}
	if r.Found {
		w.AcceptedAt = encodeFacetCut(r.Acceptance.AcceptedAt)
	}
	return &pb.ReadAcceptanceResponse{Outcome: &pb.ReadAcceptanceResponse_Result{Result: w}}, nil
}

// ReadDisposition shares the fixed-Core-service audience of ReadAcceptance. It
// discloses one element's durable final disposition; the owner still checks its
// acceptance-read permission for the exact Element.
func (s *Server) ReadDisposition(ctx context.Context, in *pb.ReadDispositionRequest) (*pb.ReadDispositionResponse, error) {
	caller, err := s.Caller(ctx)
	core := s.config.CoreServices[caller.Subject]
	if err != nil || !core.Valid() {
		return nil, status.Error(codes.PermissionDenied, "fixed Core service required")
	}
	element := values.ElementID(in.GetElement())
	if !element.Valid() {
		return nil, status.Error(codes.InvalidArgument, "invalid element")
	}
	o, ok := s.owner.(dispositionControlOwner)
	if !ok {
		return nil, status.Error(codes.Unimplemented, "disposition control unavailable")
	}
	r, err := o.ReadDispositionControl(ctx, caller, element)
	if err != nil {
		f, e := relationshipFailure(err)
		return &pb.ReadDispositionResponse{Outcome: &pb.ReadDispositionResponse_Failure{Failure: f}}, e
	}
	if r.Found {
		if !r.Key.Valid() || r.Key.Core != core || r.Key.Facet != s.config.Expected || r.Key.Element != element || !r.Cut.Valid() {
			return nil, status.Error(codes.Internal, "disposition result unavailable")
		}
	} else if r.Key.Valid() || r.Cut.Valid() {
		return nil, status.Error(codes.Internal, "disposition result unavailable")
	}
	w := &pb.ServiceDispositionRead{Found: proto.Bool(r.Found)}
	if r.Found {
		w.Key = encodeServiceKey(r.Key)
		w.Cut = encodeFacetCut(r.Cut)
	}
	return &pb.ReadDispositionResponse{Outcome: &pb.ReadDispositionResponse_Result{Result: w}}, nil
}
