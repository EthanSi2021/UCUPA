package fact

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/token"
)

// CarrierBinding is the parent owner's unique decision. The source fence and
// target acceptance are necessary inputs, never substitute decision makers.
// It remains an immutable receipt after contact repair or installation.
type CarrierBinding struct {
	Acceptance CarrierAcceptance
	At         contract.CoreReadCut
	Revision   uint64
}

func (b CarrierBinding) Valid() bool {
	c := b.Acceptance.Fence.Permit
	return b.Acceptance.Valid() && b.Revision != 0 && token.Valid(b.At) &&
		token.Scope(b.At) == token.Scope(c.PermitAt) && token.Serial(b.At) > token.Serial(c.PermitAt)
}

// Installation is the target's original atomic activation, not another
// parent decision. It keeps target-issued cuts distinct from parent cuts.
type CarrierInstallation struct {
	Binding  CarrierBinding
	At       contract.CoreReadCut
	Revision uint64
}

func (i CarrierInstallation) Valid() bool {
	a := i.Binding.Acceptance
	return i.Binding.Valid() && i.Revision > a.Revision && token.Valid(i.At) &&
		token.Scope(i.At) == token.Scope(a.At) && token.Serial(i.At) > token.Serial(a.At)
}

// Settlement acknowledges the original installation in one local owner's
// journal. It is not a new binding, and never authorizes a source to reopen.
type CarrierSettlement struct {
	Installation CarrierInstallation
	At           contract.CoreReadCut
	Revision     uint64
}

func (s CarrierSettlement) Valid(owner values.FactAuthorityRef) bool {
	i := s.Installation.Binding.Acceptance.Fence.Permit.Reservation.Input
	var before contract.CoreReadCut
	switch owner {
	case i.Parent:
		before = s.Installation.Binding.At
	case i.Source:
		before = s.Installation.Binding.Acceptance.Fence.FinalCut
	default:
		return false
	}
	return s.Installation.Valid() && s.Revision != 0 && token.Valid(s.At) &&
		token.Scope(s.At) == token.Scope(before) && token.Serial(s.At) > token.Serial(before)
}
