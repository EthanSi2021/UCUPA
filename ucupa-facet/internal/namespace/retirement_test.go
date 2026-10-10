package namespace

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-facet/contract"
)

// The retirement exercise pins the explicit disposal of one element's
// content: the controlling Core's DETACHED disclosure naming the held key is
// the only disposal evidence, one transaction records the exact final
// disposition before removing the live namespace, and the retired element can
// never be accepted at this Facet again.

const retireAltCore values.FactAuthorityRef = "00000000-0000-4000-8000-00000000ca02"
const retireOtherElement values.ElementID = "00000000-0000-4000-8000-00000000fa02"

func retireProfiles(t *testing.T, run func(*testing.T, *fixture, bool)) {
	t.Helper()
	for _, durable := range []bool{false, true} {
		name := "memory"
		if durable {
			name = "durable"
		}
		t.Run(name, func(t *testing.T) { run(t, newFixture(t, durable), durable) })
	}
}

// TestRetireContentDisposesNamespace covers the positive local chain: the
// disposition records the retired key, cut and exact manifest, the live
// content and relationship leave with one transaction while tombstones stay,
// the original answer replays without another Core read, and the disposed
// namespace durably refuses every later acceptance or write of the element.
func TestRetireContentDisposesNamespace(t *testing.T) {
	retireProfiles(t, func(t *testing.T, x *fixture, _ bool) {
		ctx := context.Background()
		second := fixtureType("second")
		x.put(t, "put-first", "payload", contract.ExpectAbsent())
		secondPut, err := x.f.Put(ctx, testActor, x.key, second, fixturePayload("two"), contract.ExpectAbsent(), "put-second")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = x.f.Delete(ctx, testActor, x.key, second, secondPut.Revision, "delete-second"); err != nil {
			t.Fatal(err)
		}
		manifest := buildManifest(&x.f.state, testElement)
		digest := manifestDigest(manifest)
		x.setState(contract.CoreDetached)
		result, err := x.f.RetireContent(ctx, testActor, x.key, "retire")
		if err != nil || result.Status != contract.Committed {
			t.Fatalf("retire: %+v %v", result, err)
		}
		x.auth.mu.Lock()
		required := x.auth.requirements
		x.auth.mu.Unlock()
		if len(required) != 1 || required[0].Permission != contract.PermissionRetireContent {
			t.Fatalf("retire permission: %+v", required)
		}
		retired, ok := x.f.state.Retired[string(testElement)]
		if !ok {
			t.Fatal("final disposition missing")
		}
		if retired.Key != packKey(x.key) || retired.Digest != digest || !reflect.DeepEqual(retired.Manifest, manifest) {
			t.Fatalf("disposition: %+v", retired)
		}
		if retired.Cut != x.f.state.Cut {
			t.Fatalf("disposition cut %d differs from state cut %d", retired.Cut, x.f.state.Cut)
		}
		if len(x.f.state.Acceptances) != 0 || len(x.f.state.Contacts) != 0 || len(x.f.state.Records) != 0 {
			t.Fatal("live namespace not closed in one transaction")
		}
		if _, kept := x.f.state.Tombstones[recordID(testElement, second.Bytes())]; !kept {
			t.Fatal("deletion tombstone discarded")
		}
		// The committed operation replays its original answer without another
		// Core read.
		calls := x.source.calls.Load()
		replayed, err := x.f.RetireContent(ctx, testActor, x.key, "retire")
		if err != nil || replayed.Status != contract.Committed || replayed.Cut != result.Cut || x.source.calls.Load() != calls {
			t.Fatalf("retire replay: %+v %v", replayed, err)
		}
		// A second disposal request under another operation is refused
		// durably, without reading any evidence.
		_, err = x.f.RetireContent(ctx, testActor, x.key, "retire-again")
		mustError(t, err, contract.ErrNamespaceRetired)
		if x.source.calls.Load() != calls {
			t.Fatal("durable second refusal consulted the Core")
		}
		if _, journalled := x.f.operations[byteString("retire-again")]; !journalled {
			t.Fatal("persistent second refusal missing from journal")
		}
		if err = validateHistory(x.f.authority, x.f.scope, x.f.state, x.f.events); err != nil {
			t.Fatal(err)
		}
		// The disposed namespace never accepts this element again, even on a
		// fresh valid proposal.
		newKey := testKey(x.f.AuthorityRef(), testCore, testElement, 5)
		proposed := testProof(newKey, contract.CoreNone, 5)
		proposed.Pending = newKey
		x.source.current = proposed
		_, err = x.f.AcceptAttachment(ctx, testActor, newKey, "local:core", "reaccept")
		mustError(t, err, contract.ErrNamespaceRetired)
		if err = validateHistory(x.f.authority, x.f.scope, x.f.state, x.f.events); err != nil {
			t.Fatal(err)
		}
		// The retired lineage keeps no write or query basis.
		_, err = x.f.Put(ctx, testActor, x.key, x.typ, fixturePayload("forged"), contract.ExpectAbsent(), "dead-write")
		mustError(t, err, contract.ErrUnavailable)
		read, err := x.f.ReadAcceptance(testActor, x.key)
		if err != nil || read.Found {
			t.Fatalf("retired lineage kept: %+v %v", read, err)
		}
	})
}

