package rpcfailure

import (
	"errors"
	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/ucupa-core/contract"
)

// A Store error after a dispatched control write cannot prove non-commit.
// Inspect is read-only; its failure does not make a statement about a write.
func Transition(err error, mutation bool) (netpb.Failure, error) {
	switch {
	case errors.Is(err, contract.ErrStore):
		if mutation {
			return netpb.Failure_INDETERMINATE, nil
		}
		return netpb.Failure_UNAVAILABLE, nil
	case errors.Is(err, contract.ErrInvalid):
		return netpb.Failure_INVALID, nil
	case errors.Is(err, contract.ErrDenied):
		return netpb.Failure_DENIED, nil
	case errors.Is(err, contract.ErrNotFound):
		return netpb.Failure_NOT_FOUND, nil
	case errors.Is(err, contract.ErrConflict), errors.Is(err, contract.ErrRevisionConflict), errors.Is(err, contract.ErrOperationConflict), errors.Is(err, contract.ErrMountBusy), errors.Is(err, contract.ErrAttachmentStateConflict):
		return netpb.Failure_CONFLICT, nil
	case errors.Is(err, contract.ErrUnavailable), errors.Is(err, contract.ErrMountCoordinationRequired):
		return netpb.Failure_UNAVAILABLE, nil
	}
	return Control(err)
}
