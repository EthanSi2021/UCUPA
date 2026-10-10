package contract

import (
	"ucupa/shared/values"
	"ucupa/ucupa-realm/internal/token"
)

// OperationID identifies one exact administration mutation within one Realm
// Store identity or one in-memory Realm lifetime.
type OperationID string

// BindingSetRevision is an opaque equality token; its representation is not public.
type BindingSetRevision = token.BindingSetRevision

type PermissionBinding struct {
	Subject    values.PrincipalRef
	Permission values.PermissionID
	Scope      values.OperationScope
}

type Seed struct {
	ID              values.RealmID
	LocalPrincipals []values.PrincipalID
	Bindings        []PermissionBinding
	Credentials     []CredentialBootstrap
}
