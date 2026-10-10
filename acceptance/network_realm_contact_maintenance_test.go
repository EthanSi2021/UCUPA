package acceptance_test

import (
	"context"
	"io"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"ucupa/composition"
	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	corepb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
	coresdk "ucupa/ucupa-core/sdk/go"
	realm "ucupa/ucupa-realm/contract"
)

// openMaintenance reopens the durable Core Store exclusively in the test
// process for real contact maintenance: it binds the given network Realm
// address with the Core's own service material, establishes the operating
// caller with the user credential and returns the maintenance handle plus an
// idempotent close. The original Store file is exclusive, so the Core
// product process must be stopped before the first call.
func (f *networkRealmContactFixture) openMaintenance(t *testing.T, address values.NextHop) (*core.RealmMaintenance, values.CallerContext, func()) {
	t.Helper()
	connections := &composition.NetworkRealmConnections{}
	if err := connections.BindNetworkEndpoint(address, f.actor.Realm, composition.NetworkRealmEndpointConfig{
		Roots: f.realmRoots, SPKISHA256: f.realmSPKI, Service: &f.serviceCert,
		RPCDeadline: 5 * time.Second, ConnectTimeout: time.Second,
		MaxResponseBytes: 8 << 20, MaxMetadataBytes: 8192}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connections.UnbindNetworkEndpoint(address) })
	maintenance, err := core.OpenRealmMaintenance(connections, filepath.Join(f.dir, "core.db"),
		corecontract.Config{ExpectedRealm: f.actor.Realm,
			PublicAccessPolicy: &values.PublicAccessPolicy{ContextTTL: time.Minute, ObservationTTL: time.Minute}})
	if err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	closeMaintenance := func() { once.Do(func() { _ = maintenance.Close() }) }
	t.Cleanup(closeMaintenance)
	caller, err := connections.EstablishCaller(context.Background(), address, f.actor.Realm,
		realm.CredentialEvidence{Audience: f.actor.Realm, ID: realm.CredentialID(f.id), Secret: realm.CredentialSecret(f.secret)})
	if err != nil {
		closeMaintenance()
		t.Fatal(err)
	}
	return maintenance, caller, closeMaintenance
}

