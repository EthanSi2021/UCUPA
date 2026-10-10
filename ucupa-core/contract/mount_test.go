package contract

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"ucupa/shared/values"
)

func TestMountValueSurfaceAndStoredEnums(t *testing.T) {
	for _, tc := range []struct {
		value   any
		methods []string
	}{
		{NextHopRevision{}, []string{"Equal"}},
		{MountKey{}, []string{"Valid"}},
		{MountAttemptKey{}, nil},
		{MountAdmissionKey{}, nil},
	} {
		typ := reflect.TypeOf(tc.value)
		if typ.NumMethod() != len(tc.methods) {
			t.Fatalf("%s gained methods", typ)
		}
		for i, name := range tc.methods {
			if typ.Method(i).Name != name {
				t.Fatalf("%s method changed", typ)
			}
		}
	}
	typ := reflect.TypeOf(NextHopRevision{})
	if !typ.Comparable() || typ.NumField() != 4 {
		t.Fatal("contact representation shape changed")
	}
	for i := 0; i < typ.NumField(); i++ {
		if typ.Field(i).IsExported() {
			t.Fatal("contact exposes raw representation")
		}
	}
	key := reflect.TypeOf(MountKey{})
	names := []string{"ParentAuthority", "MountAnchorUUID", "ProposalRevision", "CarrierAuthority"}
	types := []reflect.Type{reflect.TypeOf(values.FactAuthorityRef("")), reflect.TypeOf(values.ElementID("")), reflect.TypeOf(MountRevision{}), reflect.TypeOf(values.FactAuthorityRef(""))}
	if key.NumField() != len(names) {
		t.Fatal("key shape changed")
	}
	for i, name := range names {
		field := key.Field(i)
		if field.Name != name || field.Type != types[i] || !field.IsExported() {
			t.Fatalf("key field %d changed", i)
		}
	}
	if MountEstablish != 1 || MountUpdate != 2 || MountRevoke != 3 || MountNone != 0 || MountEstablishing != 1 || MountActive != 2 || MountRevoking != 3 || MountDetached != 4 ||
		MountPending != 1 || MountCommitted != 2 || MountAborted != 3 || MountAbortNone != 0 || MountAbortDenied != 1 || MountAbortNotFound != 2 || MountAbortConflict != 3 || MountAbortRevisionConflict != 4 ||
		MountNotBlocked != 0 || MountBlockedUnavailable != 1 || MountBlockedOwnerMismatch != 2 || MountBlockedPeerStore != 3 {
		t.Fatal("stored mount enums changed")
	}
	for _, tc := range []struct {
		err  error
		text string
	}{
		{ErrMountBusy, "Core mount control or content write gate is occupied"},
		{ErrMountCoordinationRequired, "Core mount range change requires coordination"},
		{ErrMountContextRequired, "Core read requires a mount context"},
		{ErrUnavailable, "Core peer is unavailable"},
		{ErrOwnerMismatch, "Core peer identity does not match"},
	} {
		if tc.err.Error() != tc.text || !errors.Is(fmt.Errorf("wrapped: %w", tc.err), tc.err) {
			t.Fatalf("error changed: %v", tc.err)
		}
	}
}
