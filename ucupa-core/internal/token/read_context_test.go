package token

import (
	"testing"

	"ucupa/shared/values"
)

func TestReadIdentifiersPreserveValuesAndCopyBoundaries(t *testing.T) {
	owner, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	run, scope, callers := [32]byte{1}, [32]byte{2}, [32]byte{3}
	actor := values.PrincipalRef{Realm: "realm", Principal: "reader"}
	const element values.ElementID = "00000000-0000-4000-8000-000000000001"
	cut := FromParts(scope, 8)
	snapshot := CoreReadSnapshotFromParts(owner, run, 4, actor, element, cut)
	if !ValidCoreReadSnapshot(snapshot) || SnapshotOwner(snapshot) != owner || SnapshotRun(snapshot) != run || SnapshotID(snapshot) != 4 || SnapshotActor(snapshot) != actor || SnapshotScope(snapshot) != element || SnapshotCut(snapshot) != cut {
		t.Fatal("snapshot reconstruction or projection changed")
	}
	connected := ConnectedCutFromParts(owner, run, 5, callers)
	if !ValidConnectedCut(connected) || ConnectedOwner(connected) != owner || ConnectedRun(connected) != run || ConnectedID(connected) != 5 || ConnectedCallers(connected) != callers || !connected.Equal(connected) {
		t.Fatal("connected identifier reconstruction or projection changed")
	}
	copyRun, copyCallers := SnapshotRun(snapshot), ConnectedCallers(connected)
	copyRun[0], copyCallers[0] = 99, 99
	copyActor := SnapshotActor(snapshot)
	copyActor.Principal = "other"
	if SnapshotRun(snapshot) != run || SnapshotActor(snapshot) != actor || ConnectedCallers(connected) != callers {
		t.Fatal("getter exposed mutable representation")
	}
	if snapshot == CoreReadSnapshotFromParts(owner, copyRun, 4, actor, element, cut) || snapshot == CoreReadSnapshotFromParts(owner, run, 5, actor, element, cut) || snapshot == CoreReadSnapshotFromParts(owner, run, 4, copyActor, element, cut) {
		t.Fatal("snapshot equality omitted a binding")
	}
	if connected.Equal(ConnectedCutFromParts(owner, copyRun, 5, callers)) || connected.Equal(ConnectedCutFromParts(owner, run, 6, callers)) || connected.Equal(ConnectedCutFromParts(owner, run, 5, copyCallers)) {
		t.Fatal("connected equality omitted a binding")
	}
	for _, invalid := range []CoreReadSnapshot{
		{}, CoreReadSnapshotFromParts("", run, 4, actor, element, cut),
		CoreReadSnapshotFromParts(owner, [32]byte{}, 4, actor, element, cut),
		CoreReadSnapshotFromParts(owner, run, 0, actor, element, cut),
		CoreReadSnapshotFromParts(owner, run, 4, values.PrincipalRef{}, element, cut),
		CoreReadSnapshotFromParts(owner, run, 4, actor, "", cut),
		CoreReadSnapshotFromParts(owner, run, 4, actor, element, CoreReadCut{}),
	} {
		if ValidCoreReadSnapshot(invalid) {
			t.Fatal("invalid snapshot shape accepted")
		}
	}
	for _, invalid := range []ConnectedCut{
		{}, ConnectedCutFromParts("", run, 5, callers), ConnectedCutFromParts(owner, [32]byte{}, 5, callers),
		ConnectedCutFromParts(owner, run, 0, callers), ConnectedCutFromParts(owner, run, 5, [32]byte{}),
	} {
		if ValidConnectedCut(invalid) {
			t.Fatal("invalid connected identifier shape accepted")
		}
	}
}
