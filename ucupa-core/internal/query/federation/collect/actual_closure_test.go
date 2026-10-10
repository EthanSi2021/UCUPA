package collect_test

import (
	"context"
	"errors"
	"testing"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	federationcontract "ucupa/ucupa-core/contract/federation"
)

// Migrated from the root TestFederatedClosureStablePrivateAndBound.
func TestFederatedClosureStablePrivateAndBound(t *testing.T) {
	fixture := newActualFixture(t, true, nil)
	request := federationcontract.FederationReadRequest{Boundary: fixture.root, Intent: "query", Purpose: [32]byte{4}}
	firstRead, err := fixture.openQuery(t, request, 3, actualReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	first, err := collectActualPage(firstRead.read, false)
	if err != nil || first.Closure == ([32]byte{}) {
		t.Fatal(err)
	}
	second, err := collectActualPage(firstRead.read, false)
	if err != nil || second.Closure != first.Closure {
		t.Fatal("unstable closure", err)
	}
	independent, err := fixture.openQuery(t, request, 3, actualReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	other, err := collectActualPage(independent.read, false)
	if err != nil || other.Closure == first.Closure {
		t.Fatal("independent query was linkable", err)
	}
	for _, source := range first.SourceGraph.Sources {
		if first.Closure == source.CutStamp || first.Closure == source.AuthorizationStamp {
			t.Fatal("raw source stamp")
		}
	}

	fixture.write(t, 2, 9780, actualID(9720), corecontract.OperationID("closure-write"))
	second, err = collectActualPage(firstRead.read, false)
	if err != nil || second.Closure != first.Closure {
		t.Fatal("retained closure followed live writes", err)
	}
	if err := firstRead.read.Retire(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := collectActualPage(firstRead.read, false); !errors.Is(err, values.PublicInvalidContext) {
		t.Fatal("closed recipe accepted", err)
	}
	firstRead.tracker.requireReleased(t, actualOwnerRefs(fixture, 3)...)
	if err := independent.read.Retire(context.Background()); err != nil {
		t.Fatal(err)
	}
	independent.tracker.requireReleased(t, actualOwnerRefs(fixture, 3)...)
}
