package commit

import (
	"ucupa/ucupa-core/contract"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func (s *coreState) captureTransferManifest(input transferfact.Intent, parent transferfact.ParentHold) (transferfact.Slice, string, error) {
	if err := s.guardTransferScope(input.Boundary); err != nil {
		return transferfact.Slice{}, "", err
	}
	// Recheck the boundary CAS and recapture all descendants at the fence cut,
	// never reuse the earlier INTENT preflight as final evidence.
	var revision contract.Revision
	if a, ok := s.elements.Anchor(input.Boundary); ok {
		revision = a.Revision
	} else if e, ok := s.elements.Entity(input.Boundary); ok {
		revision = e.Revision
	} else {
		return transferfact.Slice{}, "", contract.ErrNotFound
	}
	if revision != input.Expected {
		return transferfact.Slice{}, "", contract.ErrRevisionConflict
	}
	slice, err := s.selectTransferSource(input, parent)
	if err != nil {
		return transferfact.Slice{}, "", err
	}
	slice, err = s.captureTransferEvidence(slice)
	if err != nil {
		return transferfact.Slice{}, "", err
	}
	manifest, err := transferencoding.EncodeSourceManifest(input, slice)
	return slice, manifest, err
}
