package fact

import (
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/geometry"
)

const departureController = values.FactAuthorityRef("00000000-0000-4000-8000-000000000901")

func departureFixture(t *testing.T) State {
	t.Helper()
	s := New()
	for index, input := range []Input{
		{Kind: OperationCreateRoot, Element: testRootID},
		{Kind: OperationCreateAnchor, Element: testAnchorID, Final: contract.SpatialTuple{Parent: testRootID, LocalPose: geometry.IdentityPose(), Range: testSphere(t, 10)}},
		{Kind: OperationCreateEntity, Element: testEntityID, Final: contract.SpatialTuple{Parent: testAnchorID, LocalPose: geometry.IdentityPose(), Range: testSphere(t, 1)}},
	} {
		plan, err := s.Plan(input, ParentContext{})
		if err != nil {
			t.Fatal(err)
		}
		out, err := s.Apply(plan, testRevision(input.Element, uint64(index)+1))
		if err != nil || out.Status != contract.MutationCommitted {
			t.Fatalf("fixture: %+v %v", out, err)
		}
	}
	return s
}

func TestDepartureIsNotDeletionOrUUIDReuse(t *testing.T) {
	s := departureFixture(t)
	before := s.Clone()
	if err := s.Depart([]values.ElementID{testEntityID, testAnchorID}, departureController, "transfer\xff"); err != nil {
		t.Fatal(err)
	}
	if s.Equal(before) || !before.HasDirectContent(testAnchorID) {
		t.Fatal("clone did not retain independent live records")
	}
	if got := s.Counts(); got != (Counts{Claims: 3, Anchors: 1, Departures: 2}) {
		t.Fatalf("counts: %+v", got)
	}
	for index, id := range []values.ElementID{testAnchorID, testEntityID} {
		d, ok := s.Departure(id)
		if !ok || d.Controller != departureController || d.Operation != "transfer\xff" || d.Revision != testRevision(id, uint64(index)+2) || !s.Claimed(id) {
			t.Fatal("lost original lineage")
		}
		if _, deleted := s.Tombstone(id); deleted || s.Kind(id) != 0 {
			t.Fatal("departure is active or permanently deleted")
		}
		p, err := s.Plan(Input{Kind: OperationCreateEntity, Element: id, Final: contract.SpatialTuple{Parent: testRootID, LocalPose: geometry.IdentityPose(), Range: testSphere(t, 1)}}, ParentContext{})
		if err == nil && p.Outcome().Status != contract.MutationRejected {
			t.Fatal("ordinary create reused departed UUID")
		}
	}
	if err := s.Validate([32]byte{1}, 3, ExternalParent{}); err != nil {
		t.Fatal(err)
	}
	if live := s.WithoutHistory(); !live.Initialized() || live.Counts().Departures != 0 || live.Claimed(testAnchorID) {
		t.Fatal("read projection retained history")
	}
}

func TestDepartureChecksWholeClosureBeforeChangingTables(t *testing.T) {
	missing := departureFixture(t)
	before := missing.Clone()
	if err := missing.Depart([]values.ElementID{testEntityID}, "", "out"); err == nil || !missing.Equal(before) {
		t.Fatal("missing controller was inferred or partially applied")
	}
	for _, ids := range [][]values.ElementID{nil, {testAnchorID}, {testEntityID, testEntityID}, {testEntityID, testExternalID}, {testEntityID, testRootID}} {
		s := departureFixture(t)
		before := s.Clone()
		if err := s.Depart(ids, departureController, "transfer"); err == nil {
			t.Fatalf("accepted incomplete/invalid closure: %v", ids)
		}
		if !s.Equal(before) {
			t.Fatalf("partial change for %v", ids)
		}
	}
}

func TestResumeDepartureRequiresExactInactiveGeneration(t *testing.T) {
	s := departureFixture(t)
	entity, _ := s.Entity(testEntityID)
	if err := s.Depart([]values.ElementID{testEntityID}, departureController, "out"); err != nil {
		t.Fatal(err)
	}
	exact, _ := s.Departure(testEntityID)
	for _, wrong := range []Departure{{}, {Controller: departureController, Revision: exact.Revision, Operation: "older"}, {Controller: departureController, Revision: testRevision(testEntityID, 2), Operation: exact.Operation}, {Revision: exact.Revision, Operation: exact.Operation}} {
		before := s.Clone()
		if err := s.ResumeDeparture(testEntityID, wrong); err == nil || !s.Equal(before) {
			t.Fatal("wrong generation resumed", err)
		}
	}
	if err := s.ResumeDeparture(testEntityID, exact); err != nil || !s.Claimed(testEntityID) || s.Kind(testEntityID) != 0 {
		t.Fatal("claim erased or record invented", err)
	}
	// Only the owner installs and publishes the complete candidate. Clearing a
	// departure alone cannot create a valid current state or reuse the UUID.
	if err := s.Validate([32]byte{1}, 3, ExternalParent{}); err == nil {
		t.Fatal("missing installation passed validation")
	}
	if err := s.RestoreEntity(entity); err != nil {
		t.Fatal(err)
	}
	before := s.Clone()
	if err := s.ResumeDeparture(testEntityID, exact); err == nil || !s.Equal(before) {
		t.Fatal("active generation resumed")
	}
}

func TestDepartureValidationRejectsDuplicateOrUnissuedRepresentation(t *testing.T) {
	for _, corrupt := range []func(*State){
		func(s *State) {
			delete(s.entities, testEntityID)
			s.departures[testEntityID] = Departure{Revision: testRevision(testEntityID, 3), Operation: "t"}
		},
		func(s *State) {
			s.departures[testEntityID] = Departure{Controller: departureController, Revision: testRevision(testEntityID, 3), Operation: "t"}
		},
		func(s *State) {
			delete(s.entities, testEntityID)
			s.departures[testEntityID] = Departure{Controller: departureController, Revision: testRevision(testEntityID, 4), Operation: "t"}
		},
		func(s *State) {
			delete(s.entities, testEntityID)
			s.departures[testEntityID] = Departure{Controller: departureController, Revision: testRevision(testEntityID, 3)}
		},
		func(s *State) {
			delete(s.entities, testEntityID)
			delete(s.claimed, testEntityID)
			s.departures[testEntityID] = Departure{Controller: departureController, Revision: testRevision(testEntityID, 3), Operation: "t"}
		},
	} {
		s := departureFixture(t)
		corrupt(&s)
		if err := s.Validate([32]byte{1}, 3, ExternalParent{}); err == nil {
			t.Fatal("invalid departure passed fact validation")
		}
	}
}
