package encoding

import (
	"ucupa/ucupa-core/contract"
	storerecord "ucupa/ucupa-core/internal/store/record"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

type StoredCarrierFence struct {
	Permit   storedCarrierPermit         `json:"permit"`
	FinalCut *storerecord.StoredMountCut `json:"finalCut"`
	Digest   [32]byte                    `json:"digest"`
	Revision uint64                      `json:"revision"`
}

type storedCarrierAcceptance struct {
	Fence    StoredCarrierFence          `json:"fence"`
	At       *storerecord.StoredMountCut `json:"at"`
	Revision uint64                      `json:"revision"`
}

func storeCarrierAcceptance(a transferfact.CarrierAcceptance) storedCarrierAcceptance {
	return storedCarrierAcceptance{Fence: storeCarrierFence(a.Fence), At: storerecord.StoreMountCut(a.At), Revision: a.Revision}
}

func (s storedCarrierAcceptance) acceptance() (transferfact.CarrierAcceptance, error) {
	f, err := s.Fence.fence()
	if err != nil {
		return transferfact.CarrierAcceptance{}, err
	}
	at, err := s.At.Cut()
	if err != nil {
		return transferfact.CarrierAcceptance{}, err
	}
	a := transferfact.CarrierAcceptance{Fence: f, At: at, Revision: s.Revision}
	if !a.Valid() {
		return transferfact.CarrierAcceptance{}, contract.ErrInvalid
	}
	return a, nil
}

func storeCarrierFence(f transferfact.CarrierFence) StoredCarrierFence {
	return StoredCarrierFence{Permit: storeCarrierPermit(f.Permit), FinalCut: storerecord.StoreMountCut(f.FinalCut), Digest: f.ManifestDigest, Revision: f.Revision}
}

func (s StoredCarrierFence) fence() (transferfact.CarrierFence, error) {
	c, err := s.Permit.permit()
	if err != nil {
		return transferfact.CarrierFence{}, err
	}
	at, err := s.FinalCut.Cut()
	if err != nil {
		return transferfact.CarrierFence{}, err
	}
	f := transferfact.CarrierFence{Permit: c, FinalCut: at, ManifestDigest: s.Digest, Revision: s.Revision}
	if !f.Valid() {
		return transferfact.CarrierFence{}, contract.ErrInvalid
	}
	return f, nil
}

func encodeCarrierAcceptanceEvent(e TransferEvent, s *StoredTransferEvent) error {
	if e.Kind == TransferCarrierParentSettled || e.Kind == TransferCarrierSourceSettled {
		if !e.CarrierInstallation.Valid() {
			return contract.ErrInvalid
		}
		i := StoreCarrierInstallation(e.CarrierInstallation)
		s.CarrierInstallation = &i
	} else if e.CarrierInstallation != (transferfact.CarrierInstallation{}) {
		return contract.ErrInvalid
	}
	if e.Kind == TransferCarrierInstalled {
		if !e.CarrierBinding.Valid() {
			return contract.ErrInvalid
		}
		b := StoreCarrierBinding(e.CarrierBinding)
		s.CarrierBinding = &b
	} else if e.CarrierBinding != (transferfact.CarrierBinding{}) {
		return contract.ErrInvalid
	}
	if e.Kind == TransferCarrierBound {
		if !e.CarrierAcceptance.Valid() {
			return contract.ErrInvalid
		}
		a := storeCarrierAcceptance(e.CarrierAcceptance)
		s.CarrierAcceptance = &a
	} else if e.CarrierAcceptance != (transferfact.CarrierAcceptance{}) {
		return contract.ErrInvalid
	}
	if e.Kind == TransferCarrierAccepted {
		if !e.CarrierFence.Valid() {
			return contract.ErrInvalid
		}
		f := storeCarrierFence(e.CarrierFence)
		s.CarrierFence = &f
	} else if e.CarrierFence != (transferfact.CarrierFence{}) {
		return contract.ErrInvalid
	}
	return nil
}

func decodeCarrierAcceptanceEvent(s StoredTransferEvent, e *TransferEvent) error {
	if s.Kind == TransferCarrierParentSettled || s.Kind == TransferCarrierSourceSettled {
		if s.CarrierInstallation == nil {
			return contract.ErrStore
		}
		i, err := s.CarrierInstallation.installation()
		if err != nil {
			return err
		}
		e.CarrierInstallation = i
	} else if s.CarrierInstallation != nil {
		return contract.ErrStore
	}
	if s.Kind == TransferCarrierInstalled {
		if s.CarrierBinding == nil {
			return contract.ErrStore
		}
		b, err := s.CarrierBinding.binding()
		if err != nil {
			return err
		}
		e.CarrierBinding = b
	} else if s.CarrierBinding != nil {
		return contract.ErrStore
	}
	if s.Kind == TransferCarrierBound {
		if s.CarrierAcceptance == nil {
			return contract.ErrStore
		}
		a, err := s.CarrierAcceptance.acceptance()
		if err != nil {
			return err
		}
		e.CarrierAcceptance = a
	} else if s.CarrierAcceptance != nil {
		return contract.ErrStore
	}
	if s.Kind == TransferCarrierAccepted {
		if s.CarrierFence == nil {
			return contract.ErrStore
		}
		f, err := s.CarrierFence.fence()
		if err != nil {
			return err
		}
		e.CarrierFence = f
	} else if s.CarrierFence != nil {
		return contract.ErrStore
	}
	return nil
}
