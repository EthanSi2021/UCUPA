package token

import (
	"testing"

	"ucupa/shared/values"
)

func TestMountContactRetainsExactIdentityAndCopyBoundaries(t *testing.T) {
	parent, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	carrier, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	const id values.ElementID = "00000000-0000-4000-8000-000000000001"
	scope := [32]byte{1}
	proposal := MountRevisionFromParts(scope, id, 7)
	key := MountKey{ParentAuthority: parent, CarrierAuthority: carrier, MountAnchorUUID: id, ProposalRevision: proposal}
	if !key.Valid() {
		t.Fatal("original exact relationship rejected")
	}
	revision := NextHopRevisionFromParts(scope, key, true, 8)
	if !ValidNextHopRevision(revision) || NextHopScope(revision) != scope || NextHopKey(revision) != key || !NextHopIncoming(revision) || NextHopSerial(revision) != 8 || !revision.Equal(revision) {
		t.Fatal("contact reconstruction or projection changed")
	}
	bytes := NextHopScopeBytes(revision)
	bytes[0] = 9
	projected := NextHopKey(revision)
	projected.CarrierAuthority = parent
	if NextHopScope(revision) != scope || NextHopKey(revision) != key {
		t.Fatal("getter exposed writable token representation")
	}
	if revision.Equal(NextHopRevisionFromParts(scope, key, false, 8)) || revision.Equal(NextHopRevisionFromParts(scope, key, true, 9)) {
		t.Fatal("direction/serial omitted from equality")
	}
	invalid := []NextHopRevision{{}, NextHopRevisionFromParts([32]byte{}, key, true, 8), NextHopRevisionFromParts(scope, MountKey{}, true, 8), NextHopRevisionFromParts(scope, key, true, 0)}
	for _, value := range invalid {
		if ValidNextHopRevision(value) {
			t.Fatal("invalid contact accepted")
		}
	}
	badKey := key
	badKey.CarrierAuthority = parent
	if badKey.Valid() {
		t.Fatal("same owner accepted")
	}
	badKey = key
	badKey.ProposalRevision = MountRevisionFromParts(scope, "00000000-0000-4000-8000-000000000002", 7)
	if badKey.Valid() {
		t.Fatal("proposal for different anchor accepted")
	}
}
