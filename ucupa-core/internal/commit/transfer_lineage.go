package commit

import (
	"crypto/sha256"
	"encoding/json"
	"slices"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	"ucupa/ucupa-core/internal/token"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func installedLineageStep(member transferfact.Member, installed transferfact.TargetInstallation) (transferfact.LineageStep, error) {
	d := installed.Decision
	i := d.Acceptance.Fence.Input
	dBytes, err := json.Marshal(transferencoding.StoreTransferDecision(d))
	if err != nil {
		return transferfact.LineageStep{}, err
	}
	encoded := transferencoding.StoredTransferInstallation{Decision: transferencoding.StoreTransferDecision(d), Scope: token.Scope(installed.At), Cut: token.Serial(installed.At), Revision: installed.Revision}
	iBytes, err := json.Marshal(encoded)
	if err != nil {
		return transferfact.LineageStep{}, err
	}
	revision := member.Entity.Revision
	if member.Kind == contract.ElementAnchor {
		revision = member.Anchor.Revision
	}
	return transferfact.LineageStep{Controller: i.Key().Controller, Source: i.Source, Target: i.Target, Operation: i.Operation, Kind: member.Kind, SourceRevision: revision,
		FinalCut: d.Acceptance.Fence.FinalCut, InstalledCut: installed.At, Decision: sha256.Sum256(dBytes), Installation: sha256.Sum256(iBytes),
		SourceAddress: i.SourceAddress, TargetAddress: i.TargetAddress}, nil
}

// Enrich a fresh source capture from this owner's actual installed generations.
// Only transfer control captures decode these bounded manifests. Ordinary reads
// never walk history, and the outgoing evidence excludes unrelated old members.
func (s *coreState) captureTransferLineage(slice transferfact.Slice) (transferfact.Slice, error) {
	selected := make(map[values.ElementID]acceptedTransfer)
	for _, member := range slice.Members() {
		for _, accepted := range s.transfers.targets {
			if accepted.installedRevision() > selected[member.ID()].installedRevision() && slices.Contains(accepted.members, member.ID()) {
				selected[member.ID()] = accepted
			}
		}
	}
	decoded := make(map[transferfact.TransferKey]transferfact.Slice)
	chains := make(map[values.ElementID][]transferfact.LineageStep)
	total := 0
	for _, member := range slice.Members() {
		r := selected[member.ID()]
		if !r.installed() {
			continue
		}
		key := r.key()
		previous, ok := decoded[key]
		if !ok {
			var err error
			previous, err = s.acceptedManifest(r)
			if err != nil {
				return transferfact.Slice{}, err
			}
			decoded[key] = previous
		}
		for _, old := range previous.Members() {
			if old.ID() != member.ID() {
				continue
			}
			if old.Kind != member.Kind {
				return transferfact.Slice{}, contract.ErrStore
			}
			chain := previous.Lineage(member.ID())
			if len(chain)+1 > transferfact.MaxLineageSteps-total {
				return transferfact.Slice{}, values.PublicResourceExhausted
			}
			step, err := r.lineageStep(old)
			if err != nil {
				return transferfact.Slice{}, err
			}
			chains[member.ID()] = append(chain, step)
			total += len(chain) + 1
		}
		if len(chains[member.ID()]) == 0 {
			return transferfact.Slice{}, contract.ErrStore
		}
	}
	return slice.WithLineage(chains)
}

// Returning claims must anchor at this owner's most recent exact departure.
// A valid-looking chain alone never overrides an active UUID, permanent delete
// or unfinished local obligation. Future operation recovery settles that earlier
// obligation first; no timeout clears it here.
func (s *coreState) transferReturns(slice transferfact.Slice) (map[values.ElementID]elementfact.Departure, error) {
	returns := make(map[values.ElementID]elementfact.Departure)
	decoded := make(map[transferfact.TransferKey]transferfact.Slice)
	for _, member := range slice.Members() {
		id := member.ID()
		departure, departed := s.elements.Departure(id)
		if !departed {
			if s.elements.Claimed(id) {
				return nil, contract.ErrConflict
			}
			continue
		}
		key := transferfact.TransferKey{Controller: departure.Controller, Operation: departure.Operation}
		r, local := s.transfers.records[departure.Operation]
		local = local && r.Input.Key() == key
		carrier, carried := s.transfers.carrierSources[key]
		if local && r.Progress != transferfact.IntentSettled || !local && (!carried || !carrier.Settlement.Valid(s.authority)) {
			return nil, values.PublicUnavailable
		}
		previous, ok := decoded[key]
		if !ok {
			var err error
			if local {
				var f transferfact.SourceFence
				var raw string
				f, raw, err = s.sourceFence(r)
				if err == nil {
					previous, err = transferencoding.DecodeTransferManifest(f, raw)
				}
			} else {
				var f transferfact.CarrierFence
				var raw string
				f, raw, err = s.carrierSourceFence(carrier)
				if err == nil {
					previous, err = transferencoding.DecodeCarrierManifest(f, raw)
				}
			}
			if err != nil {
				return nil, err
			}
			decoded[key] = previous
		}
		var expected transferfact.LineageStep
		for _, old := range previous.Members() {
			if old.ID() != id {
				continue
			}
			var err error
			if local {
				expected, err = installedLineageStep(old, s.transfers.events[r.Revision-1].Installation)
			} else {
				expected, err = installedCarrierLineageStep(old, carrier.Settlement.Installation)
			}
			if err != nil {
				return nil, err
			}
		}
		if expected.SourceRevision != departure.Revision {
			return nil, contract.ErrStore
		}
		if expected.Kind != member.Kind {
			return nil, contract.ErrConflict
		}
		chain := slice.Lineage(id)
		last := -1
		for index, step := range chain {
			if step.Source == s.authority {
				last = index
			}
		}
		if last < 0 || chain[last] != expected {
			return nil, contract.ErrConflict
		}
		// Earlier custody is also immutable: do not splice a genuine local
		// successor onto a different purported origin or erase a prior cycle.
		if !slices.Equal(chain[:last], previous.Lineage(id)) {
			return nil, contract.ErrConflict
		}
		returns[id] = departure
	}
	return returns, nil
}
