package commit

import (
	"encoding/json"
	"errors"
	"fmt"
	"go.etcd.io/bbolt"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	storebytes "ucupa/ucupa-core/internal/store"
	storerecord "ucupa/ucupa-core/internal/store/record"
	"ucupa/ucupa-core/internal/token"
)

type storedMountBinding struct {
	Actor     storerecord.StoredMountPrincipal `json:"actor"`
	PeerActor storerecord.StoredMountPrincipal `json:"peerActor"`
	Command   storerecord.StoredMountCommand   `json:"command"`
}

type storedMountAddress struct {
	Actor    storerecord.StoredMountPrincipal        `json:"actor"`
	Element  string                                  `json:"element"`
	Key      *storerecord.StoredMountKey             `json:"key"`
	Expected *storerecord.StoredMountContactRevision `json:"expected"`
	NextHop  string                                  `json:"nextHop"`
}

type storedMountEvent struct {
	Kind       MountEventKind                   `json:"kind"`
	Before     *storerecord.StoredMountCut      `json:"before"`
	After      *storerecord.StoredMountCut      `json:"after"`
	Operation  []byte                           `json:"operation,omitempty"`
	Binding    *storedMountBinding              `json:"binding,omitempty"`
	Request    *storerecord.StoredMountRequest  `json:"request,omitempty"`
	Prepared   *storerecord.StoredMountPrepared `json:"prepared,omitempty"`
	Closed     *storerecord.StoredMountClosed   `json:"closed,omitempty"`
	Finish     *storerecord.StoredMountEvidence `json:"finish,omitempty"`
	Address    *storedMountAddress              `json:"address,omitempty"`
	Source     string                           `json:"source,omitempty"`
	Authorized bool                             `json:"authorized"`
}

func storeMountEvent(event MountEvent) storedMountEvent {
	stored := storedMountEvent{
		Kind: event.Kind, Before: storerecord.StoreMountCut(event.BeforeCut), After: storerecord.StoreMountCut(event.AfterCut),
		Operation: []byte(event.Operation), Request: storerecord.StoreMountRequest(event.Request),
		Prepared: storerecord.StoreMountPrepared(event.Prepared), Closed: storerecord.StoreMountClosed(event.Closed),
		Finish: storerecord.StoreMountEvidence(event.Finish.Evidence), Source: string(event.Source), Authorized: event.Authorized,
	}
	if event.Binding != (MountBinding{}) {
		stored.Binding = &storedMountBinding{Actor: storerecord.StoreMountPrincipal(event.Binding.Actor), PeerActor: storerecord.StoreMountPrincipal(event.Binding.PeerActor), Command: storerecord.StoreMountCommand(event.Binding.Command)}
	}
	if event.Address != (NextHopBinding{}) {
		stored.Address = &storedMountAddress{Actor: storerecord.StoreMountPrincipal(event.Address.Actor), Element: string(event.Address.Element), Key: storerecord.StoreMountKey(event.Address.Key), Expected: storerecord.StoreMountContactRevision(event.Address.Expected), NextHop: string(event.Address.NextHop)}
	}
	return stored
}

func (stored storedMountEvent) event() (MountEvent, error) {
	event := MountEvent{Kind: stored.Kind, Operation: contract.OperationID(string(stored.Operation)), Source: values.FactAuthorityRef(stored.Source), Authorized: stored.Authorized}
	var err error
	if event.BeforeCut, err = stored.Before.Cut(); err != nil {
		return MountEvent{}, err
	}
	if event.AfterCut, err = stored.After.Cut(); err != nil {
		return MountEvent{}, err
	}
	if stored.Binding != nil {
		event.Binding.Actor = stored.Binding.Actor.DecodePrincipal()
		event.Binding.PeerActor = stored.Binding.PeerActor.DecodePrincipal()
		if event.Binding.Command, err = stored.Binding.Command.Command(); err != nil {
			return MountEvent{}, err
		}
	}
	if event.Request, err = stored.Request.Request(); err != nil {
		return MountEvent{}, err
	}
	if event.Prepared, err = stored.Prepared.DecodePrepared(); err != nil {
		return MountEvent{}, err
	}
	if event.Closed, err = stored.Closed.Closed(); err != nil {
		return MountEvent{}, err
	}
	if event.Finish.Evidence, err = stored.Finish.Evidence(); err != nil {
		return MountEvent{}, err
	}
	if stored.Address != nil {
		event.Address.Actor = stored.Address.Actor.DecodePrincipal()
		event.Address.Element = values.ElementID(stored.Address.Element)
		event.Address.NextHop = values.NextHop(stored.Address.NextHop)
		if event.Address.Key, err = stored.Address.Key.Key(); err != nil {
			return MountEvent{}, err
		}
		if event.Address.Expected, err = stored.Address.Expected.Revision(); err != nil {
			return MountEvent{}, err
		}
	}
	if event.Kind < MountEventReserve || event.Kind > MountEventAddress || !token.Valid(event.BeforeCut) || !token.Valid(event.AfterCut) || token.Scope(event.BeforeCut) != token.Scope(event.AfterCut) {
		return MountEvent{}, errors.New("stored mount event shape is invalid")
	}
	// Decoding may not silently normalize a present-zero subobject into absence.
	// Semantic stage/input validation belongs to the shared transition function.
	encoded, err := json.Marshal(stored)
	if err != nil {
		return MountEvent{}, err
	}
	canonical, err := json.Marshal(storeMountEvent(event))
	if err != nil {
		return MountEvent{}, err
	}
	if string(encoded) != string(canonical) {
		return MountEvent{}, errors.New("stored mount event loses canonical fields")
	}
	return event, nil
}

func writeMountEvents(bucket storeByteSink, events []MountEvent) error {
	for index, event := range events {
		encoded, err := json.Marshal(storeMountEvent(event))
		if err != nil {
			return err
		}
		if err := bucket.Put(storebytes.EncodeUint64(uint64(index)+1), encoded); err != nil {
			return err
		}
	}
	return nil
}

func loadMountEvents(bucket *bbolt.Bucket) ([]MountEvent, error) {
	events := []MountEvent(nil)
	err := bucket.ForEach(func(key, encoded []byte) error {
		index, err := storebytes.DecodeUint64(key)
		if err != nil || index != uint64(len(events))+1 || encoded == nil {
			return errors.New("mount journal key is invalid or discontinuous")
		}
		var stored storedMountEvent
		if err := storebytes.DecodeCanonicalJSON(encoded, &stored); err != nil {
			return fmt.Errorf("mount event %d: %w", index, err)
		}
		event, err := stored.event()
		if err != nil {
			return fmt.Errorf("mount event %d: %w", index, err)
		}
		events = append(events, event)
		return nil
	})
	return events, err
}
