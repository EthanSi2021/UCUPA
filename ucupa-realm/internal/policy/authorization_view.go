package policy

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"sort"

	"ucupa/shared/values"
	contract "ucupa/ucupa-realm/contract"
	token "ucupa/ucupa-realm/internal/token"
)

// Views are derived from the already-verified administration history. Keeping
// the last related change (including deletion) prevents revoke/regrant ABA.
// The private run key makes all old establishments/views invalid after restart.
func (state *realmState) initializeAuthorizationViews() {
	state.viewKey = freshAuthorizationValue()
	state.bindingEpochs = make(map[bindingKey]uint64)
	for key := range state.bindings {
		state.bindingEpochs[key] = 1
	}
	for _, op := range state.operations {
		if op.outcome.kind != outcomeSuccess || token.Generation(op.outcome.revision) <= token.Generation(op.binding.expected) {
			continue
		}
		key := bindingKey{subject: op.binding.target.Subject, requirement: values.Requirement{Permission: op.binding.target.Permission, Scope: op.binding.target.Scope}}
		if token.Generation(op.outcome.revision) > state.bindingEpochs[key] {
			state.bindingEpochs[key] = token.Generation(op.outcome.revision)
		}
	}
}

// TrustLocalCaller is an explicit trusted-assembly operation. It must never be
// exposed as an unauthenticated public request. Remote callers instead need an
// authenticated transport (outside this local profile).
func (realm *Realm) TrustLocalCaller(subject values.PrincipalRef) (values.CallerContext, error) {
	if realm == nil || realm.state == nil || !subject.Valid() {
		return values.CallerContext{}, contract.ErrInvalid
	}
	s := realm.state
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.retired {
		return values.CallerContext{}, contract.ErrStore
	}
	if subject.Realm == s.id {
		if _, ok := s.localPrincipals[subject.Principal]; !ok {
			return values.CallerContext{}, contract.ErrUnauthenticated
		}
	}
	return s.establishLocked(subject, [16]byte{}), nil
}

// EstablishCaller verifies existing credential evidence, then issues a
// run-local context without retaining the caller-held secret.
func (realm *Realm) EstablishCaller(evidence contract.CredentialEvidence) (values.CallerContext, error) {
	if realm == nil || realm.state == nil || !validCredentialEvidenceShape(evidence) {
		return values.CallerContext{}, contract.ErrUnauthenticated
	}
	s := realm.state
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.retired {
		return values.CallerContext{}, contract.ErrStore
	}
	subject, ok := s.authenticateLocked(evidence)
	if !ok {
		return values.CallerContext{}, contract.ErrUnauthenticated
	}
	return s.establishLocked(subject, [16]byte(evidence.ID)), nil
}

// ForkCaller derives an isolated local establishment from already verified
// online evidence. It preserves credential revocation and grants no permission.
// This trusted composition port never accepts a remote self-declared subject.
// The context carries the original request's cancellation; this local kernel
// path is bounded in-process work and does not wait on it.
func (realm *Realm) ForkCaller(ctx context.Context, caller values.CallerContext) (values.CallerContext, error) {
	_ = ctx
	if realm == nil || realm.state == nil {
		return values.CallerContext{}, contract.ErrStore
	}
	s := realm.state
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.retired {
		return values.CallerContext{}, contract.ErrStore
	}
	if caller.Credential == ([16]byte{}) || !s.validCallerLocked(caller) {
		return values.CallerContext{}, contract.ErrUnauthenticated
	}
	return s.establishLocked(caller.Subject, caller.Credential), nil
}

func (state *realmState) establishLocked(subject values.PrincipalRef, credential [16]byte) values.CallerContext {
	caller := values.CallerContext{Subject: subject, Credential: credential, Establishment: freshAuthorizationValue()}
	caller.Proof = state.callerProof(caller)
	return caller
}

func freshAuthorizationValue() [32]byte {
	var value [32]byte
	// Go's crypto/rand.Read fills the entire slice or terminates on entropy failure.
	_, _ = rand.Read(value[:])
	return value
}

