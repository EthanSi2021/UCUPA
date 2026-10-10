package namespace

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ucupa/shared/values"
	"ucupa/ucupa-facet/contract"
)

// The recovery exercise serves one Core address: the same Core that accepted
// the original relationship, revoked it locally, and now proposes the
// recovery. A successor read never happens — the Core's own DETACHED
// disclosure is the revocation evidence.

const recoveryFacet values.FactAuthorityRef = "00000000-0000-4000-8000-00000000ee01"

type recoverySource struct {
	mu       sync.Mutex
	current  contract.CoreEvidence
	resolves map[contract.PublicAcceptanceRef]contract.CoreProposalRead
	offline  bool
	calls    atomic.Int64
}

func (s *recoverySource) read(ctx context.Context, k values.FacetAcceptanceKey) (contract.CoreEvidence, error) {
	s.calls.Add(1)
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Err() != nil || s.offline || s.current.Authority != k.Core {
		return contract.CoreEvidence{}, contract.ErrUnavailable
	}
	return s.current, nil
}
func (s *recoverySource) ReadProposal(ctx context.Context, _ values.NextHop, k values.FacetAcceptanceKey) (contract.CoreEvidence, error) {
	return s.read(ctx, k)
}
func (s *recoverySource) ReadAttachment(ctx context.Context, _ values.NextHop, k values.FacetAcceptanceKey) (contract.CoreEvidence, error) {
	e, err := s.read(ctx, k)
	e.Pending = values.FacetAcceptanceKey{}
	return e, err
}
func (s *recoverySource) ReadSuccessor(context.Context, values.NextHop, values.FactAuthorityRef, values.ElementID) (contract.CoreSuccessorRead, error) {
	panic("recovery must not read a successor statement")
}
func (s *recoverySource) ResolveProposal(ctx context.Context, _ values.NextHop, ref contract.PublicAcceptanceRef) (contract.CoreProposalRead, error) {
	s.calls.Add(1)
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Err() != nil || s.offline {
		return contract.CoreProposalRead{}, contract.ErrUnavailable
	}
	read, ok := s.resolves[ref]
	if !ok || read.Key.Core != ref.Core || read.Key.Element != ref.Element || read.Proposal != ref.Proposal {
		return contract.CoreProposalRead{}, contract.ErrUnavailable
	}
	return read, nil
}

// newRecoveryFixture establishes the original relationship and its local
// revocation: the Facet accepts one key, stores content under it, and the
// same Core then discloses the key DETACHED with no transfer predecessor.
func newRecoveryFixture(t *testing.T, durable bool) (*fixture, *recoverySource, contract.MutationResult) {
	t.Helper()
	x := &fixture{auth: &testAuthorization{}, typ: fixtureType("example")}
	source := &recoverySource{resolves: map[contract.PublicAcceptanceRef]contract.CoreProposalRead{}}
	var err error
	if durable {
		x.path = filepath.Join(t.TempDir(), "facet.db")
		x.f, err = CreateDurable(x.auth, source, testInitial, x.path, nil, nil)
	} else {
		x.f, err = New(x.auth, source, testInitial, nil)
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = Close(x.f) })
	x.key = testKey(x.f.AuthorityRef(), testCore, testElement, 2)
	proposed := testProof(x.key, contract.CoreNone, 2)
	proposed.Pending = x.key
	source.current = proposed
	x.accepted, err = x.f.AcceptAttachment(context.Background(), testActor, x.key, "local:core", "accept")
	if err != nil {
		t.Fatal(err)
	}
	source.current = testProof(x.key, contract.CoreActive, 3)
	initial := x.put(t, "initial-put", "payload", contract.ExpectAbsent())
	source.current = testProof(x.key, contract.CoreDetached, 4)
	return x, source, initial
}

// recoveryEvidence is the same Core's answer after its local detach and new
// recovery proposal: DETACHED with the revoked key of the same Core as the
// predecessor and one pending proposal addressing the same Facet.
func recoveryEvidence(x *fixture, serial uint64) (values.FacetAcceptanceKey, contract.CoreEvidence) {
	newKey := testKey(x.key.Facet, x.key.Core, x.key.Element, serial)
	evidence := testProof(newKey, contract.CoreDetached, serial)
	evidence.Active = x.key
	evidence.Pending = newKey
	return newKey, evidence
}

