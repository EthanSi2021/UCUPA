package commit

import (
	"maps"
	"slices"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

type transferState struct {
	records        map[contract.OperationID]transferfact.IntentRecord
	events         []transferencoding.TransferEvent
	targets        map[transferfact.TransferKey]acceptedTransfer
	carrierSources map[transferfact.TransferKey]carrierSourceRecord
	edges          map[transferfact.EdgeKey]edgeRecord
	replacements   map[contract.OperationID]transferfact.FacetReplacementControl
}

type edgeRecord struct {
	closure      transferfact.EdgeClosure
	installation transferfact.EdgeInstallation
}

func newTransferState() transferState {
	return transferState{records: make(map[contract.OperationID]transferfact.IntentRecord), targets: make(map[transferfact.TransferKey]acceptedTransfer), carrierSources: make(map[transferfact.TransferKey]carrierSourceRecord), edges: make(map[transferfact.EdgeKey]edgeRecord), replacements: make(map[contract.OperationID]transferfact.FacetReplacementControl)}
}

func (s transferState) clone() transferState {
	out := newTransferState()
	for id, r := range s.records {
		out.records[id] = r
	}
	out.events = slices.Clone(s.events)
	out.targets = maps.Clone(s.targets) // immutable member/proof lists, never modified after acceptance
	out.carrierSources = maps.Clone(s.carrierSources)
	out.edges = maps.Clone(s.edges)
	out.replacements = maps.Clone(s.replacements)
	return out
}

func (s transferState) equal(other transferState) bool {
	if (s.records == nil) != (other.records == nil) || len(s.records) != len(other.records) || !slices.Equal(s.events, other.events) ||
		(s.targets == nil) != (other.targets == nil) || len(s.targets) != len(other.targets) ||
		(s.carrierSources == nil) != (other.carrierSources == nil) || !maps.Equal(s.carrierSources, other.carrierSources) ||
		(s.edges == nil) != (other.edges == nil) || !maps.Equal(s.edges, other.edges) ||
		(s.replacements == nil) != (other.replacements == nil) || !maps.Equal(s.replacements, other.replacements) {
		return false
	}
	for id, r := range s.records {
		if other.records[id] != r {
			return false
		}
	}
	for key, r := range s.targets {
		otherRecord, ok := other.targets[key]
		if !ok || r.parent != otherRecord.parent || r.receipt != otherRecord.receipt || r.carrier != otherRecord.carrier || r.installation != otherRecord.installation || r.carrierInstallation != otherRecord.carrierInstallation || r.completion != otherRecord.completion || !slices.Equal(r.members, otherRecord.members) || !slices.Equal(r.closures, otherRecord.closures) {
			return false
		}
	}
	return true
}

func (s *coreState) transferClaimed(id contract.OperationID) bool {
	if _, ok := s.transfers.records[id]; ok {
		return true
	}
	_, replaced := s.transfers.replacements[id]
	return replaced
}

// BeginTransferIntentLocked records input only, after source authorization.
// It is not approval to export. Before fence, the runtime must independently
// establish source/target-limited approval and revalidate the original views.
func (o *Owner) BeginTransferIntentLocked(input transferfact.Intent, maxRecords int) (transferfact.IntentRecord, error) {
	if !input.Valid() || maxRecords <= 0 {
		return transferfact.IntentRecord{}, contract.ErrInvalid
	}
	if err := o.transferPreflightReadyLocked(); err != nil {
		return transferfact.IntentRecord{}, err
	}
	if input.Source != o.authority {
		return transferfact.IntentRecord{}, contract.ErrConflict
	}
	if old, ok := o.state.transfers.records[input.Operation]; ok {
		if old.Input != input {
			return transferfact.IntentRecord{}, contract.ErrOperationConflict
		}
		return old, nil
	}
	if _, ok := o.state.operations[input.Operation]; ok || o.state.mountOperationClaimed(input.Operation) {
		return transferfact.IntentRecord{}, contract.ErrOperationConflict
	}
	if o.state.transferIntentCount() >= transferencoding.MaxTransferIntents {
		return transferfact.IntentRecord{}, values.PublicResourceExhausted
	}
	if _, err := transferencoding.EncodeTransferIntent(input); err != nil {
		return transferfact.IntentRecord{}, err
	}
	if _, err := o.PreflightTransferSourceLocked(input.Boundary, input.Expected, maxRecords); err != nil {
		return transferfact.IntentRecord{}, err
	}
	next := o.state.clone()
	event := transferencoding.TransferEvent{Kind: transferencoding.TransferPending, At: next.currentCut(), Input: input}
	if err := next.applyTransferEvent(event); err != nil {
		return transferfact.IntentRecord{}, err
	}
	if err := o.publishStateLocked(next); err != nil {
		return transferfact.IntentRecord{}, err
	}
	return o.state.transfers.records[input.Operation], nil
}

// CancelTransferIntentLocked is valid only before a fence exists. The caller
// must hold the one owner lock and pass the original authenticated identity.
// Future fence transitions use this same record and disallow cancellation;
// this is not permission to cancel a remote source with unknown durable state.
func (o *Owner) CancelTransferIntentLocked(actor values.PrincipalRef, id contract.OperationID, expected uint64) (transferfact.IntentRecord, error) {
	if !actor.Valid() || len(id) == 0 || len(id) > 128 || expected == 0 {
		return transferfact.IntentRecord{}, contract.ErrInvalid
	}
	if err := o.transferPreflightReadyLocked(); err != nil {
		return transferfact.IntentRecord{}, err
	}
	r, ok := o.state.transfers.records[id]
	if !ok {
		return transferfact.IntentRecord{}, contract.ErrNotFound
	}
	if r.Input.Actor != actor {
		return transferfact.IntentRecord{}, values.PublicDenied
	}
	if r.Progress == transferfact.IntentCancelled {
		// Repeating the original cancellation compares its original expected,
		// not the new revision returned by the cancelled result.
		event := o.state.transfers.events[r.Revision-1]
		if event.Expected != expected {
			return transferfact.IntentRecord{}, contract.ErrRevisionConflict
		}
		return r, nil
	}
	if r.Progress != transferfact.IntentPending || r.Revision != expected {
		return transferfact.IntentRecord{}, contract.ErrRevisionConflict
	}
	next := o.state.clone()
	event := transferencoding.TransferEvent{Kind: transferencoding.TransferCancelled, At: next.currentCut(), Operation: id, Actor: actor, Expected: expected}
	if err := next.applyTransferEvent(event); err != nil {
		return transferfact.IntentRecord{}, err
	}
	if err := o.publishStateLocked(next); err != nil {
		return transferfact.IntentRecord{}, err
	}
	return o.state.transfers.records[id], nil
}

// TransferIntentLocked returns one value, never a table or an authorization.
// Disclosure authorization belongs to the eventual calling operation.
func (o *Owner) TransferIntentLocked(id contract.OperationID) (transferfact.IntentRecord, error) {
	if err := o.transferPreflightReadyLocked(); err != nil {
		return transferfact.IntentRecord{}, err
	}
	r, ok := o.state.transfers.records[id]
	if !ok {
		return transferfact.IntentRecord{}, contract.ErrNotFound
	}
	return r, nil
}
