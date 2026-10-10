package token

import "testing"

func TestDistinctOpaqueRevisionKindsPreserveParts(t *testing.T) {
	scope := [32]byte{1, 2, 3}
	binding, credential := Binding(scope, 7), Credential(scope, 7)
	if !Valid(binding) || !Valid(credential) || Generation(Next(binding)) != 8 || Generation(Next(credential)) != 8 {
		t.Fatal("revision advance changed")
	}
	if Scope(Next(binding)) != scope || Scope(Next(credential)) != scope {
		t.Fatal("scope changed")
	}
	copy := ScopeBytes(binding)
	copy[0] = 9
	if Scope(binding) != scope {
		t.Fatal("representation escaped by reference")
	}
	if Valid(BindingSetRevision{}) || Valid(CredentialSetRevision{}) || Valid(Binding(scope, 0)) {
		t.Fatal("zero revision accepted")
	}
}
