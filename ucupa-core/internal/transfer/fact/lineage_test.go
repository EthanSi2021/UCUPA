package fact

import (
	"errors"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/token"
)

func lineageStep(member Member) LineageStep {
	return LineageStep{Controller: targetOwner, Source: targetOwner, Target: sourceOwner, Operation: "previous", Kind: member.Kind,
		SourceRevision: token.RevisionFromParts([32]byte{2}, member.ID(), 1), FinalCut: token.FromParts([32]byte{2}, 20),
		InstalledCut: token.FromParts([32]byte{1}, 1), Decision: [32]byte{1}, Installation: [32]byte{2}, SourceAddress: "local:previous", TargetAddress: "local:current"}
}

func TestLineageValuesAreBoundedMemberSpecificAndDetached(t *testing.T) {
	e, f, m := tree(t)
	s := mustSelect(t, e, f, m)
	member := s.Members()[0]
	step := lineageStep(member)
	chain := []LineageStep{step}
	input := map[values.ElementID][]LineageStep{member.ID(): chain}
	bound, err := s.WithLineage(input)
	if err != nil || len(s.Lineage(member.ID())) != 0 {
		t.Fatal("mutated source slice", err)
	}
	chain[0].Decision[0]++
	delete(input, member.ID())
	out := bound.Lineage(member.ID())
	if len(out) != 1 || out[0] != step {
		t.Fatal("aliased input")
	}
	out[0].Installation[0]++
	if bound.Lineage(member.ID())[0] != step || bound.Lineage(id(999)) != nil {
		t.Fatal("aliased output or unselected history")
	}
	for _, invalid := range []map[values.ElementID][]LineageStep{{id(999): {step}}, {member.ID(): nil}} {
		if _, err := s.WithLineage(invalid); !errors.Is(err, contract.ErrInvalid) {
			t.Fatal("invalid member selection", err)
		}
	}
	if _, err := s.WithLineage(map[values.ElementID][]LineageStep{member.ID(): make([]LineageStep, MaxLineageSteps+1)}); !errors.Is(err, values.PublicResourceExhausted) {
		t.Fatal("unbounded custody chain", err)
	}
}

func TestLineageRejectsDisconnectedOrInvalidValues(t *testing.T) {
	e, f, m := tree(t)
	s := mustSelect(t, e, f, m)
	member := s.Members()[0]
	for name, change := range map[string]func(*LineageStep){
		"missing-controller":   func(s *LineageStep) { s.Controller = "" },
		"missing-source":       func(s *LineageStep) { s.Source = "" },
		"wrong-recipient":      func(s *LineageStep) { s.Target = targetOwner },
		"empty-operation":      func(s *LineageStep) { s.Operation = "" },
		"wrong-kind":           func(s *LineageStep) { s.Kind = contract.ElementEntity },
		"wrong-member":         func(s *LineageStep) { s.SourceRevision = token.RevisionFromParts([32]byte{2}, id(999), 1) },
		"wrong-source-scope":   func(s *LineageStep) { s.SourceRevision = revision(2) },
		"equal-scopes":         func(s *LineageStep) { s.InstalledCut = s.FinalCut },
		"future-install":       func(s *LineageStep) { s.InstalledCut = token.FromParts([32]byte{1}, 101) },
		"missing-decision":     func(s *LineageStep) { s.Decision = [32]byte{} },
		"missing-installation": func(s *LineageStep) { s.Installation = [32]byte{} },
		"no-address":           func(s *LineageStep) { s.TargetAddress = "" },
	} {
		t.Run(name, func(t *testing.T) {
			step := lineageStep(member)
			change(&step)
			if _, err := s.WithLineage(map[values.ElementID][]LineageStep{member.ID(): {step}}); !errors.Is(err, contract.ErrConflict) {
				t.Fatal("invalid step accepted", err)
			}
		})
	}
	last := lineageStep(member)
	first := last
	first.Source = values.FactAuthorityRef(id(30))
	first.Controller = first.Source
	first.Target = last.Source
	first.SourceRevision = token.RevisionFromParts([32]byte{3}, member.ID(), 1)
	first.FinalCut, first.InstalledCut = token.FromParts([32]byte{3}, 2), token.FromParts([32]byte{2}, 20)
	for _, incorrect := range []string{"none", "backwards-cut", "wrong-owner", "same-controller-operation", "duplicate-operation"} {
		chain := []LineageStep{first, last}
		switch incorrect {
		case "backwards-cut":
			chain[0].InstalledCut = token.FromParts([32]byte{2}, 21)
		case "wrong-owner":
			chain[0].Target = sourceOwner
		case "same-controller-operation":
			chain[0].Controller = chain[1].Controller
		case "duplicate-operation":
			chain = append(chain, first, last)
		}
		_, err := s.WithLineage(map[values.ElementID][]LineageStep{member.ID(): chain})
		if (err == nil) != (incorrect == "none") {
			t.Fatal("chain continuity", incorrect, err)
		}
	}
}
