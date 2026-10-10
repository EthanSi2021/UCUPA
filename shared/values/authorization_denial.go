package values

// AuthorizationDenialView is runtime evidence that one exact requirement is
// absent for an authenticated caller. It is deliberately not AuthorizationView:
// validating a denial never authorizes a fact read or a mutation.
type AuthorizationDenialView [32]byte
