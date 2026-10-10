package policy

import (
	"crypto/rand"
	"fmt"
	"sync"

	"ucupa/shared/values"
	contract "ucupa/ucupa-realm/contract"
	token "ucupa/ucupa-realm/internal/token"
)

type storeFailure uint8

const (
	storeFailureNone storeFailure = iota
	storeFailureDefinitePrecommit
	storeFailureIndeterminate
)

// Realm owns one exact permission relation. New constructs its in-memory
// profile; CreateDurable and OpenDurable bind the same semantics to one
// Realm-owned Store.
type Realm struct {
	state *realmState
}

// realmState is the single owner-session state. Realm remains a small public
// handle so an ordinary Go value copy cannot split the mutex, relations,
// revisions, Store lifetime, or retirement gate.
type realmState struct {
	mu                   sync.RWMutex
	id                   values.RealmID
	localPrincipals      map[values.PrincipalID]struct{}
	bindings             map[bindingKey]struct{}
	revision             contract.BindingSetRevision
	operations           map[contract.OperationID]retainedOperation
	credentials          map[contract.CredentialID]credentialRecord
	credentialRevision   contract.CredentialSetRevision
	credentialOperations map[contract.CredentialOperationID]retainedCredentialOperation
	store                *localStore
	retired              bool
	viewKey              [32]byte
	bindingEpochs        map[bindingKey]uint64
}

type normalizedRealm struct {
	id                   values.RealmID
	localPrincipals      map[values.PrincipalID]struct{}
	bindings             map[bindingKey]struct{}
	revision             contract.BindingSetRevision
	operations           map[contract.OperationID]retainedOperation
	credentials          map[contract.CredentialID]credentialRecord
	credentialRevision   contract.CredentialSetRevision
	credentialOperations map[contract.CredentialOperationID]retainedCredentialOperation
}

func New(seed contract.Seed) (*Realm, error) {
	normalized, err := normalizeSeed(seed)
	if err != nil {
		return nil, err
	}
	if err := initializeOwnerState(&normalized); err != nil {
		return nil, err
	}
	return materializeRealm(normalized), nil
}

func normalizeSeed(seed contract.Seed) (normalizedRealm, error) {
	if seed.ID == "" {
		return normalizedRealm{}, contract.ErrInvalidSeed
	}

	value := normalizedRealm{
		id:                   seed.ID,
		localPrincipals:      make(map[values.PrincipalID]struct{}, len(seed.LocalPrincipals)),
		bindings:             make(map[bindingKey]struct{}, len(seed.Bindings)),
		operations:           make(map[contract.OperationID]retainedOperation),
		credentials:          make(map[contract.CredentialID]credentialRecord, len(seed.Credentials)),
		credentialOperations: make(map[contract.CredentialOperationID]retainedCredentialOperation),
	}

	for _, principal := range seed.LocalPrincipals {
		if principal == "" {
			return normalizedRealm{}, contract.ErrInvalidSeed
		}
		value.localPrincipals[principal] = struct{}{}
	}
	for _, binding := range seed.Bindings {
		requirement := values.Requirement{Permission: binding.Permission, Scope: binding.Scope}
		if !binding.Subject.Valid() || !requirement.Valid() {
			return normalizedRealm{}, contract.ErrInvalidSeed
		}
		if binding.Subject.Realm == seed.ID {
			if _, exists := value.localPrincipals[binding.Subject.Principal]; !exists {
				return normalizedRealm{}, contract.ErrInvalidSeed
			}
		}
		if !validSeedBinding(seed.ID, value.localPrincipals, binding) {
			return normalizedRealm{}, contract.ErrInvalidSeed
		}
		value.bindings[bindingKey{subject: binding.Subject, requirement: requirement}] = struct{}{}
	}
	for _, credential := range seed.Credentials {
		if _, registered := value.localPrincipals[credential.Subject]; !registered ||
			!validCredentialEvidenceShape(credential.Evidence) ||
			credential.Evidence.Audience != seed.ID {
			return normalizedRealm{}, contract.ErrInvalidSeed
		}
		if _, claimed := value.credentials[credential.Evidence.ID]; claimed {
			return normalizedRealm{}, contract.ErrInvalidSeed
		}
		value.credentials[credential.Evidence.ID] = credentialRecord{
			principal: credential.Subject,
			active:    true,
			verifier:  credentialVerifier(credential.Evidence),
		}
	}

	return value, nil
}

