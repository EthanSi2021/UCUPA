package namespace

import (
	"errors"
	"reflect"

	bolt "go.etcd.io/bbolt"

	"ucupa/shared/values"
	facetcontract "ucupa/ucupa-facet/contract"
)

func (s *localStore) save(owner values.FactAuthorityRef, scope [32]byte, next facetState, events []event) error {
	if s.db == nil {
		return facetcontract.ErrStore
	}
	raw, err := encodeDocument(storeDocument{owner, scope, next, events})
	if err != nil {
		return err
	}
	precommitFailure := false
	err = s.db.Update(func(tx *bolt.Tx) error {
		if err := s.writerGate(tx, owner); err != nil {
			return err
		}
		previous, err := readTransaction(tx)
		if err != nil {
			return err
		}
		if previous.Authority != owner || previous.Scope != scope || previous.Facts.PublicKey != next.PublicKey {
			return facetcontract.ErrStore
		}
		ordinary := len(previous.Events)+1 == len(events) &&
			(len(previous.Events) == 0 || reflect.DeepEqual(previous.Events, events[:len(events)-1]))
		// A contact decision uses this same owner transaction but has its own
		// journal: content cut and namespace events must remain byte-identical.
		contactOnly := len(previous.Events) == len(events) && reflect.DeepEqual(previous.Events, events) &&
			previous.Facts.RealmContact != nil && next.RealmContact != nil &&
			len(previous.Facts.RealmContact.Events)+1 == len(next.RealmContact.Events)
		if contactOnly {
			before, after := previous.Facts, next
			before.RealmContact, after.RealmContact = nil, nil
			contactOnly = reflect.DeepEqual(before, after)
		}
		if !ordinary && !contactOnly {
			return facetcontract.ErrStore
		}
		if err = tx.Bucket(recordBucketName).Put(documentKey, raw); err != nil {
			return err
		}
		if s.failBeforeCommit != nil {
			fail := s.failBeforeCommit
			s.failBeforeCommit = nil
			if err = fail(); err != nil {
				precommitFailure = true
				return err
			}
		}
		return nil
	})
	if err == nil {
		if s.failAfterCommit != nil {
			fail := s.failAfterCommit
			s.failAfterCommit = nil
			if err := fail(); err != nil {
				return errors.Join(facetcontract.ErrIndeterminate, err)
			}
		}
		return nil
	}
	if precommitFailure {
		return errors.Join(facetcontract.ErrUnavailable, err)
	}
	if errors.Is(err, facetcontract.ErrStore) {
		return err
	}
	return errors.Join(facetcontract.ErrIndeterminate, err)
}