// TestRetireContentSurvivesReopen proves the durable boundary: the committed
// disposition and its zero-evidence durable refusal both replay identically
// after a restart, without contacting the Core, and the element stays closed.
func TestRetireContentSurvivesReopen(t *testing.T) {
	x := newFixture(t, true)
	ctx := context.Background()
	x.setState(contract.CoreDetached)
	result, err := x.f.RetireContent(ctx, testActor, x.key, "retire")
	if err != nil {
		t.Fatal(err)
	}
	_, err = x.f.RetireContent(ctx, testActor, x.key, "retire-again")
	mustError(t, err, contract.ErrNamespaceRetired)
	if err = Close(x.f); err != nil {
		t.Fatal(err)
	}
	x.f, err = OpenDurable(x.auth, x.source, x.path, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	retired, ok := x.f.state.Retired[string(testElement)]
	if !ok || retired.Key != packKey(x.key) {
		t.Fatalf("disposition lost across reopen: %+v %v", retired, ok)
	}
	// Both original answers replay without a live Core.
	x.source.offline = true
	replayed, err := x.f.RetireContent(ctx, testActor, x.key, "retire")
	if err != nil || replayed.Status != contract.Committed || replayed.Cut != result.Cut {
		t.Fatalf("committed replay after reopen: %+v %v", replayed, err)
	}
	if _, err = x.f.RetireContent(ctx, testActor, x.key, "retire-again"); !errors.Is(err, contract.ErrNamespaceRetired) {
		t.Fatalf("refusal replay after reopen: %v", err)
	}
	// The closed namespace still refuses a fresh valid proposal.
	x.source.offline = false
	newKey := testKey(x.f.AuthorityRef(), testCore, testElement, 5)
	proposed := testProof(newKey, contract.CoreNone, 5)
	proposed.Pending = newKey
	x.source.current = proposed
	_, err = x.f.AcceptAttachment(ctx, testActor, newKey, "local:core", "reaccept")
	mustError(t, err, contract.ErrNamespaceRetired)
}

// TestRetireContentEvidenceShapes pins both disposal shapes against the
// recorded disclosure: the revoked lineage (Case A), including this Facet's
// own open recovery proposal, and the still-open accepted proposal whose
// stored predecessor may live at another Core (Case B). Every other shape —
// a lost Core, an unauthorized actor, an active record, another Facet's
// pending or predecessor — fails temporarily without touching the journal.
func TestRetireContentEvidenceShapes(t *testing.T) {
	retireProfiles(t, func(t *testing.T, x *fixture, durable bool) {
		ctx := context.Background()
		// A key this Facet never held is refused durably on zero evidence.
		stranger := testKey(x.f.AuthorityRef(), testCore, testElement, 9)
		_, err := x.f.RetireContent(ctx, testActor, stranger, "retire-stranger")
		mustError(t, err, contract.ErrAcceptanceNotFound)
		if _, journalled := x.f.operations[byteString("retire-stranger")]; !journalled {
			t.Fatal("no-acceptance refusal missing from journal")
		}
		x.setState(contract.CoreDetached)
		// A lost Core keeps the request retryable.
		x.source.offline = true
		_, err = x.f.RetireContent(ctx, testActor, x.key, "retire")
		mustError(t, err, contract.ErrUnavailable)
		x.source.offline = false
		// An unauthorized actor is refused without evidence.
		x.auth.deny.Store(true)
		_, err = x.f.RetireContent(ctx, testActor, x.key, "retire")
		mustError(t, err, contract.ErrDenied)
		x.auth.deny.Store(false)
		// An empty operation is invalid input.
		_, err = x.f.RetireContent(ctx, testActor, x.key, "")
		mustError(t, err, contract.ErrInvalid)
		// An active record is not disposal evidence.
		x.source.current = testProof(x.key, contract.CoreActive, 5)
		_, err = x.f.RetireContent(ctx, testActor, x.key, "retire")
		mustError(t, err, contract.ErrUnavailable)
		// An open recovery proposal of another Core is not this Facet's.
		foreignPending := testProof(x.key, contract.CoreDetached, 4)
		foreignPending.Pending = testKey(x.f.AuthorityRef(), retireAltCore, testElement, 4)
		x.source.current = foreignPending
		_, err = x.f.RetireContent(ctx, testActor, x.key, "retire")
		mustError(t, err, contract.ErrUnavailable)
		// An accepted proposal whose predecessor belongs to another Facet is
		// not this namespace's obligation window.
		foreignPredecessor := testProof(x.key, contract.CoreDetached, 2)
		foreignPredecessor.Active = testKey(recoveryFacet, testCore, testElement, 1)
		foreignPredecessor.Pending = x.key
		x.source.current = foreignPredecessor
		_, err = x.f.RetireContent(ctx, testActor, x.key, "retire")
		mustError(t, err, contract.ErrUnavailable)
		// A detached record naming another Facet's key proves nothing here.
		x.source.current = testProof(testKey(recoveryFacet, testCore, testElement, 2), contract.CoreDetached, 4)
		_, err = x.f.RetireContent(ctx, testActor, x.key, "retire")
		mustError(t, err, contract.ErrUnavailable)
		if _, journalled := x.f.operations[byteString("retire")]; journalled {
			t.Fatal("temporary failure journalled")
		}
		if err = validateHistory(x.f.authority, x.f.scope, x.f.state, x.f.events); err != nil {
			t.Fatal(err)
		}
		// Case A with this Facet's own open recovery proposal disposes.
		a := newFixture(t, durable)
		ownRecovery := testProof(a.key, contract.CoreDetached, 4)
		ownRecovery.Pending = testKey(a.f.AuthorityRef(), testCore, testElement, 4)
		a.source.current = ownRecovery
		if _, err = a.f.RetireContent(ctx, testActor, a.key, "retire"); err != nil {
			t.Fatalf("Case A with open recovery: %v", err)
		}
		// Case B with a predecessor at the same Core disposes.
		b := newFixture(t, durable)
		sameCoreWindow := testProof(b.key, contract.CoreDetached, 2)
		sameCoreWindow.Active = testKey(b.f.AuthorityRef(), testCore, testElement, 1)
		sameCoreWindow.Pending = b.key
		b.source.current = sameCoreWindow
		if _, err = b.f.RetireContent(ctx, testActor, b.key, "retire"); err != nil {
			t.Fatalf("Case B same Core: %v", err)
		}
		// Case B with a predecessor at another Core disposes the same held
		// acceptance.
		foreignCoreWindow := testProof(x.key, contract.CoreDetached, 2)
		foreignCoreWindow.Active = testKey(x.f.AuthorityRef(), retireAltCore, testElement, 1)
		foreignCoreWindow.Pending = x.key
		x.source.current = foreignCoreWindow
		disposed, err := x.f.RetireContent(ctx, testActor, x.key, "retire")
		if err != nil || disposed.Status != contract.Committed {
			t.Fatalf("Case B other Core: %+v %v", disposed, err)
		}
		retired, ok := x.f.state.Retired[string(testElement)]
		if !ok || retired.Key != packKey(x.key) {
			t.Fatalf("disposition key: %+v %v", retired, ok)
		}
	})
}

// TestDecideRetireRefusesUnfinishedObligations exercises the deterministic
// transition directly: fenced elements, pending installs, disposed namespaces
// and missing acceptances are refused without state changes, while evidence
// outside the shape fails temporarily.
func TestDecideRetireRefusesUnfinishedObligations(t *testing.T) {
	k := packKey(testKey(fixtureFacetOwner, testCore, testElement, 2))
	p := packProof(testProof(k.value(), contract.CoreDetached, 4))
	newState := func() *facetState {
		s := emptyState()
		s.Acceptances[k.id()] = accepted{Key: k, At: 1}
		s.Contacts[k.id()] = contact{Address: "local:core", Serial: 1}
		return &s
	}
	run := func(s *facetState, code string) {
		t.Helper()
		out, err := decideRetire(s, command{Kind: "retire", Key: k}, p, departureData{})
		if err != nil || out.Status != contract.Rejected || out.Error != code {
			t.Fatalf("got %+v %v; want rejection %q", out, err, code)
		}
		if s.Cut != 1 {
			t.Fatalf("rejection %q consumed a cut", code)
		}
	}
	s := newState()
	s.Fenced[string(testElement)] = fencedNamespace{}
	run(s, "fenced")
	s = newState()
	s.Pending["install"] = pendingInstall{Element: testElement}
	run(s, "pending")
	s = newState()
	s.Retired[string(testElement)] = retiredNamespace{Key: k}
	run(s, "retired")
	s = newState()
	delete(s.Acceptances, k.id())
	run(s, "acceptance")
	// Evidence outside the retirement shape fails temporarily.
	s = newState()
	bad := packProof(testProof(k.value(), contract.CoreActive, 4))
	if _, err := decideRetire(s, command{Kind: "retire", Key: k}, bad, departureData{}); !errors.Is(err, contract.ErrUnavailable) {
		t.Fatalf("shape violation: %v", err)
	}
	if _, ok := s.Acceptances[k.id()]; !ok {
		t.Fatal("temporary failure removed the acceptance")
	}
	// The same transition commits the closed namespace in one step.
	s = newState()
	manifest := buildManifest(s, testElement)
	out, err := decideRetire(s, command{Kind: "retire", Key: k}, p, departureData{})
	if err != nil || out.Status != contract.Committed {
		t.Fatalf("commit: %+v %v", out, err)
	}
	retired, ok := s.Retired[string(testElement)]
	if !ok || retired.Key != k || retired.Digest != manifestDigest(manifest) || retired.Cut != 2 {
		t.Fatalf("disposition: %+v %v", retired, ok)
	}
	if len(s.Acceptances) != 0 || len(s.Contacts) != 0 || len(s.Records) != 0 {
		t.Fatal("commit left live state behind")
	}
}

// TestRetirementHistoryBindsEvidence proves the replay boundary: a committed
// disposal must carry its disclosure evidence, and no plan or foreign key
// may be attached to the recorded transition.
func TestRetirementHistoryBindsEvidence(t *testing.T) {
	x := newFixture(t, false)
	ctx := context.Background()
	x.setState(contract.CoreDetached)
	if _, err := x.f.RetireContent(ctx, testActor, x.key, "retire"); err != nil {
		t.Fatal(err)
	}
	if err := validateHistory(x.f.authority, x.f.scope, x.f.state, x.f.events); err != nil {
		t.Fatal(err)
	}
	tampered := func(mutate func(*event)) {
		t.Helper()
		events := append([]event(nil), x.f.events...)
		mutate(&events[len(events)-1])
		if err := validateHistory(x.f.authority, x.f.scope, x.f.state, events); !errors.Is(err, contract.ErrStore) {
			t.Fatalf("forged history accepted: %v", err)
		}
	}
	tampered(func(e *event) { e.Core = proof{} })
	tampered(func(e *event) {
		e.Core = packProof(testProof(testKey(recoveryFacet, testCore, testElement, 2), contract.CoreDetached, 4))
	})
	tampered(func(e *event) { e.Plan = transferPlan{Authority: testCore} })
}

// TestRetireContentControlChain covers the public Core-service chain: the
// bound proposal authorizes the disposal, the committed outcome and the
// durable refusal replay identically, the disposition port discloses the
// retired key to the Core and a proven absence to everyone else, and all of
// it survives a restart.
func TestRetireContentControlChain(t *testing.T) {
	for _, durable := range []bool{false, true} {
		name := "memory"
		if durable {
			name = "durable"
		}
		t.Run(name, func(t *testing.T) {
			x := newPublicAcceptanceFixture(t, durable)
			ctx := context.Background()
			x.source.address = "local:core"
			x.accept(t, "accept")
			x.source.current = testProof(x.key, contract.CoreDetached, 4)
			out, err := x.f.RetireContentControl(ctx, x.caller, x.ref, "retire")
			if err != nil || out.Status != contract.Committed {
				t.Fatalf("control retire: %+v %v", out, err)
			}
			if out.Inspection.Found {
				t.Fatal("disposed acceptance still reported")
			}
			// The committed operation replays without another Core read.
			calls := x.source.calls.Load()
			again, err := x.f.RetireContentControl(ctx, x.caller, x.ref, "retire")
			if err != nil || again.Status != contract.Committed || again.Inspection.Found || x.source.calls.Load() != calls {
				t.Fatalf("control replay: %+v %v", again, err)
			}
			disposition, err := x.f.ReadDispositionControl(context.Background(), x.caller, testElement)
			if err != nil || !disposition.Found || disposition.Key != x.key {
				t.Fatalf("disposition: %+v %v", disposition, err)
			}
			expected := cutValue(x.f.scope, x.f.state.Retired[string(testElement)].Cut)
			if disposition.Cut != expected {
				t.Fatalf("disposition cut: %+v", disposition.Cut)
			}
			absent, err := x.f.ReadDispositionControl(context.Background(), x.caller, retireOtherElement)
			if err != nil || absent != (contract.DispositionRead{}) {
				t.Fatalf("proven absence: %+v %v", absent, err)
			}
			// A second disposal request under another operation is refused
			// durably through the public chain.
			_, err = x.f.RetireContentControl(ctx, x.caller, x.ref, "retire-again")
			if !errors.Is(err, values.PublicUnavailable) {
				t.Fatalf("public durable refusal: %v", err)
			}
			if _, journalled := x.f.operations[byteString("retire-again")]; !journalled {
				t.Fatal("public second refusal missing from journal")
			}
			// An unbound proposal ref is not a disposal selector.
			other := contract.PublicAcceptanceRef{Core: testCore, Element: retireOtherElement, Proposal: x.ref.Proposal}
			if _, err = x.f.RetireContentControl(ctx, x.caller, other, "retire-other"); !errors.Is(err, values.PublicUnavailable) {
				t.Fatalf("unbound ref: %v", err)
			}
			if _, journalled := x.f.operations[byteString("retire-other")]; journalled {
				t.Fatal("unbound ref journalled")
			}
			if !durable {
				return
			}
			if err = Close(x.f); err != nil {
				t.Fatal(err)
			}
			x.f, err = OpenDurable(x.auth, x.source, x.path, x.config, nil)
			if err != nil {
				t.Fatal(err)
			}
			x.source.offline = true
			replayed, err := x.f.RetireContentControl(ctx, x.caller, x.ref, "retire")
			if err != nil || replayed.Status != contract.Committed || replayed.Inspection.Found || x.source.calls.Load() != calls {
				t.Fatalf("reopened replay: %+v %v", replayed, err)
			}
			reread, err := x.f.ReadDispositionControl(context.Background(), x.caller, testElement)
			if err != nil || reread != disposition {
				t.Fatalf("reopened disposition: %+v %v", reread, err)
			}
		})
	}
}

// TestRetireContentControlRefusals pins the public chain's safe failures: a
// lost Core and evidence outside the shape stay retryable and unjournalled,
// and a denied authorization view never reaches the sole transition.
func TestRetireContentControlRefusals(t *testing.T) {
	x := newPublicAcceptanceFixture(t, false)
	ctx := context.Background()
	x.source.address = "local:core"
	x.accept(t, "accept")
	x.source.current = testProof(x.key, contract.CoreDetached, 4)
	x.source.offline = true
	if _, err := x.f.RetireContentControl(ctx, x.caller, x.ref, "retire"); !errors.Is(err, values.PublicUnavailable) {
		t.Fatalf("offline control: %v", err)
	}
	if _, journalled := x.f.operations[byteString("retire")]; journalled {
		t.Fatal("temporary failure journalled")
	}
	x.source.offline = false
	x.auth.decision.Store(uint32(values.AuthorizationDenied))
	if _, err := x.f.RetireContentControl(ctx, x.caller, x.ref, "retire"); !errors.Is(err, values.PublicDenied) {
		t.Fatalf("denied control: %v", err)
	}
	x.auth.decision.Store(uint32(values.AuthorizationValid))
	x.source.current = testProof(x.key, contract.CoreActive, 5)
	if _, err := x.f.RetireContentControl(ctx, x.caller, x.ref, "retire"); !errors.Is(err, values.PublicUnavailable) {
		t.Fatalf("active evidence: %v", err)
	}
	if _, journalled := x.f.operations[byteString("retire")]; journalled {
		t.Fatal("temporary failure journalled")
	}
	x.source.current = testProof(x.key, contract.CoreDetached, 6)
	out, err := x.f.RetireContentControl(ctx, x.caller, x.ref, "retire")
	if err != nil || out.Status != contract.Committed {
		t.Fatalf("retry after refusals: %+v %v", out, err)
	}
}

// TestRetireContentAfterCoreTransfer covers the third disposal shape: the
// element left the acceptance's Core under a completed transfer, so the old
// Core's proven missing disclosure plus its durable successor statement are
// the disposal evidence. The statement is recorded next to the retire
// decision, a proven absence or a foreign statement never disposes, a lost
// old Core stays retryable, and the committed disposal replays identically
// after a restart.
func TestRetireContentAfterCoreTransfer(t *testing.T) {
	for _, durable := range []bool{false, true} {
		name := "memory"
		if durable {
			name = "durable"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			auth := &testAuthorization{}
			source := &multiCoreSource{answers: map[values.NextHop]contract.CoreEvidence{}, successors: map[values.NextHop]contract.CoreSuccessorRead{}}
			path := filepath.Join(t.TempDir(), "facet.db")
			var f *Owner
			var err error
			if durable {
				f, err = CreateDurable(auth, source, testInitial, path, nil, nil)
			} else {
				f, err = New(auth, source, testInitial, nil)
			}
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = Close(f) })
			key := testKey(f.AuthorityRef(), testCore, testElement, 2)
			proposed := testProof(key, contract.CoreNone, 2)
			proposed.Pending = key
			source.answers[previousCoreAddress] = proposed
			if _, err = f.AcceptAttachment(ctx, testActor, key, previousCoreAddress, "accept"); err != nil {
				t.Fatal(err)
			}
			// The element left the old Core; its disclosure proves the element
			// gone and nothing else.
			source.answers[previousCoreAddress] = testProof(key, contract.CoreMissing, 3)
			// A lost old Core keeps the disposal retryable and unjournalled.
			_, err = f.RetireContent(ctx, testActor, key, "retire")
			mustError(t, err, contract.ErrUnavailable)
			if _, journalled := f.operations[byteString("retire")]; journalled {
				t.Fatal("temporary failure journalled")
			}
			// A proven absence — a plain deletion — is not disposal evidence.
			source.successors[previousCoreAddress] = contract.CoreSuccessorRead{Authority: key.Core, Element: key.Element}
			_, err = f.RetireContent(ctx, testActor, key, "retire")
			mustError(t, err, contract.ErrUnavailable)
			kept, err := f.ReadAcceptance(testActor, key)
			if err != nil || !kept.Found {
				t.Fatalf("absence lost the acceptance: %+v %v", kept, err)
			}
			// A statement by another Core never disposes this lineage.
			source.successors[previousCoreAddress] = departureStatement(key.Element, retireAltCore, reacceptCore)
			_, err = f.RetireContent(ctx, testActor, key, "retire")
			mustError(t, err, contract.ErrUnavailable)
			// The old Core's own successor statement is the disposal evidence.
			statement := departureStatement(key.Element, key.Core, reacceptCore)
			source.successors[previousCoreAddress] = statement
			disposed, err := f.RetireContent(ctx, testActor, key, "retire")
			if err != nil || disposed.Status != contract.Committed {
				t.Fatalf("transferred retire: %+v %v", disposed, err)
			}
			retired, ok := f.state.Retired[string(testElement)]
			if !ok || retired.Key != packKey(key) {
				t.Fatalf("disposition: %+v %v", retired, ok)
			}
			// The successor statement is recorded next to the retire decision.
			index := f.operations[byteString("retire")]
			if f.events[index].Departure != packDeparture(statement) {
				t.Fatalf("departure evidence missing: %+v", f.events[index].Departure)
			}
			if err = validateHistory(f.authority, f.scope, f.state, f.events); err != nil {
				t.Fatal(err)
			}
			// The committed disposal replays its original answer without
			// another Core read, and a second request under another operation
			// is refused durably.
			calls := source.calls.Load()
			replayed, err := f.RetireContent(ctx, testActor, key, "retire")
			if err != nil || replayed.Status != contract.Committed || replayed.Cut != disposed.Cut || source.calls.Load() != calls {
				t.Fatalf("replay: %+v %v", replayed, err)
			}
			_, err = f.RetireContent(ctx, testActor, key, "retire-again")
			mustError(t, err, contract.ErrNamespaceRetired)
			// A forged window never validates: erasing or redirecting the
			// recorded departure, or pairing the departure with a local
			// disclosure, breaks the history.
			tampered := func(mutate func(*event)) {
				t.Helper()
				events := append([]event(nil), f.events...)
				mutate(&events[index])
				if err := validateHistory(f.authority, f.scope, f.state, events); !errors.Is(err, contract.ErrStore) {
					t.Fatalf("forged history accepted: %v", err)
				}
			}
			tampered(func(e *event) { e.Departure = departureData{} })
			tampered(func(e *event) {
				e.Departure = packDeparture(contract.CoreSuccessorRead{Found: true, Authority: key.Core, Element: key.Element,
					Operation: "transfer", Source: key.Core, Target: key.Core, FinalCut: 1})
			})
			tampered(func(e *event) { e.Core = packProof(testProof(key, contract.CoreDetached, 4)) })
			if !durable {
				return
			}
			// The recorded disposal survives a restart: the journal replays
			// with the departure evidence and both answers return identically.
			if err = Close(f); err != nil {
				t.Fatal(err)
			}
			reopened, err := OpenDurable(auth, source, path, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = Close(reopened) })
			afterRestart, err := reopened.RetireContent(ctx, testActor, key, "retire")
			if err != nil || afterRestart.Status != contract.Committed || afterRestart.Cut != disposed.Cut {
				t.Fatalf("reopened replay: %+v %v", afterRestart, err)
			}
			if _, err = reopened.RetireContent(ctx, testActor, key, "retire-again"); !errors.Is(err, contract.ErrNamespaceRetired) {
				t.Fatalf("reopened refusal: %v", err)
			}
		})
	}
}

