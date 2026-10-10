package encoding

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/token"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

type storedCoreInstallation struct {
	Source  *StoredTransferInstallation `json:"source,omitempty"`
	Carrier *storedCarrierInstallation  `json:"carrier,omitempty"`
}

type storedEdgeInstallation struct {
	Parent storedCoreInstallation `json:"parent"`
	Mount  values.ElementID       `json:"mount"`
}

func storeCoreInstallation(i transferfact.CoreInstallation) storedCoreInstallation {
	if i.Source != (transferfact.TargetInstallation{}) {
		return storedCoreInstallation{Source: &StoredTransferInstallation{Decision: StoreTransferDecision(i.Source.Decision), Scope: token.Scope(i.Source.At), Cut: token.Serial(i.Source.At), Revision: i.Source.Revision}}
	}
	carrier := StoreCarrierInstallation(i.Carrier)
	return storedCoreInstallation{Carrier: &carrier}
}

func (s storedCoreInstallation) installation() (transferfact.CoreInstallation, error) {
	if (s.Source == nil) == (s.Carrier == nil) {
		return transferfact.CoreInstallation{}, contract.ErrInvalid
	}
	var i transferfact.CoreInstallation
	var err error
	if s.Source != nil {
		i.Source.Decision, err = s.Source.Decision.decision()
		i.Source.At = token.FromParts(s.Source.Scope, s.Source.Cut)
		i.Source.Revision = s.Source.Revision
	} else {
		i.Carrier, err = s.Carrier.installation()
	}
	if err != nil || !i.Valid() {
		return transferfact.CoreInstallation{}, contract.ErrInvalid
	}
	return i, nil
}
