package facetgrpc

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	facetpb "ucupa/ucupa-facet/bindings/go/protobuf"
	facetcontract "ucupa/ucupa-facet/contract"
)

// maxReplacementEntryBytes bounds one relayed namespace entry on the peer
// interface, matching the Facet content payload limit.
const maxReplacementEntryBytes = 1 << 20

// replacementOwner is the fixed-service replacement peer port. Its methods
// authorize the calling Core's service principal themselves.
type replacementOwner interface {
	CheckNamespaceExport(context.Context, values.CallerContext, values.FacetAcceptanceKey, facetcontract.OperationID) (facetcontract.NamespaceSourceState, error)
	FenceNamespace(context.Context, values.CallerContext, values.FacetAcceptanceKey, facetcontract.OperationID, values.FactAuthorityRef) (facetcontract.NamespaceFenced, error)
	ReadNamespaceSource(context.Context, values.CallerContext, values.FacetAcceptanceKey, facetcontract.OperationID, values.FacetTypeRef) (facetcontract.NamespaceSourceEntry, error)
	SettleNamespace(context.Context, values.CallerContext, values.FacetAcceptanceKey, facetcontract.OperationID, values.FactAuthorityRef) error
	CheckNamespaceImport(context.Context, values.CallerContext, values.FacetAcceptanceKey, facetcontract.OperationID, facetcontract.NamespaceManifest, values.NextHop) (facetcontract.NamespaceImportState, error)
	StageNamespaceEntry(context.Context, values.CallerContext, values.FacetAcceptanceKey, facetcontract.OperationID, values.FacetTypeRef, []byte) error
	AcceptNamespace(context.Context, values.CallerContext, values.FacetAcceptanceKey, facetcontract.OperationID, values.NextHop) (facetcontract.NamespaceAccepted, error)
	InstallNamespace(context.Context, values.CallerContext, values.FacetAcceptanceKey, facetcontract.OperationID, values.FactAuthorityRef, uint64) error
}

func replacementFailure(err error) (networkpb.Failure, error) {
	switch {
	case errors.Is(err, facetcontract.ErrInvalid):
		return networkpb.Failure_INVALID, nil
	case errors.Is(err, facetcontract.ErrDenied):
		return networkpb.Failure_DENIED, nil
	case errors.Is(err, facetcontract.ErrKeyNotFound):
		return networkpb.Failure_NOT_FOUND, nil
	case errors.Is(err, facetcontract.ErrOperationConflict), errors.Is(err, facetcontract.ErrRevisionConflict), errors.Is(err, facetcontract.ErrSnapshotChanged):
		return networkpb.Failure_CONFLICT, nil
	case errors.Is(err, facetcontract.ErrCapacity):
		return networkpb.Failure_RESOURCE_EXHAUSTED, nil
	case errors.Is(err, facetcontract.ErrUnavailable), errors.Is(err, facetcontract.ErrStore):
		return networkpb.Failure_UNAVAILABLE, nil
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return 0, status.FromContextError(err).Err()
	}
	return 0, status.Error(codes.Internal, "replacement result unavailable")
}

// replacementCaller admits only an approved fixed Core service and derives the
// peer key from its bound relationship to this Facet. The operation identifier
// comes from the request message, never from the bound relationship.
func (s *Server) replacementCaller(ctx context.Context, in *facetpb.ServiceAttachmentKey, operationID []byte) (replacementOwner, values.CallerContext, values.FacetAcceptanceKey, facetcontract.OperationID, error) {
	caller, err := s.Caller(ctx)
	core := s.config.CoreServices[caller.Subject]
	if err != nil || !core.Valid() {
		return nil, values.CallerContext{}, values.FacetAcceptanceKey{}, "", status.Error(codes.PermissionDenied, "fixed Core service required")
	}
	key, err := decodeServiceKey(in)
	if err != nil || key.Core != core || key.Facet != s.config.Expected {
		return nil, values.CallerContext{}, values.FacetAcceptanceKey{}, "", status.Error(codes.InvalidArgument, "invalid service relationship")
	}
	op := facetcontract.OperationID(operationID)
	if len(op) == 0 || len(op) > 128 {
		return nil, values.CallerContext{}, values.FacetAcceptanceKey{}, "", status.Error(codes.InvalidArgument, "invalid operation")
	}
	o, ok := s.owner.(replacementOwner)
	if !ok {
		return nil, values.CallerContext{}, values.FacetAcceptanceKey{}, "", status.Error(codes.Unimplemented, "namespace replacement unavailable")
	}
	return o, caller, key, op, nil
}

