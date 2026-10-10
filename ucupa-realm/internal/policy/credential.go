package policy

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"

	"ucupa/shared/values"
	contract "ucupa/ucupa-realm/contract"
	token "ucupa/ucupa-realm/internal/token"
)

const credentialVerifierDomain = "ucupa.realm.online-credential\x00"

var dummyCredentialVerifier = sha256.Sum256([]byte("ucupa.realm.online-credential.dummy"))

type credentialRecord struct {
	principal values.PrincipalID
	verifier  [32]byte
	active    bool
}

type credentialOperationKind uint8

const (
	credentialOperationRotate credentialOperationKind = iota + 1
	credentialOperationRevoke
)

type credentialOperationBinding struct {
	actor               values.PrincipalRef
	kind                credentialOperationKind
	target              contract.CredentialID
	replacement         contract.CredentialID
	replacementVerifier [32]byte
	expected            contract.CredentialSetRevision
}

type credentialOutcomeKind uint8

const (
	credentialOutcomeSuccess credentialOutcomeKind = iota + 1
	credentialOutcomeNotFound
	credentialOutcomeRevisionConflict
	credentialOutcomeConflict
)

type retainedCredentialOutcome struct {
	kind     credentialOutcomeKind
	revision contract.CredentialSetRevision
}

func (outcome retainedCredentialOutcome) result() (contract.CredentialSetRevision, error) {
	switch outcome.kind {
	case credentialOutcomeSuccess:
		return outcome.revision, nil
	case credentialOutcomeNotFound:
		return contract.CredentialSetRevision{}, contract.ErrNotFound
	case credentialOutcomeRevisionConflict:
		return contract.CredentialSetRevision{}, contract.ErrRevisionConflict
	case credentialOutcomeConflict:
		return contract.CredentialSetRevision{}, contract.ErrCredentialConflict
	default:
		return contract.CredentialSetRevision{}, contract.ErrStore
	}
}

type retainedCredentialOperation struct {
	binding credentialOperationBinding
	outcome retainedCredentialOutcome
}

type credentialChange uint8

const (
	credentialUnchanged credentialChange = iota
	credentialRotate
	credentialRevoke
)

type credentialMutationPlan struct {
	change      credentialChange
	target      contract.CredentialID
	replacement contract.CredentialID
	record      credentialRecord
	outcome     retainedCredentialOutcome
}

type credentialMutationState struct {
	id              values.RealmID
	bindingRevision contract.BindingSetRevision
	credentials     map[contract.CredentialID]credentialRecord
	revision        contract.CredentialSetRevision
}

func newCredentialSetRevisionScope() ([32]byte, error) {
	return newBindingSetRevisionScope()
}

func (realm *Realm) Authenticate(evidence contract.CredentialEvidence) (values.PrincipalRef, error) {
	if !validCredentialEvidenceShape(evidence) {
		return values.PrincipalRef{}, contract.ErrInvalid
	}
	if realm == nil || realm.state == nil {
		return values.PrincipalRef{}, contract.ErrStore
	}

	state := realm.state
	state.mu.RLock()
	defer state.mu.RUnlock()
	if state.retired {
		return values.PrincipalRef{}, contract.ErrStore
	}
	actor, ok := state.authenticateLocked(evidence)
	if !ok {
		return values.PrincipalRef{}, contract.ErrUnauthenticated
	}
	return actor, nil
}

func (realm *Realm) CredentialSetRevision() contract.CredentialSetRevision {
	if realm == nil || realm.state == nil {
		return contract.CredentialSetRevision{}
	}
	state := realm.state
	state.mu.RLock()
	defer state.mu.RUnlock()
	if state.retired {
		return contract.CredentialSetRevision{}
	}
	return state.credentialRevision
}

func (realm *Realm) RotateCredential(
	actorProof contract.CredentialEvidence,
	oldID contract.CredentialID,
	replacement contract.CredentialEvidence,
	expectedRevision contract.CredentialSetRevision,
	operationID contract.CredentialOperationID,
) (contract.CredentialSetRevision, error) {
	if !validCredentialEvidenceShape(actorProof) || !validCredentialID(oldID) ||
		!validCredentialEvidenceShape(replacement) || !token.Valid(expectedRevision) ||
		operationID == "" {
		return contract.CredentialSetRevision{}, contract.ErrInvalid
	}
	return realm.mutateCredential(
		actorProof,
		credentialOperationRotate,
		oldID,
		replacement,
		expectedRevision,
		operationID,
	)
}