// TestNetworkRealmContactProductMaintenance proves the maintenance matrix with
// only real product processes and the original maintenance API: a live
// current address authorizes a move to an address that is not yet serving,
// an offline current contact discloses nothing without an explicit
// same-Realm candidate, that candidate repairs disclosure and retained
// decisions, the product proves the new address online at open and keeps the
// original recovery semantics, a dead contact is repaired through a
// candidate of the same Realm, and revoked permissions deny online without
// moving the contact.
func TestNetworkRealmContactProductMaintenance(t *testing.T) {
	f := newNetworkRealmContactFixture(t)
	realmProcess := f.startRealm()
	coreProcess := f.startCore()
	client := f.connect(t)
	ctx := context.Background()

	// One committed write proves the whole chain live before maintenance
	// takes the original Store exclusively.
	write, err := client.Core().CreateAnchor(ctx, &corepb.CreateAnchorRequest{
		Handle:  f.enter(t, client, corepb.Intent_CREATE_ANCHOR),
		Element: proto.String(networkContactChild), OperationId: []byte("maintenance-write"), Final: networkContactTuple(10)})
	if err != nil || write.GetResult() == nil {
		t.Fatalf("create anchor: %v %v", write, err)
	}

	// The live current address authorizes disclosure and the move to an
	// address that is not yet serving; the commit itself is legal while the
	// new address is offline.
	coreProcess.gracefulStop()
	maintenance, caller, closeMaintenance := f.openMaintenance(t, f.realmAddress)
	contact, err := maintenance.Inspect(context.Background(), caller, "")
	if err != nil || contact.NextHop != f.realmAddress || contact.Owner != f.owner {
		t.Fatalf("inspect current contact: %v %v", contact, err)
	}
	spare := f.spareAddress(t)
	moved, err := maintenance.Update(context.Background(), caller, corecontract.RealmContactUpdate{
		OperationID: "contact-move-spare", Owner: f.owner, Realm: f.actor.Realm,
		Expected: contact.Revision, NextHop: spare})
	if err != nil || moved.Decision != corecontract.RealmContactCommitted || moved.Contact.NextHop != spare {
		t.Fatalf("commit while the new address is offline: %v %v", moved, err)
	}

	// The moved contact is offline: without an explicit candidate nothing
	// authorizes — no local pass — while the same-Realm candidate repairs
	// both disclosure and the retained decision.
	if _, err = maintenance.Inspect(context.Background(), caller, ""); err == nil {
		t.Fatal("offline current contact passed without a candidate")
	}
	inspected, err := maintenance.Inspect(context.Background(), caller, f.realmAddress)
	if err != nil || inspected.NextHop != spare || inspected.Revision == contact.Revision {
		t.Fatalf("candidate repair after the move: %v %v", inspected, err)
	}
	retained, err := maintenance.Recover(context.Background(), caller, "contact-move-spare", f.realmAddress)
	if err != nil || retained.Decision != corecontract.RealmContactCommitted || retained.Contact.NextHop != spare {
		t.Fatalf("recover the retained move: %v %v", retained, err)
	}
	closeMaintenance()

	// The product proves the new address online at open and serves the same
	// Store: the moved contact keeps the original recovery semantics.
	realmProcess = f.relaunchRealm(t, realmProcess, spare)
	coreProcess = f.relaunchCore(t, coreProcess, spare)
	restored := f.connect(t)
	again, err := restored.Core().RecoverOutcome(ctx, &corepb.RecoverOutcomeRequest{Reference: f.reference(),
		OperationId: []byte("maintenance-write"),
		Request: &corepb.MutationCommand{Command: &corepb.MutationCommand_CreateAnchor{CreateAnchor: &corepb.CreateAnchorCommand{
			Element: proto.String(networkContactChild), OperationId: []byte("maintenance-write"), Final: networkContactTuple(10)}}}})
	if err != nil || !proto.Equal(write.GetResult(), again.GetResult()) {
		t.Fatalf("outcome after the move: %v %v", again, err)
	}

	// The old contact is dead but the same Realm answers through a
	// candidate: repair authorizes through the candidate and moves the
	// contact again.
	coreProcess.gracefulStop()
	realmProcess.gracefulStop()
	third := f.spareAddress(t)
	realmProcess = f.relaunchRealm(t, realmProcess, third)
	maintenance, caller, closeMaintenance = f.openMaintenance(t, third)
	repaired, err := maintenance.Inspect(context.Background(), caller, third)
	if err != nil || repaired.NextHop != spare {
		t.Fatalf("inspect through the repair candidate: %v %v", repaired, err)
	}
	relocated, err := maintenance.Update(context.Background(), caller, corecontract.RealmContactUpdate{
		OperationID: "contact-move-third", Owner: f.owner, Realm: f.actor.Realm,
		Expected: repaired.Revision, NextHop: third})
	if err != nil || relocated.Decision != corecontract.RealmContactCommitted || relocated.Contact.NextHop != third {
		t.Fatalf("repair update through the candidate: %v %v", relocated, err)
	}
	closeMaintenance()

	// Revoking both maintenance permissions denies online through the live
	// current address: no disclosure and no commit. Restoring them proves
	// the denials were authorization decisions and the contact never moved.
	realmProcess.gracefulStop()
	if err = f.grant(corecontract.PermissionRealmContactRead, values.AuthorityScope(f.owner), true); err != nil {
		t.Fatal(err)
	}
	if err = f.grant(corecontract.PermissionRealmContactUpdate, values.AuthorityScope(f.owner), true); err != nil {
		t.Fatal(err)
	}
	realmProcess = f.relaunchRealm(t, realmProcess, third)
	maintenance, deniedCaller, closeMaintenance := f.openMaintenance(t, third)
	if _, err = maintenance.Inspect(context.Background(), deniedCaller, third); err == nil {
		t.Fatal("revoked maintenance read passed")
	}
	if _, err = maintenance.Update(context.Background(), deniedCaller, corecontract.RealmContactUpdate{
		OperationID: "contact-move-denied", Owner: f.owner, Realm: f.actor.Realm,
		Expected: relocated.Contact.Revision, NextHop: spare}); err == nil {
		t.Fatal("revoked maintenance update committed")
	}
	closeMaintenance()
	realmProcess.gracefulStop()
	if err = f.grant(corecontract.PermissionRealmContactRead, values.AuthorityScope(f.owner), false); err != nil {
		t.Fatal(err)
	}
	if err = f.grant(corecontract.PermissionRealmContactUpdate, values.AuthorityScope(f.owner), false); err != nil {
		t.Fatal(err)
	}
	realmProcess = f.relaunchRealm(t, realmProcess, third)
	maintenance, freshCaller, closeMaintenance := f.openMaintenance(t, third)
	confirmed, err := maintenance.Inspect(context.Background(), freshCaller, third)
	if err != nil || confirmed.NextHop != third || confirmed.Revision != relocated.Contact.Revision {
		t.Fatalf("denied update changed the contact: %v %v", confirmed, err)
	}
	closeMaintenance()

	// The full product chain serves through the repaired contact.
	coreProcess = f.relaunchCore(t, coreProcess, third)
	final := f.connect(t)
	if outcome, readErr := final.Core().ReadElement(ctx, &corepb.ReadElementRequest{
		Handle: f.enter(t, final, corepb.Intent_READ), Address: &networkpb.Address{}, PrepareObserve: proto.Bool(false)}); readErr != nil || outcome.GetResult().GetRecord() == nil {
		t.Fatalf("read through the repaired contact: %v %v", outcome, readErr)
	}
}

