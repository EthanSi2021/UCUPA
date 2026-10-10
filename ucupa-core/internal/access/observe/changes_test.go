package observe

import (
	"reflect"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	facetfact "ucupa/ucupa-core/internal/facet/fact"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

const (
	root   values.ElementID = "00000000-0000-4000-8000-000000000001"
	child  values.ElementID = "00000000-0000-4000-8000-000000000002"
	entity values.ElementID = "00000000-0000-4000-8000-000000000003"
	other  values.ElementID = "00000000-0000-4000-8000-000000000004"
)

func facts(x float64, deleted bool) View {
	e := elementfact.NewProjection()
	e.AddAnchor(root, elementfact.Anchor{ID: root})
	e.AddAnchor(child, elementfact.Anchor{ID: child, Parent: root})
	if !deleted {
		e.AddEntity(entity, elementfact.Entity{ID: entity, Parent: child, LocalPose: contract.Pose{Translation: contract.Vec3{X: x}}})
	}
	return View{Present: true, CutSerial: 2, Elements: e.Build().View, Mount: mountfact.New().View, Attachments: facetfact.New().View}
}

func TestChangesKeepsClosureExactPathAndControlRules(t *testing.T) {
	before, moved, deleted := facts(0, false), facts(1, false), facts(0, true)
	for _, tc := range []struct {
		name                string
		after               View
		selection           Selection
		spatial, attachment bool
	}{
		{"same", before, Selection{Boundary: root}, false, false},
		{"closure", moved, Selection{Boundary: root}, true, false},
		{"other closure", moved, Selection{Boundary: other}, false, false},
		{"exact", moved, Selection{Exact: entity}, true, false},
		{"other exact", moved, Selection{Exact: other}, false, false},
		{"path excludes leaf", moved, Selection{Boundary: root, Path: map[values.ElementID]bool{child: true}}, false, false},
		{"exact supplements path", moved, Selection{Path: map[values.ElementID]bool{child: true}, Exact: entity}, true, false},
		{"control ignores movement", moved, Selection{Exact: entity, Control: true}, false, false},
		{"control detects deletion", deleted, Selection{Exact: entity, Control: true}, false, true},
		{"closure detects deletion", deleted, Selection{Boundary: root}, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spatial, attachment := Changes(before, tc.after, tc.selection)
			if spatial != tc.spatial || attachment != tc.attachment {
				t.Fatalf("got spatial=%v attachment=%v, want %v/%v", spatial, attachment, tc.spatial, tc.attachment)
			}
		})
	}
}

func TestChangesKeepsAttachmentAndFederationFilters(t *testing.T) {
	before := facts(0, false)
	after := before
	builder := facetfact.NewProjectionBuilder()
	builder.Add(child, facetfact.Record{Attachment: contract.FacetAttachment{State: contract.FacetDetached}})
	after.Attachments = builder.Build().View
	for _, tc := range []struct {
		selection Selection
		want      bool
	}{
		{Selection{Boundary: root}, false},
		{Selection{Boundary: root, IncludeAttachment: true}, true},
		{Selection{Path: map[values.ElementID]bool{child: true}, Exact: entity, IncludeAttachment: true}, true},
		{Selection{Path: map[values.ElementID]bool{child: true}, Exact: entity, IncludeAttachment: true, Federated: true, FederatedRead: true}, false},
	} {
		spatial, attachment := Changes(before, after, tc.selection)
		if spatial || attachment != tc.want {
			t.Fatal(spatial, attachment, tc)
		}
	}
	mount := mountfact.New()
	mount.ReplaceIncoming(mountfact.Incoming{Detached: true})
	after = before
	after.Mount = mount.View
	if spatial, _ := Changes(before, after, Selection{Exact: entity}); spatial {
		t.Fatal("local exact read observed unrelated incoming context")
	}
	if spatial, _ := Changes(before, after, Selection{Exact: entity, Federated: true}); !spatial {
		t.Fatal("federated read lost incoming relation invalidation")
	}
	mount = mountfact.New()
	mount.ReplaceOutgoing(child, mountfact.Record{State: contract.MountDetached})
	after.Mount = mount.View
	if spatial, _ := Changes(before, after, Selection{Boundary: root}); !spatial {
		t.Fatal("closure lost outgoing relation change")
	}
	if spatial, _ := Changes(before, after, Selection{Exact: child}); spatial {
		t.Fatal("exact read gained outgoing relation invalidation")
	}
}

