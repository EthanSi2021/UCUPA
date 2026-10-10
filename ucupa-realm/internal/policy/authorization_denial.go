package policy

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"

	"ucupa/shared/values"
	contract "ucupa/ucupa-realm/contract"
)

// CaptureDenialView is a trusted authorization-owner port for proving a closed
// undisclosable branch. It does not enumerate bindings or grant an operation.
// Only DENIED together with a nonzero view is a captured denial. Invalid caller
// evidence produces no view, and an existing grant returns VALID with no view.
func (realm *Realm) CaptureDenialView(caller values.CallerContext, requirement values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	if realm == nil || realm.state == nil {
		return values.AuthorizationUnavailable, values.AuthorizationDenialView{}
	}
	s := realm.state
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.retired {
		return values.AuthorizationUnavailable, values.AuthorizationDenialView{}
	}
	if !s.validCallerLocked(caller) || !requirement.Valid() {
		return values.AuthorizationDenied, values.AuthorizationDenialView{}
	}
	key := bindingKey{subject: caller.Subject, requirement: requirement}
	if _, exists := s.bindings[key]; exists {
		return values.AuthorizationValid, values.AuthorizationDenialView{}
	}
	encoded, err := encodePermissionBinding(contract.PermissionBinding{Subject: caller.Subject, Permission: requirement.Permission, Scope: requirement.Scope})
	if err != nil {
		return values.AuthorizationUnavailable, values.AuthorizationDenialView{}
	}
	mac := hmac.New(sha256.New, s.viewKey[:])
	mac.Write([]byte("ucupa.realm.authorization-denial\x00"))
	mac.Write(caller.Proof[:])
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], uint64(len(encoded)))
	mac.Write(n[:])
	mac.Write(encoded)
	binary.BigEndian.PutUint64(n[:], s.bindingEpochs[key])
	mac.Write(n[:])
	return values.AuthorizationDenied, values.AuthorizationDenialView(mac.Sum(nil))
}

// ValidateDenialView checks the original absence, including grant/revoke ABA.
// VALID means the denial is still valid, never that the requirement is allowed.
func (realm *Realm) ValidateDenialView(caller values.CallerContext, view values.AuthorizationDenialView, requirement values.Requirement) values.AuthorizationDecision {
	if view == (values.AuthorizationDenialView{}) {
		return values.AuthorizationDenied
	}
	decision, current := realm.CaptureDenialView(caller, requirement)
	if decision == values.AuthorizationUnavailable {
		return decision
	}
	if decision == values.AuthorizationValid {
		return values.AuthorizationChanged
	}
	if current == (values.AuthorizationDenialView{}) {
		return values.AuthorizationDenied
	}
	if !hmac.Equal(view[:], current[:]) {
		return values.AuthorizationChanged
	}
	return values.AuthorizationValid
}
