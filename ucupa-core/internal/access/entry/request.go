package entry

import (
	federationcontract "ucupa/ucupa-core/contract/federation"
	accesspkg "ucupa/ucupa-core/internal/access"

	"context"
	"errors"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
)

// PublicCall consumes and produces exact canonical public JSON. Caller evidence
// is established independently; no request field can override it. The local
// transport is explicit and remains separate from this owner operation.
func (core *Operations) PublicCall(ctx context.Context, caller values.CallerContext, request []byte) ([]byte, error) {
	return core.publicCall(ctx, caller, request, nil)
}

// PublicCallWithFederation uses only relationship-selected, authenticated
// connections. Network adapters provide identity, never owner adjudication.
func (core *Operations) PublicCallWithFederation(ctx context.Context, caller values.CallerContext, request []byte, connections federationcontract.FederationConnections) ([]byte, error) {
	if connections == nil {
		return nil, values.PublicInvalid
	}
	return core.publicCall(ctx, caller, request, connections)
}

func (core *Operations) publicCall(ctx context.Context, caller values.CallerContext, request []byte, connections federationcontract.FederationConnections) ([]byte, error) {
	if core == nil || core.access == nil {
		return nil, values.PublicUnavailable
	}
	if ctx == nil {
		return nil, values.PublicInvalid
	}
	select {
	case <-ctx.Done():
		return nil, values.PublicUnavailable
	default:
	}
	if err := core.admission.Acquire(); err != nil {
		return nil, err
	}
	defer core.admission.Release()
	defer core.access.DrainRetired()
	ctx = accesspkg.WithAdmission(ctx, core.admission)
	if len(request) > values.PublicJSONLimits().Bytes {
		return nil, values.PublicInvalid
	}
	// Admission units conservatively cover decoded envelopes and bounded
	// response maps/encoding, in addition to retained contexts and queues.
	reserved := uint64(len(request))*8 + (1 << 20)
	core.owner.Lock()
	reservation, reserveErr := core.access.ReserveLocked(reserved)
	core.owner.Unlock()
	if reserveErr != nil {
		return nil, reserveErr
	}
	defer core.access.Release(reservation)
	parsed, err := values.ParseCanonicalJSON(request, values.PublicJSONLimits())
	if err != nil {
		return nil, values.PublicInvalid
	}
	object, ok := parsed.(map[string]any)
	if !ok {
		return nil, values.PublicInvalid
	}
	operation, ok := object["op"].(string)
	if !ok {
		return nil, values.PublicInvalid
	}
	var result map[string]any
	switch operation {
	case "enter":
		result, err = core.publicEnter(ctx, caller, object, connections != nil)
	case "close":
		result, err = core.publicClose(caller, object)
	case "read", "list", "query":
		if connections != nil {
			result, err = core.federation.Read(ctx, caller, operation, object, connections)
		} else {
			result, err = core.local.Read(ctx, caller, operation, object)
		}
	case "begin-observe", "poll", "release-observe":
		result, err = core.federation.Observe(ctx, caller, operation, object)
	case "create-anchor", "create-entity", "update-anchor", "update-entity", "delete-anchor", "delete-entity", "recover-outcome":
		result, err = core.mutations.Mutate(ctx, caller, operation, object)
	default:
		err = values.PublicInvalid
	}
	if err != nil {
		return nil, publicCoreError(err)
	}
	return values.CanonicalJSONValue(result)
}

func publicCoreError(err error) error {
	var public values.PublicError
	if errors.As(err, &public) {
		return public
	}
	switch {
	case errors.Is(err, contract.ErrDenied):
		return values.PublicDenied
	case errors.Is(err, contract.ErrInvalid):
		return values.PublicInvalid
	case errors.Is(err, contract.ErrNotFound):
		return values.PublicNotFound
	case errors.Is(err, contract.ErrReadCutExpired):
		return values.PublicInvalidContext
	case errors.Is(err, contract.ErrSnapshotCapacity):
		return values.PublicResourceExhausted
	case errors.Is(err, contract.ErrOperationConflict), errors.Is(err, contract.ErrRevisionConflict), errors.Is(err, contract.ErrConflict):
		return values.PublicConflict
	default:
		return values.PublicUnavailable
	}
}
