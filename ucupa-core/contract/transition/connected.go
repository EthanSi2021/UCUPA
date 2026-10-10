// Package transition defines designated transfer contacts. The
// opaque evidence has no public fields/methods and cannot be minted by callers.
// Adapters use trusted local peers or authenticated network peers; Ref strings
// are not network credentials, and these local values are not wire messages.
package transition

import (
	"context"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/transfer/exchange"
)

type Frozen = exchange.Frozen
type Acceptance = exchange.Acceptance
type Decision = exchange.Decision
type Completion = exchange.Completion

// SourceTransport is used by the target, never by a caller supplying proofs.
type SourceTransport interface {
	ReadTransitionFence(context.Context, values.NextHop, values.FactAuthorityRef, contract.TransitionApprovalBinding) (Frozen, error)
	ReadTransitionDecision(context.Context, values.NextHop, values.FactAuthorityRef, contract.TransitionApprovalBinding) (Decision, error)
}

// TargetTransport fixes the original target caller/view outside the source.
// It must not renew that view during an outstanding authorization decision.
type TargetTransport interface {
	CheckImport(context.Context, values.NextHop, values.FactAuthorityRef, contract.TransitionApprovalBinding) error
	AcceptImport(context.Context, values.NextHop, values.FactAuthorityRef, contract.TransitionApprovalBinding) (Acceptance, error)
	Install(context.Context, values.NextHop, values.FactAuthorityRef, contract.TransitionApprovalBinding) (Completion, error)
}

type SourcePeer interface {
	AuthorityRef() values.FactAuthorityRef
	ReadTransitionFence(context.Context, values.FactAuthorityRef, contract.TransitionApprovalBinding) (Frozen, error)
	ReadTransitionDecision(context.Context, values.FactAuthorityRef, contract.TransitionApprovalBinding) (Decision, error)
}

type TargetPeer interface {
	AuthorityRef() values.FactAuthorityRef
	CheckTransitionImport(context.Context, values.CallerContext, contract.TransitionApprovalBinding, values.AuthorizationView) error
	AcceptTransitionImport(context.Context, values.FactAuthorityRef, values.CallerContext, contract.TransitionApprovalBinding, values.AuthorizationView, SourceTransport) (Acceptance, error)
	InstallTransition(context.Context, values.FactAuthorityRef, contract.TransitionApprovalBinding, SourceTransport) (Completion, error)
}
