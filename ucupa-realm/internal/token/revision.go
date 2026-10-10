// Package token owns the private representation of Realm equality tokens.
// It owns no facts, locks, I/O, or lifetime registry.
package token

type revision struct {
	scope      [32]byte
	generation uint64
}

type BindingSetRevision revision
type CredentialSetRevision revision

type value interface {
	BindingSetRevision | CredentialSetRevision
}

func Binding(scope [32]byte, generation uint64) BindingSetRevision {
	return BindingSetRevision{scope: scope, generation: generation}
}
func Credential(scope [32]byte, generation uint64) CredentialSetRevision {
	return CredentialSetRevision{scope: scope, generation: generation}
}
func Scope[T value](r T) [32]byte    { return revision(r).scope }
func ScopeBytes[T value](r T) []byte { s := Scope(r); return s[:] }
func Generation[T value](r T) uint64 { return revision(r).generation }
func Valid[T value](r T) bool        { return Scope(r) != ([32]byte{}) && Generation(r) != 0 }

// Next is called only after the policy's overflow check. Tokens are values;
// advancement does not publish or commit a revision.
func Next[T value](r T) T {
	v := revision(r)
	v.generation++
	return T(v)
}
