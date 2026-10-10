package encoding

import (
	"encoding/json"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	storerecord "ucupa/ucupa-core/internal/store/record"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func EncodeCarrierPermit(c transferfact.CarrierControl) ([]byte, error) {
	if !c.Valid() || c.PermitAt == (contract.CoreReadCut{}) || c.CancelledAt != (contract.CoreReadCut{}) {
		return nil, contract.ErrInvalid
	}
	if _, err := EncodeCarrierIntent(c.Reservation.Input); err != nil {
		return nil, err
	}
	return json.Marshal(storeCarrierPermit(c))
}

// Peer control evidence reuses the existing exact token and opaque-ID codecs.
// No decoded value substitutes for authentication of the source that supplied it.
type storedCarrierPermit struct {
	Input           storedCarrierIntent                     `json:"input"`
	Previous        *storerecord.StoredMountAdmission       `json:"previous"`
	PreviousContact *storerecord.StoredMountContactRevision `json:"previousContact"`
	Tuple           storerecord.StoredTuple                 `json:"tuple"`
	NextKey         *storerecord.StoredMountKey             `json:"nextKey"`
	ReservationAt   *storerecord.StoredMountCut             `json:"reservationAt"`
	PermitAt        *storerecord.StoredMountCut             `json:"permitAt"`
}

type storedCarrierRefusal struct {
	Controller    values.FactAuthorityRef     `json:"controller"`
	Operation     []byte                      `json:"operation"`
	Source        values.FactAuthorityRef     `json:"source"`
	ReservationAt *storerecord.StoredMountCut `json:"reservationAt"`
	PermitAt      *storerecord.StoredMountCut `json:"permitAt"`
	At            *storerecord.StoredMountCut `json:"at"`
	Revision      uint64                      `json:"revision"`
}

func storeCarrierPermit(c transferfact.CarrierControl) storedCarrierPermit {
	r := c.Reservation
	return storedCarrierPermit{Input: storeCarrierIntent(r.Input), Previous: storerecord.StoreMountAdmission(r.Previous), PreviousContact: storerecord.StoreMountContactRevision(r.PreviousContact),
		Tuple: storerecord.StoreTuple(r.Tuple), NextKey: storerecord.StoreMountKey(r.NextKey), ReservationAt: storerecord.StoreMountCut(r.At), PermitAt: storerecord.StoreMountCut(c.PermitAt)}
}

func (s storedCarrierPermit) permit() (transferfact.CarrierControl, error) {
	i, err := s.Input.intent()
	if err != nil {
		return transferfact.CarrierControl{}, err
	}
	p, err := s.Previous.Admission()
	if err != nil {
		return transferfact.CarrierControl{}, err
	}
	contact, err := s.PreviousContact.Revision()
	if err != nil {
		return transferfact.CarrierControl{}, err
	}
	tuple, err := s.Tuple.Tuple()
	if err != nil {
		return transferfact.CarrierControl{}, err
	}
	k, err := s.NextKey.Key()
	if err != nil {
		return transferfact.CarrierControl{}, err
	}
	at, err := s.ReservationAt.Cut()
	if err != nil {
		return transferfact.CarrierControl{}, err
	}
	permitAt, err := s.PermitAt.Cut()
	if err != nil {
		return transferfact.CarrierControl{}, err
	}
	c := transferfact.CarrierControl{Reservation: transferfact.CarrierReservation{Input: i, Previous: p, PreviousContact: contact, Tuple: tuple, NextKey: k, At: at}, PermitAt: permitAt}
	if !c.Valid() || c.PermitAt == (contract.CoreReadCut{}) {
		return transferfact.CarrierControl{}, contract.ErrInvalid
	}
	return c, nil
}

func storeCarrierRefusal(r transferfact.CarrierRefusal) storedCarrierRefusal {
	return storedCarrierRefusal{Controller: r.Key.Controller, Operation: []byte(r.Key.Operation), Source: r.Source,
		ReservationAt: storerecord.StoreMountCut(r.ReservationAt), PermitAt: storerecord.StoreMountCut(r.PermitAt), At: storerecord.StoreMountCut(r.At), Revision: r.Revision}
}

func (s storedCarrierRefusal) refusal() (transferfact.CarrierRefusal, error) {
	at, err := s.At.Cut()
	if err != nil {
		return transferfact.CarrierRefusal{}, err
	}
	rAt, err := s.ReservationAt.Cut()
	if err != nil {
		return transferfact.CarrierRefusal{}, err
	}
	pAt, err := s.PermitAt.Cut()
	if err != nil {
		return transferfact.CarrierRefusal{}, err
	}
	r := transferfact.CarrierRefusal{Key: transferfact.TransferKey{Controller: s.Controller, Operation: contract.OperationID(string(s.Operation))}, Source: s.Source,
		ReservationAt: rAt, PermitAt: pAt, At: at, Revision: s.Revision}
	if !r.Valid() {
		return transferfact.CarrierRefusal{}, contract.ErrInvalid
	}
	return r, nil
}

func encodeCarrierRefusalEvent(e TransferEvent, s *StoredTransferEvent) error {
	if e.Kind == TransferCarrierRefused || e.Kind == TransferCarrierFenced {
		if !e.CarrierPermit.Valid() || e.CarrierPermit.PermitAt == (contract.CoreReadCut{}) || e.CarrierPermit.CancelledAt != (contract.CoreReadCut{}) {
			return contract.ErrInvalid
		}
		v := storeCarrierPermit(e.CarrierPermit)
		s.CarrierPermit = &v
	} else if e.CarrierPermit != (transferfact.CarrierControl{}) {
		return contract.ErrInvalid
	}
	if e.Kind == TransferCarrierAborted {
		if !e.CarrierRefusal.Valid() {
			return contract.ErrInvalid
		}
		v := storeCarrierRefusal(e.CarrierRefusal)
		s.CarrierRefusal = &v
	} else if e.CarrierRefusal != (transferfact.CarrierRefusal{}) {
		return contract.ErrInvalid
	}
	return nil
}

func decodeCarrierRefusalEvent(s StoredTransferEvent, e *TransferEvent) error {
	if s.Kind == TransferCarrierRefused || s.Kind == TransferCarrierFenced {
		if s.CarrierPermit == nil {
			return contract.ErrStore
		}
		c, err := s.CarrierPermit.permit()
		if err != nil {
			return err
		}
		e.CarrierPermit = c
	} else if s.CarrierPermit != nil {
		return contract.ErrStore
	}
	if s.Kind == TransferCarrierAborted {
		if s.CarrierRefusal == nil {
			return contract.ErrStore
		}
		r, err := s.CarrierRefusal.refusal()
		if err != nil {
			return err
		}
		e.CarrierRefusal = r
	} else if s.CarrierRefusal != nil {
		return contract.ErrStore
	}
	return nil
}
