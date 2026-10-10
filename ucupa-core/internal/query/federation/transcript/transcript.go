// Package transcript owns the canonical byte framing shared by Federation
// traversal and authenticated entry commitments. It owns no source, key,
// grant, fact, read context or long-lived state.
package transcript

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"hash"
	"math"

	"ucupa/shared/values"
	federationvalue "ucupa/ucupa-core/internal/query/federation/value"
	coretoken "ucupa/ucupa-core/internal/token"
)

// Writer accumulates one call-local transcript. A digest alone is not
// authorization; the original source and read-basis checks remain mandatory.
type Writer struct{ h hash.Hash }

func (w Writer) Number(n uint64) {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], n)
	_, _ = w.h.Write(b[:])
}

func (w Writer) Bytes(b []byte) { w.Number(uint64(len(b))); _, _ = w.h.Write(b) }

func (w Writer) Text(parts ...string) {
	for _, p := range parts {
		w.Bytes([]byte(p))
	}
}

func (w Writer) Token(t values.PublicToken) { w.Text(string(t.Owner), t.Kind, t.Value) }

func (w Writer) Relation(r *federationvalue.FederationMountRelation) {
	if r == nil {
		w.Number(0)
		return
	}
	w.Number(1)
	if r.Incoming {
		w.Number(1)
	} else {
		w.Number(0)
	}
	w.Number(uint64(r.State))
	key := r.Key
	w.Text(string(key.ParentAuthority), string(key.MountAnchorUUID), string(key.CarrierAuthority))
	w.Bytes(coretoken.MountRevisionScopeBytes(key.ProposalRevision))
	w.Text(string(coretoken.MountRevisionElement(key.ProposalRevision)))
	w.Number(coretoken.MountRevisionSerial(key.ProposalRevision))
	w.Text(string(r.Admission.OperationID))
	w.Token(r.Revision)
	w.Token(r.Contact)
	w.Text(string(r.NextHop))
	w.Number(math.Float64bits(r.Range.Radius()))
	if r.WriteGateOpen {
		w.Number(1)
	} else {
		w.Number(0)
	}
}

// SHA256 starts a private traversal digest with the original SHA-256 algorithm.
func SHA256() Writer { return Writer{h: sha256.New()} }

// HMACSHA256 starts a commitment with the existing finite receipt key.
// It does not mint, store or authenticate that receipt.
func HMACSHA256(key []byte) Writer { return Writer{h: hmac.New(sha256.New, key)} }

// Sum returns a detached digest; it never exposes the underlying hash writer.
func (w Writer) Sum() []byte { return w.h.Sum(nil) }
