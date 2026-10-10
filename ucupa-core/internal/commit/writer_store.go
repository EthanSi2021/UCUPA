package commit

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"

	"go.etcd.io/bbolt"
	"ucupa/shared/values"
)

var writerJournalKey = []byte{8}

// writerCut covers facts, tombstones, retained unchanged outcomes and control
// history. Writer control itself is excluded: handoff does not mutate data cut.
func writerCut(tx *bbolt.Tx) ([32]byte, error) {
	h := sha256.New()
	field := func(v []byte) {
		var n [8]byte
		binary.BigEndian.PutUint64(n[:], uint64(len(v)))
		h.Write(n[:])
		h.Write(v)
	}
	for _, name := range [][]byte{elementBucketName, operationBucketName, commitBucketName, mountEventBucketName} {
		b := tx.Bucket(name)
		if b == nil {
			return [32]byte{}, values.PublicUnavailable
		}
		field(name)
		if err := b.ForEach(func(k, v []byte) error {
			if bytes.Equal(name, elementBucketName) && bytes.Equal(k, writerJournalKey) {
				return nil
			}
			if v == nil {
				return values.PublicUnavailable
			}
			field(k)
			field(v)
			return nil
		}); err != nil {
			return [32]byte{}, err
		}
	}
	var result [32]byte
	copy(result[:], h.Sum(nil))
	return result, nil
}
func readWriter(tx *bbolt.Tx, owner values.FactAuthorityRef) (writerJournal, error) {
	b := tx.Bucket(elementBucketName)
	if b == nil {
		return writerJournal{}, values.PublicUnavailable
	}
	j, err := decodeWriter(b.Get(writerJournalKey), owner)
	if err != nil {
		return j, err
	}
	if j.pending() {
		cut, e := writerCut(tx)
		if e != nil || cut != j.Records[len(j.Records)-1].FinalCut {
			return j, values.PublicUnavailable
		}
	}
	return j, nil
}
func loadWriter(db *bbolt.DB, owner values.FactAuthorityRef) (writerJournal, error) {
	var j writerJournal
	err := db.View(func(tx *bbolt.Tx) error { var e error; j, e = readWriter(tx, owner); return e })
	return j, err
}
func putWriter(tx *bbolt.Tx, j writerJournal) error {
	if !j.valid() {
		return values.PublicUnavailable
	}
	raw, err := json.Marshal(j)
	if err != nil {
		return err
	}
	if len(raw) > 1<<20 {
		return values.PublicResourceExhausted
	}
	return tx.Bucket(elementBucketName).Put(writerJournalKey, raw)
}
func sameWriter(a, b writerJournal) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}
func (s *localStore) writerGate(tx *bbolt.Tx) error {
	j, err := readWriter(tx, s.authority)
	if err != nil || j.pending() || j.Epoch != s.epoch || !sameWriter(j, s.writer) {
		return values.PublicUnavailable
	}
	return nil
}

// Reconciliation must preserve this run's qualification as well as its facts.
// A valid new journal is not permission for an old process to resume serving.
func (s *localStore) checkWriter() error {
	return s.db.View(s.writerGate)
}

// installWriter is the only transaction allowed to advance a closed epoch.
// It cannot carry a business mutation or run on an unqualified ordinary Open.
func (s *localStore) installWriter(j writerJournal, cfg *values.WriterStartup) (writerJournal, error) {
	if err := j.qualified(cfg); err != nil {
		return j, err
	}
	if !j.pending() {
		return j, nil
	}
	next := j
	next.Records = append([]writerRecord(nil), j.Records...)
	next.Records[len(next.Records)-1].Installed = true
	next.Epoch++
	err := s.db.Update(func(tx *bbolt.Tx) error {
		current, e := readWriter(tx, j.Owner)
		if e != nil || !sameWriter(current, j) {
			return values.PublicUnavailable
		}
		if e = current.qualified(cfg); e != nil {
			return e
		}
		if e := putWriter(tx, next); e != nil {
			return e
		}
		if s.failBeforeCommit != nil {
			fail := s.failBeforeCommit
			s.failBeforeCommit = nil
			return fail()
		}
		return nil
	})
	if err == nil && s.failAfterCommit != nil {
		fail := s.failAfterCommit
		s.failAfterCommit = nil
		err = fail()
	}
	if err != nil {
		return j, errors.Join(values.PublicIndeterminate, err)
	}
	return next, nil
}
