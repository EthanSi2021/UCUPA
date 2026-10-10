package namespace

import (
	"context"
	"errors"
	"testing"
	"ucupa/shared/values"
	facetcontract "ucupa/ucupa-facet/contract"
)

// The test changes availability only during the existing lock-free Core read;
// it does not add a production fault injection or move authorization timing.
type lateUnavailableCore struct {
	*publicStoreControlSource
	duringRead func()
}

func (s *lateUnavailableCore) ReadControl(ctx context.Context, address values.NextHop, key values.FacetAcceptanceKey, prepare bool) (facetcontract.CoreControlRead, error) {
	result, err := s.publicStoreControlSource.ReadControl(ctx, address, key, prepare)
	if err == nil && s.duringRead != nil {
		s.duringRead()
	}
	return result, err
}

func TestObservationNewCursorKeepsLateOwnerAvailabilityCheck(t *testing.T) {
	x, typ, core := newPublicStoreFixture(t)
	prepared := publicStoreGet(t, x, typ, true)
	started := publicStoreCall(t, x, map[string]any{"op": "begin-observe", "preparation": prepared["preparation"]})
	publicStoreCall(t, x, publicStorePut(x, typ, "cursor-owner-guard", "payload"))
	x.f.cores = &lateUnavailableCore{publicStoreControlSource: core, duringRead: func() {
		x.f.mu.Lock()
		x.f.corrupt = true
		x.f.mu.Unlock()
	}}
	raw, err := publicStoreRequest(t, x, map[string]any{"op": "poll", "cursor": started["cursor"]})
	if raw != nil || !errors.Is(err, values.PublicUnavailable) {
		t.Fatalf("new cursor bypassed late owner guard: %q %v", raw, err)
	}
}

func TestClosedOwnerKeepsRequestValidationBeforeAvailability(t *testing.T) {
	x, _, _ := newPublicStoreFixture(t)
	if err := Close(x.f); err != nil {
		t.Fatal(err)
	}
	for _, raw := range [][]byte{[]byte("{"), []byte("[]"), []byte("{\"op\":\"unknown\"}")} {
		result, err := x.f.PublicCall(context.Background(), x.caller, raw)
		if result != nil || !errors.Is(err, values.PublicInvalid) {
			t.Fatalf("resource manager changed parse priority for %q: %q %v", raw, result, err)
		}
	}
}
