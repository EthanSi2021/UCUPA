package values

import (
	"encoding/binary"
	"testing"
)

func TestCoreCutEvidenceCopiedAndValidated(t *testing.T) {
	raw := make([]byte, 40)
	raw[0] = 1
	binary.BigEndian.PutUint64(raw[32:], 1)
	v, err := NewCoreCutEvidence(raw)
	if err != nil || !v.Valid() {
		t.Fatal(err)
	}
	raw[0] = 2
	copy := v.Bytes()
	copy[0] = 3
	if v.Bytes()[0] != 1 {
		t.Fatal("alias")
	}
	for _, bad := range [][]byte{nil, make([]byte, 40), make([]byte, 39)} {
		if _, err = NewCoreCutEvidence(bad); err == nil {
			t.Fatal("invalid cut")
		}
	}
	if (CoreCutEvidence{}).Valid() {
		t.Fatal("zero valid")
	}
}