func TestPublicationViewIsOnlyFiniteCurrentFacts(t *testing.T) {
	typ := reflect.TypeOf(View{})
	want := []reflect.Type{reflect.TypeOf(false), reflect.TypeOf(uint64(0)), reflect.TypeOf(elementfact.View{}), reflect.TypeOf(mountfact.View{}), reflect.TypeOf(facetfact.View{}), reflect.TypeOf(transferfact.Visibility{})}
	if typ.NumField() != len(want) {
		t.Fatal("publication view acquired unrelated state")
	}
	for i, fieldType := range want {
		if typ.Field(i).Type != fieldType {
			t.Fatal("publication view acquired mutable owner/history data", typ.Field(i))
		}
	}
}

func TestPendingTransferEndsOnlyAffectedObservation(t *testing.T) {
	before, after := facts(0, false), facts(0, true)
	after.Transfers = transferfact.NewVisibility(map[values.ElementID]values.ElementID{entity: entity}, nil, nil, nil, nil)
	for _, test := range []struct {
		selection Selection
		lost      bool
	}{
		{Selection{Boundary: root}, true},
		{Selection{Exact: entity}, true},
		{Selection{Exact: entity, Control: true}, true},
		{Selection{Boundary: other}, false},
		{Selection{Exact: child}, false},
		{Selection{Path: map[values.ElementID]bool{child: true}}, false},
		{Selection{Path: map[values.ElementID]bool{child: true, entity: true}}, true},
	} {
		if lost := SourceLost(before, after, test.selection); lost != test.lost {
			t.Fatalf("selection %+v: lost=%v want %v", test.selection, lost, test.lost)
		}
		if SourceLost(after, after, test.selection) {
			t.Fatal("existing departure repeatedly terminated observation")
		}
		if SourceLost(before, facts(0, true), test.selection) {
			t.Fatal("ordinary deletion became a transfer fence")
		}
	}
}

func TestEmptyCarrierFenceEndsExistingSourceObservation(t *testing.T) {
	before := View{Present: true, Mount: mountfact.New().View}
	closed := mountfact.New()
	closed.ReplaceIncoming(mountfact.Incoming{Gate: mountfact.GateFenced})
	after := View{Present: true, Mount: closed.View}
	if !SourceLost(before, after, Selection{Boundary: root, Federated: true}) || SourceLost(after, after, Selection{Boundary: root, Federated: true}) {
		t.Fatal("empty carrier fence must produce one GAP, not an empty success or repeated gaps")
	}
}

func TestParentWithdrawalEndsExistingSourceObservation(t *testing.T) {
	before := View{Present: true, Mount: mountfact.New().View}
	closed := mountfact.New()
	closed.ReplaceIncoming(mountfact.Incoming{Gate: mountfact.GateReparenting})
	after := View{Present: true, Mount: closed.View}
	if !SourceLost(before, after, Selection{Boundary: root, Federated: true}) || SourceLost(after, after, Selection{Boundary: root, Federated: true}) {
		t.Fatal("parent withdrawal must produce one GAP even without local content")
	}
}