func (s *Server) CheckSourceExport(ctx context.Context, in *facetpb.ReplacementStepRequest) (*facetpb.CheckSourceExportResponse, error) {
	o, caller, key, op, err := s.replacementCaller(ctx, in.Key, in.GetOperationId())
	if err != nil {
		return nil, err
	}
	state, err := o.CheckNamespaceExport(ctx, caller, key, op)
	if err != nil {
		f, e := replacementFailure(err)
		return &facetpb.CheckSourceExportResponse{Outcome: &facetpb.CheckSourceExportResponse_Failure{Failure: f}}, e
	}
	if state.Key != key {
		return nil, status.Error(codes.Internal, "replacement result unavailable")
	}
	w := &facetpb.SourceExportState{Found: proto.Bool(state.Found), Fenced: proto.Bool(state.Fenced), Refused: proto.Bool(state.Refused), Settled: proto.Bool(state.Settled)}
	if state.Manifest != nil {
		manifest, e := facetcontract.EncodeNamespaceManifest(*state.Manifest)
		if e != nil || len(manifest) == 0 {
			return nil, status.Error(codes.Internal, "replacement result unavailable")
		}
		w.FinalCut = proto.Uint64(state.Manifest.FinalCut)
		w.ManifestDigest = append([]byte(nil), state.Manifest.Digest[:]...)
		w.Manifest = manifest
	}
	return &facetpb.CheckSourceExportResponse{Outcome: &facetpb.CheckSourceExportResponse_Result{Result: w}}, nil
}

func (s *Server) FenceSource(ctx context.Context, in *facetpb.FenceSourceRequest) (*facetpb.FenceSourceResponse, error) {
	o, caller, key, op, err := s.replacementCaller(ctx, in.Key, in.GetOperationId())
	if err != nil {
		return nil, err
	}
	target := values.FactAuthorityRef(in.GetTarget())
	if !target.Valid() {
		return nil, status.Error(codes.InvalidArgument, "invalid replacement target")
	}
	fenced, err := o.FenceNamespace(ctx, caller, key, op, target)
	if err != nil {
		f, e := replacementFailure(err)
		return &facetpb.FenceSourceResponse{Outcome: &facetpb.FenceSourceResponse_Failure{Failure: f}}, e
	}
	if fenced.Key != key {
		return nil, status.Error(codes.Internal, "replacement result unavailable")
	}
	manifest, e := facetcontract.EncodeNamespaceManifest(fenced.Manifest)
	if e != nil || len(manifest) == 0 || fenced.Manifest.Element != key.Element {
		return nil, status.Error(codes.Internal, "replacement result unavailable")
	}
	return &facetpb.FenceSourceResponse{Outcome: &facetpb.FenceSourceResponse_Result{Result: &facetpb.FencedSource{Key: proto.Clone(in.Key).(*facetpb.ServiceAttachmentKey), FinalCut: proto.Uint64(fenced.Manifest.FinalCut), ManifestDigest: append([]byte(nil), fenced.Manifest.Digest[:]...), Manifest: manifest}}}, nil
}

func (s *Server) ReadSourceEntry(ctx context.Context, in *facetpb.ReadSourceEntryRequest) (*facetpb.ReadSourceEntryResponse, error) {
	o, caller, key, op, err := s.replacementCaller(ctx, in.Key, in.GetOperationId())
	if err != nil {
		return nil, err
	}
	typ, err := values.NewFacetTypeRef(in.GetTypeRef())
	if err != nil || !typ.Valid() {
		return nil, status.Error(codes.InvalidArgument, "invalid type reference")
	}
	entry, err := o.ReadNamespaceSource(ctx, caller, key, op, typ)
	if err != nil {
		f, e := replacementFailure(err)
		return &facetpb.ReadSourceEntryResponse{Outcome: &facetpb.ReadSourceEntryResponse_Failure{Failure: f}}, e
	}
	if entry.Type != typ || len(entry.Payload) > maxReplacementEntryBytes {
		return nil, status.Error(codes.Internal, "replacement result unavailable")
	}
	return &facetpb.ReadSourceEntryResponse{Outcome: &facetpb.ReadSourceEntryResponse_Result{Result: &facetpb.SourceEntry{TypeRef: typ.Bytes(), Payload: entry.Payload}}}, nil
}

func (s *Server) SettleSource(ctx context.Context, in *facetpb.SettleSourceRequest) (*facetpb.SettleSourceResponse, error) {
	o, caller, key, op, err := s.replacementCaller(ctx, in.Key, in.GetOperationId())
	if err != nil {
		return nil, err
	}
	target := values.FactAuthorityRef(in.GetTarget())
	if !target.Valid() {
		return nil, status.Error(codes.InvalidArgument, "invalid replacement target")
	}
	if err = o.SettleNamespace(ctx, caller, key, op, target); err != nil {
		f, e := replacementFailure(err)
		return &facetpb.SettleSourceResponse{Outcome: &facetpb.SettleSourceResponse_Failure{Failure: f}}, e
	}
	return &facetpb.SettleSourceResponse{Outcome: &facetpb.SettleSourceResponse_Result{Result: &facetpb.ReplacementStepDone{Done: proto.Bool(true)}}}, nil
}

