package namespace

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

	bolt "go.etcd.io/bbolt"

	"ucupa/ucupa-facet/contract"
)

func TestOwnerStoreFormatAndCorruption(t *testing.T) {
	if string(formatMarker) != "ucupa.facet.local@4d8e1a6f3c9b7250e6a2f8d5c1b7e3a9" {
		t.Fatal("marker")
	}
	cases := map[string]func(*storeDocument){
		"authority":          func(d *storeDocument) { d.Authority = "wrong" },
		"scope":              func(d *storeDocument) { d.Scope = [32]byte{} },
		"missing public key": func(d *storeDocument) { d.Facts.PublicKey = [32]byte{} },
		"cut":                func(d *storeDocument) { d.Facts.Cut++ },
		"acceptance": func(d *storeDocument) {
			for k, v := range d.Facts.Acceptances {
				v.At++
				d.Facts.Acceptances[k] = v
			}
		},
		"contact": func(d *storeDocument) {
			for k, v := range d.Facts.Contacts {
				v.Address = "local:forged"
				d.Facts.Contacts[k] = v
			}
		},
		"missing contact": func(d *storeDocument) {
			for k := range d.Facts.Contacts {
				delete(d.Facts.Contacts, k)
			}
		},
		"record": func(d *storeDocument) {
			for k, v := range d.Facts.Records {
				v.Payload = []byte("forged")
				d.Facts.Records[k] = v
			}
		},
		"receipt":             func(d *storeDocument) { d.Events[0].Outcome.AcceptedAt++ },
		"history":             func(d *storeDocument) { d.Events = d.Events[1:] },
		"duplicate operation": func(d *storeDocument) { d.Events = append(d.Events, d.Events[0]) },
		"unexpected evidence": func(d *storeDocument) { d.Events[0].Input.Owner = "bad" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			x := newFixture(t, true)
			x.put(t, "put", "original", contract.ExpectAbsent())
			if err := Close(x.f); err != nil {
				t.Fatal(err)
			}
			db, err := bolt.Open(x.path, 0600, nil)
			if err != nil {
				t.Fatal(err)
			}
			err = db.Update(func(tx *bolt.Tx) error {
				b := tx.Bucket(recordBucketName)
				var doc storeDocument
				if e := json.Unmarshal(b.Get(documentKey), &doc); e != nil {
					return e
				}
				mutate(&doc)
				raw, e := json.Marshal(doc)
				if e != nil {
					return e
				}
				return b.Put(documentKey, raw)
			})
			if err != nil {
				t.Fatal(err)
			}
			_ = db.Close()
			before, _ := os.ReadFile(x.path)
			f, err := OpenDurable(x.auth, x.source, x.path, nil, nil)
			if f != nil || !errors.Is(err, contract.ErrStore) {
				t.Fatalf("corrupt opened: %v", err)
			}
			after, _ := os.ReadFile(x.path)
			if !bytes.Equal(before, after) {
				t.Fatal("repaired corruption")
			}
		})
	}
}
func TestMutationCommitFailuresAndLostResponse(t *testing.T) {
	for _, kind := range []string{"accept", "contact", "put", "delete", "rejected", "unchanged"} {
		t.Run(kind, func(t *testing.T) {
			x := newFixture(t, true)
			ctx := context.Background()
			first := x.put(t, "base", "data", contract.ExpectAbsent())
			key := testKey(x.key.Facet, x.key.Core, x.key.Element, 5)
			action := func() (contract.MutationResult, error) {
				switch kind {
				case "accept":
					return x.f.AcceptAttachment(ctx, testActor, key, "local:core", "tested")
				case "contact":
					return x.f.UpdateCoreNextHop(testActor, "tested", x.key, x.accepted.NextHopRevision, "local:repaired")
				case "delete":
					return x.f.Delete(ctx, testActor, x.key, x.typ, first.Revision, "tested")
				case "rejected":
					return x.f.Put(ctx, testActor, x.key, x.typ, []byte("null"), contract.ExpectAbsent(), "tested")
				case "unchanged":
					return x.f.Put(ctx, testActor, x.key, x.typ, fixturePayload("data"), contract.ExpectRevision(first.Revision), "tested")
				default:
					return x.f.Put(ctx, testActor, x.key, x.typ, fixturePayload("next"), contract.ExpectRevision(first.Revision), "tested")
				}
			}
			if kind == "accept" {
				x.source.current = testProof(key, contract.CoreNone, 5)
				x.source.current.Pending = key
			}
			before := x.f.state.Cut
			events := len(x.f.events)
			x.f.store.failBeforeCommit = func() error { return errors.New("abort") }
			_, err := action()
			mustError(t, err, contract.ErrUnavailable)
			if x.f.state.Cut != before || len(x.f.events) != events {
				t.Fatal("precommit advanced")
			}
			x.f.store.loseResponse = func() error { return errors.New("lost") }
			_, err = action()
			mustError(t, err, contract.ErrIndeterminate)
			if len(x.f.events) != events+1 {
				t.Fatal("lost response didn't retain")
			}
			if err = Close(x.f); err != nil {
				t.Fatal(err)
			}
			x.f, err = OpenDurable(x.auth, x.source, x.path, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			r, err := action()
			if kind == "rejected" {
				mustError(t, err, contract.ErrRevisionConflict)
			} else if err != nil {
				t.Fatal(err)
			}
			if len(x.f.events) != events+1 || r.Status == 0 {
				t.Fatal("retry repeated operation")
			}
		})
	}
}
func TestCanonicalDocumentAndCurrentMarkerOnly(t *testing.T) {
	x := newFixture(t, true)
	if err := Close(x.f); err != nil {
		t.Fatal(err)
	}
	for _, raw := range [][]byte{nil, []byte("unknown"), []byte("ucupa.facet.local@b8d829214ff348a784e989839e938394"),
		[]byte("ucupa.facet.local@e4f6b8c0d3a92517b0d4e6f2a7c3e9d5"),
		[]byte("ucupa.facet.local@7c3e9d5a2b8f4160c9d7b5e3a6f2c841")} {
		db, err := bolt.Open(x.path, 0600, nil)
		if err != nil {
			t.Fatal(err)
		}
		err = db.Update(func(tx *bolt.Tx) error { return tx.Bucket(recordBucketName).Put(formatMarkerKey, raw) })
		if err != nil {
			t.Fatal(err)
		}
		_ = db.Close()
		before, _ := os.ReadFile(x.path)
		if f, err := OpenDurable(x.auth, x.source, x.path, nil, nil); f != nil || !errors.Is(err, contract.ErrStore) {
			t.Fatal("extra reader")
		}
		after, _ := os.ReadFile(x.path)
		if !bytes.Equal(before, after) {
			t.Fatal("marker rewrite")
		}
	}
	doc := storeDocument{Authority: x.f.authority, Scope: x.f.scope, Facts: x.f.state, Events: x.f.events}
	raw, err := encodeDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]byte{append(append([]byte(nil), raw...), []byte("{}")...), append([]byte(" "), raw...), []byte("{}")} {
		if _, err = decodeDocument(bad); !errors.Is(err, contract.ErrStore) {
			t.Fatal("noncanonical accepted")
		}
	}
}
