package commit

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"math"
	"reflect"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	commitbinding "ucupa/ucupa-core/internal/commit/binding"
)

// Contact decisions are retained in the existing Core aggregate. The bound
// is a Store/receipt bound, not a limit on how many times an address may be
// discovered: exhausted history fails closed instead of evicting old IDs.
const maxRealmContactDecisions = 1024
const maxRealmContactBytes = 1 << 20

type realmContactEvent struct {
	actor   values.PrincipalRef
	request contract.RealmContactUpdate
	result  contract.RealmContactResult
}

type realmContactState struct {
	initial contract.RealmInitialization
	current values.NextHop
	serial  uint64
	events  []realmContactEvent
}

func newRealmContact(initial contract.RealmInitialization) (realmContactState, error) {
	if !initial.Valid() {
		return realmContactState{}, contract.ErrConflict
	}
	state := realmContactState{initial: initial, current: initial.NextHop, serial: 1}
	if raw, err := json.Marshal(state.wire()); err != nil || len(raw) > maxRealmContactBytes {
		return realmContactState{}, contract.ErrCapacity
	}
	return state, nil
}

func (state realmContactState) clone() realmContactState {
	state.events = append([]realmContactEvent(nil), state.events...)
	return state
}

func (state realmContactState) attachment(owner values.FactAuthorityRef, scope [32]byte) contract.RealmAttachment {
	return contract.RealmAttachment{
		Owner: owner, Realm: state.initial.Realm, NextHop: state.current,
		Revision: realmContactRevision(owner, state.initial.Realm, scope, state.serial),
	}
}

