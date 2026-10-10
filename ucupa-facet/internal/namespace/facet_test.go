package namespace

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-facet/contract"
)

func TestTypedFacetLifecycle(t *testing.T) {
	profiles(t, func(t *testing.T, x *fixture) {
		ctx := context.Background()
		empty, err := x.f.ListTypes(ctx, testActor, x.key)
		if err != nil || len(empty.Types) != 0 || !empty.Cut.Valid() || empty.Acceptance.Key != x.key {
			t.Fatalf("empty: %+v %v", empty, err)
		}
		missing, err := x.f.Get(ctx, testActor, x.key, x.typ)
		mustError(t, err, contract.ErrKeyNotFound)
		if !missing.Cut.Valid() || !missing.Core.Cut.Valid() {
			t.Fatal("missing proof")
		}
		first := x.put(t, "put", "one", contract.ExpectAbsent())
		unchanged := x.put(t, "unchanged", "one", contract.ExpectRevision(first.Revision))
		if unchanged.Status != contract.Unchanged || !unchanged.Cut.Equal(first.Cut) || !unchanged.Revision.Equal(first.Revision) {
			t.Fatal("unchanged advanced")
		}
		next := x.put(t, "next", "two", contract.ExpectRevision(first.Revision))
		if next.Status != contract.Committed || next.Revision.Equal(first.Revision) || next.Cut.Equal(first.Cut) {
			t.Fatal("no advancement")
		}
		got, err := x.f.Get(ctx, testActor, x.key, x.typ)
		if err != nil || fixtureText(got.Payload) != "two" || !got.Revision.Equal(next.Revision) {
			t.Fatalf("get: %+v %v", got, err)
		}
		deleted, err := x.f.Delete(ctx, testActor, x.key, x.typ, next.Revision, "delete")
		if err != nil || deleted.Cut.Equal(next.Cut) || deleted.Revision.Equal(next.Revision) {
			t.Fatalf("delete: %+v %v", deleted, err)
		}
		_, err = x.f.Get(ctx, testActor, x.key, x.typ)
		mustError(t, err, contract.ErrKeyNotFound)
		recreated := x.put(t, "recreate", "new", contract.ExpectAbsent())
		if recreated.Revision.Equal(first.Revision) || recreated.Revision.Equal(deleted.Revision) {
			t.Fatal("revision reuse")
		}
	})
}
func TestPayloadIsolationAndTypeIndependence(t *testing.T) {
	profiles(t, func(t *testing.T, x *fixture) {
		ctx := context.Background()
		payload := fixturePayload("original")
		r, err := x.f.Put(ctx, testActor, x.key, x.typ, payload, contract.ExpectAbsent(), "put")
		if err != nil {
			t.Fatal(err)
		}
		payload[0] = 'X'
		got, err := x.f.Get(ctx, testActor, x.key, x.typ)
		if err != nil || fixtureText(got.Payload) != "original" {
			t.Fatal("input alias")
		}
		got.Payload[0] = 'Y'
		again, _ := x.f.Get(ctx, testActor, x.key, x.typ)
		if fixtureText(again.Payload) != "original" {
			t.Fatal("output alias")
		}
		other := fixtureType("second")
		_, err = x.f.Put(ctx, testActor, x.key, other, fixturePayload("independent"), contract.ExpectAbsent(), "second")
		if err != nil {
			t.Fatal(err)
		}
		_, err = x.f.Put(ctx, testActor, x.key, other, fixturePayload("bad"), contract.ExpectRevision(r.Revision), "cross-key")
		mustError(t, err, contract.ErrRevisionConflict)
	})
}

func TestOpaquePayloadDoesNotRequireTypeArtifact(t *testing.T) {
	profiles(t, func(t *testing.T, x *fixture) {
		typ, err := values.NewFacetTypeRef(bytes.Repeat([]byte{0xa5}, 32))
		if err != nil {
			t.Fatal(err)
		}
		payload := []byte{0xff, 0x00, '{', '\n'}
		ctx := context.Background()
		put, err := x.f.Put(ctx, testActor, x.key, typ, payload, contract.ExpectAbsent(), "opaque-put")
		if err != nil || put.Status != contract.Committed {
			t.Fatalf("opaque put: %+v %v", put, err)
		}
		got, err := x.f.Get(ctx, testActor, x.key, typ)
		if err != nil || !bytes.Equal(got.Payload, payload) {
			t.Fatalf("opaque get: %+v %v", got, err)
		}
	})
}

