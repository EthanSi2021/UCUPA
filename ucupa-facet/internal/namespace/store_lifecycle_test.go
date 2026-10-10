package namespace

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	bolt "go.etcd.io/bbolt"

	"ucupa/ucupa-facet/contract"
)

func TestAuthorityLifecycleAndReopen(t *testing.T) {
	x := newFixture(t, true)
	other := newFixture(t, false)
	if !x.f.AuthorityRef().Valid() || x.f.AuthorityRef() == other.f.AuthorityRef() {
		t.Fatal("identity")
	}
	original := x.put(t, "put", "persisted", contract.ExpectAbsent())
	owner := x.f.AuthorityRef()
	if err := Close(x.f); err != nil {
		t.Fatal(err)
	}
	var err error
	x.f, err = OpenDurable(x.auth, x.source, x.path, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if x.f.AuthorityRef() != owner {
		t.Fatal("identity changed")
	}
	replay := x.put(t, "put", "persisted", contract.ExpectAbsent())
	if replay != original {
		t.Fatal("reopen replay")
	}
}
func TestStoreLifecycleRejectsWithoutWriting(t *testing.T) {
	x := newFixture(t, false)
	missing := filepath.Join(t.TempDir(), "missing.db")
	if f, err := OpenDurable(x.auth, x.source, missing, nil, nil); f != nil || !errors.Is(err, contract.ErrStore) {
		t.Fatal("missing accepted")
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatal("created missing")
	}
	for _, kind := range []string{"empty", "foreign"} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "input.db")
			if kind == "empty" {
				if err := os.WriteFile(path, nil, 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				db, err := bolt.Open(path, 0600, nil)
				if err != nil {
					t.Fatal(err)
				}
				if err = db.Update(func(tx *bolt.Tx) error {
					b, e := tx.CreateBucket([]byte("foreign"))
					if e != nil {
						return e
					}
					return b.Put(fixturePayload("a"), fixturePayload("b"))
				}); err != nil {
					t.Fatal(err)
				}
				_ = db.Close()
			}
			before, _ := os.ReadFile(path)
			if f, err := OpenDurable(x.auth, x.source, path, nil, nil); f != nil || !errors.Is(err, contract.ErrStore) {
				t.Fatal("invalid opened")
			}
			after, _ := os.ReadFile(path)
			if !bytes.Equal(before, after) {
				t.Fatal("failed open wrote")
			}
			if f, err := CreateDurable(x.auth, x.source, testInitial, path, nil, nil); f != nil || !errors.Is(err, contract.ErrStore) {
				t.Fatal("create overwrote")
			}
			after, _ = os.ReadFile(path)
			if !bytes.Equal(before, after) {
				t.Fatal("create failure wrote")
			}
		})
	}
	y := newFixture(t, true)
	if f, err := OpenDurable(y.auth, y.source, y.path, nil, nil); f != nil || !errors.Is(err, contract.ErrStore) {
		t.Fatal("second writer")
	}
}
