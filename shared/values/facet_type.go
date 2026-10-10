package values

import "errors"

// FacetTypeRef is an opaque 32-byte content-key component. It does not prove
// a payload format, and is not a URL, display name or service address.
// Its zero value is invalid.
type FacetTypeRef struct {
	bytes string
}

// NewFacetTypeRef takes an owned copy of any exact 32-byte value.
func NewFacetTypeRef(raw []byte) (FacetTypeRef, error) {
	if len(raw) != 32 {
		return FacetTypeRef{}, errors.New("FacetTypeRef must contain exactly 32 bytes")
	}
	return FacetTypeRef{bytes: string(raw)}, nil
}

func (ref FacetTypeRef) Valid() bool { return len(ref.bytes) == 32 }

// Bytes returns a copy; callers cannot mutate the identity through either input
// or output buffers. The value is comparable and can be used as a map key.
func (ref FacetTypeRef) Bytes() []byte { return []byte(ref.bytes) }

func (ref FacetTypeRef) Equal(other FacetTypeRef) bool { return ref == other }

// Less orders refs by their raw bytes, without Unicode or name normalization.
func (ref FacetTypeRef) Less(other FacetTypeRef) bool { return ref.bytes < other.bytes }