func TestOpaquePayloadAtExactCapacity(t *testing.T) {
	profiles(t, func(t *testing.T, x *fixture) {
		ctx := context.Background()
		payload := bytes.Repeat([]byte{0xff}, maxPayloadBytes)
		put, err := x.f.Put(ctx, testActor, x.key, x.typ, payload, contract.ExpectAbsent(), "exact-capacity")
		if err != nil || put.Status != contract.Committed {
			t.Fatalf("exact-capacity put: %+v %v", put, err)
		}
		if x.path != "" {
			if err := Close(x.f); err != nil {
				t.Fatal(err)
			}
			x.f, err = OpenDurable(x.auth, x.source, x.path, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
		}
		got, err := x.f.Get(ctx, testActor, x.key, x.typ)
		if err != nil || !bytes.Equal(got.Payload, payload) || !got.Revision.Equal(put.Revision) {
			t.Fatalf("exact-capacity read: payload=%d revision=%v err=%v", len(got.Payload), got.Revision.Equal(put.Revision), err)
		}
	})
}
func TestOpaqueTypeOrderingAndCapacity(t *testing.T) {
	x := newFixture(t, false)
	ctx := context.Background()
	rawTypes := [][]byte{{255, 0}, {0}, {1}, {127}, bytes.Repeat([]byte{254}, 256)}
	for i, raw := range rawTypes {
		typ := fixtureType(fmt.Sprintf("%x", raw))
		if _, err := x.f.Put(ctx, testActor, x.key, typ, fixturePayload("data"), contract.ExpectAbsent(), contract.OperationID(fmt.Sprint("raw", i))); err != nil {
			t.Fatal(err)
		}
	}
	listed, err := x.f.ListTypes(ctx, testActor, x.key)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < len(listed.Types); i++ {
		if !listed.Types[i-1].Less(listed.Types[i]) {
			t.Fatal("not raw sorted")
		}
	}
	for i := len(rawTypes); i < 256; i++ {
		typ := fixtureType(fmt.Sprint("type", i))
		_, err = x.f.Put(ctx, testActor, x.key, typ, []byte("null"), contract.ExpectAbsent(), contract.OperationID(fmt.Sprint("fill", i)))
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err = x.f.Put(ctx, testActor, x.key, x.typ, []byte("null"), contract.ExpectAbsent(), "overflow")
	mustError(t, err, contract.ErrCapacity)
	listed, err = x.f.ListTypes(ctx, testActor, x.key)
	if err != nil || len(listed.Types) != 256 {
		t.Fatal("truncation")
	}
}
func TestCoreStateDistinctions(t *testing.T) {
	profiles(t, func(t *testing.T, x *fixture) {
		x.put(t, "initial", "secret", contract.ExpectAbsent())
		for _, test := range []struct {
			state contract.CoreState
			want  error
		}{{contract.CoreMissing, contract.ErrElementNotFound}, {contract.CoreNone, contract.ErrNamespaceNone}, {contract.CoreDetached, contract.ErrNamespaceDetached}} {
			x.setState(test.state)
			r, err := x.f.Get(context.Background(), testActor, x.key, x.typ)
			mustError(t, err, test.want)
			if r.Cut.Valid() || len(r.Payload) != 0 || !r.Core.Cut.Valid() {
				t.Fatal("forged Facet evidence")
			}
		}
		x.setState(contract.CoreActive)
		x.source.mu.Lock()
		x.source.current.Active.Facet = fixtureFacetOwner
		x.source.mu.Unlock()
		_, err := x.f.Get(context.Background(), testActor, x.key, x.typ)
		mustError(t, err, contract.ErrNotCurrentOwner)
		x.setState(contract.CoreActive)
		x.source.mu.Lock()
		x.source.current.Active = testKey(x.key.Facet, x.key.Core, x.key.Element, 1)
		x.source.mu.Unlock()
		_, err = x.f.Get(context.Background(), testActor, x.key, x.typ)
		mustError(t, err, contract.ErrAttachmentChanged)
	})
}
