package encoding

import (
	"encoding/json"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	storerecord "ucupa/ucupa-core/internal/store/record"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// A carrier intent is part of the existing transfer journal. Its parent facts
// are derived by ordered replay, not saved in a second mount state table.
type storedCarrierIntent struct {
	Operation       []byte                           `json:"operation"`
	Actor           storerecord.StoredMountPrincipal `json:"actor"`
	Parent          values.FactAuthorityRef          `json:"parent"`
	Source          values.FactAuthorityRef          `json:"source"`
	Target          values.FactAuthorityRef          `json:"target"`
	Mount           values.ElementID                 `json:"mount"`
	ExpectedElement storerecord.StoredRevision       `json:"expectedElement"`
	ExpectedMount   *storerecord.StoredRevision      `json:"expectedMount"`
	ParentAddress   values.NextHop                   `json:"parentAddress"`
	SourceAddress   values.NextHop                   `json:"sourceAddress"`
	TargetAddress   values.NextHop                   `json:"targetAddress"`
}

func storeCarrierIntent(i transferfact.CarrierIntent) storedCarrierIntent {
	return storedCarrierIntent{Operation: []byte(i.Operation), Actor: storerecord.StoreMountPrincipal(i.Actor), Parent: i.Parent, Source: i.Source, Target: i.Target,
		Mount: i.Mount, ExpectedElement: storerecord.StoreRevision(i.ExpectedElement), ExpectedMount: storerecord.StoreMountRevision(i.ExpectedMount),
		ParentAddress: i.ParentAddress, SourceAddress: i.SourceAddress, TargetAddress: i.TargetAddress}
}

func EncodeCarrierIntent(i transferfact.CarrierIntent) ([]byte, error) {
	if !i.Valid() {
		return nil, contract.ErrInvalid
	}
	if len(i.Actor.Realm) > MaxTransferIntentBytes || len(i.Actor.Principal) > MaxTransferIntentBytes {
		return nil, values.PublicResourceExhausted
	}
	data, err := json.Marshal(storeCarrierIntent(i))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxTransferIntentBytes {
		return nil, values.PublicResourceExhausted
	}
	return data, nil
}

func (s storedCarrierIntent) intent() (transferfact.CarrierIntent, error) {
	e, err := s.ExpectedElement.Revision()
	if err != nil {
		return transferfact.CarrierIntent{}, err
	}
	m, err := storerecord.LoadMountRevision(s.ExpectedMount)
	if err != nil {
		return transferfact.CarrierIntent{}, err
	}
	i := transferfact.CarrierIntent{Operation: contract.OperationID(string(s.Operation)), Actor: s.Actor.DecodePrincipal(), Parent: s.Parent, Source: s.Source, Target: s.Target,
		Mount: s.Mount, ExpectedElement: e, ExpectedMount: m, ParentAddress: s.ParentAddress, SourceAddress: s.SourceAddress, TargetAddress: s.TargetAddress}
	_, err = EncodeCarrierIntent(i)
	return i, err
}

func encodeCarrierEvent(e TransferEvent, s *StoredTransferEvent) error {
	if err := encodeCarrierRefusalEvent(e, s); err != nil {
		return err
	}
	if e.Kind == TransferCarrierReserved {
		if _, err := EncodeCarrierIntent(e.Carrier); err != nil {
			return err
		}
		i := storeCarrierIntent(e.Carrier)
		s.Carrier = &i
	}
	s.CarrierExpected = storerecord.StoreMountCut(e.CarrierExpected)
	return nil
}

func decodeCarrierEvent(s StoredTransferEvent, e *TransferEvent) error {
	if err := decodeCarrierRefusalEvent(s, e); err != nil {
		return err
	}
	if s.Kind == TransferCarrierReserved {
		if s.Carrier == nil {
			return contract.ErrStore
		}
		i, err := s.Carrier.intent()
		if err != nil {
			return err
		}
		e.Carrier = i
	} else if s.Carrier != nil {
		return contract.ErrStore
	}
	if s.Kind == TransferCarrierPermitted || s.Kind == TransferCarrierCancelled {
		if s.CarrierExpected == nil {
			return contract.ErrStore
		}
		at, err := s.CarrierExpected.Cut()
		if err != nil {
			return err
		}
		e.CarrierExpected = at
	} else if s.CarrierExpected != nil {
		return contract.ErrStore
	}
	return nil
}
