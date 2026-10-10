package commit

import (
	"crypto/sha256"
	"encoding/json"
	"ucupa/ucupa-core/contract"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func installedCarrierLineageStep(member transferfact.Member, installed transferfact.CarrierInstallation) (transferfact.LineageStep, error) {
	b := installed.Binding
	i := b.Acceptance.Fence.Permit.Reservation.Input
	dBytes, err := json.Marshal(transferencoding.StoreCarrierBinding(b))
	if err != nil {
		return transferfact.LineageStep{}, err
	}
	iBytes, err := json.Marshal(transferencoding.StoreCarrierInstallation(installed))
	if err != nil {
		return transferfact.LineageStep{}, err
	}
	revision := member.Entity.Revision
	if member.Kind == contract.ElementAnchor {
		revision = member.Anchor.Revision
	}
	return transferfact.LineageStep{Controller: i.Parent, Source: i.Source, Target: i.Target, Operation: i.Operation, Kind: member.Kind, SourceRevision: revision,
		FinalCut: b.Acceptance.Fence.FinalCut, InstalledCut: installed.At, Decision: sha256.Sum256(dBytes), Installation: sha256.Sum256(iBytes),
		SourceAddress: i.SourceAddress, TargetAddress: i.TargetAddress}, nil
}

func (r acceptedTransfer) installedRevision() uint64 {
	if r.carrier.Revision != 0 {
		return r.carrierInstallation.Revision
	}
	return r.installation.Revision
}

func (r acceptedTransfer) key() transferfact.TransferKey {
	if r.carrier.Revision != 0 {
		return r.carrier.Fence.Permit.Reservation.Input.Key()
	}
	return r.receipt.Fence.Key()
}

func (s *coreState) acceptedManifest(r acceptedTransfer) (transferfact.Slice, error) {
	if r.carrier.Revision != 0 {
		return transferencoding.DecodeCarrierManifest(r.carrier.Fence, s.transfers.events[r.carrier.Revision-1].Manifest)
	}
	return transferencoding.DecodeTransferManifest(r.receipt.Fence, s.transfers.events[r.receipt.Revision-1].Manifest)
}

func (r acceptedTransfer) lineageStep(member transferfact.Member) (transferfact.LineageStep, error) {
	if r.carrier.Revision != 0 {
		return installedCarrierLineageStep(member, r.carrierInstallation)
	}
	return installedLineageStep(member, r.installation)
}
