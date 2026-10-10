package federation_test

import (
	federationcontract "ucupa/ucupa-core/contract/federation"

	"context"
	"errors"
	"sync"
	"testing"
	"time"
	"ucupa/shared/values"
)

type blockingFederationConnections struct {
	base    federationcontract.FederationConnections
	entered chan struct{}
	once    sync.Once
}

func (b *blockingFederationConnections) OpenFederationRead(ctx context.Context, caller values.CallerContext, hop values.NextHop, owner values.FactAuthorityRef, request federationcontract.FederationReadRequest) (federationcontract.FederationReadConnection, error) {
	part, err := b.base.OpenFederationRead(ctx, caller, hop, owner, request)
	if err != nil {
		return nil, err
	}
	return &blockingFederationPart{FederationReadConnection: part, block: b}, nil
}

type blockingFederationPart struct {
	federationcontract.FederationReadConnection
	block *blockingFederationConnections
}

func (p *blockingFederationPart) Project(ctx context.Context, request federationcontract.FederationProjectionRequest, cursor *values.PublicToken, limit uint16) (federationcontract.FederationProjectionPage, error) {
	p.block.once.Do(func() { close(p.block.entered) })
	<-ctx.Done()
	return federationcontract.FederationProjectionPage{}, values.PublicUnavailable
}

func TestFederationPublicCloseCancelsConstructionAndReleasesSources(t *testing.T) {
	f, connections, caller := vectorFixture(t, true)
	blocking := &blockingFederationConnections{base: connections, entered: make(chan struct{})}
	handle := federatedTestHandle(t, f, caller, blocking, "query")
	request := map[string]any{"op": "query", "handle": handle, "start": pathJSON(f.root, nil), "basis": pathJSON(f.root, nil), "sphere": map[string]any{"center": []any{0., 0., 0.}, "radius": 100.}}
	encoded, err := values.CanonicalJSONValue(request)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := f.owners[0].Access().PublicCallWithFederation(context.Background(), caller, encoded, blocking)
		done <- err
	}()
	select {
	case <-blocking.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("source not reached")
	}
	if _, err := federatedTestCall(t, f.owners[0], caller, blocking, map[string]any{"op": "close", "handle": handle}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("closed read disclosed result")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("close failed to cancel source work")
	}
	connections.requireReleased(t)
}

func TestFederationPublicRetainedPagesKeepOldCutAndBindConditions(t *testing.T) {
	f, connections, caller := vectorFixture(t, true)
	handle := federatedTestHandle(t, f, caller, connections, "query")
	request := map[string]any{"op": "query", "handle": handle, "start": pathJSON(f.root, nil), "basis": pathJSON(f.root, nil), "sphere": map[string]any{"center": []any{0., 0., 0.}, "radius": 100.}, "limit": float64(1)}
	first, err := federatedTestCall(t, f.owners[0], caller, connections, request)
	if err != nil || first["continuation"] == nil {
		t.Fatalf("first: %+v %v", first, err)
	}
	if _, err := f.owners[2].Elements().CreateEntity(storeTestActor, storeTestID(9752), storeTestTuple(t, storeTestID(9720), 0, 1), "after-retained-first"); err != nil {
		t.Fatal(err)
	}
	request["continuation"] = first["continuation"]
	request["upDepth"] = "-1"
	if _, err := federatedTestCall(t, f.owners[0], caller, connections, request); !errors.Is(err, values.PublicInvalidContext) {
		t.Fatal("changed condition used old cursor", err)
	}
	delete(request, "upDepth")
	request["limit"] = float64(256)
	next, err := federatedTestCall(t, f.owners[0], caller, connections, request)
	if err != nil {
		t.Fatal(err)
	}
	if next["status"] != "COMPLETE" || len(next["matches"].([]any)) != 2 {
		t.Fatal("new element entered old page", next)
	}
	if _, err := federatedTestCall(t, f.owners[0], caller, connections, map[string]any{"op": "close", "handle": handle}); err != nil {
		t.Fatal(err)
	}
}

type invalidCancelledFederationConnections struct {
	base   federationcontract.FederationConnections
	cancel context.CancelFunc
}

func (c invalidCancelledFederationConnections) OpenFederationRead(ctx context.Context, caller values.CallerContext, hop values.NextHop, owner values.FactAuthorityRef, request federationcontract.FederationReadRequest) (federationcontract.FederationReadConnection, error) {
	part, err := c.base.OpenFederationRead(ctx, caller, hop, owner, request)
	if err != nil {
		return nil, err
	}
	c.cancel()
	return invalidFederationPart{part}, nil
}

type invalidFederationPart struct {
	federationcontract.FederationReadConnection
}

func (p invalidFederationPart) Part() federationcontract.FederationReadPart {
	part := p.FederationReadConnection.Part()
	part.Purpose[0] ^= 1
	return part
}

func TestFederationPublicCancelledInvalidPartIsReleased(t *testing.T) {
	f, connections, caller := vectorFixture(t, true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	invalid := invalidCancelledFederationConnections{connections, cancel}
	handle := federatedTestHandle(t, f, caller, invalid, "query")
	request, err := values.CanonicalJSONValue(map[string]any{"op": "query", "handle": handle, "start": pathJSON(f.root, nil), "basis": pathJSON(f.root, nil), "sphere": map[string]any{"center": []any{0., 0., 0.}, "radius": 100.}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.owners[0].Access().PublicCallWithFederation(ctx, caller, request, invalid); err == nil {
		t.Fatal("cancelled invalid source produced public result")
	}
	if _, err := federatedTestCall(t, f.owners[0], caller, invalid, map[string]any{"op": "close", "handle": handle}); err != nil {
		t.Fatal(err)
	}
	connections.requireReleased(t)
}
