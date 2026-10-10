package namespace

import (
	"crypto/rand"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	accessstate "ucupa/ucupa-facet/internal/access"

	"ucupa/shared/values"
	"ucupa/ucupa-facet/contract"
)

const (
	maxPayloadBytes = 1 << 20
	maxTypes        = 256
	// Namespace replacement keeps every buffer and durable queue bounded.
	maxManifestEntries = contract.MaxNamespaceManifestEntries
	maxPendingInstalls = 8
	maxStagedImports   = 4
	maxStagedBytes     = 512 << 20
	maxPendingBytes    = 256 << 20
)

// Owner is a copyable local handle. Copying it never copies the commit lock,
// maps, cut, journal or access manager: they belong to one private ownerState.
type Owner struct{ *ownerState }

// ownerState has one state, lock, operation namespace and commit sequence.
// Core acquisition runs outside this lock. Authorization always flows through
// the one durable Realm contact of this original Store via the lock-free
// snapshot and its adapter; the bounded trusted-local view validator
// participates in public commit/read ordering and must not reenter an owner.
type ownerState struct {
	mu         sync.Mutex
	authority  values.FactAuthorityRef
	scope      [32]byte
	realmPort  contract.RealmConnections
	contact    atomic.Pointer[contract.RealmAttachment]
	authorizer contract.Authorizer
	cores      contract.CoreSource
	state      facetState
	events     []event
	operations map[byteString]int
	store      *localStore
	public     *accessstate.Manager
	corrupt    bool
	fenced     bool
	// staged holds the in-memory target-side import staging buffers, keyed by
	// core and element. It is guarded by mu, lazily created and never durable:
	// recovery re-stages from the controlling Core.
	staged map[string]*stagedImport
}

// New creates one in-memory Facet owner whose state binds the generated
// identity to the one trusted Realm contact. The contact is never caller
// input; a durable Store records the same binding in its initial document.
func New(port contract.RealmConnections, cores contract.CoreSource, initial contract.RealmInitialization, config *values.PublicAccessPolicy) (*Owner, error) {
	if nilDependency(port) || nilDependency(cores) || !initial.Valid() {
		return nil, contract.ErrInvalid
	}
	limits, err := values.NormalizePublicPolicy(config)
	if err != nil {
		return nil, contract.ErrInvalid
	}
	owner, err := values.NewFactAuthorityRef()
	if err != nil {
		return nil, errors.Join(contract.ErrStore, err)
	}
	var scope [32]byte
	if _, err = rand.Read(scope[:]); err != nil || scope == [32]byte{} {
		return nil, errors.Join(contract.ErrStore, err)
	}
	contact, err := newRealmContact(initial)
	if err != nil {
		return nil, err
	}
	f := assemble(port, cores, owner, scope)
	f.state.RealmContact = &contact
	f.refreshContactLocked()
	if _, err := rand.Read(f.state.PublicKey[:]); err != nil {
		return nil, contract.ErrStore
	}
	f.public = accessstate.New(f.authority, limits, &f.mu)
	return f, nil
}
func assemble(port contract.RealmConnections, cores contract.CoreSource, owner values.FactAuthorityRef, scope [32]byte) *Owner {
	f := &Owner{ownerState: &ownerState{authority: owner, scope: scope, realmPort: port, cores: cores, state: emptyState(), operations: map[byteString]int{}}}
	f.authorizer = &contactAuthorizer{owner: f.ownerState}
	return f
}
func (f *Owner) AuthorityRef() values.FactAuthorityRef {
	if f == nil || f.ownerState == nil {
		return ""
	}
	return f.authority
}

// authorize resolves one typed requirement through the owner's current durable
// Realm contact. An unreachable or unverifiable Realm is never compressed
// into a denial. The returned approval is the contact generation captured
// before the authorization: consuming it at a later final read or commit
// boundary requires the same generation to still be current.
func (f *Owner) authorize(actor values.PrincipalRef, p values.PermissionID, k values.FacetAcceptanceKey, typ values.FacetTypeRef) (contract.RealmAttachment, error) {
	if f == nil || f.ownerState == nil || f.authorizer == nil {
		return contract.RealmAttachment{}, contract.ErrStore
	}
	if !actor.Valid() {
		return contract.RealmAttachment{}, contract.ErrDenied
	}
	approval, ok := f.contactSnapshot()
	if !ok {
		return contract.RealmAttachment{}, contract.ErrUnavailable
	}
	scope := values.FacetElementScope(f.authority, k.Element)
	if typ.Valid() {
		scope = values.FacetKeyScope(f.authority, k.Element, typ)
	}
	switch f.authorizer.AuthorizePrincipal(actor, values.Requirement{Permission: p, Scope: scope}) {
	case values.AuthorizationValid:
		return approval, nil
	case values.AuthorizationDenied:
		return contract.RealmAttachment{}, contract.ErrDenied
	default:
		return contract.RealmAttachment{}, contract.ErrUnavailable
	}
}

// contactSnapshot reads the lock-free current contact generation.
func (f *Owner) contactSnapshot() (contract.RealmAttachment, bool) {
	if f == nil || f.ownerState == nil {
		return contract.RealmAttachment{}, false
	}
	if attachment := f.contact.Load(); attachment != nil && attachment.Revision != (contract.RealmContactRevision{}) {
		return *attachment, true
	}
	return contract.RealmAttachment{}, false
}

// settledContact reports whether an authorization approval's contact
// generation is still the current one at a final read or commit boundary.
func (f *Owner) settledContact(approval contract.RealmAttachment) bool {
	current, ok := f.contactSnapshot()
	return ok && current == approval
}

// refreshContactLocked republishes the lock-free contact snapshot from the
// state installed under the commit lock. A state without a complete contact
// publishes an unusable authorization dependency, never a fallback.
func (f *Owner) refreshContactLocked() {
	if f == nil || f.ownerState == nil {
		return
	}
	if f.state.RealmContact == nil {
		f.contact.Store(nil)
		return
	}
	attachment := f.state.RealmContact.attachment(f.authority, f.scope)
	f.contact.Store(&attachment)
}

func (f *Owner) available() error {
	if f.corrupt || f.fenced || (f.store != nil && f.store.db == nil) {
		return contract.ErrStore
	}
	if f.state.PublicKey == ([32]byte{}) {
		f.corrupt = true
		return contract.ErrStore
	}
	return nil
}
func (f *Owner) validContext(k values.FacetAcceptanceKey) bool {
	return f != nil && f.ownerState != nil && k.Valid() && k.Facet == f.authority
}
func validOperation(id contract.OperationID) bool { return len(id) > 0 && len(id) <= 128 }

func nilDependency(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan, reflect.Interface:
		return v.IsNil()
	}
	return false
}
