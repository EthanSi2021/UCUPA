package encoding

import (
	"encoding/json"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	storerecord "ucupa/ucupa-core/internal/store/record"
	"ucupa/ucupa-core/internal/token"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// The existing journal stores exact input once and derives the parent's held
// relation from ordered replay. There is no additional persistence domain.
type storedParentInitial struct {
	Input    StoredTransferIntent        `json:"input"`
	At       *storerecord.StoredMountCut `json:"at"`
	Revision uint64                      `json:"revision"`
}

type storedParentIntent struct {
	Initial         storedParentInitial         `json:"initial"`
	Role            transferfact.ParentRole     `json:"role"`
	Parent          values.FactAuthorityRef     `json:"parent"`
	Mount           values.ElementID            `json:"mount"`
	ExpectedElement storerecord.StoredRevision  `json:"expectedElement"`
	ExpectedMount   *storerecord.StoredRevision `json:"expectedMount"`
	ParentAddress   values.NextHop              `json:"parentAddress"`
}

type StoredParentSettlement struct {
	Installation StoredTransferInstallation  `json:"installation"`
	At           *storerecord.StoredMountCut `json:"at"`
	Revision     uint64                      `json:"revision"`
}

type storedParentRelease struct {
	Cancelled *storedParentInitial    `json:"cancelled,omitempty"`
	Settled   *StoredParentSettlement `json:"settled,omitempty"`
}

func storeParentInitial(r transferfact.IntentRecord) storedParentInitial {
	return storedParentInitial{Input: storeTransferIntent(r.Input), At: storerecord.StoreMountCut(r.Admission), Revision: r.Revision}
}

func (s storedParentInitial) record(progress transferfact.IntentProgress) (transferfact.IntentRecord, error) {
	i, err := s.Input.intent()
	if err != nil {
		return transferfact.IntentRecord{}, err
	}
	at, err := s.At.Cut()
	return transferfact.IntentRecord{Input: i, Admission: at, Revision: s.Revision, Progress: progress}, err
}

func storeTransferParentIntent(i transferfact.ParentIntent) storedParentIntent {
	return storedParentIntent{Initial: storeParentInitial(i.Initial), Role: i.Role, Parent: i.Parent, Mount: i.Mount,
		ExpectedElement: storerecord.StoreRevision(i.ExpectedElement), ExpectedMount: storerecord.StoreMountRevision(i.ExpectedMount), ParentAddress: i.ParentAddress}
}

func EncodeTransferParentIntent(i transferfact.ParentIntent) ([]byte, error) {
	if !i.Valid() {
		return nil, contract.ErrInvalid
	}
	if _, err := EncodeTransferIntent(i.Initial.Input); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(storeTransferParentIntent(i))
	if len(raw) > MaxTransferIntentBytes {
		return nil, values.PublicResourceExhausted
	}
	return raw, err
}

func (s storedParentIntent) intent() (transferfact.ParentIntent, error) {
	r, err := s.Initial.record(transferfact.IntentPending)
	if err != nil {
		return transferfact.ParentIntent{}, err
	}
	a, err := s.ExpectedElement.Revision()
	if err != nil {
		return transferfact.ParentIntent{}, err
	}
	m, err := storerecord.LoadMountRevision(s.ExpectedMount)
	if err != nil {
		return transferfact.ParentIntent{}, err
	}
	i := transferfact.ParentIntent{Initial: r, Role: s.Role, Parent: s.Parent, Mount: s.Mount, ExpectedElement: a, ExpectedMount: m, ParentAddress: s.ParentAddress}
	_, err = EncodeTransferParentIntent(i)
	return i, err
}

func encodeTransferParentEvent(e TransferEvent, s *StoredTransferEvent) error {
	if e.Kind == TransferParentReserved {
		if _, err := EncodeTransferParentIntent(e.Parent); err != nil {
			return err
		}
		v := storeTransferParentIntent(e.Parent)
		s.Parent = &v
	} else if e.Parent != (transferfact.ParentIntent{}) {
		return contract.ErrInvalid
	}
	if e.Kind != TransferParentReleased {
		if e.ParentRelease != (transferfact.ParentRelease{}) {
			return contract.ErrInvalid
		}
		return nil
	}
	r := e.ParentRelease
	if (r.Cancelled == (transferfact.IntentRecord{})) == (r.Settled == (transferfact.SourceSettlement{})) {
		return contract.ErrInvalid
	}
	s.ParentRelease = &storedParentRelease{}
	if r.Cancelled != (transferfact.IntentRecord{}) {
		c := r.Cancelled
		if c.Progress != transferfact.IntentCancelled || c.FinalCut != (contract.CoreReadCut{}) || c.FenceRevision != 0 || c.DecisionRevision != 0 || c.Origin != (contract.SpatialTuple{}) {
			return contract.ErrInvalid
		}
		v := storeParentInitial(c)
		s.ParentRelease.Cancelled = &v
	} else {
		t := r.Settled
		s.ParentRelease.Settled = &StoredParentSettlement{Installation: StoredTransferInstallation{
			Decision: StoreTransferDecision(t.Installation.Decision), Scope: token.Scope(t.Installation.At),
			Cut: token.Serial(t.Installation.At), Revision: t.Installation.Revision}, At: storerecord.StoreMountCut(t.At), Revision: t.Revision}
	}
	return nil
}

func decodeTransferParentEvent(s StoredTransferEvent, e *TransferEvent) error {
	if s.Kind == TransferParentReserved {
		if s.Parent == nil {
			return contract.ErrInvalid
		}
		i, err := s.Parent.intent()
		if err != nil {
			return err
		}
		e.Parent = i
	} else if s.Parent != nil {
		return contract.ErrInvalid
	}
	if s.Kind != TransferParentReleased {
		if s.ParentRelease != nil {
			return contract.ErrInvalid
		}
		return nil
	}
	r := s.ParentRelease
	if r == nil || (r.Cancelled == nil) == (r.Settled == nil) {
		return contract.ErrInvalid
	}
	if r.Cancelled != nil {
		c, err := r.Cancelled.record(transferfact.IntentCancelled)
		e.ParentRelease.Cancelled = c
		return err
	}
	d, err := r.Settled.Installation.Decision.decision()
	if err != nil {
		return err
	}
	at, err := r.Settled.At.Cut()
	i := r.Settled.Installation
	e.ParentRelease.Settled = transferfact.SourceSettlement{Installation: transferfact.TargetInstallation{Decision: d,
		At: token.FromParts(i.Scope, i.Cut), Revision: i.Revision}, At: at, Revision: r.Settled.Revision}
	return err
}
