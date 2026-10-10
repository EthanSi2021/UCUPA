package entry

import (
	"context"
	corepb "ucupa/ucupa-core/bindings/go/protobuf"
)

func (s *Service) Enter(ctx context.Context, in *corepb.EnterRequest) (*corepb.EnterResponse, error) {
	out := new(corepb.EnterResponse)
	if err := s.call(ctx, "enter", in, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) Close(ctx context.Context, in *corepb.CloseRequest) (*corepb.CloseResponse, error) {
	out := new(corepb.CloseResponse)
	if err := s.call(ctx, "close", in, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) ReadElement(ctx context.Context, in *corepb.ReadElementRequest) (*corepb.ReadElementResponse, error) {
	out := new(corepb.ReadElementResponse)
	if err := s.call(ctx, "read", in, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) ListDirect(ctx context.Context, in *corepb.ListDirectRequest) (*corepb.ListDirectResponse, error) {
	out := new(corepb.ListDirectResponse)
	if err := s.call(ctx, "list", in, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) QueryRegion(ctx context.Context, in *corepb.QueryRegionRequest) (*corepb.QueryRegionResponse, error) {
	out := new(corepb.QueryRegionResponse)
	if err := s.call(ctx, "query", in, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) BeginObserve(ctx context.Context, in *corepb.BeginObserveRequest) (*corepb.BeginObserveResponse, error) {
	out := new(corepb.BeginObserveResponse)
	if err := s.call(ctx, "begin-observe", in, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) PollChanges(ctx context.Context, in *corepb.PollChangesRequest) (*corepb.PollChangesResponse, error) {
	out := new(corepb.PollChangesResponse)
	if err := s.call(ctx, "poll", in, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) ReleaseObserve(ctx context.Context, in *corepb.ReleaseObserveRequest) (*corepb.ReleaseObserveResponse, error) {
	out := new(corepb.ReleaseObserveResponse)
	if err := s.call(ctx, "release-observe", in, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) CreateAnchor(ctx context.Context, in *corepb.CreateAnchorRequest) (*corepb.CreateAnchorResponse, error) {
	out := new(corepb.CreateAnchorResponse)
	if err := s.call(ctx, "create-anchor", in, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) CreateEntity(ctx context.Context, in *corepb.CreateEntityRequest) (*corepb.CreateEntityResponse, error) {
	out := new(corepb.CreateEntityResponse)
	if err := s.call(ctx, "create-entity", in, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) UpdateAnchor(ctx context.Context, in *corepb.UpdateAnchorRequest) (*corepb.UpdateAnchorResponse, error) {
	out := new(corepb.UpdateAnchorResponse)
	if err := s.call(ctx, "update-anchor", in, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) UpdateEntity(ctx context.Context, in *corepb.UpdateEntityRequest) (*corepb.UpdateEntityResponse, error) {
	out := new(corepb.UpdateEntityResponse)
	if err := s.call(ctx, "update-entity", in, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) DeleteAnchor(ctx context.Context, in *corepb.DeleteAnchorRequest) (*corepb.DeleteAnchorResponse, error) {
	out := new(corepb.DeleteAnchorResponse)
	if err := s.call(ctx, "delete-anchor", in, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) DeleteEntity(ctx context.Context, in *corepb.DeleteEntityRequest) (*corepb.DeleteEntityResponse, error) {
	out := new(corepb.DeleteEntityResponse)
	if err := s.call(ctx, "delete-entity", in, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) RecoverOutcome(ctx context.Context, in *corepb.RecoverOutcomeRequest) (*corepb.RecoverOutcomeResponse, error) {
	out := new(corepb.RecoverOutcomeResponse)
	if err := s.call(ctx, "recover-outcome", in, out); err != nil {
		return nil, err
	}
	return out, nil
}
