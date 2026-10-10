package encoding

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	storerecord "ucupa/ucupa-core/internal/store/record"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// The canonical manifest carries only a stable source boundary. No transient
// control slot, peer history, external content or approval credential is copied.
// Token shape uses the original codec; shape alone never authenticates a peer.
type storedTransferMount struct {
	Admission *storerecord.StoredMountAdmission       `json:"admission"`
	Revision  *storerecord.StoredRevision             `json:"revision"`
	NextHop   values.NextHop                          `json:"nextHop"`
	Contact   *storerecord.StoredMountContactRevision `json:"contact"`
}

func storeTransferMount(r mountfact.Record) *storedTransferMount {
	if r.State == contract.MountNone {
		return nil
	}
	return &storedTransferMount{Admission: storerecord.StoreMountAdmission(r.Admission), Revision: storerecord.StoreMountRevision(r.Revision), NextHop: r.Contact.NextHop, Contact: storerecord.StoreMountContactRevision(r.Contact.Revision)}
}

func (s *storedTransferMount) record() (mountfact.Record, error) {
	if s == nil {
		return mountfact.Record{}, nil
	}
	a, err := s.Admission.Admission()
	if err != nil {
		return mountfact.Record{}, err
	}
	r, err := storerecord.LoadMountRevision(s.Revision)
	if err != nil {
		return mountfact.Record{}, err
	}
	c, err := s.Contact.Revision()
	if err != nil {
		return mountfact.Record{}, err
	}
	return mountfact.Record{State: contract.MountActive, Key: a.MountKey, Admission: a, Revision: r, Contact: mountfact.Contact{NextHop: s.NextHop, Revision: c}}, nil
}

type storedEdgeFence struct {
	Source  *storedTransferFence `json:"source,omitempty"`
	Carrier *StoredCarrierFence  `json:"carrier,omitempty"`
	Mount   values.ElementID     `json:"mount"`
}

func storeEdgeFence(f transferfact.EdgeFence) *storedEdgeFence {
	r := &storedEdgeFence{Mount: f.Mount}
	if f.Source != (transferfact.SourceFence{}) {
		v := storeTransferFence(f.Source)
		r.Source = &v
	} else {
		v := storeCarrierFence(f.Carrier)
		r.Carrier = &v
	}
	return r
}

func (s *storedEdgeFence) fence() (transferfact.EdgeFence, error) {
	if s == nil || (s.Source == nil) == (s.Carrier == nil) {
		return transferfact.EdgeFence{}, contract.ErrInvalid
	}
	f := transferfact.EdgeFence{Mount: s.Mount}
	var err error
	if s.Source != nil {
		f.Source, err = s.Source.fence()
	} else {
		f.Carrier, err = s.Carrier.fence()
	}
	if err != nil || !f.Valid() {
		return transferfact.EdgeFence{}, contract.ErrInvalid
	}
	return f, nil
}
