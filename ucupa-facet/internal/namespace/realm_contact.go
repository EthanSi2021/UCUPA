package namespace

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"math"
	"reflect"

	"ucupa/shared/values"
	"ucupa/ucupa-facet/contract"
)

const maxRealmContactDecisions = 1024
const maxRealmContactBytes = 1 << 20

// These exported fields are the private Facet Store document encoding. They
// live in the original owner document, not in a second contact database.
type realmContactEvent struct {
	Actor   values.PrincipalRef
	Request contract.RealmContactUpdate
	Result  contract.RealmContactResult
}

type realmContactState struct {
	Initial contract.RealmInitialization
	Current values.NextHop
	Serial  uint64
	Events  []realmContactEvent
}

func newRealmContact(initial contract.RealmInitialization) (realmContactState, error) {
	if !initial.Valid() {
		return realmContactState{}, contract.ErrInvalid
	}
	state := realmContactState{Initial: initial, Current: initial.NextHop, Serial: 1}
	if raw, err := json.Marshal(state); err != nil || len(raw) > maxRealmContactBytes {
		return realmContactState{}, contract.ErrCapacity
	}
	return state, nil
}

func (state realmContactState) clone() realmContactState {
	state.Events = append([]realmContactEvent(nil), state.Events...)
	return state
}

func (state realmContactState) attachment(owner values.FactAuthorityRef, scope [32]byte) contract.RealmAttachment {
	return contract.RealmAttachment{Owner: owner, Realm: state.Initial.Realm, NextHop: state.Current,
		Revision: realmContactRevision(owner, state.Initial.Realm, scope, state.Serial)}
}

func realmContactRevision(owner values.FactAuthorityRef, realm values.RealmID, scope [32]byte, serial uint64) contract.RealmContactRevision {
	if !owner.Valid() || realm == "" || scope == ([32]byte{}) || serial == 0 {
		return contract.RealmContactRevision{}
	}
	mac := hmac.New(sha256.New, scope[:])
	_, _ = mac.Write([]byte("ucupa.facet.realm-contact-revision\x00"))
	_, _ = mac.Write([]byte(owner))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(realm))
	var number [8]byte
	binary.BigEndian.PutUint64(number[:], serial)
	_, _ = mac.Write(number[:])
	var revision contract.RealmContactRevision
	copy(revision[:], mac.Sum(nil))
	return revision
}

// prepareRealmContact accepts an already-authorized request. A nil candidate
// means a byte-for-byte retry of an old operation; it never re-executes.
func (state realmContactState) prepareRealmContact(owner values.FactAuthorityRef, scope [32]byte, actor values.PrincipalRef, request contract.RealmContactUpdate) (*realmContactState, contract.RealmContactResult, error) {
	if !owner.Valid() || scope == ([32]byte{}) || !actor.Valid() || !validOperation(request.OperationID) ||
		request.Owner != owner || request.Realm != state.Initial.Realm || !request.NextHop.Valid() ||
		request.Expected == (contract.RealmContactRevision{}) || !state.Initial.Valid() || !state.Current.Valid() || state.Serial == 0 {
		return nil, contract.RealmContactResult{}, contract.ErrInvalid
	}
	for _, event := range state.Events {
		if event.Request.OperationID != request.OperationID {
			continue
		}
		if event.Actor != actor || event.Request != request {
			return nil, contract.RealmContactResult{}, contract.ErrOperationConflict
		}
		return nil, event.Result, nil
	}
	if len(state.Events) >= maxRealmContactDecisions {
		return nil, contract.RealmContactResult{}, contract.ErrCapacity
	}
	next := state.clone()
	result := contract.RealmContactResult{OperationID: request.OperationID}
	switch {
	case request.Expected != state.attachment(owner, scope).Revision:
		result.Decision = contract.RealmContactRejected
	case request.NextHop == state.Current:
		result.Decision = contract.RealmContactUnchanged
	case state.Serial == math.MaxUint64:
		return nil, contract.RealmContactResult{}, contract.ErrCapacity
	default:
		next.Current = request.NextHop
		next.Serial++
		result.Decision = contract.RealmContactCommitted
	}
	result.Contact = next.attachment(owner, scope)
	next.Events = append(next.Events, realmContactEvent{Actor: actor, Request: request, Result: result})
	if raw, err := json.Marshal(next); err != nil || len(raw) > maxRealmContactBytes {
		return nil, contract.RealmContactResult{}, contract.ErrCapacity
	}
	return &next, result, nil
}

