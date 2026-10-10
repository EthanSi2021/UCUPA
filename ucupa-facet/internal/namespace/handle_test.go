package namespace

import (
	"context"
	"errors"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-facet/contract"
)

func TestCopiedNamespaceHandleSharesCommittedFactsAndLock(t *testing.T) {
	profiles(t, func(t *testing.T, x *fixture) {
		copy := *x.f
		if &copy.mu != &x.f.mu {
			t.Fatal("copying a namespace handle copied the commit lock")
		}
		written := x.put(t, "copy-create", "first", contract.ExpectAbsent())
		read, err := copy.Get(context.Background(), testActor, x.key, x.typ)
		if err != nil || read.Revision != written.Revision || fixtureText(read.Payload) != "first" {
			t.Fatalf("copied handle missed committed facts: %+v %v", read, err)
		}
		updated, err := copy.Put(context.Background(), testActor, x.key, x.typ, fixturePayload("second"), contract.ExpectRevision(written.Revision), "copy-update")
		if err != nil {
			t.Fatal(err)
		}
		read, err = x.f.Get(context.Background(), testActor, x.key, x.typ)
		if err != nil || read.Revision != updated.Revision || fixtureText(read.Payload) != "second" {
			t.Fatalf("original handle missed copied-handle commit: %+v %v", read, err)
		}
	})
}

func TestZeroNamespaceHandleDoesNotCreateAnOwner(t *testing.T) {
	for _, f := range []*Owner{nil, {}} {
		if f.AuthorityRef() != "" || Close(f) != nil {
			t.Fatal("zero handle acquired identity or lifecycle")
		}
		if result, err := f.PublicCall(context.Background(), values.CallerContext{}, []byte(`{}`)); result != nil || !errors.Is(err, values.PublicUnavailable) {
			t.Fatalf("zero handle public call: %q %v", result, err)
		}
		if _, err := f.ReadAcceptance(testActor, values.FacetAcceptanceKey{}); !errors.Is(err, contract.ErrInvalid) {
			t.Fatalf("zero handle acceptance: %v", err)
		}
	}
}
