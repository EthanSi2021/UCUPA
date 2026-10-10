package fact

import (
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
)

func TestRootDepartureIsWholeSpaceAndNeverFreshCreate(t *testing.T) {
	ids := []values.ElementID{testRootID, testAnchorID, testEntityID}
	for _, selected := range [][]values.ElementID{nil, {testRootID}, {testRootID, testAnchorID}, {testRootID, testRootID, testEntityID}, {testRootID, testAnchorID, testExternalID}} {
		s := departureFixture(t)
		before := s.Clone()
		if err := s.DepartRoot(selected, departureController, "root-out", testRootID); err == nil || !s.Equal(before) {
			t.Fatal("partial root departure", selected, err)
		}
	}
	s := departureFixture(t)
	if err := s.DepartRoot(ids, departureController, "root-out", testRootID); err != nil {
		t.Fatal(err)
	}
	if id, ok := s.RetiredRoot(); !ok || id != testRootID || s.RootExists() {
		t.Fatal("not uniquely retired")
	}
	if err := s.Validate([32]byte{1}, 3, ExternalParent{}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []values.ElementID{testRootID, testExternalID} {
		p, err := s.Plan(Input{Kind: OperationCreateRoot, Element: id}, ParentContext{})
		if err == nil && p.Outcome().Status != contract.MutationRejected {
			t.Fatal("retired state treated as fresh Store")
		}
	}
	for _, mutate := range []func(*State){
		func(s *State) { d := s.departures[testRootID]; d.Root = false; s.departures[testRootID] = d },
		func(s *State) { d := s.departures[testEntityID]; d.Root = true; s.departures[testEntityID] = d },
		func(s *State) {
			fresh := departureFixture(t)
			a, _ := fresh.Anchor(testRootID)
			delete(s.departures, testRootID)
			_ = s.RestoreAnchor(a)
			d := s.departures[testEntityID]
			d.Root = true
			s.departures[testEntityID] = d
		},
	} {
		bad := s.Clone()
		mutate(&bad)
		if err := bad.Validate([32]byte{1}, 3, ExternalParent{}); err == nil {
			t.Fatal("invalid retired root role accepted")
		}
	}
}