func recoveryProfiles(t *testing.T, run func(*testing.T, *fixture, *recoverySource, contract.MutationResult)) {
	t.Helper()
	for _, d := range []bool{false, true} {
		name := "memory"
		if d {
			name = "durable"
		}
		t.Run(name, func(t *testing.T) {
			x, source, initial := newRecoveryFixture(t, d)
			run(t, x, source, initial)
		})
	}
}

func TestRecoveryRebindsLineageWithoutPayloadCopy(t *testing.T) {
	recoveryProfiles(t, func(t *testing.T, x *fixture, source *recoverySource, initial contract.MutationResult) {
		ctx := context.Background()
		// The revoked relationship keeps its stored content but loses its
		// write and query basis: the Core's DETACHED disclosure refuses both
		// as a recorded decision.
		_, err := x.f.Put(ctx, testActor, x.key, x.typ, fixturePayload("forged"), contract.ExpectAbsent(), "dead-write")
		mustError(t, err, contract.ErrNamespaceDetached)
		if _, ok := x.f.operations[byteString("dead-write")]; !ok {
			t.Fatal("persistent dead-write refusal missing from journal")
		}
		if _, err = x.f.Get(ctx, testActor, x.key, x.typ); !errors.Is(err, contract.ErrNamespaceDetached) {
			t.Fatalf("revoked content still readable: %v", err)
		}
		newKey, evidence := recoveryEvidence(x, 6)
		source.current = evidence
		result, err := x.f.AcceptAttachment(ctx, testActor, newKey, "local:core", "recover")
		if err != nil || result.Status != contract.Committed {
			t.Fatalf("recovery acceptance: %+v %v", result, err)
		}
		// The committed operation replays its original answer without another
		// Core read.
		calls := source.calls.Load()
		replayed, err := x.f.AcceptAttachment(ctx, testActor, newKey, "local:core", "recover")
		if err != nil || replayed.Status != contract.Committed || source.calls.Load() != calls {
			t.Fatalf("recovery replay: %+v %v", replayed, err)
		}
		// The Core settles the recovered relationship; the payload stayed in
		// this Facet's records and only the acceptance lineage moved.
		source.current = testProof(newKey, contract.CoreActive, 7)
		stored, err := x.f.Get(ctx, testActor, newKey, x.typ)
		if err != nil || string(stored.Payload) != string(fixturePayload("payload")) ||
			stored.Revision != initial.Revision || stored.Acceptance.Key != newKey {
			t.Fatalf("payload moved or changed: %+v %v", stored, err)
		}
		read, err := x.f.ReadAcceptance(testActor, newKey)
		if err != nil || !read.Found || read.CoreNextHop != "local:core" {
			t.Fatalf("recovered lineage missing: %+v %v", read, err)
		}
		previous, err := x.f.ReadAcceptance(testActor, x.key)
		if err != nil || previous.Found {
			t.Fatalf("revoked lineage kept: %+v %v", previous, err)
		}
		// The recovered acceptance is the only writable lineage; the revoked
		// key no longer resolves to an accepted relation.
		_, err = x.f.Put(ctx, testActor, x.key, x.typ, fixturePayload("forged"), contract.ExpectAbsent(), "stale-write")
		mustError(t, err, contract.ErrUnavailable)
		if _, err = x.f.Put(ctx, testActor, newKey, x.typ, fixturePayload("next"), contract.ExpectRevision(initial.Revision), "rebound-write"); err != nil {
			t.Fatal(err)
		}
	})
}