func (realm *Realm) RevokeCredential(
	actorProof contract.CredentialEvidence,
	targetID contract.CredentialID,
	expectedRevision contract.CredentialSetRevision,
	operationID contract.CredentialOperationID,
) (contract.CredentialSetRevision, error) {
	if !validCredentialEvidenceShape(actorProof) || !validCredentialID(targetID) ||
		!token.Valid(expectedRevision) || operationID == "" {
		return contract.CredentialSetRevision{}, contract.ErrInvalid
	}
	return realm.mutateCredential(
		actorProof,
		credentialOperationRevoke,
		targetID,
		contract.CredentialEvidence{},
		expectedRevision,
		operationID,
	)
}

func (realm *Realm) mutateCredential(
	actorProof contract.CredentialEvidence,
	kind credentialOperationKind,
	target contract.CredentialID,
	replacement contract.CredentialEvidence,
	expected contract.CredentialSetRevision,
	operationID contract.CredentialOperationID,
) (contract.CredentialSetRevision, error) {
	if realm == nil || realm.state == nil {
		return contract.CredentialSetRevision{}, contract.ErrStore
	}

	session := realm.state
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.retired {
		return contract.CredentialSetRevision{}, contract.ErrStore
	}
	actor, authenticated := session.authenticateLocked(actorProof)
	if !authenticated {
		return contract.CredentialSetRevision{}, contract.ErrUnauthenticated
	}
	permission := contract.PermissionRotateOwnCredential
	if kind == credentialOperationRevoke {
		permission = contract.PermissionRevokeCredential
	}
	if !session.managerAuthorizedLocked(actor, permission) {
		return contract.CredentialSetRevision{}, contract.ErrDenied
	}
	if kind == credentialOperationRotate && replacement.Audience != session.id {
		return contract.CredentialSetRevision{}, contract.ErrInvalid
	}

	binding := credentialOperationBinding{
		actor:    actor,
		kind:     kind,
		target:   target,
		expected: expected,
	}
	if kind == credentialOperationRotate {
		binding.replacement = replacement.ID
		binding.replacementVerifier = credentialVerifier(replacement)
	}
	if retained, exists := session.credentialOperations[operationID]; exists {
		if retained.binding != binding {
			return contract.CredentialSetRevision{}, contract.ErrOperationConflict
		}
		return retained.outcome.result()
	}
	if kind == credentialOperationRotate && target != actorProof.ID {
		return contract.CredentialSetRevision{}, contract.ErrDenied
	}
	if kind == credentialOperationRevoke && target == actorProof.ID {
		return contract.CredentialSetRevision{}, contract.ErrInvalid
	}

	state := credentialMutationState{
		id:              session.id,
		bindingRevision: session.revision,
		credentials:     session.credentials,
		revision:        session.credentialRevision,
	}
	var plan credentialMutationPlan
	if session.store == nil {
		var err error
		plan, err = planCredentialMutation(binding, state)
		if err != nil {
			return contract.CredentialSetRevision{}, err
		}
	} else {
		var failure storeFailure
		var err error
		plan, failure, err = session.store.commitCredentialOperation(operationID, binding, state)
		if err != nil {
			if failure != storeFailureDefinitePrecommit {
				session.retireLocked()
			}
			return contract.CredentialSetRevision{}, err
		}
	}

	session.publishCredentialMutationLocked(operationID, binding, plan)
	return plan.outcome.result()
}

func (state *realmState) authenticateLocked(evidence contract.CredentialEvidence) (values.PrincipalRef, bool) {
	stored := dummyCredentialVerifier
	record, claimed := state.credentials[evidence.ID]
	active := claimed && record.active && evidence.Audience == state.id
	if active {
		stored = record.verifier
	}
	presented := credentialVerifier(evidence)
	matched := hmac.Equal(stored[:], presented[:])
	if !active || !matched {
		return values.PrincipalRef{}, false
	}
	return values.PrincipalRef{Realm: state.id, Principal: record.principal}, true
}

