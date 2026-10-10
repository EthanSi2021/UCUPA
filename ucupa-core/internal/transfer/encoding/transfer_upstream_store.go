package encoding

import (
	"ucupa/ucupa-core/contract"
	storerecord "ucupa/ucupa-core/internal/store/record"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// Source manifests and target acceptance retain the same upstream promise
// codec. Neither copies the parent's Element tree or invents a new Store.
type StoredTransferParentHold struct {
	Input     storedParentIntent                      `json:"input"`
	Admission *storerecord.StoredMountAdmission       `json:"admission"`
	Tuple     storerecord.StoredTuple                 `json:"tuple"`
	Contact   *storerecord.StoredMountContactRevision `json:"contact"`
	At        *storerecord.StoredMountCut             `json:"at"`
	Revision  uint64                                  `json:"revision"`
}

func storeTransferParentHold(h transferfact.ParentHold) StoredTransferParentHold {
	return StoredTransferParentHold{Input: storeTransferParentIntent(h.Input), Admission: storerecord.StoreMountAdmission(h.Admission),
		Tuple: storerecord.StoreTuple(h.Tuple), Contact: storerecord.StoreMountContactRevision(h.Contact), At: storerecord.StoreMountCut(h.At), Revision: h.Revision}
}

func (s StoredTransferParentHold) hold() (transferfact.ParentHold, error) {
	i, err := s.Input.intent()
	if err != nil {
		return transferfact.ParentHold{}, err
	}
	a, err := s.Admission.Admission()
	if err != nil {
		return transferfact.ParentHold{}, err
	}
	tuple, err := s.Tuple.Tuple()
	if err != nil {
		return transferfact.ParentHold{}, err
	}
	c, err := s.Contact.Revision()
	if err != nil {
		return transferfact.ParentHold{}, err
	}
	at, err := s.At.Cut()
	h := transferfact.ParentHold{Input: i, Admission: a, Tuple: tuple, Contact: c, At: at, Revision: s.Revision}
	if err != nil || !h.Valid() {
		return transferfact.ParentHold{}, contract.ErrInvalid
	}
	return h, nil
}
