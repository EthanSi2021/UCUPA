package encoding

import (
	"ucupa/ucupa-core/contract"
	storerecord "ucupa/ucupa-core/internal/store/record"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

type storedCarrierBinding struct {
	Acceptance storedCarrierAcceptance     `json:"acceptance"`
	At         *storerecord.StoredMountCut `json:"at"`
	Revision   uint64                      `json:"revision"`
}

type storedCarrierInstallation struct {
	Binding  storedCarrierBinding        `json:"binding"`
	At       *storerecord.StoredMountCut `json:"at"`
	Revision uint64                      `json:"revision"`
}

func StoreCarrierBinding(b transferfact.CarrierBinding) storedCarrierBinding {
	return storedCarrierBinding{Acceptance: storeCarrierAcceptance(b.Acceptance), At: storerecord.StoreMountCut(b.At), Revision: b.Revision}
}

func (s storedCarrierBinding) binding() (transferfact.CarrierBinding, error) {
	a, err := s.Acceptance.acceptance()
	if err != nil {
		return transferfact.CarrierBinding{}, err
	}
	at, err := s.At.Cut()
	if err != nil {
		return transferfact.CarrierBinding{}, err
	}
	b := transferfact.CarrierBinding{Acceptance: a, At: at, Revision: s.Revision}
	if !b.Valid() {
		return transferfact.CarrierBinding{}, contract.ErrInvalid
	}
	return b, nil
}

func StoreCarrierInstallation(i transferfact.CarrierInstallation) storedCarrierInstallation {
	return storedCarrierInstallation{Binding: StoreCarrierBinding(i.Binding), At: storerecord.StoreMountCut(i.At), Revision: i.Revision}
}

func (s storedCarrierInstallation) installation() (transferfact.CarrierInstallation, error) {
	b, err := s.Binding.binding()
	if err != nil {
		return transferfact.CarrierInstallation{}, err
	}
	at, err := s.At.Cut()
	if err != nil {
		return transferfact.CarrierInstallation{}, err
	}
	i := transferfact.CarrierInstallation{Binding: b, At: at, Revision: s.Revision}
	if !i.Valid() {
		return transferfact.CarrierInstallation{}, contract.ErrInvalid
	}
	return i, nil
}
