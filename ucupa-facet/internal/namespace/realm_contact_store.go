package namespace

import (
	"encoding/json"

	"ucupa/shared/values"
	"ucupa/ucupa-facet/contract"
)

// The enclosing Facet document is canonical JSON. These private wire values
// keep opaque identities and operation IDs as exact byte strings, including
// values that are not valid UTF-8.
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

func (state realmContactState) MarshalJSON() ([]byte, error) {
	w := realmContactWire{InitialRealm: []byte(state.Initial.Realm), InitialHop: state.Initial.NextHop,
		CurrentHop: state.Current, Serial: state.Serial}
	for _, event := range state.Events {
		w.Events = append(w.Events, realmContactEventWire{
			ActorRealm: []byte(event.Actor.Realm), ActorPrincipal: []byte(event.Actor.Principal),
			Operation: []byte(event.Request.OperationID), Owner: event.Request.Owner, Realm: []byte(event.Request.Realm),
			Expected: event.Request.Expected, NextHop: event.Request.NextHop, Decision: event.Result.Decision,
			ResultNextHop: event.Result.Contact.NextHop, ResultRevision: event.Result.Contact.Revision,
		})
	}
	return json.Marshal(w)
}

func (state *realmContactState) UnmarshalJSON(raw []byte) error {
	if len(raw) > maxRealmContactBytes {
		return contract.ErrStore
	}
	var w realmContactWire
	if err := json.Unmarshal(raw, &w); err != nil {
		return err
	}
	result := realmContactState{Initial: contract.RealmInitialization{Realm: values.RealmID(w.InitialRealm), NextHop: w.InitialHop},
		Current: w.CurrentHop, Serial: w.Serial}
	for _, event := range w.Events {
		id := contract.OperationID(event.Operation)
		result.Events = append(result.Events, realmContactEvent{
			Actor: values.PrincipalRef{Realm: values.RealmID(event.ActorRealm), Principal: values.PrincipalID(event.ActorPrincipal)},
			Request: contract.RealmContactUpdate{OperationID: id, Owner: event.Owner,
				Realm: values.RealmID(event.Realm), Expected: event.Expected, NextHop: event.NextHop},
			Result: contract.RealmContactResult{OperationID: id, Decision: event.Decision,
				Contact: contract.RealmAttachment{Owner: event.Owner, Realm: values.RealmID(event.Realm),
					NextHop: event.ResultNextHop, Revision: event.ResultRevision}},
		})
	}
	*state = result
	return nil
}
