package namespace

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	bolt "go.etcd.io/bbolt"
	"ucupa/shared/values"
)

var writerJournalKey = []byte{2}

func readWriter(tx *bolt.Tx, owner values.FactAuthorityRef) (writerJournal, error) {
	b := tx.Bucket(recordBucketName)
	if b == nil {
		return writerJournal{}, values.PublicUnavailable
	}
	j, err := decodeWriter(b.Get(writerJournalKey), owner)
	if err != nil {
		return j, err
	}
	if j.pending() && sha256.Sum256(b.Get(documentKey)) != j.Records[len(j.Records)-1].FinalCut {
		return j, values.PublicUnavailable
	}
	return j, nil
}
func loadWriter(db *bolt.DB, owner values.FactAuthorityRef) (writerJournal, error) {
	var j writerJournal
	err := db.View(func(tx *bolt.Tx) error { var e error; j, e = readWriter(tx, owner); return e })
	return j, err
}
func putWriter(tx *bolt.Tx, j writerJournal) error {
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
	return tx.Bucket(recordBucketName).Put(writerJournalKey, raw)
}
func sameWriter(a, b writerJournal) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}

func (s *localStore) checkWriter(owner values.FactAuthorityRef) error {
	return s.db.View(func(tx *bolt.Tx) error { return s.writerGate(tx, owner) })
}
func (s *localStore) writerGate(tx *bolt.Tx, owner values.FactAuthorityRef) error {
	j, err := readWriter(tx, owner)
	if err != nil || j.pending() || j.Epoch != s.epoch || !sameWriter(j, s.writer) {
		return values.PublicUnavailable
	}
	return nil
}
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
	err := s.db.Update(func(tx *bolt.Tx) error {
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
