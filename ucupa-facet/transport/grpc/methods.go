package facetgrpc

import (
	"context"
	facetpb "ucupa/ucupa-facet/bindings/go/protobuf"
)

func (s *Service) Get(ctx context.Context, in *facetpb.GetRequest) (*facetpb.GetResponse, error) {
	out := new(facetpb.GetResponse)
	if err := s.call(ctx, "get", in, out); err != nil {
		return nil, err
	}
	return out, nil
}
func (s *Service) ListTypes(ctx context.Context, in *facetpb.ListTypesRequest) (*facetpb.ListTypesResponse, error) {
	out := new(facetpb.ListTypesResponse)
	if err := s.call(ctx, "list-types", in, out); err != nil {
		return nil, err
	}
	return out, nil
}
func (s *Service) Put(ctx context.Context, in *facetpb.PutRequest) (*facetpb.PutResponse, error) {
	out := new(facetpb.PutResponse)
	if err := s.call(ctx, "put", in, out); err != nil {
		return nil, err
	}
	return out, nil
}
func (s *Service) Delete(ctx context.Context, in *facetpb.DeleteRequest) (*facetpb.DeleteResponse, error) {
	out := new(facetpb.DeleteResponse)
	if err := s.call(ctx, "delete", in, out); err != nil {
		return nil, err
	}
	return out, nil
}
func (s *Service) RecoverOutcome(ctx context.Context, in *facetpb.RecoverOutcomeRequest) (*facetpb.RecoverOutcomeResponse, error) {
	out := new(facetpb.RecoverOutcomeResponse)
	if err := s.call(ctx, "recover-outcome", in, out); err != nil {
		return nil, err
	}
	return out, nil
}
func (s *Service) BeginObserve(ctx context.Context, in *facetpb.BeginObserveRequest) (*facetpb.BeginObserveResponse, error) {
	out := new(facetpb.BeginObserveResponse)
	if err := s.call(ctx, "begin-observe", in, out); err != nil {
		return nil, err
	}
	return out, nil
}
func (s *Service) PollChanges(ctx context.Context, in *facetpb.PollChangesRequest) (*facetpb.PollChangesResponse, error) {
	out := new(facetpb.PollChangesResponse)
	if err := s.call(ctx, "poll", in, out); err != nil {
		return nil, err
	}
	return out, nil
}
func (s *Service) ReleaseObserve(ctx context.Context, in *facetpb.ReleaseObserveRequest) (*facetpb.ReleaseObserveResponse, error) {
	out := new(facetpb.ReleaseObserveResponse)
	if err := s.call(ctx, "release-observe", in, out); err != nil {
		return nil, err
	}
	return out, nil
}
