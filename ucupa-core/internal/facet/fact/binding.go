package fact

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	coretoken "ucupa/ucupa-core/internal/token"
)

type Kind uint8

const (
	Propose Kind = iota + 1
	Cancel
	Activate
	Detach
	Release
	Address
)

type Code uint8

const (
	OK Code = iota
	Missing
	RevisionConflict
	StateConflict
	ProposalNotCurrent
	AcceptanceMissing
	Changed
	ContactConflict
	StoreFailure
	DispositionMissing
)

func (code Code) Err() error {
	switch code {
	case OK:
		return nil
	case Missing:
		return contract.ErrNotFound
	case RevisionConflict:
		return contract.ErrAttachmentRevisionConflict
	case StateConflict:
		return contract.ErrAttachmentStateConflict
	case ProposalNotCurrent:
		return contract.ErrProposalNotCurrent
	case AcceptanceMissing:
		return contract.ErrAcceptanceNotFound
	case Changed:
		return contract.ErrAttachmentChanged
	case ContactConflict:
		return contract.ErrFacetContactRevisionConflict
	case DispositionMissing:
		return contract.ErrDispositionNotFound
	default:
		return contract.ErrStore
	}
}

type Binding struct {
	Actor    values.PrincipalRef
	Kind     Kind
	Element  values.ElementID
	Target   values.FactAuthorityRef
	Address  values.NextHop
	Expected values.AttachmentRevision
	Key      values.FacetAcceptanceKey
	Contact  contract.FacetContactRevision
}

func (b Binding) Permission() values.PermissionID {
	switch b.Kind {
	case Propose:
		return contract.PermissionFacetPropose
	case Cancel:
		return contract.PermissionFacetCancel
	case Activate:
		return contract.PermissionFacetActivate
	case Detach:
		return contract.PermissionFacetDetach
	case Release:
		return contract.PermissionFacetRelease
	case Address:
		return contract.PermissionFacetContactUpdate
	default:
		return ""
	}
}
func (b Binding) Valid() bool {
	if !b.Actor.Valid() || !b.Element.Valid() {
		return false
	}
	if b.Kind == Address {
		return b.Target == "" && b.Expected == (values.AttachmentRevision{}) && b.Key.Valid() && b.Key.Element == b.Element && b.Address.Valid() && coretoken.ValidFacetContactRevision(b.Contact)
	}
	if !b.Expected.Valid() || b.Contact != (contract.FacetContactRevision{}) || b.Key != (values.FacetAcceptanceKey{}) {
		return false
	}
	if b.Kind == Propose || b.Kind == Release {
		return b.Target.Valid() && b.Address.Valid()
	}
	return b.Kind >= Cancel && b.Kind <= Detach && b.Target == "" && b.Address == ""
}
