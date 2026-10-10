package commit

import (
	"errors"
	"path/filepath"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/access/observe"
	commitbinding "ucupa/ucupa-core/internal/commit/binding"
	elementfact "ucupa/ucupa-core/internal/element/fact"
)

// This test-local sink checks the actual publication boundary. It is not a
// production state getter or a substitute for the public operation regressions.
type publicationProbe struct {
	check func(observe.View, observe.View)
}

func (probe publicationProbe) Publish(before, after observe.View) {
	probe.check(before, after)
}

func TestOwnerPublicationReconcilesBeforeInstallingState(t *testing.T) {
	for _, mode := range []string{"success", "rollback", "lost-response", "third-state"} {
		t.Run(mode, func(t *testing.T) {
			authority, err := values.NewFactAuthorityRef()
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "core.db")
			owner, err := CreateDurable(path, authority, [32]byte{1}, contract.RealmInitialization{Realm: "fixture-realm", NextHop: "local:fixture"}, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = owner.CloseStore() })
			before := owner.state
			published := 0
			owner.Lock()
			err = owner.SetPublicationSinkLocked(publicationProbe{check: func(old, next observe.View) {
				published++
				if owner.state != before {
					t.Error("state installed before synchronous observation")
				}
				if !old.Present || !next.Present || old.CutSerial != before.cutSerial {
					t.Error("publication lost its original cut")
				}
				disk, loadErr := owner.store.reload()
				if loadErr != nil {
					t.Fatal(loadErr)
				}
				if disk.cutSerial != next.CutSerial || disk.elements.Counts().Anchors != next.Elements.Counts().Anchors {
					t.Error("observation preceded durable publication")
				}
			}})
			if err != nil {
				t.Fatal(err)
			}
			failure := errors.New("one-shot Store failure")
			switch mode {
			case "rollback":
				owner.store.failBeforeCommit = func() error { return failure }
			case "lost-response":
				owner.store.failAfterCommit = func() error { return failure }
			case "third-state":
				owner.store.failAfterCommit = func() error {
					committed, err := owner.store.reload()
					if err != nil {
						return err
					}
					third, _, err := committed.prepareMutation("foreign-decision", commitbinding.Binding{
						Actor: values.PrincipalRef{Realm: "owner-test", Principal: "writer"}, Kind: elementfact.OperationCreateRoot, Element: "20000000-0000-4000-8000-000000000002",
					})
					if err != nil {
						return err
					}
					if err := owner.store.persist(third); err != nil {
						return err
					}
					return failure
				}
			}
			result, err := owner.ApplyElementLocked("create-root", commitbinding.Binding{
				Actor: values.PrincipalRef{Realm: "owner-test", Principal: "writer"}, Kind: elementfact.OperationCreateRoot, Element: "20000000-0000-4000-8000-000000000001",
			})
			owner.Unlock()
			if mode == "success" {
				if err != nil || result.Status != contract.MutationCommitted {
					t.Fatalf("successful commit: %#v %v", result, err)
				}
			} else if !errors.Is(err, contract.ErrStore) || !errors.Is(err, failure) {
				t.Fatalf("lost original Store error: %v", err)
			}
			if mode == "third-state" {
				if owner.state != nil || published != 0 {
					t.Fatal("unreconcilable Store was published or remained writable")
				}
				return
			}
			if published != 1 {
				t.Fatalf("publication count=%d", published)
			}
			if mode == "rollback" {
				if !sameCompleteCoreState(owner.state, before) {
					t.Fatal("rollback changed the aggregate")
				}
			} else if owner.state.cutSerial != before.cutSerial+1 || owner.state.elements.Counts().Anchors != 1 {
				t.Fatal("committed state was not installed")
			}
		})
	}
}