func validSeedBinding(
	realmID values.RealmID,
	localPrincipals map[values.PrincipalID]struct{},
	binding contract.PermissionBinding,
) bool {
	targetRealm, realmScope := binding.Scope.RealmTarget()
	adminPermission := isAdministrationPermission(binding.Permission)
	if !realmScope && !adminPermission {
		_, _, element := binding.Scope.CoreElementTarget()
		_, _, namespace := binding.Scope.FacetElementTarget()
		_, _, key := binding.Scope.FacetKeyTarget()
		_, _, closure := binding.Scope.AnchorClosureTarget()
		_, authority := binding.Scope.AuthorityTarget()
		return element || namespace || key || closure || authority
	}
	if !realmScope || !adminPermission || targetRealm != realmID || binding.Subject.Realm != realmID {
		return false
	}
	_, registered := localPrincipals[binding.Subject.Principal]
	return registered
}

func initializeOwnerState(value *normalizedRealm) error {
	if value == nil {
		return contract.ErrInvalidSeed
	}
	scope, err := newBindingSetRevisionScope()
	if err != nil {
		return fmt.Errorf("%w: create binding-set revision scope: %w", contract.ErrStore, err)
	}
	value.revision = token.Binding(scope, 1)
	credentialScope, err := newCredentialSetRevisionScope()
	if err != nil {
		return fmt.Errorf("%w: create credential-set revision scope: %w", contract.ErrStore, err)
	}
	value.credentialRevision = token.Credential(credentialScope, 1)
	if value.operations == nil {
		value.operations = make(map[contract.OperationID]retainedOperation)
	}
	if value.credentials == nil {
		value.credentials = make(map[contract.CredentialID]credentialRecord)
	}
	if value.credentialOperations == nil {
		value.credentialOperations = make(map[contract.CredentialOperationID]retainedCredentialOperation)
	}
	return nil
}

func materializeRealm(value normalizedRealm) *Realm {
	result := &Realm{
		state: &realmState{
			id:                   value.id,
			localPrincipals:      value.localPrincipals,
			bindings:             value.bindings,
			revision:             value.revision,
			operations:           value.operations,
			credentials:          value.credentials,
			credentialRevision:   value.credentialRevision,
			credentialOperations: value.credentialOperations,
		},
	}
	result.state.initializeAuthorizationViews()
	return result
}

func newBindingSetRevisionScope() ([32]byte, error) {
	for {
		var scope [32]byte
		if _, err := rand.Read(scope[:]); err != nil {
			return [32]byte{}, err
		}
		if scope != ([32]byte{}) {
			return scope, nil
		}
	}
}

func (realm *Realm) ID() values.RealmID {
	if realm == nil || realm.state == nil {
		return ""
	}
	state := realm.state
	state.mu.RLock()
	defer state.mu.RUnlock()
	return state.id
}

// ActiveID is the local connection's identity and availability check. ID
// remains stable after Close for diagnostics, but a retired owner must never
// be accepted as a live authorization dependency.
func (realm *Realm) ActiveID() (values.RealmID, bool) {
	if realm == nil || realm.state == nil {
		return "", false
	}
	state := realm.state
	state.mu.RLock()
	defer state.mu.RUnlock()
	return state.id, !state.retired
}

// Close retires a durable Realm instance and releases its Store lock. It is a
// no-op for an in-memory or already retired Realm.
func (realm *Realm) Close() error {
	if realm == nil || realm.state == nil {
		return nil
	}
	state := realm.state
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.store == nil || state.retired {
		return nil
	}
	state.retired = true
	return state.store.close()
}

func (state *realmState) retireLocked() {
	if state.retired {
		return
	}
	state.retired = true
	if state.store != nil {
		_ = state.store.close()
	}
}
