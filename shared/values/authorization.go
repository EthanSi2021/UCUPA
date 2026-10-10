package values

// CallerContext is evidence from a trusted local establishment boundary, not
// a Principal supplied in a public request. Its issuer must verify Proof; its
// shape alone never authorizes anything. It is not a remote authentication format.
type CallerContext struct {
	Subject       PrincipalRef
	Credential    [16]byte
	Establishment [32]byte
	Proof         [32]byte
}

// AuthorizationView is only exchanged through trusted owner ports, never in
// client envelopes. Related grant or establishment changes invalidate it.
type AuthorizationView [32]byte

type AuthorizationDecision uint8

const (
	AuthorizationUnavailable AuthorizationDecision = iota
	AuthorizationValid
	AuthorizationChanged
	AuthorizationDenied
)
