package encoding

import (
	"ucupa/ucupa-core/contract"
	storerecord "ucupa/ucupa-core/internal/store/record"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

type storedCoreCompletion struct {
	At       *storerecord.StoredMountCut `json:"at"`
	Revision uint64                      `json:"revision"`
}

func SettlementParent(e TransferEvent) transferfact.CoreInstallation {
	return transferfact.CoreInstallation{Source: e.Installation, Carrier: e.CarrierInstallation}
}

func IsTransferSettlement(k TransferEventKind) bool {
	return k == TransferSourceSettled || k == TransferCarrierParentSettled || k == TransferCarrierSourceSettled
}

func encodeTransferCompletion(e TransferEvent, s *StoredTransferEvent) error {
	if !IsTransferSettlement(e.Kind) {
		if e.Completion != (transferfact.CoreCompletion{}) {
			return contract.ErrInvalid
		}
		return nil
	}
	if !e.Completion.Valid() || e.Completion.Installed != SettlementParent(e) {
		return contract.ErrInvalid
	}
	s.Completion = &storedCoreCompletion{At: storerecord.StoreMountCut(e.Completion.At), Revision: e.Completion.Revision}
	return nil
}

func decodeTransferCompletion(s StoredTransferEvent, e *TransferEvent) error {
	if !IsTransferSettlement(s.Kind) {
		if s.Completion != nil {
			return contract.ErrInvalid
		}
		return nil
	}
	if s.Completion == nil {
		return contract.ErrInvalid
	}
	at, err := s.Completion.At.Cut()
	if err != nil {
		return err
	}
	e.Completion = transferfact.CoreCompletion{Installed: SettlementParent(*e), At: at, Revision: s.Completion.Revision}
	if !e.Completion.Valid() {
		return contract.ErrInvalid
	}
	return nil
}