// holdProxy relays the raw TLS bytes of one pinned Realm address to the real
// Realm service. Arming makes the relay hold the first request bytes seen
// after that point: those bytes are the exact barrier proving the Core's
// write is waiting on its online authorization, and nothing reaches the
// Realm until the test releases the hold.
type holdProxy struct {
	listener net.Listener
	target   string
	mu       sync.Mutex
	armed    bool
	holding  bool
	released bool
	seen     chan struct{}
	release  chan struct{}
}

func newHoldProxy(t *testing.T, target values.NextHop) *holdProxy {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	proxy := &holdProxy{listener: listener, target: addressHostPort(target),
		seen: make(chan struct{}), release: make(chan struct{})}
	go proxy.serve()
	t.Cleanup(func() {
		_ = listener.Close()
		proxy.stopHolding()
	})
	return proxy
}

func (p *holdProxy) address() values.NextHop {
	return values.NextHop("https://" + p.listener.Addr().String())
}

func (p *holdProxy) arm() {
	p.mu.Lock()
	p.armed = true
	p.mu.Unlock()
}

// stopHolding releases the barrier exactly once; it is safe from cleanup.
func (p *holdProxy) stopHolding() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.released {
		p.released = true
		close(p.release)
	}
}

func (p *holdProxy) serve() {
	for {
		conn, err := p.listener.Accept()
		if err != nil {
			return
		}
		go p.relay(conn)
	}
}

func (p *holdProxy) relay(client net.Conn) {
	target, err := net.Dial("tcp", p.target)
	if err != nil {
		_ = client.Close()
		return
	}
	go func() {
		_, _ = io.Copy(client, target)
		_ = client.Close()
	}()
	p.pump(client, target)
}

