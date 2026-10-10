package fact

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/token"
)

// FacetReplacementIntent is exact caller input for one element namespace
// replacement. The controlling Core issues it locally: Source is the element's
// current content owner and Target its accepting successor. SourceKey and
// SourceNextHop snapshot the current source relation so later peer steps stay
// recoverable after the commit switch ends that relation. SourceAddress is the
// controlling Core's own service address retained for downstream peer reads.
// The input grants the source no permission to fence and the target no content
// acceptance; both peer relations are established only through their own
// durable evidence.
type FacetReplacementIntent struct {
	Operation     contract.OperationID
	Actor         values.PrincipalRef
	Element       values.ElementID
	Source        values.FactAuthorityRef
	Target        values.FactAuthorityRef
	SourceKey     values.FacetAcceptanceKey
	SourceNextHop values.NextHop
	SourceAddress values.NextHop
	TargetAddress values.NextHop
	Expected      values.AttachmentRevision
}

func (i FacetReplacementIntent) Valid() bool {
	return len(i.Operation) > 0 && len(i.Operation) <= 128 && i.Actor.Valid() && i.Element.Valid() &&
		i.Source.Valid() && i.Target.Valid() && i.Source != i.Target && i.SourceKey.Valid() &&
		i.SourceKey.Facet == i.Source && i.SourceKey.Element == i.Element && i.SourceNextHop.Valid() &&
		i.SourceAddress.Valid() && i.TargetAddress.Valid() && i.Expected.Valid()
}

// FacetReplacementFence binds the source Facet's durable final cut and the
// complete-namespace manifest digest of that exact cut. The manifest bytes
// stay in the control journal; this value alone carries no content.
type FacetReplacementFence struct {
	FinalCut       uint64
	ManifestDigest [32]byte
}

func (f FacetReplacementFence) Valid() bool {
	return f.FinalCut != 0
}

// FacetReplacementRefusal marks the source Facet's durable no-fence
// disposition for this exact operation. The source journal retains the full
// proof; this marker only rebinds the controller's own control facts.
type FacetReplacementRefusal struct {
	Source values.FactAuthorityRef
}

func (r FacetReplacementRefusal) Valid() bool { return r.Source.Valid() }

func (r FacetReplacementRefusal) Matches(c FacetReplacementControl) bool {
	return r.Valid() && r.Source == c.Input.Source
}

// FacetReplacementControl retains the finite current control facts of one
// replacement in the controlling Core. At is the reservation cut and also
// issues the fresh Proposal; every later milestone is a strictly later cut of
// the same scope. Terminal states never free the original operation ID.
type FacetReplacementControl struct {
	Input         FacetReplacementIntent
	At            contract.CoreReadCut
	Proposal      values.AttachmentRevision
	PermitAt      contract.CoreReadCut
	Fence         FacetReplacementFence
	FenceRevision uint64 // immutable fence event, never replaced by later milestones
	FencedAt      contract.CoreReadCut
	AcceptedAt    contract.CoreReadCut
	CommittedAt   contract.CoreReadCut
	InstalledAt   contract.CoreReadCut
	SettledAt     contract.CoreReadCut
	CancelledAt   contract.CoreReadCut
	Refusal       FacetReplacementRefusal
}

// Terminal reports whether the replacement reached one of its permanent
// outcomes. A closed replacement frees its element for new control while its
// operation ID and journal stay retained forever.
func (c FacetReplacementControl) Terminal() bool {
	return c.SettledAt != (contract.CoreReadCut{}) || c.CancelledAt != (contract.CoreReadCut{})
}

func (c FacetReplacementControl) Valid() bool {
	if !c.Input.Valid() || !token.Valid(c.At) || !c.Proposal.Valid() {
		return false
	}
	scope := token.Scope(c.At)
	previous := token.Serial(c.At)
	// Recorded milestones must preserve the chain order and share the
	// reservation scope; an absent milestone excludes every later one.
	open := true
	for _, at := range []contract.CoreReadCut{c.PermitAt, c.FencedAt, c.AcceptedAt, c.CommittedAt, c.InstalledAt, c.SettledAt} {
		if at == (contract.CoreReadCut{}) {
			open = false
			continue
		}
		if !open || !token.Valid(at) || token.Scope(at) != scope || token.Serial(at) <= previous {
			return false
		}
		previous = token.Serial(at)
	}
	if (c.FencedAt == (contract.CoreReadCut{})) != (c.Fence == (FacetReplacementFence{})) || (c.FencedAt == (contract.CoreReadCut{})) != (c.FenceRevision == 0) {
		return false
	}
	if c.FencedAt != (contract.CoreReadCut{}) && !c.Fence.Valid() {
		return false
	}
	// A durable refusal requires a dispatched permit and permanently excludes
	// the fence it refused; it is not itself a terminal outcome.
	if c.Refusal != (FacetReplacementRefusal{}) &&
		(!c.Refusal.Matches(c) || c.PermitAt == (contract.CoreReadCut{}) || c.FencedAt != (contract.CoreReadCut{})) {
		return false
	}
	if c.CancelledAt == (contract.CoreReadCut{}) {
		return true
	}
	if !token.Valid(c.CancelledAt) || token.Scope(c.CancelledAt) != scope || token.Serial(c.CancelledAt) <= token.Serial(c.At) {
		return false
	}
	// Any cancellation precedes the fence and every later milestone.
	if c.FencedAt != (contract.CoreReadCut{}) || c.AcceptedAt != (contract.CoreReadCut{}) ||
		c.CommittedAt != (contract.CoreReadCut{}) || c.InstalledAt != (contract.CoreReadCut{}) || c.SettledAt != (contract.CoreReadCut{}) {
		return false
	}
	if c.PermitAt == (contract.CoreReadCut{}) {
		// The original caller's free window closes when the permit is dispatched.
		return c.Refusal == (FacetReplacementRefusal{}) && token.Serial(c.CancelledAt) > token.Serial(c.At)
	}
	// After the permit only the source's durable refusal reopens cancellation.
	return c.Refusal.Matches(c) && token.Serial(c.CancelledAt) > token.Serial(c.PermitAt)
}
