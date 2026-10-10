package rpcfailure

import (
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
)

// MountPeer is the single mapping for control/peer owner errors. Facet
// control uses it for errors outside its attachment-specific vocabulary.
func MountPeer(err error) (networkpb.Failure, error) {
	switch {
	case errors.Is(err, corecontract.ErrInvalid):
		return networkpb.Failure_INVALID, nil
	case errors.Is(err, corecontract.ErrDenied):
		return networkpb.Failure_DENIED, nil
	case errors.Is(err, corecontract.ErrNotFound):
		return networkpb.Failure_NOT_FOUND, nil
	case errors.Is(err, corecontract.ErrConflict), errors.Is(err, corecontract.ErrRevisionConflict), errors.Is(err, corecontract.ErrOperationConflict), errors.Is(err, corecontract.ErrMountBusy):
		return networkpb.Failure_CONFLICT, nil
	case errors.Is(err, corecontract.ErrStore), errors.Is(err, corecontract.ErrUnavailable):
		return networkpb.Failure_UNAVAILABLE, nil
	}
	var p values.PublicError
	if errors.As(err, &p) {
		return Control(err)
	}
	if status.Code(err) != codes.Unknown {
		return 0, err
	}
	return 0, status.Error(codes.Internal, "mount result unavailable")
}
