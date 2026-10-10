package rpcfailure

import (
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
)

// Control maps the public control error vocabulary to the network failure
// enum. Unknown errors stay transport failures; callers must not invent a
// second mapping table in an RPC group.
func Control(err error) (networkpb.Failure, error) {
	var p values.PublicError
	if errors.As(err, &p) {
		if n, ok := networkpb.Failure_value[string(p)]; ok && n != 0 {
			return networkpb.Failure(n), nil
		}
	}
	return 0, status.Error(codes.Internal, "control result unavailable")
}