func realmContactRevision(owner values.FactAuthorityRef, realm values.RealmID, scope [32]byte, serial uint64) contract.RealmContactRevision {
	if !owner.Valid() || realm == "" || scope == ([32]byte{}) || serial == 0 {
		return contract.RealmContactRevision{}
	}
	mac := hmac.New(sha256.New, scope[:])
	_, _ = mac.Write([]byte("ucupa.core.realm-contact-revision\x00"))
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

// prepareRealmContact applies a single already-authorized local decision.
// Returning a nil candidate means an identical retry: no second commit.
// Authorization and writer qualification belong to the caller, not replay.
func (state realmContactState) prepareRealmContact(owner values.FactAuthorityRef, scope [32]byte, actor values.PrincipalRef, request contract.RealmContactUpdate) (*realmContactState, contract.RealmContactResult, error) {
	if !owner.Valid() || scope == ([32]byte{}) || !actor.Valid() || !commitbinding.ValidOperationID(request.OperationID) ||
		request.Owner != owner || request.Realm != state.initial.Realm || !request.NextHop.Valid() ||
		request.Expected == (contract.RealmContactRevision{}) || !state.initial.Valid() || !state.current.Valid() || state.serial == 0 {
		return nil, contract.RealmContactResult{}, contract.ErrConflict
	}
	for _, event := range state.events {
		if event.request.OperationID != request.OperationID {
			continue
		}
		if event.actor != actor || event.request != request {
			return nil, contract.RealmContactResult{}, contract.ErrOperationConflict
		}
		return nil, event.result, nil
	}
	if len(state.events) >= maxRealmContactDecisions {
		return nil, contract.RealmContactResult{}, contract.ErrCapacity
	}
	next := state.clone()
	result := contract.RealmContactResult{OperationID: request.OperationID}
	switch {
	case request.Expected != state.attachment(owner, scope).Revision:
		result.Decision = contract.RealmContactRejected
	case request.NextHop == state.current:
		result.Decision = contract.RealmContactUnchanged
	case state.serial == math.MaxUint64:
		return nil, contract.RealmContactResult{}, contract.ErrCapacity
	default:
		next.current = request.NextHop
		next.serial++
		result.Decision = contract.RealmContactCommitted
	}
	result.Contact = next.attachment(owner, scope)
	next.events = append(next.events, realmContactEvent{actor: actor, request: request, result: result})
	if raw, err := json.Marshal(next.wire()); err != nil || len(raw) > maxRealmContactBytes {
		return nil, contract.RealmContactResult{}, contract.ErrCapacity
	}
	return &next, result, nil
}

// validateRealmContactHistory replays only the original contact decisions.
// It never calls Realm to rejudge historical authorization.
func (state realmContactState) validateRealmContactHistory(owner values.FactAuthorityRef, scope [32]byte) error {
	if !owner.Valid() || scope == ([32]byte{}) || !state.initial.Valid() || !state.current.Valid() ||
		state.serial == 0 || len(state.events) > maxRealmContactDecisions {
		return contract.ErrStore
	}
	replay, err := newRealmContact(state.initial)
	if err != nil {
		return contract.ErrStore
	}
	for _, event := range state.events {
		next, result, err := replay.prepareRealmContact(owner, scope, event.actor, event.request)
		if err != nil || next == nil || result != event.result {
			return contract.ErrStore
		}
		replay = *next
	}
	if !reflect.DeepEqual(replay, state) {
		return contract.ErrStore
	}
	return nil
}

func replayRealmContact(final realmContactState, owner values.FactAuthorityRef, scope [32]byte) (realmContactState, error) {
	if err := final.validateRealmContactHistory(owner, scope); err != nil {
		return realmContactState{}, err
	}
	return final.clone(), nil
}

func sameRealmContact(left, right realmContactState) bool {
	return reflect.DeepEqual(left, right)
}

func (state realmContactState) claimed(id contract.OperationID) bool {
	for _, event := range state.events {
		if event.request.OperationID == id {
			return true
		}
	}
	return false
}

// RealmContactOutcomeLocked returns only a retained original decision. The
// control layer must authorize its caller before invoking this read.
func (owner *Owner) RealmContactOutcomeLocked(id contract.OperationID) (contract.RealmContactResult, bool) {
	if owner == nil || owner.closed || owner.fenced || owner.state == nil ||
		!owner.state.realmContact.initial.Valid() {
		return contract.RealmContactResult{}, false
	}
	for _, event := range owner.state.realmContact.events {
		if event.request.OperationID == id {
			return event.result, true
		}
	}
	return contract.RealmContactResult{}, false
}

// RealmContactLocked reads the contact under the existing aggregate lock. A
// missing contact cannot be synthesized from the assembly configuration.
func (owner *Owner) RealmContactLocked() (contract.RealmAttachment, error) {
	if owner == nil || owner.closed || owner.fenced || owner.state == nil ||
		!owner.state.realmContact.initial.Valid() {
		return contract.RealmAttachment{}, contract.ErrStore
	}
	return owner.state.realmContact.attachment(owner.authority, owner.state.scope), nil
}

// ApplyRealmContactLocked is the original Core Store publication boundary for
// one already-authorized contact decision. The caller holds Owner's write
// lock; this method neither contacts Realm nor issues authorization.
func (owner *Owner) ApplyRealmContactLocked(actor values.PrincipalRef, request contract.RealmContactUpdate) (contract.RealmContactResult, error) {
	if owner == nil || owner.closed || owner.fenced || owner.state == nil ||
		!owner.state.realmContact.initial.Valid() {
		return contract.RealmContactResult{}, contract.ErrStore
	}
	if !owner.state.realmContact.claimed(request.OperationID) {
		if _, used := owner.state.operations[request.OperationID]; used || owner.state.mountOperationClaimed(request.OperationID) {
			return contract.RealmContactResult{}, contract.ErrOperationConflict
		}
	}
	nextContact, result, err := owner.state.realmContact.prepareRealmContact(owner.authority, owner.state.scope, actor, request)
	if err != nil || nextContact == nil {
		return result, err
	}
	next := owner.state.clone()
	next.realmContact = *nextContact
	if err := owner.publishStateLocked(next); err != nil {
		return contract.RealmContactResult{}, err
	}
	return result, nil
}
