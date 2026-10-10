package namespace

import (
	"context"
	"errors"
	accessstate "ucupa/ucupa-facet/internal/access"

	"ucupa/shared/values"
	facetcontract "ucupa/ucupa-facet/contract"
	requestvalue "ucupa/ucupa-facet/internal/namespace/request"
)

func (f *Owner) PublicCall(ctx context.Context, caller values.CallerContext, request []byte) ([]byte, error) {
	if f == nil || f.ownerState == nil || f.public == nil {
		return nil, values.PublicUnavailable
	}
	if ctx == nil {
		return nil, values.PublicInvalid
	}
	if ctx.Err() != nil {
		return nil, values.PublicUnavailable
	}
	if !f.public.Enter() {
		return nil, values.PublicResourceExhausted
	}
	defer f.public.Leave()
	defer f.public.DrainRetired(ctx)
	if len(request) > values.PublicJSONLimits().Bytes {
		return nil, values.PublicInvalid
	}
	reserved := uint64(len(request))*8 + (4 << 20)
	f.mu.Lock()
	if err := f.public.ReserveLocked(reserved); err != nil {
		f.mu.Unlock()
		return nil, err
	}
	f.mu.Unlock()
	defer func() { f.mu.Lock(); f.public.ReleaseBytesLocked(reserved); f.mu.Unlock() }()
	value, err := values.ParseCanonicalJSON(request, values.PublicJSONLimits())
	if err != nil {
		return nil, values.PublicInvalid
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, values.PublicInvalid
	}
	operation, _ := object["op"].(string)
	var result map[string]any
	switch operation {
	case "get", "list-types":
		result, err = f.publicRead(ctx, caller, operation, object)
	case "put", "delete", "recover-outcome":
		result, err = f.publicMutation(ctx, caller, operation, object)
	case "begin-observe", "poll", "release-observe":
		result, err = f.publicObserve(ctx, caller, operation, object)
	default:
		err = values.PublicInvalid
	}
	if err != nil {
		return nil, publicFacetError(err)
	}
	return values.CanonicalJSONValue(result)
}

func publicFacetError(err error) error {
	var public values.PublicError
	if errors.As(err, &public) {
		return public
	}
	switch {
	case errors.Is(err, facetcontract.ErrDenied):
		return values.PublicDenied
	case errors.Is(err, facetcontract.ErrInvalid):
		return values.PublicInvalid
	case errors.Is(err, facetcontract.ErrKeyNotFound):
		return values.PublicNotFound
	case errors.Is(err, facetcontract.ErrCapacity):
		return values.PublicResourceExhausted
	case errors.Is(err, facetcontract.ErrRevisionConflict), errors.Is(err, facetcontract.ErrOperationConflict),
		errors.Is(err, facetcontract.ErrProposalNotCurrent), errors.Is(err, facetcontract.ErrSubjectAuthorityConflict), errors.Is(err, facetcontract.ErrContactRevisionConflict):
		return values.PublicConflict
	case errors.Is(err, facetcontract.ErrIndeterminate):
		return values.PublicIndeterminate
	default:
		return values.PublicUnavailable
	}
}

func facetViewError(decision values.AuthorizationDecision, observe bool) error {
	switch decision {
	case values.AuthorizationDenied:
		return values.PublicDenied
	case values.AuthorizationChanged:
		if observe {
			return values.PublicGap
		}
		return values.PublicInvalidContext
	}
	return values.PublicUnavailable
}

func (f *Owner) publicAvailableLocked() error {
	if f == nil || f.ownerState == nil || f.public == nil || f.public.ClosedLocked() {
		return values.PublicUnavailable
	}
	return f.available()
}
func (f *Owner) issuePublicLocked(caller values.CallerContext, kind string, parent *accessstate.Context) (*accessstate.Context, error) {
	if err := f.publicAvailableLocked(); err != nil {
		return nil, err
	}
	return f.public.IssueLocked(caller, kind, parent)
}
func (f *Owner) publicContextLocked(caller values.CallerContext, raw any, kind string) (*accessstate.Context, error) {
	if err := f.publicAvailableLocked(); err != nil {
		return nil, err
	}
	token, err := requestvalue.Token(raw, kind)
	if err != nil || token.Owner != f.authority {
		return nil, values.PublicInvalid
	}
	return f.public.ResolveLocked(caller, token)
}
