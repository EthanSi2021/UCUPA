package namespace

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	bolt "go.etcd.io/bbolt"

	"ucupa/shared/values"
	"ucupa/ucupa-facet/contract"
)

func TestOpaqueCallerAndOperationBytesSurviveReopen(t *testing.T) {
	x := newFixture(t, true)
	ctx := context.Background()
	actor := values.PrincipalRef{Realm: values.RealmID(string([]byte{255, 0})), Principal: values.PrincipalID(string([]byte{254, 1}))}
	op := contract.OperationID(string([]byte{253, 0, 1}))
	typ := fixtureType("opaque-identity")
	result, err := x.f.Put(ctx, actor, x.key, typ, fixturePayload("raw"), contract.ExpectAbsent(), op)
	if err != nil {
		t.Fatal(err)
	}
	if err = Close(x.f); err != nil {
		t.Fatal(err)
	}
	x.f, err = OpenDurable(x.auth, x.source, x.path, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := x.f.Put(ctx, actor, x.key, typ, fixturePayload("raw"), contract.ExpectAbsent(), op)
	if err != nil || result != replay {
		t.Fatalf("opaque bytes changed %+v %v", replay, err)
	}
	r, err := x.f.Get(ctx, actor, x.key, typ)
	if err != nil || fixtureText(r.Payload) != "raw" {
		t.Fatal(err)
	}
}
func TestUnverifiableCoreEvidenceNeverProvesAbsence(t *testing.T) {
	for _, mutate := range []func(*contract.CoreEvidence){
		func(e *contract.CoreEvidence) { e.Cut = values.CoreCutEvidence{} },
		func(e *contract.CoreEvidence) { e.Authority = fixtureFacetOwner },
		func(e *contract.CoreEvidence) { e.Element = "00000000-0000-4000-8000-00000000ff99" },
		func(e *contract.CoreEvidence) { e.State = 0 },
		func(e *contract.CoreEvidence) { e.AttachmentRevision = values.AttachmentRevision{} },
		func(e *contract.CoreEvidence) {
			raw := e.Cut.Bytes()
			raw[0] = 5
			e.Cut, _ = values.NewCoreCutEvidence(raw)
		},
		func(e *contract.CoreEvidence) {
			raw := e.Cut.Bytes()
			binary.BigEndian.PutUint64(raw[32:], 1)
			e.Cut, _ = values.NewCoreCutEvidence(raw)
		},
	} {
		x := newFixture(t, false)
		mutate(&x.source.current)
		r, err := x.f.Get(context.Background(), testActor, x.key, x.typ)
		if !errors.Is(err, contract.ErrUnavailable) || r.Cut.Valid() || r.Core.Cut.Valid() || len(r.Payload) != 0 {
			t.Fatalf("bad proof leaked %+v %v", r, err)
		}
	}
}
func TestContactChangeDoesNotCancelValidInFlightPut(t *testing.T) {
	x := newFixture(t, false)
	entered, release := make(chan struct{}), make(chan struct{})
	x.source.hook = func() { close(entered); <-release }
	done := make(chan error, 1)
	go func() {
		_, err := x.f.Put(context.Background(), testActor, x.key, x.typ, fixturePayload("data"), contract.ExpectAbsent(), "put")
		done <- err
	}()
	<-entered
	r, err := x.f.UpdateCoreNextHop(testActor, "contact", x.key, x.accepted.NextHopRevision, "local:new")
	if err != nil {
		t.Fatal(err)
	}
	close(release)
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	current, err := x.f.ReadAcceptance(testActor, x.key)
	if err != nil || current.CoreNextHop != r.CoreNextHop || current.NextHopRevision != r.NextHopRevision {
		t.Fatal("write restored old address")
	}
}
func TestFacetPublicSurfaceHasNoPredecessorOrRouting(t *testing.T) {
	for _, dir := range []string{".", "../../contract", "../../instance"} {
		checkFacetSurface(t, dir)
	}
}

// Check the actual implementation and both public Go boundaries after the
// move; scanning only the now-empty module root would silently lose coverage.
func checkFacetSurface(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	forbidden := map[string]bool{"Reconcile": true, "ElementExistence": true, "ConfirmElement": true, "FacetType": true, "BatchGet": true, "Observe": true, "ResolveAuthority": true}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, entry.Name()), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			switch d := node.(type) {
			case *ast.FuncDecl:
				if forbidden[d.Name.Name] {
					t.Errorf("forbidden function %s", d.Name.Name)
				}
			case *ast.TypeSpec:
				if forbidden[d.Name.Name] {
					t.Errorf("forbidden type %s", d.Name.Name)
				}
			}
			return true
		})
		for _, imp := range file.Imports {
			if strings.Contains(imp.Path.Value, "ucupa-core") || strings.Contains(imp.Path.Value, "ucupa-realm") || strings.Contains(imp.Path.Value, "composition") {
				t.Fatalf("owner import %s", imp.Path.Value)
			}
		}
	}
}
func TestStoreCannotBeSilentlyRepairedByMutation(t *testing.T) {
	x := newFixture(t, true)
	// Mutate the already-open test Store, leaving the live projection intact.
	err := x.f.store.db.Update(func(tx *bolt.Tx) error { return tx.Bucket(recordBucketName).Put(documentKey, fixturePayload("{}")) })
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(x.path)
	_, err = x.f.Put(context.Background(), testActor, x.key, x.typ, fixturePayload("data"), contract.ExpectAbsent(), "attempt")
	mustError(t, err, contract.ErrStore)
	after, _ := os.ReadFile(x.path)
	if !bytes.Equal(before, after) {
		t.Fatal("commit repaired corrupt Store")
	}

}
