package contract_test

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"strings"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-facet/contract"
)

func TestNamespaceErrorVocabularyAndIdentity(t *testing.T) {
	for _, tc := range []struct {
		value error
		text  string
	}{
		{contract.ErrInvalid, "invalid Facet input"},
		{contract.ErrDenied, "Facet authorization denied"},
		{contract.ErrUnavailable, "Facet source unavailable"},
		{contract.ErrStore, "Facet Store failure"},
		{contract.ErrIndeterminate, "Facet commit outcome indeterminate"},
		{contract.ErrOperationConflict, "Facet operation conflict"},
		{contract.ErrProposalNotCurrent, "Facet proposal not current"},
		{contract.ErrSubjectAuthorityConflict, "Facet subject authority conflict"},
		{contract.ErrAcceptanceNotFound, "Facet acceptance not found"},
		{contract.ErrContactRevisionConflict, "Facet contact revision conflict"},
		{contract.ErrElementNotFound, "Core Element not found"},
		{contract.ErrNamespaceNone, "Core Facet namespace none"},
		{contract.ErrNamespaceDetached, "Core Facet namespace detached"},
		{contract.ErrNotCurrentOwner, "Facet not current owner"},
		{contract.ErrAttachmentChanged, "Core Facet attachment changed"},
		{contract.ErrKeyNotFound, "Facet key not found"},
		{contract.ErrRevisionConflict, "Facet revision conflict"},
		{contract.ErrCapacity, "Facet capacity exceeded"},
		{contract.ErrSnapshotChanged, "Facet snapshot changed during Core check"},
	} {
		if tc.value.Error() != tc.text || !errors.Is(fmt.Errorf("outer: %w", tc.value), tc.value) {
			t.Fatalf("error contract changed: %v", tc.value)
		}
	}
}

func TestNamespaceValueAndPermissionSurface(t *testing.T) {
	if contract.CoreMissing != 1 || contract.CoreNone != 2 || contract.CoreActive != 3 || contract.CoreDetached != 4 ||
		contract.Committed != 1 || contract.Unchanged != 2 || contract.Rejected != 3 {
		t.Fatal("stored enum values changed")
	}
	for _, tc := range []struct {
		got  values.PermissionID
		want string
	}{
		{contract.PermissionPut, "ucupa.facet.put"},
		{contract.PermissionGet, "ucupa.facet.get"},
		{contract.PermissionDelete, "ucupa.facet.delete"},
		{contract.PermissionAccept, "ucupa.facet.element.accept"},
		{contract.PermissionReadAcceptance, "ucupa.facet.element.acceptance-read"},
		{contract.PermissionListTypes, "ucupa.facet.element.list-types"},
		{contract.PermissionCoreContactUpdate, "ucupa.facet.element.core-contact-update"},
		{contract.PermissionObserveElement, "ucupa.facet.element.observe"},
		{contract.PermissionObserveKey, "ucupa.facet.observe"},
	} {
		if string(tc.got) != tc.want {
			t.Fatalf("permission changed: %q", tc.got)
		}
	}
	if reflect.TypeOf(contract.PublicAcceptanceRef{}).NumMethod() != 0 {
		t.Fatal("acceptance reference exposed an owner validation method")
	}
	for _, tc := range []struct {
		typ    reflect.Type
		fields []string
	}{
		{reflect.TypeOf(contract.CoreEvidence{}), []string{"Authority", "Element", "State", "AttachmentRevision", "Active", "Pending", "Cut"}},
		{reflect.TypeOf(contract.AcceptanceRead{}), []string{"Found", "Acceptance", "CoreNextHop", "NextHopRevision", "Cut"}},
		{reflect.TypeOf(contract.MutationResult{}), []string{"Status", "Cut", "Acceptance", "CoreNextHop", "NextHopRevision", "Revision", "Core"}},
		{reflect.TypeOf(contract.ContentRead{}), []string{"Payload", "Revision", "Types", "Cut", "Acceptance", "Core"}},
	} {
		if tc.typ.NumField() != len(tc.fields) {
			t.Fatalf("%s field count changed", tc.typ)
		}
		for i, want := range tc.fields {
			if tc.typ.Field(i).Name != want {
				t.Fatalf("%s field %d changed", tc.typ, i)
			}
		}
	}
	raw := string([]byte{255, 0, 1})
	if string(contract.OperationID(raw)) != raw {
		t.Fatal("operation bytes changed")
	}
}

func TestContractDoesNotExportPrivateWorkflowPorts(t *testing.T) {
	forbidden := map[string]bool{"ContractWitness": true, "PublicAuthorizer": true, "CoreControlSource": true, "CoreProposalSource": true, "Facets": true, "Config": true}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), e.Name(), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if d, ok := n.(*ast.TypeSpec); ok && forbidden[d.Name.Name] {
				t.Errorf("private workflow declaration exported: %s", d.Name.Name)
			}
			return true
		})
	}
}