func TestTargetCompletionPublishesCoverageWithoutChangingElements(t *testing.T) {
	after := facts(0, false)
	before := after
	before.Transfers = transferfact.NewVisibility(map[values.ElementID]values.ElementID{entity: entity}, nil, nil, nil, nil)
	for _, test := range []struct {
		selection Selection
		changed   bool
	}{
		{Selection{Boundary: root}, true}, {Selection{Exact: entity}, true},
		{Selection{Boundary: other}, false}, {Selection{Exact: child}, false},
		{Selection{Exact: entity, Control: true}, false},
	} {
		spatial, attachment := Changes(before, after, test.selection)
		if spatial != test.changed || attachment || SourceLost(before, after, test.selection) {
			t.Fatal("completion lost coverage or became a new departure", test, spatial, attachment)
		}
	}
}

func TestTerminalProofRequiresUniqueSettledDecisionOrStaysPending(t *testing.T) {
	before, after := facts(0, false), facts(0, true)
	selection := Selection{Boundary: root}
	after.Transfers = transferfact.NewVisibility(map[values.ElementID]values.ElementID{entity: entity}, nil, nil, nil, nil)
	lost := LostDepartures(before, after, selection)
	if len(lost) != 1 || lost[0] != entity {
		t.Fatalf("lost departures = %v", lost)
	}
	if _, ok := SettledSuccessor(lost, after); ok {
		t.Fatal("unproven loss disclosed a successor")
	}
	if AllPendingDecisions(lost, after) {
		t.Fatal("unrelated loss was deferred as this owner's own pending decision")
	}
	after.Transfers = transferfact.NewVisibility(map[values.ElementID]values.ElementID{entity: entity}, nil, nil, nil, map[values.ElementID]bool{entity: true})
	if _, ok := SettledSuccessor(lost, after); ok {
		t.Fatal("unsettled decision disclosed terminal evidence")
	}
	if !AllPendingDecisions(lost, after) {
		t.Fatal("own unsettled decision was not deferred for a later proof")
	}
	terminal := contract.TransitionTerminal{Operation: "operation", Source: "00000000-0000-4000-8000-000000000009",
		Target: "00000000-0000-4000-8000-00000000000a", FinalCut: 9, Settled: true, NextHop: "local:target"}
	after.Transfers = transferfact.NewVisibility(map[values.ElementID]values.ElementID{entity: entity}, nil, nil,
		map[values.ElementID]contract.TransitionTerminal{entity: terminal}, nil)
	proved, ok := SettledSuccessor(lost, after)
	if !ok || proved != terminal {
		t.Fatalf("settled decision lost its terminal evidence: %#v %v", proved, ok)
	}
	if AllPendingDecisions(lost, after) {
		t.Fatal("settled decision stayed pending")
	}
}

func TestSettledSuccessorRejectsContradictingOrPartialDecisions(t *testing.T) {
	after := facts(0, true)
	both := map[values.ElementID]values.ElementID{entity: entity, child: child}
	one := contract.TransitionTerminal{Operation: "one", FinalCut: 1, Settled: true}
	two := contract.TransitionTerminal{Operation: "two", FinalCut: 2, Settled: true}
	after.Transfers = transferfact.NewVisibility(both, nil, nil, map[values.ElementID]contract.TransitionTerminal{entity: one, child: two}, nil)
	if _, ok := SettledSuccessor([]values.ElementID{entity, child}, after); ok {
		t.Fatal("contradicting decisions were merged into one terminal evidence")
	}
	after.Transfers = transferfact.NewVisibility(both, nil, nil, map[values.ElementID]contract.TransitionTerminal{entity: one}, nil)
	if _, ok := SettledSuccessor([]values.ElementID{entity, child}, after); ok {
		t.Fatal("one proven element disclosed the whole unproven loss")
	}
	after.Transfers = transferfact.NewVisibility(both, nil, nil, map[values.ElementID]contract.TransitionTerminal{entity: one, child: one}, nil)
	if proved, ok := SettledSuccessor([]values.ElementID{entity, child}, after); !ok || proved != one {
		t.Fatalf("one decision proving all losses lost its evidence: %#v %v", proved, ok)
	}
	if _, ok := SettledSuccessor(nil, after); ok {
		t.Fatal("no loss invented a terminal evidence")
	}
}
