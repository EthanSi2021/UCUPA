package store

import (
	"bytes"
	"errors"
	"fmt"

	"go.etcd.io/bbolt"
	"ucupa/shared/values"
)

// Identity guards replacement of an already opened owner Store. It carries
// only persistent identity headers, never an owner state or mutation callback.
type Identity struct {
	Authority values.FactAuthorityRef
	Scope     [32]byte
	PublicKey [32]byte
}

// Sink exposes only byte writes. The codec cannot commit, open another bucket,
// or obtain the database/transaction through this value.
type Sink struct{ bucket *bbolt.Bucket }

func (s Sink) Put(key, value []byte) error {
	if s.bucket == nil {
		return bbolt.ErrTxClosed
	}
	return s.bucket.Put(key, value)
}

// WriteSinks exposes the four existing Core buckets only while the owner's
// original managed transaction is active. It has no commit/rollback authority.
// Stream encoded records into these sinks; no second in-memory image is built.
type WriteSinks struct {
	Elements    Sink
	Operations  Sink
	Commits     Sink
	MountEvents Sink
}

// PrepareCreate runs inside the owner's existing db.Update callback. Return
// preparation/write errors from that callback so its original rollback applies.
func PrepareCreate(tx *bbolt.Tx) (*WriteSinks, error) { return prepareWrite(tx, nil) }

// PrepareReplace may stage deletions before discovering an invalid later bucket.
// Its caller must propagate errors through the managed transaction, never commit
// a partially prepared Store. The returned sinks expire with that transaction.
func PrepareReplace(tx *bbolt.Tx, identity Identity) (*WriteSinks, error) {
	return prepareWrite(tx, &identity)
}

func prepareWrite(tx *bbolt.Tx, identity *Identity) (*WriteSinks, error) {
	if tx == nil || tx.DB() == nil {
		return nil, bbolt.ErrTxClosed
	}
	var err error
	names := [...]string{ElementBucket, OperationBucket, CommitBucket, MountEventBucket}
	var buckets [4]*bbolt.Bucket
	for index, name := range names {
		if identity == nil {
			buckets[index], err = tx.CreateBucket([]byte(name))
			if err != nil {
				return nil, err
			}
		} else {
			buckets[index] = tx.Bucket([]byte(name))
			if buckets[index] == nil {
				return nil, errors.New("Core Store bucket is missing")
			}
		}
	}
	if identity != nil {
		elements := buckets[0]
		if !bytes.Equal(elements.Get([]byte{FormatMarkerTag}), []byte(FormatMarker)) ||
			!bytes.Equal(elements.Get([]byte{TokenScopeTag}), identity.Scope[:]) ||
			!bytes.Equal(elements.Get([]byte{PublicRevisionTag}), identity.PublicKey[:]) ||
			!bytes.Equal(elements.Get([]byte{AuthorityRefTag}), []byte(identity.Authority)) {
			return nil, errors.New("Core Store marker, identity or scope changed")
		}
		for _, bucket := range buckets {
			if bucket.Sequence() != 0 {
				return nil, errors.New("Core Store bucket sequence is not zero")
			}
		}
		for _, bucket := range buckets {
			if err := clearBucket(bucket); err != nil {
				return nil, err
			}
		}
	}
	return &WriteSinks{Elements: Sink{buckets[0]}, Operations: Sink{buckets[1]}, Commits: Sink{buckets[2]}, MountEvents: Sink{buckets[3]}}, nil
}

func clearBucket(bucket *bbolt.Bucket) error {
	cursor := bucket.Cursor()
	for key, value := cursor.First(); key != nil; key, value = cursor.Next() {
		if value == nil {
			return fmt.Errorf("nested bucket %q is not recognized", key)
		}
		if err := cursor.Delete(); err != nil {
			return err
		}
	}
	return nil
}
