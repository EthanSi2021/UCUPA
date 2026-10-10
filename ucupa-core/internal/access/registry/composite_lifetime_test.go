package registry_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	federationcontract "ucupa/ucupa-core/contract/federation"
	"ucupa/ucupa-core/internal/access/registry"
	"ucupa/ucupa-core/internal/query/federation/collect"
	"ucupa/ucupa-core/internal/query/federation/source"
)

type noRemoteSources struct{ opens atomic.Int32 }

func (p *noRemoteSources) OpenFederationRead(context.Context, values.CallerContext, values.NextHop, values.FactAuthorityRef, federationcontract.FederationReadRequest) (federationcontract.FederationReadConnection, error) {
	p.opens.Add(1)
	return nil, values.PublicUnavailable
}

type pausedSourceRetirement struct {
	federationcontract.FederationReadConnection
	entered chan struct{}
	resume  chan struct{}
	calls   atomic.Int32
	once    sync.Once
}

func (p *pausedSourceRetirement) Release(ctx context.Context) error {
	p.calls.Add(1)
	p.once.Do(func() { close(p.entered) })
	select {
	case <-p.resume:
	case <-ctx.Done():
		return ctx.Err()
	}
	return p.FederationReadConnection.Release(ctx)
}
func TestCompositeRemovalCancelsBeforeLockFreeRetirementAndExactRefund(t *testing.T) {
	ttl := time.Hour
	f := newResourceComponents(t, contract.Config{SnapshotRetention: &ttl, PublicAccessPolicy: &values.PublicAccessPolicy{ContextTTL: ttl, ObservationTTL: ttl}})
	root := resourceID(93000)
	if _, err := f.elements.CreateRoot(resourceActor, root, "federation-root"); err != nil {
		t.Fatal(err)
	}
	caller := values.CallerContext{Subject: resourceActor}
	request := federationcontract.FederationReadRequest{Boundary: root, Intent: "list", Purpose: [32]byte{1}}
	local, err := source.OpenLocal(f.sources, context.Background(), caller, request)
	if err != nil {
		t.Fatal(err)
	}
	sourceCount, sourceBytes := registry.PublicResourceCountsForTest(f.resources)
	if sourceCount != 1 || sourceBytes == 0 {
		t.Fatal("actual source not charged")
	}
	paused := &pausedSourceRetirement{FederationReadConnection: local, entered: make(chan struct{}), resume: make(chan struct{})}
	remote := &noRemoteSources{}
	read, err := collect.New(context.Background(), collect.Config{Caller: caller, Source: f.sources.AuthorityRef(), PartRequest: request, Local: paused, Connections: remote, SourceLimit: 1, Request: collect.Request{Operation: "list", Address: collect.Address{Origin: root}}})
	if err != nil {
		t.Fatal(err)
	}
	var unblock sync.Once
	resume := func() { unblock.Do(func() { close(paused.resume) }) }
	t.Cleanup(func() { resume(); _ = read.Retire(context.Background()) })
	f.owner.Lock()
	space, err := f.resources.IssueSpaceLocked(caller, root)
	f.owner.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	_, withSpaceBytes := registry.PublicResourceCountsForTest(f.resources)
	f.owner.Lock()
	receipt, err := f.resources.IssueCompositeReadLocked(caller, root, space.Ref, "list", "fixed")
	if err == nil && !f.resources.InstallCompositeLocked(receipt.Ref, read) {
		err = errors.New("composite not installed")
	}
	f.owner.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	count, before := registry.PublicResourceCountsForTest(f.resources)
	compositeBytes := before - withSpaceBytes
	if count != 3 || compositeBytes <= collect.RecipeBytes || receipt.Lifetime == nil {
		t.Fatal("actual composite not charged", count, before)
	}
	f.owner.Lock()
	f.resources.RemoveLocked(space.Ref)
	f.owner.Unlock()
	select {
	case <-receipt.Lifetime.Done():
	default:
		t.Fatal("removal did not immediately cancel construction")
	}
	done := make(chan struct{})
	go func() { f.resources.DrainRetired(); close(done) }()
	select {
	case <-paused.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("retirement not reached")
	}
	// Reading the registry while network retirement is held proves the owner
	// lock is not carried into Release. The full recipe remains charged.
	inspected := make(chan [2]uint64, 1)
	go func() {
		count, bytes := registry.PublicResourceCountsForTest(f.resources)
		inspected <- [2]uint64{uint64(count), bytes}
	}()
	select {
	case got := <-inspected:
		if got != [2]uint64{1, sourceBytes + compositeBytes} {
			t.Fatal("early refund or source removal", got, sourceBytes, compositeBytes)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("retirement held the owner lock")
	}
	resume()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("retirement did not finish")
	}
	if count, bytes := registry.PublicResourceCountsForTest(f.resources); count != 0 || bytes != 0 {
		t.Fatal("retirement leaked", count, bytes)
	}
	if err := local.Check(context.Background(), false); !errors.Is(err, values.PublicInvalidContext) {
		t.Fatal("source remained usable", err)
	}
	f.owner.Lock()
	f.resources.RemoveLocked(space.Ref)
	f.resources.RemoveLocked(receipt.Ref)
	f.owner.Unlock()
	f.resources.DrainRetired()
	if count, bytes := registry.PublicResourceCountsForTest(f.resources); count != 0 || bytes != 0 || paused.calls.Load() != 1 || remote.opens.Load() != 0 {
		t.Fatal("repeat removal changed refund or release", count, bytes, paused.calls.Load(), remote.opens.Load())
	}
}
