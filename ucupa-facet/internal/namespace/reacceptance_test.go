package namespace

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ucupa/shared/values"
	"ucupa/ucupa-facet/contract"
)

// The re-acceptance exercise serves two Core addresses: the element's
// previous Core keeps the acceptance's contact, and the transferred-to Core
// exposes the pending proposal that names the same Facet as predecessor.

const reacceptCore values.FactAuthorityRef = "00000000-0000-4000-8000-00000000ca02"

const (
	previousCoreAddress    values.NextHop = "local:previous-core"
	transferredCoreAddress values.NextHop = "local:new-core"
)

type multiCoreSource struct {
	mu         sync.Mutex
	answers    map[values.NextHop]contract.CoreEvidence
	successors map[values.NextHop]contract.CoreSuccessorRead
	resolves   map[contract.PublicAcceptanceRef]resolveAnswer
	calls      atomic.Int64
}

// resolveAnswer pairs one public selector with the Core answer that the
// control surface must receive for it, at the exact Core address.
type resolveAnswer struct {
	address values.NextHop
	read    contract.CoreProposalRead
}

func (s *multiCoreSource) read(address values.NextHop) (contract.CoreEvidence, error) {
	s.calls.Add(1)
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.answers[address]
	if !ok {
		return contract.CoreEvidence{}, contract.ErrUnavailable
	}
	return e, nil
}
func (s *multiCoreSource) ReadProposal(ctx context.Context, a values.NextHop, k values.FacetAcceptanceKey) (contract.CoreEvidence, error) {
	return s.read(a)
}
func (s *multiCoreSource) ReadAttachment(ctx context.Context, a values.NextHop, k values.FacetAcceptanceKey) (contract.CoreEvidence, error) {
	return s.read(a)
}
func (s *multiCoreSource) ReadSuccessor(ctx context.Context, a values.NextHop, core values.FactAuthorityRef, element values.ElementID) (contract.CoreSuccessorRead, error) {
	s.calls.Add(1)
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Err() != nil {
		return contract.CoreSuccessorRead{}, contract.ErrUnavailable
	}
	r, ok := s.successors[a]
	if !ok {
		return contract.CoreSuccessorRead{}, contract.ErrUnavailable
	}
	return r, nil
}
func (s *multiCoreSource) ResolveProposal(ctx context.Context, address values.NextHop, ref contract.PublicAcceptanceRef) (contract.CoreProposalRead, error) {
	s.calls.Add(1)
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Err() != nil {
		return contract.CoreProposalRead{}, contract.ErrUnavailable
	}
	answer, ok := s.resolves[ref]
	if !ok || answer.address != address || answer.read.Key.Core != ref.Core || answer.read.Key.Element != ref.Element || answer.read.Proposal != ref.Proposal {
		return contract.CoreProposalRead{}, contract.ErrUnavailable
	}
	return answer.read, nil
}

func newReacceptanceFixture(t *testing.T, durable bool) (*fixture, *multiCoreSource) {
	t.Helper()
	x := &fixture{auth: &testAuthorization{}, typ: fixtureType("example")}
	source := &multiCoreSource{answers: map[values.NextHop]contract.CoreEvidence{}, successors: map[values.NextHop]contract.CoreSuccessorRead{}}
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
	source.answers[previousCoreAddress] = proposed
	x.accepted, err = x.f.AcceptAttachment(context.Background(), testActor, x.key, previousCoreAddress, "accept")
	if err != nil {
		t.Fatal(err)
	}
	source.answers[previousCoreAddress] = testProof(x.key, contract.CoreActive, 3)
	return x, source
}

// reacceptanceEvidence is the new Core's answer for the transferred element:
// DETACHED with the recorded acceptance as predecessor and one pending
// proposal that addresses the same Facet.
func reacceptanceEvidence(x *fixture, serial uint64) (values.FacetAcceptanceKey, contract.CoreEvidence) {
	newKey := testKey(x.key.Facet, reacceptCore, x.key.Element, serial)
	evidence := testProof(newKey, contract.CoreDetached, serial)
	evidence.Active = x.key
	evidence.Pending = newKey
	return newKey, evidence
}

// departureStatement is the previous Core's durable successor statement: the
// element left that Core for exactly the new Core under one transfer
// operation, fenced at a final cut and settled.
func departureStatement(element values.ElementID, previous, next values.FactAuthorityRef) contract.CoreSuccessorRead {
	return contract.CoreSuccessorRead{Found: true, Authority: previous, Element: element, Operation: "transfer",
		Source: previous, Target: next, FinalCut: 5, Settled: true}
}