func planCredentialMutation(
	binding credentialOperationBinding,
	state credentialMutationState,
) (credentialMutationPlan, error) {
	if binding.expected != state.revision {
		return credentialMutationPlan{
			outcome: retainedCredentialOutcome{kind: credentialOutcomeRevisionConflict},
		}, nil
	}

	switch binding.kind {
	case credentialOperationRotate:
		old, claimed := state.credentials[binding.target]
		if !claimed || !old.active || old.principal != binding.actor.Principal {
			return credentialMutationPlan{}, fmt.Errorf("%w: authenticated rotation source is inconsistent", contract.ErrStore)
		}
		if _, claimed := state.credentials[binding.replacement]; claimed {
			return credentialMutationPlan{
				outcome: retainedCredentialOutcome{kind: credentialOutcomeConflict},
			}, nil
		}
		if token.Generation(state.revision) == math.MaxUint64 {
			return credentialMutationPlan{}, fmt.Errorf("%w: credential-set revision space exhausted", contract.ErrStore)
		}
		revision := state.revision
		revision = token.Next(revision)
		return credentialMutationPlan{
			change:      credentialRotate,
			target:      binding.target,
			replacement: binding.replacement,
			record: credentialRecord{
				principal: old.principal,
				verifier:  binding.replacementVerifier,
				active:    true,
			},
			outcome: retainedCredentialOutcome{
				kind:     credentialOutcomeSuccess,
				revision: revision,
			},
		}, nil
	case credentialOperationRevoke:
		record, claimed := state.credentials[binding.target]
		if !claimed {
			return credentialMutationPlan{
				outcome: retainedCredentialOutcome{kind: credentialOutcomeNotFound},
			}, nil
		}
		if !record.active {
			return credentialMutationPlan{
				outcome: retainedCredentialOutcome{
					kind:     credentialOutcomeSuccess,
					revision: state.revision,
				},
			}, nil
		}
		if token.Generation(state.revision) == math.MaxUint64 {
			return credentialMutationPlan{}, fmt.Errorf("%w: credential-set revision space exhausted", contract.ErrStore)
		}
		revision := state.revision
		revision = token.Next(revision)
		return credentialMutationPlan{
			change: credentialRevoke,
			target: binding.target,
			record: credentialRecord{
				principal: record.principal,
			},
			outcome: retainedCredentialOutcome{
				kind:     credentialOutcomeSuccess,
				revision: revision,
			},
		}, nil
	default:
		return credentialMutationPlan{}, contract.ErrInvalid
	}
}

func (state *realmState) publishCredentialMutationLocked(
	operationID contract.CredentialOperationID,
	binding credentialOperationBinding,
	plan credentialMutationPlan,
) {
	if plan.change != credentialUnchanged {
		next := make(map[contract.CredentialID]credentialRecord, len(state.credentials)+1)
		for id, record := range state.credentials {
			next[id] = record
		}
		switch plan.change {
		case credentialRotate:
			old := next[plan.target]
			next[plan.target] = credentialRecord{principal: old.principal}
			next[plan.replacement] = plan.record
		case credentialRevoke:
			next[plan.target] = plan.record
		}
		state.credentials = next
		state.credentialRevision = plan.outcome.revision
	}
	state.credentialOperations[operationID] = retainedCredentialOperation{
		binding: binding,
		outcome: plan.outcome,
	}
}

func validCredentialEvidenceShape(evidence contract.CredentialEvidence) bool {
	return evidence.Audience != "" &&
		uint64(len(evidence.Audience)) <= uint64(math.MaxUint32) &&
		validCredentialID(evidence.ID) &&
		evidence.Secret != (contract.CredentialSecret{})
}

func validCredentialID(id contract.CredentialID) bool {
	return id != (contract.CredentialID{})
}

func credentialVerifier(evidence contract.CredentialEvidence) [32]byte {
	mac := hmac.New(sha256.New, evidence.Secret[:])
	_, _ = mac.Write([]byte(credentialVerifierDomain))
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(evidence.Audience)))
	_, _ = mac.Write(length[:])
	_, _ = mac.Write([]byte(evidence.Audience))
	_, _ = mac.Write(evidence.ID[:])
	var verifier [32]byte
	copy(verifier[:], mac.Sum(nil))
	return verifier
}
