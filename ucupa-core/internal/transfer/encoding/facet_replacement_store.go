package encoding

import (
	"encoding/json"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	storerecord "ucupa/ucupa-core/internal/store/record"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// A facet replacement is part of the existing transfer journal. Its control
// milestones are derived by ordered replay, never saved in a second table.
type storedFacetReplacementIntent struct {
	Operation     []byte                           `json:"operation"`
	Actor         storerecord.StoredMountPrincipal `json:"actor"`
	Element       values.ElementID                 `json:"element"`
	Source        values.FactAuthorityRef          `json:"source"`
	Target        values.FactAuthorityRef          `json:"target"`
	SourceKey     storerecord.StoredAttachmentKey  `json:"sourceKey"`
	SourceNextHop values.NextHop                   `json:"sourceNextHop"`
	SourceAddress values.NextHop                   `json:"sourceAddress"`
	TargetAddress values.NextHop                   `json:"targetAddress"`
	Expected      []byte                           `json:"expected"`
}

func storeFacetReplacementIntent(i transferfact.FacetReplacementIntent) storedFacetReplacementIntent {
	return storedFacetReplacementIntent{Operation: []byte(i.Operation), Actor: storerecord.StoreMountPrincipal(i.Actor),
		Element: i.Element, Source: i.Source, Target: i.Target, SourceKey: storerecord.StoreAttachmentKey(i.SourceKey),
		SourceNextHop: i.SourceNextHop, SourceAddress: i.SourceAddress, TargetAddress: i.TargetAddress, Expected: i.Expected.Bytes()}
}

func EncodeFacetReplacementIntent(i transferfact.FacetReplacementIntent) ([]byte, error) {
	if !i.Valid() {
		return nil, contract.ErrInvalid
	}
	if len(i.Actor.Realm) > MaxTransferIntentBytes || len(i.Actor.Principal) > MaxTransferIntentBytes {
		return nil, values.PublicResourceExhausted
	}
	data, err := json.Marshal(storeFacetReplacementIntent(i))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxTransferIntentBytes {
		return nil, values.PublicResourceExhausted
	}
	return data, nil
}

func (s storedFacetReplacementIntent) intent() (transferfact.FacetReplacementIntent, error) {
	expected, err := values.NewAttachmentRevision(s.Expected)
	if err != nil {
		return transferfact.FacetReplacementIntent{}, err
	}
	sourceKey, err := s.SourceKey.Key()
	if err != nil {
		return transferfact.FacetReplacementIntent{}, err
	}
	i := transferfact.FacetReplacementIntent{Operation: contract.OperationID(string(s.Operation)), Actor: s.Actor.DecodePrincipal(),
		Element: s.Element, Source: s.Source, Target: s.Target, SourceKey: sourceKey,
		SourceNextHop: s.SourceNextHop, SourceAddress: s.SourceAddress, TargetAddress: s.TargetAddress, Expected: expected}
	_, err = EncodeFacetReplacementIntent(i)
	return i, err
}

type storedFacetReplacementFence struct {
	FinalCut uint64   `json:"finalCut"`
	Digest   [32]byte `json:"digest"`
}

type storedFacetReplacementRefusal struct {
	Source values.FactAuthorityRef `json:"source"`
}

func encodeFacetReplacementEvent(e TransferEvent, s *StoredTransferEvent) error {
	if e.Kind == TransferFacetReserved {
		if _, err := EncodeFacetReplacementIntent(e.Facet); err != nil {
			return err
		}
		i := storeFacetReplacementIntent(e.Facet)
		s.Facet = &i
	} else if e.Facet != (transferfact.FacetReplacementIntent{}) {
		return contract.ErrInvalid
	}
	if e.Kind == TransferFacetFenced {
		if !e.FacetFence.Valid() {
			return contract.ErrInvalid
		}
		s.FacetFence = &storedFacetReplacementFence{FinalCut: e.FacetFence.FinalCut, Digest: e.FacetFence.ManifestDigest}
	} else if e.FacetFence != (transferfact.FacetReplacementFence{}) {
		return contract.ErrInvalid
	}
	if e.Kind == TransferFacetRefused {
		if !e.FacetRefusal.Valid() {
			return contract.ErrInvalid
		}
		s.FacetRefusal = &storedFacetReplacementRefusal{Source: e.FacetRefusal.Source}
	} else if e.FacetRefusal != (transferfact.FacetReplacementRefusal{}) {
		return contract.ErrInvalid
	}
	return nil
}

func decodeFacetReplacementEvent(s StoredTransferEvent, e *TransferEvent) error {
	if s.Kind == TransferFacetReserved {
		if s.Facet == nil {
			return contract.ErrStore
		}
		i, err := s.Facet.intent()
		if err != nil {
			return err
		}
		e.Facet = i
	} else if s.Facet != nil {
		return contract.ErrStore
	}
	if s.Kind == TransferFacetFenced {
		if s.FacetFence == nil {
			return contract.ErrStore
		}
		e.FacetFence = transferfact.FacetReplacementFence{FinalCut: s.FacetFence.FinalCut, ManifestDigest: s.FacetFence.Digest}
		if !e.FacetFence.Valid() {
			return contract.ErrInvalid
		}
	} else if s.FacetFence != nil {
		return contract.ErrStore
	}
	if s.Kind == TransferFacetRefused {
		if s.FacetRefusal == nil {
			return contract.ErrStore
		}
		e.FacetRefusal = transferfact.FacetReplacementRefusal{Source: s.FacetRefusal.Source}
		if !e.FacetRefusal.Valid() {
			return contract.ErrInvalid
		}
	} else if s.FacetRefusal != nil {
		return contract.ErrStore
	}
	return nil
}