// pump forwards request bytes to the Realm and, once armed, holds the first
// batch: the hold is the barrier, and the batch moves only on release.
func (p *holdProxy) pump(client, target net.Conn) {
	buf := make([]byte, 32<<10)
	for {
		n, err := client.Read(buf)
		if n > 0 {
			if p.tryHold() {
				close(p.seen)
				<-p.release
			}
			if _, werr := target.Write(buf[:n]); werr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

func (p *holdProxy) tryHold() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.armed || p.holding {
		return false
	}
	p.holding = true
	return true
}

// TestNetworkRealmContactProductLockProof proves with an exact barrier that a
// write waiting on its online Realm authorization never holds the original
// fact lock: a byte-level relay holds the request bytes after they leave the
// Core, and while the write is provably waiting, a Close that needs the fact
// lock succeeds immediately. Releasing the barrier lets the write commit, so
// the wait was real and the lock stayed free the whole time.
func TestNetworkRealmContactProductLockProof(t *testing.T) {
	f := newNetworkRealmContactFixture(t)
	f.startRealm()
	coreProcess := f.startCore()
	ctx := context.Background()

	// Move the persisted contact to the relay address through the real
	// maintenance path; the relay forwards the pinned TLS bytes to the real
	// Realm, so the SPKI pin still matches and open proves the address
	// online through the relay.
	proxy := newHoldProxy(t, f.realmAddress)
	coreProcess.gracefulStop()
	maintenance, caller, closeMaintenance := f.openMaintenance(t, f.realmAddress)
	contact, err := maintenance.Inspect(context.Background(), caller, "")
	if err != nil || contact.NextHop != f.realmAddress {
		t.Fatalf("inspect current contact: %v %v", contact, err)
	}
	moved, err := maintenance.Update(context.Background(), caller, corecontract.RealmContactUpdate{
		OperationID: "contact-move-proxy", Owner: f.owner, Realm: f.actor.Realm,
		Expected: contact.Revision, NextHop: proxy.address()})
	if err != nil || moved.Decision != corecontract.RealmContactCommitted || moved.Contact.NextHop != proxy.address() {
		t.Fatalf("move contact to the relay: %v %v", moved, err)
	}
	closeMaintenance()
	coreProcess = f.relaunchCore(t, coreProcess, proxy.address())
	client := f.connect(t)
	writer := f.enter(t, client, corepb.Intent_CREATE_ANCHOR)
	probe := f.enter(t, client, corepb.Intent_READ)

	// Arm, then start one write. Its authorization check leaves the Core as
	// TLS bytes; the relay holds them, so the write is provably waiting on
	// the Realm and cannot be anywhere near a commit.
	proxy.arm()
	done := make(chan struct{})
	var write *corepb.CreateAnchorResponse
	var writeErr error
	go func() {
		defer close(done)
		write, writeErr = client.Core().CreateAnchor(ctx, &corepb.CreateAnchorRequest{
			Handle: writer, Element: proto.String(networkContactChild),
			OperationId: []byte("lock-proof-write"), Final: networkContactTuple(10)})
	}()
	select {
	case <-proxy.seen:
	case <-time.After(10 * time.Second):
		t.Fatal("the write never reached its online authorization")
	}
	select {
	case <-done:
		t.Fatal("the write finished while its authorization was held")
	default:
	}

	// The probe Close needs only the fact lock: it completes while the write
	// is still held, proving the Core holds no fact lock while it waits.
	closed, closeErr := client.Core().Close(ctx, &corepb.CloseRequest{Handle: probe})
	if closeErr != nil || closed.GetResult() == nil || !closed.GetResult().GetClosed() {
		t.Fatalf("close while the write waits online: %v %v", closed, closeErr)
	}
	select {
	case <-done:
		t.Fatal("the write finished while its authorization was held")
	default:
	}

	// Releasing the barrier lets the held bytes through; the write commits,
	// proving the hold was a real wait rather than a fast failure.
	proxy.stopHolding()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the write never finished after the barrier was released")
	}
	if writeErr != nil || write.GetResult() == nil {
		t.Fatalf("write after the barrier was released: %v %v", write, writeErr)
	}
}

// recoverNoCommit polls the public outcome recovery for one operation id and
// fails as soon as any committed result is disclosed.
func recoverNoCommit(t *testing.T, client *coresdk.Client, f *networkRealmContactFixture, operation string) {
	t.Helper()
	poll, cancelPoll := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelPoll()
	for poll.Err() == nil {
		outcome, err := client.Core().RecoverOutcome(poll, &corepb.RecoverOutcomeRequest{Reference: f.reference(),
			OperationId: []byte(operation),
			Request: &corepb.MutationCommand{Command: &corepb.MutationCommand_CreateAnchor{CreateAnchor: &corepb.CreateAnchorCommand{
				Element: proto.String(networkContactChild), OperationId: []byte(operation), Final: networkContactTuple(10)}}}})
		if err == nil && outcome.GetResult() != nil {
			t.Fatalf("operation %q committed although its request was already dead: %v", operation, outcome.GetResult())
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// readNoFact proves through the public read face that one operation left no
// element behind: a serving chain answers a missing element as not found, and
// any disclosed record is a commit that must not exist.
func readNoFact(t *testing.T, client *coresdk.Client, f *networkRealmContactFixture, handle *networkpb.Token) {
	t.Helper()
	outcome, readErr := client.Core().ReadElement(context.Background(), &corepb.ReadElementRequest{Handle: handle,
		Address: &networkpb.Address{Down: &networkpb.ElementPath{Items: []string{networkContactChild}}}, PrepareObserve: proto.Bool(false)})
	if outcome.GetResult() != nil || (readErr == nil && outcome.GetFailure() != networkpb.Failure_NOT_FOUND) {
		t.Fatalf("a dead request left a fact behind: %v %v", outcome, readErr)
	}
}

// More completed requests than the Realm's 64-handle budget must succeed
// within one 600-second retention window. Live read evidence remains usable
// until Close; unused per-operation evidence must not accumulate until TTL.
func TestNetworkRealmContactCompletedRequestsReleaseEvidence(t *testing.T) {
	f := newNetworkRealmContactFixture(t)
	f.startRealm()
	f.startCore()
	client := f.connect(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	writer := f.enter(t, client, corepb.Intent_CREATE_ANCHOR)
	op := []byte("resource-recovery-write")
	command := &corepb.MutationCommand{Command: &corepb.MutationCommand_CreateAnchor{CreateAnchor: &corepb.CreateAnchorCommand{
		Element: proto.String(networkContactChild), OperationId: op, Final: networkContactTuple(10)}}}
	committed, err := client.Core().CreateAnchor(ctx, &corepb.CreateAnchorRequest{
		Handle: writer, Element: proto.String(networkContactChild), OperationId: op, Final: networkContactTuple(10)})
	if err != nil || committed.GetResult() == nil {
		t.Fatalf("initial commit = %v %v", committed, err)
	}
	for i := 0; i < 128; i++ {
		outcome, err := client.Core().RecoverOutcome(ctx, &corepb.RecoverOutcomeRequest{Reference: f.reference(), OperationId: op, Request: command})
		if err != nil || !proto.Equal(committed.GetResult(), outcome.GetResult()) {
			t.Fatalf("recovery %d exhausted evidence or changed original result: %v %v", i, outcome, err)
		}
	}
	for i := 0; i < 40; i++ {
		reader := f.enter(t, client, corepb.Intent_READ)
		outcome, err := client.Core().ReadElement(ctx, &corepb.ReadElementRequest{
			Handle: reader, Address: &networkpb.Address{}, PrepareObserve: proto.Bool(false)})
		if err != nil || outcome.GetResult() == nil {
			t.Fatalf("live read %d prematurely released evidence: %v %v", i, outcome, err)
		}
		closed, err := client.Core().Close(ctx, &corepb.CloseRequest{Handle: reader})
		if err != nil || closed.GetResult() == nil {
			t.Fatalf("close read %d = %v %v", i, closed, err)
		}
	}
}

// TestNetworkRealmContactExpiredAuthorizationWaitDoesNotCommit proves the
// original request's deadline governs the whole operation, not just the
// client call: the byte-level relay holds the write's authorization bytes
// past the client deadline, so at the expiry moment no Realm decision and no
// commit point exist yet. Releasing the barrier afterwards must not
// resurrect the dead request — no committed outcome is disclosed and no fact
// exists — while the same chain then commits a fresh request normally and
// discloses that committed outcome idempotently.
func TestNetworkRealmContactExpiredAuthorizationWaitDoesNotCommit(t *testing.T) {
	f := newNetworkRealmContactFixture(t)
	f.startRealm()
	coreProcess := f.startCore()
	ctx := context.Background()

	proxy := newHoldProxy(t, f.realmAddress)
	coreProcess.gracefulStop()
	maintenance, caller, closeMaintenance := f.openMaintenance(t, f.realmAddress)
	contact, err := maintenance.Inspect(ctx, caller, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = maintenance.Update(ctx, caller, corecontract.RealmContactUpdate{
		OperationID: "contact-move-expired-wait", Owner: f.owner, Realm: f.actor.Realm,
		Expected: contact.Revision, NextHop: proxy.address()}); err != nil {
		t.Fatal(err)
	}
	closeMaintenance()
	coreProcess = f.relaunchCore(t, coreProcess, proxy.address())
	client := f.connect(t)
	writer := f.enter(t, client, corepb.Intent_CREATE_ANCHOR)
	reader := f.enter(t, client, corepb.Intent_READ)

	// The held bytes are exactly this write's authorization request; the
	// deadline expires while they are still held.
	proxy.arm()
	expired, cancelExpired := context.WithTimeout(ctx, time.Second)
	defer cancelExpired()
	done := make(chan error, 1)
	go func() {
		_, err := client.Core().CreateAnchor(expired, &corepb.CreateAnchorRequest{
			Handle: writer, Element: proto.String(networkContactChild),
			OperationId: []byte("expired-wait-write"), Final: networkContactTuple(10)})
		done <- err
	}()
	select {
	case <-proxy.seen:
	case <-time.After(10 * time.Second):
		t.Fatal("the write never reached its online authorization")
	}
	select {
	case err := <-done:
		if status.Code(err) != codes.DeadlineExceeded {
			t.Fatalf("held write = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the client deadline was not observed")
	}

	// The deadline expired while the authorization bytes were still held: no
	// Realm decision and no commit had been reached. Releasing them now must
	// not commit the dead request.
	proxy.stopHolding()
	recoverNoCommit(t, client, f, "expired-wait-write")
	readNoFact(t, client, f, reader)

	// The same chain still commits a fresh request and discloses its
	// committed outcome idempotently, so the earlier refusal was the
	// request's own deadline and not a broken chain.
	committed, err := client.Core().CreateAnchor(ctx, &corepb.CreateAnchorRequest{
		Handle:  f.enter(t, client, corepb.Intent_CREATE_ANCHOR),
		Element: proto.String(networkContactChild), OperationId: []byte("expired-wait-write"), Final: networkContactTuple(10)})
	if err != nil || committed.GetResult() == nil {
		t.Fatalf("fresh write through the same chain: %v %v", committed, err)
	}
	again, err := client.Core().RecoverOutcome(ctx, &corepb.RecoverOutcomeRequest{Reference: f.reference(),
		OperationId: []byte("expired-wait-write"),
		Request: &corepb.MutationCommand{Command: &corepb.MutationCommand_CreateAnchor{CreateAnchor: &corepb.CreateAnchorCommand{
			Element: proto.String(networkContactChild), OperationId: []byte("expired-wait-write"), Final: networkContactTuple(10)}}}})
	if err != nil || !proto.Equal(committed.GetResult(), again.GetResult()) {
		t.Fatalf("committed outcome recovery: %v %v", again, err)
	}
}

// TestNetworkRealmContactCanceledAuthorizationWaitDoesNotCommit proves an
// explicit cancellation during the online authorization wait reaches the
// same boundary as a deadline: the caller cancels while the authorization
// bytes are held, and the release afterwards discloses no committed outcome
// and leaves no fact behind.
func TestNetworkRealmContactCanceledAuthorizationWaitDoesNotCommit(t *testing.T) {
	f := newNetworkRealmContactFixture(t)
	f.startRealm()
	coreProcess := f.startCore()
	ctx := context.Background()

	proxy := newHoldProxy(t, f.realmAddress)
	coreProcess.gracefulStop()
	maintenance, caller, closeMaintenance := f.openMaintenance(t, f.realmAddress)
	contact, err := maintenance.Inspect(ctx, caller, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = maintenance.Update(ctx, caller, corecontract.RealmContactUpdate{
		OperationID: "contact-move-canceled-wait", Owner: f.owner, Realm: f.actor.Realm,
		Expected: contact.Revision, NextHop: proxy.address()}); err != nil {
		t.Fatal(err)
	}
	closeMaintenance()
	coreProcess = f.relaunchCore(t, coreProcess, proxy.address())
	client := f.connect(t)
	writer := f.enter(t, client, corepb.Intent_CREATE_ANCHOR)
	reader := f.enter(t, client, corepb.Intent_READ)

	proxy.arm()
	canceled, cancelWrite := context.WithCancel(ctx)
	defer cancelWrite()
	done := make(chan error, 1)
	go func() {
		_, err := client.Core().CreateAnchor(canceled, &corepb.CreateAnchorRequest{
			Handle: writer, Element: proto.String(networkContactChild),
			OperationId: []byte("canceled-wait-write"), Final: networkContactTuple(10)})
		done <- err
	}()
	select {
	case <-proxy.seen:
	case <-time.After(10 * time.Second):
		t.Fatal("the write never reached its online authorization")
	}
	cancelWrite()
	select {
	case err := <-done:
		if status.Code(err) != codes.Canceled {
			t.Fatalf("canceled write = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the cancellation was not observed")
	}

	// The request was canceled before any Realm decision existed; releasing
	// the held bytes afterwards must not commit it.
	proxy.stopHolding()
	recoverNoCommit(t, client, f, "canceled-wait-write")
	readNoFact(t, client, f, reader)
}