// Validation recomputes every retained result without consulting today's
// Realm grants or any network endpoint.
func (state realmContactState) validateRealmContactHistory(owner values.FactAuthorityRef, scope [32]byte) error {
	if !owner.Valid() || scope == ([32]byte{}) || !state.Initial.Valid() || !state.Current.Valid() ||
		state.Serial == 0 || len(state.Events) > maxRealmContactDecisions {
		return contract.ErrStore
	}
	replay, err := newRealmContact(state.Initial)
	if err != nil {
		return contract.ErrStore
	}
	for _, event := range state.Events {
		next, result, err := replay.prepareRealmContact(owner, scope, event.Actor, event.Request)
		if err != nil || next == nil || result != event.Result {
			return contract.ErrStore
		}
		replay = *next
	}
	if !reflect.DeepEqual(replay, state) {
		return contract.ErrStore
	}
	return nil
}

func (f *Owner) RealmContact() (contract.RealmAttachment, error) {
	if f == nil || f.ownerState == nil {
		return contract.RealmAttachment{}, contract.ErrStore
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.realmContactLocked()
}

// CurrentRealmContact discloses the current durable contact in shared value
// shape for trusted local network assembly. It is not a permission check.
func (f *Owner) CurrentRealmContact() (values.RealmID, values.NextHop, error) {
	if f == nil || f.ownerState == nil {
		return "", "", contract.ErrStore
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	contact, err := f.realmContactLocked()
	if err != nil {
		return "", "", err
	}
	return contact.Realm, contact.NextHop, nil
}

func (f *Owner) realmContactLocked() (contract.RealmAttachment, error) {
	if err := f.available(); err != nil || f.state.RealmContact == nil {
		return contract.RealmAttachment{}, contract.ErrStore
	}
	return f.state.RealmContact.attachment(f.authority, f.scope), nil
}

func (f *Owner) realmContactOutcomeLocked(id contract.OperationID) (contract.RealmContactResult, bool) {
	if f.available() != nil || f.state.RealmContact == nil {
		return contract.RealmContactResult{}, false
	}
	for _, event := range f.state.RealmContact.Events {
		if event.Request.OperationID == id {
			return event.Result, true
		}
	}
	return contract.RealmContactResult{}, false
}

// commitRealmContact consumes one already-authorized decision at the original
// Facet owner. It is the owner-transaction seam for the authorized control
// flow and same-package transaction tests; the public Instance surface has no
// unauthenticated commit entry. It does not consult Realm while holding the
// namespace lock.
func (f *Owner) commitRealmContact(actor values.PrincipalRef, request contract.RealmContactUpdate) (contract.RealmContactResult, error) {
	if f == nil || f.ownerState == nil {
		return contract.RealmContactResult{}, contract.ErrStore
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.commitRealmContactLocked(actor, request)
}

func (f *Owner) commitRealmContactLocked(actor values.PrincipalRef, request contract.RealmContactUpdate) (contract.RealmContactResult, error) {
	if err := f.available(); err != nil || f.state.RealmContact == nil {
		return contract.RealmContactResult{}, contract.ErrStore
	}
	if _, used := f.operations[byteString(request.OperationID)]; used {
		return contract.RealmContactResult{}, contract.ErrOperationConflict
	}
	nextContact, result, err := f.state.RealmContact.prepareRealmContact(f.authority, f.scope, actor, request)
	if err != nil || nextContact == nil {
		return result, err
	}
	next := f.state.clone()
	next.RealmContact = nextContact
	if err := validateHistory(f.authority, f.scope, next, f.events); err != nil {
		return contract.RealmContactResult{}, err
	}
	if f.store != nil {
		if err := f.store.save(f.authority, f.scope, next, f.events); err != nil {
			if errors.Is(err, contract.ErrIndeterminate) {
				doc, loadErr := loadStore(f.store.db)
				loadErr = errors.Join(loadErr, f.store.checkWriter(f.authority))
				if loadErr == nil && doc.Authority == f.authority && doc.Scope == f.scope {
					f.install(doc)
				} else {
					_ = f.store.db.Close()
					f.store.db = nil
				}
			}
			return contract.RealmContactResult{}, err
		}
	}
	f.state = next
	f.refreshContactLocked()
	if f.store != nil && f.store.loseResponse != nil {
		fail := f.store.loseResponse
		f.store.loseResponse = nil
		if fail() != nil {
			return contract.RealmContactResult{}, contract.ErrIndeterminate
		}
	}
	return result, nil
}
