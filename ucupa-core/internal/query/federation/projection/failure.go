package projection

import (
	"errors"
	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
)

// ReadError preserves the Federation read-borrow mapping. Unknown errors pass
// through unchanged; this is not the general public mutation error mapping.
func ReadError(err error) error {
	switch {
	case errors.Is(err, corecontract.ErrReadCutExpired):
		return values.PublicInvalidContext
	case errors.Is(err, corecontract.ErrSnapshotCapacity):
		return values.PublicResourceExhausted
	case errors.Is(err, corecontract.ErrStore), errors.Is(err, corecontract.ErrUnavailable):
		return values.PublicUnavailable
	default:
		return err
	}
}