func reacceptanceProfiles(t *testing.T, run func(*testing.T, *fixture, *multiCoreSource)) {
	t.Helper()
	for _, d := range []bool{false, true} {
		name := "memory"
		if d {
			name = "durable"
		}
		t.Run(name, func(t *testing.T) {
			x, source := newReacceptanceFixture(t, d)
			run(t, x, source)
		})
	}
}

func TestReacceptanceRebindsLineageWithoutPayloadCopy(t *testing.T) {
	reacceptanceProfiles(t, func(t *testing.T, x *fixture, source *multiCoreSource) {
		ctx := context.Background()
		first := x.put(t, "put", "payload", contract.ExpectAbsent())
		newKey, evidence := reacceptanceEvidence(x, 6)
		source.answers[transferredCoreAddress] = evidence
		source.successors[previousCoreAddress] = departureStatement(x.key.Element, x.key.Core, newKey.Core)
		result, err := x.f.AcceptAttachment(ctx, testActor, newKey, transferredCoreAddress, "reaccept")
		if err != nil || result.Status != contract.Committed {
			t.Fatalf("re-acceptance: %+v %v", result, err)
		}
		read, err := x.f.ReadAcceptance(testActor, newKey)
		if err != nil || !read.Found || read.CoreNextHop != transferredCoreAddress {
			t.Fatalf("new lineage missing: %+v %v", read, err)
		}
		previous, err := x.f.ReadAcceptance(testActor, x.key)
		if err != nil || previous.Found {
			t.Fatalf("previous lineage kept: %+v %v", previous, err)
		}
		// The payload stays in this Facet's records; only the acceptance moved.
		source.answers[transferredCoreAddress] = testProof(newKey, contract.CoreActive, 7)
		stored, err := x.f.Get(ctx, testActor, newKey, x.typ)
		if err != nil || string(stored.Payload) != string(fixturePayload("payload")) ||
			!reflect.DeepEqual(stored.Revision, first.Revision) {
			t.Fatalf("payload moved or changed: %+v %v", stored, err)
		}
		// The new acceptance is the only writable lineage; the previous key
		// no longer resolves to an accepted relation.
		_, err = x.f.Put(ctx, testActor, x.key, x.typ, fixturePayload("forged"), contract.ExpectAbsent(), "stale-write")
		mustError(t, err, contract.ErrUnavailable)
		if _, err = x.f.Put(ctx, testActor, newKey, x.typ, fixturePayload("next"), contract.ExpectRevision(first.Revision), "rebound-write"); err != nil {
			t.Fatal(err)
		}
	})
}

// TestReacceptanceRequiresProvenSuccessorStatement pins the departure proof:
// the previous Core's successor read without a found statement is
// indistinguishable from an ordinary deletion, so without a durable successor
// statement binding the element to the new Core, the lineage must never rebind.
func TestReacceptanceRequiresProvenSuccessorStatement(t *testing.T) {
	reacceptanceProfiles(t, func(t *testing.T, x *fixture, source *multiCoreSource) {
		ctx := context.Background()
		newKey, evidence := reacceptanceEvidence(x, 6)
		source.answers[transferredCoreAddress] = evidence
		// A plain deletion at the previous Core answers without any successor
		// statement; that found=false shape is indistinguishable from an
		// element that never left under a transfer.
		source.successors[previousCoreAddress] = contract.CoreSuccessorRead{Authority: x.key.Core, Element: x.key.Element}
		_, err := x.f.AcceptAttachment(ctx, testActor, newKey, transferredCoreAddress, "reaccept")
		mustError(t, err, contract.ErrNotCurrentOwner)
		if _, ok := x.f.operations[byteString("reaccept")]; !ok {
			t.Fatal("persistent rejection missing from journal")
		}
		read, err := x.f.ReadAcceptance(testActor, x.key)
		if err != nil || !read.Found {
			t.Fatalf("unproven departure lost the previous lineage: %+v %v", read, err)
		}
		previous, err := x.f.ReadAcceptance(testActor, newKey)
		if err != nil || previous.Found {
			t.Fatalf("unproven lineage accepted: %+v %v", previous, err)
		}
	})
}