// TestRecoveryPredecessorNotAcceptedRejected pins the content evidence: the
// Core may name a revoked predecessor this Facet never accepted — its
// records were replaced or retired — and the recovery must refuse
// persistently without touching the stored lineage.
func TestRecoveryPredecessorNotAcceptedRejected(t *testing.T) {
	recoveryProfiles(t, func(t *testing.T, x *fixture, source *recoverySource, _ contract.MutationResult) {
		ctx := context.Background()
		newKey, evidence := recoveryEvidence(x, 6)
		stranger := testKey(x.key.Facet, x.key.Core, x.key.Element, 9)
		evidence.Active = stranger
		source.current = evidence
		_, err := x.f.AcceptAttachment(ctx, testActor, newKey, "local:core", "recover")
		mustError(t, err, contract.ErrAcceptanceNotFound)
		if _, ok := x.f.operations[byteString("recover")]; !ok {
			t.Fatal("persistent rejection missing from journal")
		}
		calls := source.calls.Load()
		_, err = x.f.AcceptAttachment(ctx, testActor, newKey, "local:core", "recover")
		mustError(t, err, contract.ErrAcceptanceNotFound)
		if source.calls.Load() != calls {
			t.Fatal("replayed rejection consulted the Core")
		}
		read, err := x.f.ReadAcceptance(testActor, x.key)
		if err != nil || !read.Found {
			t.Fatalf("rejected recovery lost the stored lineage: %+v %v", read, err)
		}
		previous, err := x.f.ReadAcceptance(testActor, newKey)
		if err != nil || previous.Found {
			t.Fatalf("unproven lineage accepted: %+v %v", previous, err)
		}
	})
}

func TestRecoveryCoreUnreachableRetries(t *testing.T) {
	recoveryProfiles(t, func(t *testing.T, x *fixture, source *recoverySource, _ contract.MutationResult) {
		ctx := context.Background()
		newKey, evidence := recoveryEvidence(x, 6)
		source.current = evidence
		source.offline = true
		_, err := x.f.AcceptAttachment(ctx, testActor, newKey, "local:core", "recover")
		mustError(t, err, contract.ErrUnavailable)
		if _, ok := x.f.operations[byteString("recover")]; ok {
			t.Fatal("temporary failure journalled")
		}
		source.offline = false
		result, err := x.f.AcceptAttachment(ctx, testActor, newKey, "local:core", "recover")
		if err != nil || result.Status != contract.Committed {
			t.Fatalf("retried recovery: %+v %v", result, err)
		}
		read, err := x.f.ReadAcceptance(testActor, newKey)
		if err != nil || !read.Found {
			t.Fatalf("retried acceptance missing: %+v %v", read, err)
		}
	})
}

func TestRecoveryUnknownShapeStaysTemporary(t *testing.T) {
	x, source, _ := newRecoveryFixture(t, false)
	ctx := context.Background()
	newKey, evidence := recoveryEvidence(x, 6)
	// A predecessor naming another Facet is outside every known shape; the
	// attempt stays temporary and the stored lineage is kept.
	evidence.Active = testKey(recoveryFacet, x.key.Core, x.key.Element, 2)
	source.current = evidence
	_, err := x.f.AcceptAttachment(ctx, testActor, newKey, "local:core", "recover")
	mustError(t, err, contract.ErrUnavailable)
	if _, ok := x.f.operations[byteString("recover")]; ok {
		t.Fatal("shapeless evidence journalled")
	}
	read, err := x.f.ReadAcceptance(testActor, x.key)
	if err != nil || !read.Found {
		t.Fatalf("shapeless attempt lost the stored lineage: %+v %v", read, err)
	}
}

