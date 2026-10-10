package connected

import (
	"reflect"
	"testing"
)

func TestConnectedOperationsExposeOnlyOriginalActionsAndOwnerIdentity(t *testing.T) {
	typ := reflect.TypeOf((*Operations)(nil))
	want := []string{"AuthorityRef", "ConfigureConnectedTransport", "ConnectedPeer", "ListDirectPath", "QueryRegionPath", "ReleaseConnectedCut", "ResolvePath"}
	got := make([]string, typ.NumMethod())
	for i := range got {
		got[i] = typ.Method(i).Name
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("connected public operations changed: %v", got)
	}
	for i := 0; i < typ.Elem().NumField(); i++ {
		if typ.Elem().Field(i).IsExported() {
			t.Fatalf("connected exposes private owner or query resources: %s", typ.Elem().Field(i).Name)
		}
	}
}
