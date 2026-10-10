package commit

import (
	"encoding/json"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	storebytes "ucupa/ucupa-core/internal/store"
)

// Opaque Realm, principal and OperationID bytes must not pass through JSON
// strings: encoding/json replaces invalid UTF-8 in string values.
type realmContactEventWire struct {
	ActorRealm     []byte
	ActorPrincipal []byte
	Operation      []byte
	Owner          values.FactAuthorityRef
	Realm          []byte
	Expected       contract.RealmContactRevision
	NextHop        values.NextHop
	Decision       contract.RealmContactDecision
	ResultNextHop  values.NextHop
	ResultRevision contract.RealmContactRevision
}

type realmContactWire struct {
	InitialRealm []byte
	InitialHop   values.NextHop
	CurrentHop   values.NextHop
	Serial       uint64
	Events       []realmContactEventWire
}

func (state realmContactState) wire() realmContactWire {
	w := realmContactWire{InitialRealm: []byte(state.initial.Realm), InitialHop: state.initial.NextHop,
		CurrentHop: state.current, Serial: state.serial}
	for _, event := range state.events {
		w.Events = append(w.Events, realmContactEventWire{
			ActorRealm: []byte(event.actor.Realm), ActorPrincipal: []byte(event.actor.Principal),
			Operation: []byte(event.request.OperationID), Owner: event.request.Owner, Realm: []byte(event.request.Realm),
			Expected: event.request.Expected, NextHop: event.request.NextHop, Decision: event.result.Decision,
			ResultNextHop: event.result.Contact.NextHop, ResultRevision: event.result.Contact.Revision,
		})
	}
	return w
}

func realmContactFromWire(w realmContactWire) realmContactState {
	state := realmContactState{initial: contract.RealmInitialization{Realm: values.RealmID(w.InitialRealm), NextHop: w.InitialHop},
		current: w.CurrentHop, serial: w.Serial}
	for _, event := range w.Events {
		id := contract.OperationID(event.Operation)
		state.events = append(state.events, realmContactEvent{
			actor: values.PrincipalRef{Realm: values.RealmID(event.ActorRealm), Principal: values.PrincipalID(event.ActorPrincipal)},
			request: contract.RealmContactUpdate{OperationID: id, Owner: event.Owner,
				Realm: values.RealmID(event.Realm), Expected: event.Expected, NextHop: event.NextHop},
			result: contract.RealmContactResult{OperationID: id, Decision: event.Decision,
				Contact: contract.RealmAttachment{Owner: event.Owner, Realm: values.RealmID(event.Realm),
					NextHop: event.ResultNextHop, Revision: event.ResultRevision}},
		})
	}
	return state
}

func encodeRealmContact(state realmContactState, owner values.FactAuthorityRef, scope [32]byte) ([]byte, error) {
	if err := state.validateRealmContactHistory(owner, scope); err != nil {
		return nil, err
	}
	return json.Marshal(state.wire())
}

func decodeRealmContact(encoded []byte, owner values.FactAuthorityRef, scope [32]byte) (realmContactState, error) {
	if len(encoded) > maxRealmContactBytes {
		return realmContactState{}, contract.ErrStore
	}
	var wire realmContactWire
	if err := storebytes.DecodeCanonicalJSON(encoded, &wire); err != nil {
		return realmContactState{}, contract.ErrStore
	}
	state := realmContactFromWire(wire)
	if err := state.validateRealmContactHistory(owner, scope); err != nil {
		return realmContactState{}, err
	}
	return state, nil
}