func TestRecoveryLostResponseRecoversOnReopen(t *testing.T) {
	x, source, _ := newRecoveryFixture(t, true)
	ctx := context.Background()
	newKey, evidence := recoveryEvidence(x, 6)
	source.current = evidence
	x.f.store.loseResponse = func() error { return errors.New("lost") }
	_, err := x.f.AcceptAttachment(ctx, testActor, newKey, "local:core", "recover")
	mustError(t, err, contract.ErrIndeterminate)
	if err = Close(x.f); err != nil {
		t.Fatal(err)
	}
	x.f, err = OpenDurable(x.auth, source, x.path, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	read, err := x.f.ReadAcceptance(testActor, newKey)
	if err != nil || !read.Found {
		t.Fatalf("replayed recovery missing: %+v %v", read, err)
	}
	previous, err := x.f.ReadAcceptance(testActor, x.key)
	if err != nil || previous.Found {
		t.Fatalf("revoked lineage kept after replay: %+v %v", previous, err)
	}
	// The committed operation replays its original answer without the Core.
	calls := source.calls.Load()
	result, err := x.f.AcceptAttachment(ctx, testActor, newKey, "local:core", "recover")
	if err != nil || result.Status != contract.Committed || source.calls.Load() != calls {
		t.Fatalf("replay: %+v %v", result, err)
	}
}

// newRecoveryControlFixture establishes the original acceptance through the
// public control surface itself, so the journal keeps the original public
// binding for the locally revoked key.
func newRecoveryControlFixture(t *testing.T, durable bool) (*Owner, *recoverySource, values.CallerContext, values.FacetAcceptanceKey, contract.PublicAcceptanceRef) {
	t.Helper()
	auth := &acceptanceViewAuth{}
	auth.decision.Store(uint32(values.AuthorizationValid))
	auth.version.Store(1)
	source := &recoverySource{resolves: map[contract.PublicAcceptanceRef]contract.CoreProposalRead{}}
	config := &values.PublicAccessPolicy{ContextTTL: time.Minute, ObservationTTL: time.Minute}
	var f *Owner
	var err error
	if durable {
		f, err = CreateDurable(auth, source, testInitial, filepath.Join(t.TempDir(), "facet.db"), config, nil)
	} else {
		f, err = New(auth, source, testInitial, config)
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = Close(f) })
	caller := values.CallerContext{Subject: testActor, Credential: [16]byte{1}, Proof: [32]byte{1}}
	key := testKey(f.AuthorityRef(), testCore, testElement, 2)
	token, _ := values.NewPublicToken(testCore, "attachment-revision")
	ref := contract.PublicAcceptanceRef{Core: testCore, Element: testElement, Proposal: token}
	proposed := testProof(key, contract.CoreNone, 2)
	proposed.Pending = key
	source.resolves[ref] = contract.CoreProposalRead{Key: key, Evidence: proposed, Proposal: token}
	source.current = proposed
	if _, err = f.AcceptAttachmentControl(context.Background(), caller, ref, "local:core", "control-accept"); err != nil {
		t.Fatal(err)
	}
	source.current = testProof(key, contract.CoreDetached, 4)
	return f, source, caller, key, ref
}

func recoveryControlProfiles(t *testing.T, run func(*testing.T, *Owner, *recoverySource, values.CallerContext, values.FacetAcceptanceKey, contract.PublicAcceptanceRef)) {
	t.Helper()
	for _, d := range []bool{false, true} {
		name := "memory"
		if d {
			name = "durable"
		}
		t.Run(name, func(t *testing.T) {
			f, source, caller, key, ref := newRecoveryControlFixture(t, d)
			run(t, f, source, caller, key, ref)
		})
	}
}

// controlRecovery wires the same Core's public recovery proposal: the
// DETACHED disclosure of the revoked key next to the new pending proposal.
func controlRecovery(source *recoverySource, key values.FacetAcceptanceKey) contract.PublicAcceptanceRef {
	newKey := testKey(key.Facet, key.Core, key.Element, 6)
	evidence := testProof(newKey, contract.CoreDetached, 6)
	evidence.Active = key
	evidence.Pending = newKey
	token, _ := values.NewPublicToken(key.Core, "attachment-revision")
	ref := contract.PublicAcceptanceRef{Core: key.Core, Element: key.Element, Proposal: token}
	source.resolves[ref] = contract.CoreProposalRead{Key: newKey, Evidence: evidence, Proposal: token}
	source.current = evidence
	return ref
}