func TestReacceptancePreviousCoreAliveRejected(t *testing.T) {
	reacceptanceProfiles(t, func(t *testing.T, x *fixture, source *multiCoreSource) {
		ctx := context.Background()
		newKey, evidence := reacceptanceEvidence(x, 6)
		source.answers[transferredCoreAddress] = evidence
		// The previous Core still presents the element as attached to it: no
		// departure statement exists for the element.
		source.answers[previousCoreAddress] = testProof(x.key, contract.CoreActive, 4)
		source.successors[previousCoreAddress] = contract.CoreSuccessorRead{Authority: x.key.Core, Element: x.key.Element}
		calls := source.calls.Load()
		_, err := x.f.AcceptAttachment(ctx, testActor, newKey, transferredCoreAddress, "reaccept")
		mustError(t, err, contract.ErrNotCurrentOwner)
		// The rejection is recorded; replay answers identically without
		// consulting either Core again.
		_, err = x.f.AcceptAttachment(ctx, testActor, newKey, transferredCoreAddress, "reaccept")
		mustError(t, err, contract.ErrNotCurrentOwner)
		if source.calls.Load() != calls+2 {
			t.Fatalf("rejection replayed through sources: %d", source.calls.Load()-calls)
		}
		if _, ok := x.f.operations[byteString("reaccept")]; !ok {
			t.Fatal("persistent rejection missing from journal")
		}
		read, err := x.f.ReadAcceptance(testActor, x.key)
		if err != nil || !read.Found {
			t.Fatalf("failed re-acceptance lost the previous lineage: %+v %v", read, err)
		}
	})
}

func TestReacceptancePreviousCoreUnreachableRetries(t *testing.T) {
	reacceptanceProfiles(t, func(t *testing.T, x *fixture, source *multiCoreSource) {
		ctx := context.Background()
		newKey, evidence := reacceptanceEvidence(x, 6)
		source.answers[transferredCoreAddress] = evidence
		// The previous Core does not answer; the attempt stays temporary.
		delete(source.successors, previousCoreAddress)
		_, err := x.f.AcceptAttachment(ctx, testActor, newKey, transferredCoreAddress, "reaccept")
		mustError(t, err, contract.ErrUnavailable)
		if _, ok := x.f.operations[byteString("reaccept")]; ok {
			t.Fatal("temporary failure journalled")
		}
		// Once the previous Core states the departure, the same operation
		// completes.
		source.successors[previousCoreAddress] = departureStatement(x.key.Element, x.key.Core, newKey.Core)
		result, err := x.f.AcceptAttachment(ctx, testActor, newKey, transferredCoreAddress, "reaccept")
		if err != nil || result.Status != contract.Committed {
			t.Fatalf("retry: %+v %v", result, err)
		}
		read, err := x.f.ReadAcceptance(testActor, newKey)
		if err != nil || !read.Found {
			t.Fatalf("retried acceptance missing: %+v %v", read, err)
		}
	})
}

func TestReacceptancePredecessorNotAcceptedRejected(t *testing.T) {
	reacceptanceProfiles(t, func(t *testing.T, x *fixture, source *multiCoreSource) {
		ctx := context.Background()
		newKey, evidence := reacceptanceEvidence(x, 6)
		// The new Core names a predecessor relation this Facet never accepted.
		stranger := testKey(x.key.Facet, testCore, x.key.Element, 9)
		evidence.Active = stranger
		source.answers[transferredCoreAddress] = evidence
		_, err := x.f.AcceptAttachment(ctx, testActor, newKey, transferredCoreAddress, "reaccept")
		mustError(t, err, contract.ErrAcceptanceNotFound)
		if _, ok := x.f.operations[byteString("reaccept")]; !ok {
			t.Fatal("persistent rejection missing from journal")
		}
		read, err := x.f.ReadAcceptance(testActor, x.key)
		if err != nil || !read.Found {
			t.Fatalf("previous lineage lost: %+v %v", read, err)
		}
		previous, err := x.f.ReadAcceptance(testActor, newKey)
		if err != nil || previous.Found {
			t.Fatalf("unproven lineage accepted: %+v %v", previous, err)
		}
	})
}

func TestReacceptanceUnknownShapeStaysTemporary(t *testing.T) {
	x, source := newReacceptanceFixture(t, false)
	ctx := context.Background()
	newKey := testKey(x.key.Facet, reacceptCore, x.key.Element, 6)
	// Evidence outside every accepted shape fails before the journal.
	source.answers[transferredCoreAddress] = contract.CoreEvidence{}
	_, err := x.f.AcceptAttachment(ctx, testActor, newKey, transferredCoreAddress, "reaccept")
	mustError(t, err, contract.ErrUnavailable)
	if _, ok := x.f.operations[byteString("reaccept")]; ok {
		t.Fatal("shapeless evidence journalled")
	}
	read, err := x.f.ReadAcceptance(testActor, x.key)
	if err != nil || !read.Found {
		t.Fatalf("previous lineage lost: %+v %v", read, err)
	}
}

