package commit

import (
	"errors"
	"fmt"
	"maps"

	"ucupa/ucupa-core/contract"
	commitbinding "ucupa/ucupa-core/internal/commit/binding"
	"ucupa/ucupa-core/internal/token"
)

// mountHistory positions the control journal in the one Core commit sequence.
// A journal entry without a fact change remains at its original cut; it is
// still replayed, including rejected and unchanged original results.
type mountHistory struct {
	events        []MountEvent
	index         int
	mountIssued   map[contract.MountRevision]uint64
	contactIssued map[contract.NextHopRevision]uint64
}

func newMountHistory(events []MountEvent) *mountHistory {
	return &mountHistory{events: events, mountIssued: make(map[contract.MountRevision]uint64), contactIssued: make(map[contract.NextHopRevision]uint64)}
}

func (history *mountHistory) decisions(state *coreState, issued map[contract.Revision]uint64) error {
	for history.index < len(history.events) {
		event := history.events[history.index]
		if token.Scope(event.BeforeCut) != state.scope || token.Scope(event.AfterCut) != state.scope || token.Serial(event.BeforeCut) < state.cutSerial {
			return errors.New("control journal is out of order or belongs to another Core")
		}
		if token.Serial(event.BeforeCut) != state.cutSerial || event.AfterCut != event.BeforeCut {
			return nil
		}
		if err := history.apply(state, event, issued); err != nil {
			return err
		}
	}
	return nil
}

func (history *mountHistory) commit(state *coreState, commit coreCommit, issued map[contract.Revision]uint64) error {
	if commit.operation != "" || commit.controlIndex != uint64(history.index)+1 || history.index >= len(history.events) {
		return errors.New("control commit does not bind the next journal entry")
	}
	event := history.events[history.index]
	if event.BeforeCut != state.currentCut() || token.Scope(event.AfterCut) != state.scope || token.Serial(event.AfterCut) != state.cutSerial+1 {
		return errors.New("control commit cut is invalid")
	}
	if err := history.apply(state, event, issued); err != nil {
		return err
	}
	if len(state.commits) == 0 || state.commits[len(state.commits)-1] != commit {
		return errors.New("control transition did not reproduce its commit")
	}
	return nil
}

func (history *mountHistory) apply(state *coreState, event MountEvent, issued map[contract.Revision]uint64) error {
	if err := history.validateExpected(event, state, issued); err != nil {
		return err
	}
	previousRevision := state.revisionSerial
	if err := applyMountEvent(state, event); err != nil {
		return fmt.Errorf("control journal entry %d: %w", history.index+1, err)
	}
	if len(state.mount.events) != history.index+1 || state.mount.events[history.index] != event {
		return errors.New("control transition did not reproduce its journal entry")
	}
	history.index++
	history.capture(state)
	if state.revisionSerial != previousRevision {
		if state.revisionSerial != previousRevision+1 {
			return errors.New("control event issued more than one Element Revision")
		}
		found := false
		for _, anchor := range state.elements.Anchors() {
			if token.RevisionSerial(anchor.Revision) == state.revisionSerial {
				issued[anchor.Revision] = state.cutSerial
				found = true
			}
		}
		if !found {
			return errors.New("control event issued an unrepresented Element Revision")
		}
	}
	return nil
}

// Transfer decisions issue real mount/contact revisions in the same ordered
// Core history. Register only reconstructed current facts, before a later
// ordinary control decision may legitimately use those revisions as CAS.
func (history *mountHistory) capture(state *coreState) {
	for _, record := range state.mount.facts.Outgoings() {
		if token.ValidMountRevision(record.Revision) {
			history.mountIssued[record.Revision] = token.MountRevisionSerial(record.Revision)
		}
		if token.ValidNextHopRevision(record.Contact.Revision) {
			history.contactIssued[record.Contact.Revision] = token.NextHopSerial(record.Contact.Revision)
		}
	}
	contact := state.mount.facts.Incoming().Contact.Revision
	if token.ValidNextHopRevision(contact) {
		history.contactIssued[contact] = token.NextHopSerial(contact)
	}
}

func (history *mountHistory) validateExpected(event MountEvent, state *coreState, issued map[contract.Revision]uint64) error {
	if event.Kind == MountEventReserve {
		command := event.Binding.Command
		if err := validateExpectedIssued(commitbinding.Binding{Expected: command.ExpectedElementRevision}, issued, state.scope, state.cutSerial); err != nil {
			return err
		}
		expected := command.ExpectedMountRevision
		if expected != (contract.MountRevision{}) && token.MountRevisionScope(expected) == state.scope {
			at, exists := history.mountIssued[expected]
			if !exists || at > state.cutSerial {
				return errors.New("expected MountRevision was not issued before this intent")
			}
		}
	}
	if event.Kind == MountEventAddress {
		expected := event.Address.Expected
		if expected != (contract.NextHopRevision{}) && token.NextHopScope(expected) == state.scope {
			at, exists := history.contactIssued[expected]
			if !exists || at > state.cutSerial {
				return errors.New("expected NextHopRevision was not issued before this update")
			}
		}
	}
	return nil
}

func sameMountFacts(left, right mountState) bool {
	return left.facts.Equal(right.facts) &&
		maps.Equal(left.operations, right.operations) && maps.Equal(left.addresses, right.addresses) &&
		maps.Equal(left.attempts, right.attempts)
}