func (state *realmState) callerProof(caller values.CallerContext) [32]byte {
	mac := hmac.New(sha256.New, state.viewKey[:])
	mac.Write([]byte("ucupa.realm.caller\x00"))
	for _, field := range []string{string(state.id), string(caller.Subject.Realm), string(caller.Subject.Principal)} {
		var n [8]byte
		binary.BigEndian.PutUint64(n[:], uint64(len(field)))
		mac.Write(n[:])
		mac.Write([]byte(field))
	}
	mac.Write(caller.Credential[:])
	mac.Write(caller.Establishment[:])
	return [32]byte(mac.Sum(nil))
}

func (state *realmState) validCallerLocked(caller values.CallerContext) bool {
	if !caller.Subject.Valid() || caller.Establishment == ([32]byte{}) {
		return false
	}
	proof := state.callerProof(caller)
	if !hmac.Equal(proof[:], caller.Proof[:]) {
		return false
	}
	if caller.Credential != ([16]byte{}) {
		record, ok := state.credentials[contract.CredentialID(caller.Credential)]
		return ok && record.active && caller.Subject.Realm == state.id && record.principal == caller.Subject.Principal
	}
	if caller.Subject.Realm == state.id {
		_, ok := state.localPrincipals[caller.Subject.Principal]
		return ok
	}
	return true
}

// AuthorizeView checks the entire deduplicated requirement set in one Realm
// read section. No bool-authorizer fallback can manufacture a view.
func (realm *Realm) AuthorizeView(caller values.CallerContext, requirements ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	if realm == nil || realm.state == nil {
		return values.AuthorizationUnavailable, values.AuthorizationView{}
	}
	s := realm.state
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.authorizationViewLocked(caller, requirements)
}

func (realm *Realm) ValidateView(caller values.CallerContext, view values.AuthorizationView, requirements ...values.Requirement) values.AuthorizationDecision {
	decision, current := realm.AuthorizeView(caller, requirements...)
	if decision != values.AuthorizationValid {
		return decision
	}
	if !hmac.Equal(view[:], current[:]) {
		return values.AuthorizationChanged
	}
	return values.AuthorizationValid
}

func (state *realmState) authorizationViewLocked(caller values.CallerContext, requirements []values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	if state.retired {
		return values.AuthorizationUnavailable, values.AuthorizationView{}
	}
	if !state.validCallerLocked(caller) || len(requirements) == 0 {
		return values.AuthorizationDenied, values.AuthorizationView{}
	}
	items := make([][]byte, 0, len(requirements))
	seen := make(map[values.Requirement]bool)
	for _, requirement := range requirements {
		if !requirement.Valid() {
			return values.AuthorizationDenied, values.AuthorizationView{}
		}
		if seen[requirement] {
			continue
		}
		seen[requirement] = true
		key := bindingKey{subject: caller.Subject, requirement: requirement}
		if _, ok := state.bindings[key]; !ok {
			return values.AuthorizationDenied, values.AuthorizationView{}
		}
		encoded, err := encodePermissionBinding(contract.PermissionBinding{Subject: caller.Subject, Permission: requirement.Permission, Scope: requirement.Scope})
		if err != nil {
			return values.AuthorizationUnavailable, values.AuthorizationView{}
		}
		encoded = binary.BigEndian.AppendUint64(encoded, state.bindingEpochs[key])
		items = append(items, encoded)
	}
	sort.Slice(items, func(i, j int) bool { return bytes.Compare(items[i], items[j]) < 0 })
	mac := hmac.New(sha256.New, state.viewKey[:])
	mac.Write([]byte("ucupa.realm.authorization-view\x00"))
	mac.Write(caller.Proof[:])
	for _, item := range items {
		var n [8]byte
		binary.BigEndian.PutUint64(n[:], uint64(len(item)))
		mac.Write(n[:])
		mac.Write(item)
	}
	return values.AuthorizationValid, values.AuthorizationView(mac.Sum(nil))
}