func TestRecoveryPublicControlRebindsLineage(t *testing.T) {
	recoveryControlProfiles(t, func(t *testing.T, f *Owner, source *recoverySource, caller values.CallerContext, key values.FacetAcceptanceKey, initialRef contract.PublicAcceptanceRef) {
		ctx := context.Background()
		// Evidence outside every known shape fails before the journal.
		shapelessToken, _ := values.NewPublicToken(testCore, "attachment-revision")
		shapelessRef := contract.PublicAcceptanceRef{Core: testCore, Element: key.Element, Proposal: shapelessToken}
		source.resolves[shapelessRef] = contract.CoreProposalRead{Key: testKey(f.AuthorityRef(), testCore, key.Element, 9), Proposal: shapelessToken}
		if _, err := f.AcceptAttachmentControl(ctx, caller, shapelessRef, "local:core", "control-shapeless"); !errors.Is(err, values.PublicUnavailable) {
			t.Fatalf("shapeless evidence accepted: %v", err)
		}
		if _, ok := f.operations[byteString("control-shapeless")]; ok {
			t.Fatal("shapeless refusal journalled")
		}
		if previous, err := f.InspectAcceptanceControl(context.Background(), caller, initialRef); err != nil || !previous.Found || previous.CoreNextHop != "local:core" {
			t.Fatalf("previous lineage lost: %+v %v", previous, err)
		}
		// The unreachable Core keeps the public attempt temporary.
		ref := controlRecovery(source, key)
		source.offline = true
		if _, err := f.AcceptAttachmentControl(ctx, caller, ref, "local:core", "control-recover"); !errors.Is(err, values.PublicUnavailable) {
			t.Fatalf("unreachable recovery accepted: %v", err)
		}
		if _, ok := f.operations[byteString("control-recover")]; ok {
			t.Fatal("temporary refusal journalled")
		}
		// Once the Core answers, the same public operation completes and
		// replaces the lineage in one transition.
		source.offline = false
		out, err := f.AcceptAttachmentControl(ctx, caller, ref, "local:core", "control-recover")
		if err != nil || out.Status != contract.Committed || !out.Inspection.Found || out.Inspection.CoreNextHop != "local:core" {
			t.Fatalf("public recovery: %+v %v", out, err)
		}
		// The exact original outcome replays without another Core call.
		calls := source.calls.Load()
		replayed, err := f.AcceptAttachmentControl(ctx, caller, ref, "local:core", "control-recover")
		if err != nil || replayed.Status != contract.Committed || source.calls.Load() != calls {
			t.Fatalf("public replay: %+v %v", replayed, err)
		}
		// The superseded selector no longer names an accepted lineage.
		if previous, err := f.InspectAcceptanceControl(context.Background(), caller, initialRef); err != nil || previous.Found {
			t.Fatalf("superseded selector kept: %+v %v", previous, err)
		}
		stored, err := f.InspectAcceptanceControl(context.Background(), caller, ref)
		if err != nil || !stored.Found || stored.CoreNextHop != "local:core" {
			t.Fatalf("new lineage missing: %+v %v", stored, err)
		}
	})
}

func TestRecoveryPublicControlWithoutRecordRejected(t *testing.T) {
	recoveryControlProfiles(t, func(t *testing.T, f *Owner, source *recoverySource, caller values.CallerContext, key values.FacetAcceptanceKey, initialRef contract.PublicAcceptanceRef) {
		ctx := context.Background()
		// The Core names a revoked predecessor this Facet never accepted: the
		// refusal is a recorded decision and replays without the Core.
		ref := controlRecovery(source, key)
		stranger := testKey(key.Facet, key.Core, key.Element, 9)
		read := source.resolves[ref]
		read.Evidence.Active = stranger
		source.resolves[ref] = read
		source.current = read.Evidence
		if _, err := f.AcceptAttachmentControl(ctx, caller, ref, "local:core", "control-stranger"); !errors.Is(err, values.PublicUnavailable) {
			t.Fatalf("unproven recovery accepted: %v", err)
		}
		if _, ok := f.operations[byteString("control-stranger")]; !ok {
			t.Fatal("persistent rejection missing from journal")
		}
		calls := source.calls.Load()
		if _, err := f.AcceptAttachmentControl(ctx, caller, ref, "local:core", "control-stranger"); !errors.Is(err, values.PublicUnavailable) || source.calls.Load() != calls {
			t.Fatalf("replayed rejection consulted the Core: %v", err)
		}
		if previous, err := f.InspectAcceptanceControl(context.Background(), caller, initialRef); err != nil || !previous.Found || previous.CoreNextHop != "local:core" {
			t.Fatalf("previous lineage lost: %+v %v", previous, err)
		}
		if stored, err := f.InspectAcceptanceControl(context.Background(), caller, ref); err != nil || stored.Found {
			t.Fatalf("unproven lineage accepted: %+v %v", stored, err)
		}
	})
}