// TestRetireContentControlObligationWindow retires through the public chain
// in the activation-obligation shape: the accepted proposal is still open at
// a detached record whose stored predecessor lives at another Core.
func TestRetireContentControlObligationWindow(t *testing.T) {
	for _, durable := range []bool{false, true} {
		name := "memory"
		if durable {
			name = "durable"
		}
		t.Run(name, func(t *testing.T) {
			x := newPublicAcceptanceFixture(t, durable)
			ctx := context.Background()
			x.source.address = "local:core"
			x.accept(t, "accept")
			window := testProof(x.key, contract.CoreDetached, 2)
			window.Active = testKey(x.f.AuthorityRef(), retireAltCore, testElement, 1)
			window.Pending = x.key
			x.source.current = window
			out, err := x.f.RetireContentControl(ctx, x.caller, x.ref, "retire")
			if err != nil || out.Status != contract.Committed {
				t.Fatalf("obligation-window retire: %+v %v", out, err)
			}
			disposition, err := x.f.ReadDispositionControl(context.Background(), x.caller, testElement)
			if err != nil || !disposition.Found || disposition.Key != x.key {
				t.Fatalf("disposition: %+v %v", disposition, err)
			}
		})
	}
}

// TestRetireDisposesTombstoneAccumulatedNamespace reproduces the reachable
// capacity window: the type bound counts only live records, so successive
// writes and deletions of distinct types legally accumulate more tombstones
// than the manifest bound. The explicit disposal must still commit — the
// disposition manifest is local evidence that never travels the wire — and
// the closed namespace keeps every tombstone. The tombstone history is
// journalled by the single deterministic transition itself (one full replay
// per live commit makes thousands of API commits quadratic), installed through
// the owner's recovery path, and the disposal then runs the complete live
// commit chain; the commit's own history gate proves the constructed journal
// is exactly the reachable one.
func TestRetireDisposesTombstoneAccumulatedNamespace(t *testing.T) {
	x := newFixture(t, false)
	ctx := context.Background()
	s := x.f.state.clone()
	events := append([]event(nil), x.f.events...)
	active := packProof(x.source.current)
	commit := func(c command, p proof) outcome {
		out, err := decide(&s, x.f.scope, c, p, departureData{}, transferPlan{})
		if err != nil {
			t.Fatalf("%s: %v", c.Kind, err)
		}
		events = append(events, event{Input: c, Core: p, Outcome: out})
		return out
	}
	for i := 0; i <= maxManifestEntries; i++ {
		typ := fixtureType(fmt.Sprintf("tombstone-%04d", i))
		put := x.f.base(testActor, x.key, contract.OperationID(fmt.Sprintf("put-%04d", i)), "put")
		put.Type = typ.Bytes()
		put.Payload = fixturePayload("gone")
		put.Absent = true
		created := commit(put, active)
		del := x.f.base(testActor, x.key, contract.OperationID(fmt.Sprintf("delete-%04d", i)), "delete")
		del.Type = typ.Bytes()
		del.Expected = created.Revision
		commit(del, active)
	}
	if len(s.Tombstones) != maxManifestEntries+1 {
		t.Fatalf("successive writes left %d tombstones", len(s.Tombstones))
	}
	manifest := buildManifest(&s, testElement)
	if len(manifest) <= maxManifestEntries {
		t.Fatalf("reachable manifest holds %d entries", len(manifest))
	}
	digest := manifestDigest(manifest)
	x.f.install(storeDocument{x.f.authority, x.f.scope, s, events})
	if len(x.f.state.Tombstones) != maxManifestEntries+1 {
		t.Fatal("installed history lost its tombstones")
	}
	x.setState(contract.CoreDetached)
	result, err := x.f.RetireContent(ctx, testActor, x.key, "retire")
	if err != nil || result.Status != contract.Committed {
		t.Fatalf("disposal of the accumulated namespace: %+v %v", result, err)
	}
	retired, ok := x.f.state.Retired[string(testElement)]
	if !ok || retired.Key != packKey(x.key) || retired.Digest != digest || len(retired.Manifest) != len(manifest) {
		t.Fatalf("disposition: %+v %v", retired, ok)
	}
	if len(x.f.state.Tombstones) != maxManifestEntries+1 || len(x.f.state.Acceptances) != 0 ||
		len(x.f.state.Contacts) != 0 || len(x.f.state.Records) != 0 {
		t.Fatal("disposition changed the closed namespace's live state or tombstones")
	}
	// The store roundtrip re-validates the whole journal internally, proving
	// the big disposition record persists through the durable codec.
	raw, err := encodeDocument(storeDocument{x.f.authority, x.f.scope, x.f.state, x.f.events})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = decodeDocument(raw); err != nil {
		t.Fatalf("durable document roundtrip: %v", err)
	}
	// The closed namespace still refuses revival and a second disposal.
	newKey := testKey(x.f.AuthorityRef(), testCore, testElement, 5)
	proposed := testProof(newKey, contract.CoreNone, 5)
	proposed.Pending = newKey
	x.source.current = proposed
	_, err = x.f.AcceptAttachment(ctx, testActor, newKey, "local:core", "revive")
	mustError(t, err, contract.ErrNamespaceRetired)
	_, err = x.f.RetireContent(ctx, testActor, x.key, "retire-again")
	mustError(t, err, contract.ErrNamespaceRetired)
}
