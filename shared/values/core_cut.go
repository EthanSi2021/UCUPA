package values

import (
	"encoding/binary"
	"errors"
)

// CoreCutEvidence is a copied local interoperability token, not authentication
// or a public wire format. Only the supplying Core establishes its meaning.
type CoreCutEvidence struct{ raw [40]byte }

func NewCoreCutEvidence(raw []byte) (CoreCutEvidence, error) {
	var v CoreCutEvidence
	if len(raw) != 40 {
		return v, errors.New("invalid Core cut length")
	}
	copy(v.raw[:], raw)
	if !v.Valid() {
		return CoreCutEvidence{}, errors.New("invalid Core cut")
	}
	return v, nil
}
func (v CoreCutEvidence) Valid() bool {
	var s [32]byte
	copy(s[:], v.raw[:32])
	return s != [32]byte{} && binary.BigEndian.Uint64(v.raw[32:]) != 0
}
func (v CoreCutEvidence) Bytes() []byte                    { return append([]byte(nil), v.raw[:]...) }
func (v CoreCutEvidence) Equal(other CoreCutEvidence) bool { return v == other }
