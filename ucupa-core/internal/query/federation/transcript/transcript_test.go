package transcript

import (
	"bytes"
	"encoding/hex"
	"math"
	"testing"
	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	federationvalue "ucupa/ucupa-core/internal/query/federation/value"
	coretoken "ucupa/ucupa-core/internal/token"
)

func relation(mask int) *federationvalue.FederationMountRelation {
	if mask == -1 {
		return nil
	}
	radius, _ := corecontract.NewBoundedSphere(3.5)
	key := corecontract.MountKey{ParentAuthority: "parent", MountAnchorUUID: "anchor", CarrierAuthority: "carrier", ProposalRevision: coretoken.MountRevisionFromParts([32]byte{1, 2, 255}, "anchor", 17)}
	return &federationvalue.FederationMountRelation{Incoming: mask&1 != 0, State: corecontract.MountActive, Key: key, Admission: corecontract.MountAdmissionKey{MountKey: key, OperationID: "op\x00id"}, Revision: values.PublicToken{Owner: "parent", Kind: "revision", Value: "rev"}, Contact: values.PublicToken{Owner: "parent", Kind: "contact", Value: "contact"}, NextHop: "local://peer", Range: radius, WriteGateOpen: mask&2 != 0}
}

// Frozen using the original closureWriter extracted from the current
// pre-migration worktree, before its root implementation was removed.
func TestCanonicalTranscriptMatchesPreMigrationBytes(t *testing.T) {
	cases := []struct {
		keyed bool
		mask  int
		want  string
	}{
		{false, -1, "85e12b401f3ff2ba94a4c5f5243aa4e23af708ead76ad8a3f3b98ab6d4189077"},
		{false, 0, "aeea4aeae4fedb654212dc002117cecd6622d8e8f871d76f4d1f4c5d9bddaa80"},
		{false, 1, "1062d601244f9d320628bbc93d8fe57c4024ddfdac5b7b577ee0bb4fa39ec52f"},
		{false, 2, "1b180af1b90dc979b5e02aa68adb67c4574ea4a1a56759cb5924c1433eb7b8b0"},
		{false, 3, "d82591c086a36abce6c8d1ec90eae293cf305cddeb5ed64d1cc82ec842c2a233"},
		{true, -1, "cd01433d912a286ba06898c7933d982d2d8609b26acb8a457ec62c75467d2501"},
		{true, 0, "3d8c7c63c60f2f3d4808cade0b50d6201a7cfededae6a064ed9a90cdf34ef507"},
		{true, 1, "94b7fa20ca43c8854b61028f08cfdd1ebc146b203dc808e9e57bae4b2e2003cd"},
		{true, 2, "dde240708dd377e6aecdf8edbc33341b38ddf8a185d363c2f5452d91634286b5"},
		{true, 3, "a54fcd146657efa7c447e3446879c10811e0dcef1990b7acc6502fe3596428a5"},
	}
	for _, tc := range cases {
		w := SHA256()
		if tc.keyed {
			w = HMACSHA256([]byte("finite-receipt\x00key"))
		}
		w.Text("ucupa.core.fixture", "a", "bc", "")
		w.Bytes([]byte{0, 1, 255})
		w.Number(0)
		w.Number(math.MaxUint64)
		w.Token(values.PublicToken{Owner: "owner", Kind: "receipt", Value: "opaque"})
		w.Relation(relation(tc.mask))
		if got := hex.EncodeToString(w.Sum()); got != tc.want {
			t.Fatalf("keyed=%v mask=%d: %s != %s", tc.keyed, tc.mask, got, tc.want)
		}
	}
}
func TestFramingAndDigestCopy(t *testing.T) {
	a, b := SHA256(), SHA256()
	a.Text("a", "bc")
	b.Text("ab", "c")
	if bytes.Equal(a.Sum(), b.Sum()) {
		t.Fatal("text field boundaries lost")
	}
	before := a.Sum()
	detached := a.Sum()
	detached[0] ^= 0xff
	if !bytes.Equal(before, a.Sum()) {
		t.Fatal("digest aliases writer state")
	}
	a.Text("tail")
	if bytes.Equal(before, a.Sum()) {
		t.Fatal("continued transcript ignored")
	}
}