func TestReacceptanceLostResponseRecoversOnReopen(t *testing.T) {
	x, source := newReacceptanceFixture(t, true)
	ctx := context.Background()
	newKey, evidence := reacceptanceEvidence(x, 6)
	source.answers[transferredCoreAddress] = evidence
	source.successors[previousCoreAddress] = departureStatement(x.key.Element, x.key.Core, newKey.Core)
	x.f.store.loseResponse = func() error { return errors.New("lost") }
	_, err := x.f.AcceptAttachment(ctx, testActor, newKey, transferredCoreAddress, "reaccept")
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
		t.Fatalf("replayed acceptance missing: %+v %v", read, err)
	}
	previous, err := x.f.ReadAcceptance(testActor, x.key)
	if err != nil || previous.Found {
		t.Fatalf("previous lineage kept after replay: %+v %v", previous, err)
	}
	// The committed operation replays its original answer without contacts.
	calls := source.calls.Load()
	result, err := x.f.AcceptAttachment(ctx, testActor, newKey, transferredCoreAddress, "reaccept")
	if err != nil || result.Status != contract.Committed || source.calls.Load() != calls {
		t.Fatalf("replay: %+v %v", result, err)
	}
}

// newReacceptanceControlFixture establishes the first acceptance through the
// public control surface itself, so the journal keeps the original public
// binding while the register holds the previous Core's lineage.
func newReacceptanceControlFixture(t *testing.T, durable bool) (*Owner, *multiCoreSource, values.CallerContext, values.FacetAcceptanceKey, contract.PublicAcceptanceRef) {
	t.Helper()
	auth := &acceptanceViewAuth{}
	auth.decision.Store(uint32(values.AuthorizationValid))
	auth.version.Store(1)
	source := &multiCoreSource{
		answers:    map[values.NextHop]contract.CoreEvidence{},
		successors: map[values.NextHop]contract.CoreSuccessorRead{},
		resolves:   map[contract.PublicAcceptanceRef]resolveAnswer{},
	}
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
	source.resolves[ref] = resolveAnswer{address: previousCoreAddress, read: contract.CoreProposalRead{Key: key, Evidence: proposed, Proposal: token}}
	source.answers[previousCoreAddress] = proposed
	if _, err = f.AcceptAttachmentControl(context.Background(), caller, ref, previousCoreAddress, "control-accept"); err != nil {
		t.Fatal(err)
	}
	return f, source, caller, key, ref
}

func reacceptanceControlProfiles(t *testing.T, run func(*testing.T, *Owner, *multiCoreSource, values.CallerContext, values.FacetAcceptanceKey, contract.PublicAcceptanceRef)) {
	t.Helper()
	for _, d := range []bool{false, true} {
		name := "memory"
		if d {
			name = "durable"
		}
		t.Run(name, func(t *testing.T) {
			f, source, caller, key, ref := newReacceptanceControlFixture(t, d)
			run(t, f, source, caller, key, ref)
		})
	}
}

// controlReacceptance wires the transferred Core's public proposal resolution
// and the previous Core's successor statement.
func controlReacceptance(source *multiCoreSource, key values.FacetAcceptanceKey, departure contract.CoreSuccessorRead) contract.PublicAcceptanceRef {
	newKey := testKey(key.Facet, reacceptCore, key.Element, 6)
	evidence := testProof(newKey, contract.CoreDetached, 6)
	evidence.Active = key
	evidence.Pending = newKey
	token, _ := values.NewPublicToken(reacceptCore, "attachment-revision")
	ref := contract.PublicAcceptanceRef{Core: reacceptCore, Element: key.Element, Proposal: token}
	source.resolves[ref] = resolveAnswer{address: transferredCoreAddress, read: contract.CoreProposalRead{Key: newKey, Evidence: evidence, Proposal: token}}
	source.answers[transferredCoreAddress] = evidence
	source.successors[previousCoreAddress] = departure
	return ref
}

