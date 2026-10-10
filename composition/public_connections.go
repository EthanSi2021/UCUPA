package composition

import (
	"context"
	"ucupa/shared/values"
)

// PublicCall captures only the specified endpoint. No reference lookup, retry
// on another owner, implicit trust bootstrap or interpretation of facts occurs.
func (c *LocalCoreConnections) PublicCall(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef, caller values.CallerContext, request []byte) ([]byte, error) {
	if c == nil || ctx == nil || !address.IsLocal() || !expected.Valid() {
		return nil, values.PublicInvalid
	}
	c.mu.RLock()
	endpoint := c.endpoints[address]
	c.mu.RUnlock()
	peer, port := endpoint.Read, endpoint.Public
	if peer == nil || peer.AuthorityRef() != expected || port == nil || port.AuthorityRef() != expected {
		return nil, values.PublicUnavailable
	}
	raw, err := port.PublicCall(ctx, caller, request)
	if peer.AuthorityRef() != expected || port.AuthorityRef() != expected {
		return nil, values.PublicUnavailable
	}
	return raw, err
}
func (c *LocalFacetConnections) PublicCall(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef, caller values.CallerContext, request []byte) ([]byte, error) {
	if ctx == nil {
		return nil, values.PublicInvalid
	}
	peer, err := c.Connect(address, expected)
	if err != nil {
		return nil, values.PublicUnavailable
	}
	return peer.Namespace.PublicCall(ctx, caller, request)
}
