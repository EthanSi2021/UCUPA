package value

import (
	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
)

// FederationObserveRequest operates only on a preparation captured with the
// source read. Begin has no cursor or limit; Poll requires both. Source release
// remains ReleaseFederationReadPart and releases this stream with its basis.
type FederationObserveRequest struct {
	Operation string
	Cursor    *values.PublicToken
	Limit     uint16
}

// FederationChangeGroup contains invalidation evidence, not copied facts.
// These are source-private tokens; the coordinator must not expose hidden
// source identity or substitute them for its own aggregate resume state.
// Terminal carries the one terminal successor evidence of a provably ended
// old scope; ordinary groups never carry it.
type FederationChangeGroup struct {
	ID         values.PublicToken
	Basis      values.PublicToken
	Categories []string
	Terminal   *corecontract.TransitionTerminal
}

type FederationObservationPage struct {
	Cursor values.PublicToken
	Groups []FederationChangeGroup
}
