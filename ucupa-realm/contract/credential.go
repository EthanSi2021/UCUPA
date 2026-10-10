package contract

import (
	"ucupa/shared/values"
	"ucupa/ucupa-realm/internal/token"
)

// CredentialID is one opaque, permanently claimed credential identity.
type CredentialID [16]byte

// CredentialSecret is caller-held bearer material and is never Realm-owned
// persisted state.
type CredentialSecret [32]byte

// CredentialEvidence is the complete proof presented for one online
// establishment attempt.
type CredentialEvidence struct {
	Audience values.RealmID
	ID       CredentialID
	Secret   CredentialSecret
}

// CredentialBootstrap binds initial evidence to one registered local
// Principal. Realm derives and retains only its verifier.
type CredentialBootstrap struct {
	Subject  values.PrincipalID
	Evidence CredentialEvidence
}

// CredentialOperationID identifies one exact Rotate/Revoke mutation within
// one Realm Store identity or one in-memory Realm lifetime. Its namespace is
// independent from the existing binding-administration OperationID.
type CredentialOperationID string

// CredentialSetRevision is an opaque equality token; its representation is not public.
type CredentialSetRevision = token.CredentialSetRevision
