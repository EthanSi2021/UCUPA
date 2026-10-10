// Package connected defines the existing trusted-local connected-query ports.
// Request and Reply are opaque in-process envelopes, not wire messages.
package connected

import (
	"context"
	"errors"

	"ucupa/shared/values"
	"ucupa/ucupa-core/internal/query/exchange"
)

var ErrIdentityConflict = errors.New("Core connected sources claim the same Element identity")

type Request = exchange.Request
type Reply = exchange.Reply

type PeerPort interface {
	AuthorityRef() values.FactAuthorityRef
	ReadConnected(context.Context, values.FactAuthorityRef, Request) (Reply, error)
}

type Transport interface {
	ReadConnected(context.Context, values.NextHop, values.FactAuthorityRef, Request) (Reply, error)
}
