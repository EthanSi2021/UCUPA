package session

import (
	"context"

	"ucupa/shared/values"
)

// The transport interceptor is the sole writer of these private context
// values. RPC groups only receive the bounded facts they need; session and
// delegation registries remain owned by the transport server.
type callerKey struct{}
type grantKey struct{}

func WithCaller(ctx context.Context, caller values.CallerContext) context.Context {
	return context.WithValue(ctx, callerKey{}, caller)
}

func Caller(ctx context.Context) (values.CallerContext, bool) {
	caller, ok := ctx.Value(callerKey{}).(values.CallerContext)
	return caller, ok
}

func WithGrant(ctx context.Context, key [32]byte) context.Context {
	return context.WithValue(ctx, grantKey{}, key)
}

func Grant(ctx context.Context) ([32]byte, bool) {
	key, ok := ctx.Value(grantKey{}).([32]byte)
	return key, ok
}