func (s *Server) CheckTargetImport(ctx context.Context, in *facetpb.CheckTargetImportRequest) (*facetpb.CheckTargetImportResponse, error) {
	o, caller, key, op, err := s.replacementCaller(ctx, in.Key, in.GetOperationId())
	if err != nil {
		return nil, err
	}
	manifest, err := facetcontract.DecodeNamespaceManifest(in.GetManifest())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid namespace manifest")
	}
	address := values.NextHop(in.GetCoreNextHop())
	if !address.Valid() {
		return nil, status.Error(codes.InvalidArgument, "invalid Core address")
	}
	state, err := o.CheckNamespaceImport(ctx, caller, key, op, manifest, address)
	if err != nil {
		f, e := replacementFailure(err)
		return &facetpb.CheckTargetImportResponse{Outcome: &facetpb.CheckTargetImportResponse_Failure{Failure: f}}, e
	}
	if state.Key != key {
		return nil, status.Error(codes.Internal, "replacement result unavailable")
	}
	w := &facetpb.TargetImportState{Accepted: proto.Bool(state.Accepted), Installed: proto.Bool(state.Installed)}
	if !state.Accepted && !state.Installed && state.Digest == ([32]byte{}) {
		return &facetpb.CheckTargetImportResponse{Outcome: &facetpb.CheckTargetImportResponse_Result{Result: w}}, nil
	}
	w.Digest = append([]byte(nil), state.Digest[:]...)
	w.FinalCut = proto.Uint64(state.FinalCut)
	for _, live := range state.Live {
		w.Live = append(w.Live, &facetpb.ReplacementLiveEntry{TypeRef: live.Type.Bytes(), Length: proto.Uint64(live.Length)})
	}
	return &facetpb.CheckTargetImportResponse{Outcome: &facetpb.CheckTargetImportResponse_Result{Result: w}}, nil
}

func (s *Server) StageTargetEntry(ctx context.Context, in *facetpb.StageTargetEntryRequest) (*facetpb.StageTargetEntryResponse, error) {
	o, caller, key, op, err := s.replacementCaller(ctx, in.Key, in.GetOperationId())
	if err != nil {
		return nil, err
	}
	typ, err := values.NewFacetTypeRef(in.GetTypeRef())
	if err != nil || !typ.Valid() || len(in.GetPayload()) > maxReplacementEntryBytes {
		return nil, status.Error(codes.InvalidArgument, "invalid staged entry")
	}
	if err = o.StageNamespaceEntry(ctx, caller, key, op, typ, in.GetPayload()); err != nil {
		f, e := replacementFailure(err)
		return &facetpb.StageTargetEntryResponse{Outcome: &facetpb.StageTargetEntryResponse_Failure{Failure: f}}, e
	}
	return &facetpb.StageTargetEntryResponse{Outcome: &facetpb.StageTargetEntryResponse_Result{Result: &facetpb.ReplacementStepDone{Done: proto.Bool(true)}}}, nil
}

func (s *Server) AcceptTarget(ctx context.Context, in *facetpb.AcceptTargetRequest) (*facetpb.AcceptTargetResponse, error) {
	o, caller, key, op, err := s.replacementCaller(ctx, in.Key, in.GetOperationId())
	if err != nil {
		return nil, err
	}
	address := values.NextHop(in.GetCoreNextHop())
	if !address.Valid() {
		return nil, status.Error(codes.InvalidArgument, "invalid Core address")
	}
	accepted, err := o.AcceptNamespace(ctx, caller, key, op, address)
	if err != nil {
		f, e := replacementFailure(err)
		return &facetpb.AcceptTargetResponse{Outcome: &facetpb.AcceptTargetResponse_Failure{Failure: f}}, e
	}
	if accepted.Key != key {
		return nil, status.Error(codes.Internal, "replacement result unavailable")
	}
	return &facetpb.AcceptTargetResponse{Outcome: &facetpb.AcceptTargetResponse_Result{Result: &facetpb.AcceptedTarget{Key: proto.Clone(in.Key).(*facetpb.ServiceAttachmentKey)}}}, nil
}

func (s *Server) InstallTarget(ctx context.Context, in *facetpb.InstallTargetRequest) (*facetpb.InstallTargetResponse, error) {
	o, caller, key, op, err := s.replacementCaller(ctx, in.Key, in.GetOperationId())
	if err != nil {
		return nil, err
	}
	source := values.FactAuthorityRef(in.GetSource())
	if !source.Valid() || in.GetFinalCut() == 0 {
		return nil, status.Error(codes.InvalidArgument, "invalid replacement installation")
	}
	if err = o.InstallNamespace(ctx, caller, key, op, source, in.GetFinalCut()); err != nil {
		f, e := replacementFailure(err)
		return &facetpb.InstallTargetResponse{Outcome: &facetpb.InstallTargetResponse_Failure{Failure: f}}, e
	}
	return &facetpb.InstallTargetResponse{Outcome: &facetpb.InstallTargetResponse_Result{Result: &facetpb.ReplacementStepDone{Done: proto.Bool(true)}}}, nil
}
