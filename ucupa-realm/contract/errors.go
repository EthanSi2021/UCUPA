package contract

import (
	"errors"

	"ucupa/shared/values"
)

var (
	ErrInvalid            = errors.New("invalid Realm input")
	ErrUnauthenticated    = errors.New("Realm authentication rejected")
	ErrDenied             = errors.New("Realm authorization denied")
	ErrNotFound           = errors.New("Realm target not found")
	ErrCredentialConflict = errors.New("Realm credential is already claimed")
	ErrRevisionConflict   = errors.New("Realm revision conflict")
	ErrOperationConflict  = errors.New("Realm operation conflict")
	ErrStore              = errors.New("Realm Store failure")
	ErrInvalidSeed        = errors.New("invalid Realm seed")
)

const (
	PermissionPutBinding          values.PermissionID = "ucupa.realm.binding.put"
	PermissionDeleteBinding       values.PermissionID = "ucupa.realm.binding.delete"
	PermissionRotateOwnCredential values.PermissionID = "ucupa.realm.credential.rotate-own"
	PermissionRevokeCredential    values.PermissionID = "ucupa.realm.credential.revoke"
)