func TestReacceptancePublicControlRebindsLineage(t *testing.T) {
	reacceptanceControlProfiles(t, func(t *testing.T, f *Owner, source *multiCoreSource, caller values.CallerContext, key values.FacetAcceptanceKey, initialRef contract.PublicAcceptanceRef) {
		ctx := context.Background()
		// Evidence outside every known shape fails before the journal.
		shapelessToken, _ := values.NewPublicToken(reacceptCore, "attachment-revision")
		shapelessRef := contract.PublicAcceptanceRef{Core: reacceptCore, Element: key.Element, Proposal: shapelessToken}
		source.resolves[shapelessRef] = resolveAnswer{address: transferredCoreAddress,
			read: contract.CoreProposalRead{Key: testKey(f.AuthorityRef(), reacceptCore, key.Element, 9), Proposal: shapelessToken}}
		if _, err := f.AcceptAttachmentControl(ctx, caller, shapelessRef, transferredCoreAddress, "control-shapeless"); !errors.Is(err, values.PublicUnavailable) {
			t.Fatalf("shapeless evidence accepted: %v", err)
		}
		if _, ok := f.operations[byteString("control-shapeless")]; ok {
			t.Fatal("shapeless refusal journalled")
		}
		if previous, err := f.InspectAcceptanceControl(context.Background(), caller, initialRef); err != nil || !previous.Found || previous.CoreNextHop != previousCoreAddress {
			t.Fatalf("previous lineage lost: %+v %v", previous, err)
		}
		// The unreachable previous Core keeps the public attempt temporary.
		ref := controlReacceptance(source, key, departureStatement(key.Element, key.Core, reacceptCore))
		delete(source.successors, previousCoreAddress)
		if _, err := f.AcceptAttachmentControl(ctx, caller, ref, transferredCoreAddress, "control-reaccept"); !errors.Is(err, values.PublicUnavailable) {
			t.Fatalf("unreachable departure accepted: %v", err)
		}
		if _, ok := f.operations[byteString("control-reaccept")]; ok {
			t.Fatal("temporary refusal journalled")
		}
		// Once the previous Core states the departure, the same public
		// operation completes and replaces the lineage in one transition.
		source.successors[previousCoreAddress] = departureStatement(key.Element, key.Core, reacceptCore)
		out, err := f.AcceptAttachmentControl(ctx, caller, ref, transferredCoreAddress, "control-reaccept")
		if err != nil || out.Status != contract.Committed || !out.Inspection.Found || out.Inspection.CoreNextHop != transferredCoreAddress {
			t.Fatalf("public re-acceptance: %+v %v", out, err)
		}
		// The exact original outcome replays without another Core call.
		calls := source.calls.Load()
		replayed, err := f.AcceptAttachmentControl(ctx, caller, ref, transferredCoreAddress, "control-reaccept")
		if err != nil || replayed.Status != contract.Committed || source.calls.Load() != calls {
			t.Fatalf("public replay: %+v %v", replayed, err)
		}
		// The superseded selector no longer names an accepted lineage.
		if previous, err := f.InspectAcceptanceControl(context.Background(), caller, initialRef); err != nil || previous.Found {
			t.Fatalf("superseded selector kept: %+v %v", previous, err)
		}
		stored, err := f.InspectAcceptanceControl(context.Background(), caller, ref)
		if err != nil || !stored.Found || stored.CoreNextHop != transferredCoreAddress {
			t.Fatalf("new lineage missing: %+v %v", stored, err)
		}
	})
}

func TestReacceptancePublicControlAliveCoreRejectedPersistently(t *testing.T) {
	reacceptanceControlProfiles(t, func(t *testing.T, f *Owner, source *multiCoreSource, caller values.CallerContext, key values.FacetAcceptanceKey, initialRef contract.PublicAcceptanceRef) {
		ctx := context.Background()
		// The previous Core still presents the element as its own attachment:
		// its successor read finds no departure statement.
		ref := controlReacceptance(source, key, contract.CoreSuccessorRead{Authority: key.Core, Element: key.Element})
		if _, err := f.AcceptAttachmentControl(ctx, caller, ref, transferredCoreAddress, "control-alive"); !errors.Is(err, values.PublicUnavailable) {
			t.Fatalf("alive previous Core accepted: %v", err)
		}
		// The refusal is a recorded decision: the exact replay answers
		// identically without consulting either Core again.
		if _, ok := f.operations[byteString("control-alive")]; !ok {
			t.Fatal("persistent rejection missing from journal")
		}
		calls := source.calls.Load()
		if _, err := f.AcceptAttachmentControl(ctx, caller, ref, transferredCoreAddress, "control-alive"); !errors.Is(err, values.PublicUnavailable) || source.calls.Load() != calls {
			t.Fatalf("replayed rejection contacted sources: %v", err)
		}
		if previous, err := f.InspectAcceptanceControl(context.Background(), caller, initialRef); err != nil || !previous.Found || previous.CoreNextHop != previousCoreAddress {
			t.Fatalf("previous lineage lost: %+v %v", previous, err)
		}
	})
}
