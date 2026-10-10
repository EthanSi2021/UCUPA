package contract_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	corecontract "ucupa/ucupa-core/contract"
)

func TestMountValuesMovedWithoutExpandingContract(t *testing.T) {
	want := []string{
		"type PublicMountCommand", "type PublicMountInspection", "type PublicMountContact", "type MountControlTransport",
		"const MountEstablish", "const MountUpdate", "const MountRevoke",
		"const MountNone", "const MountEstablishing", "const MountActive", "const MountRevoking", "const MountDetached",
		"const MountPending", "const MountCommitted", "const MountAborted",
		"const MountAbortNone", "const MountAbortDenied", "const MountAbortNotFound", "const MountAbortConflict", "const MountAbortRevisionConflict",
		"const MountNotBlocked", "const MountBlockedUnavailable", "const MountBlockedOwnerMismatch", "const MountBlockedPeerStore",
		"const PermissionMountControl", "const PermissionMountAccept", "const PermissionMountRead",
		"type LocalCallers", "type MountCommandKind", "type MountCommand", "type MountKey",
		"type MountAttemptKey", "type MountAdmissionKey", "type NextHopRevision", "type MountState", "type MountDecision",
		"type MountAbortCause", "type MountBlockedBy", "type MountControlResult", "type MountRead", "type NextHopUpdateResult",
		"type MountRecoveryItem", "type MountRecoveryReport", "type MountTransport", "type MountPeerPort",
		"type MountPrepareRequest", "type MountPrepared", "type MountDecisionEvidence", "type MountFinishRequest", "type MountClosed",
		"var ErrMountBusy", "var ErrMountCoordinationRequired", "var ErrMountContextRequired", "var ErrUnavailable", "var ErrOwnerMismatch",
	}
	sort.Strings(want)
	if got := exportedDeclarations(t, ".", "mount.go", "mount_peer.go"); !reflect.DeepEqual(got, want) {
		t.Fatalf("moved mount contract surface: got %v, want %v", got, want)
	}
}

func TestMountControlDoesNotExposeStoreOrMutableTokens(t *testing.T) {
	assertNoExportedStructFields(t, reflect.TypeOf(corecontract.MountRevision{}))
	assertNoExportedStructFields(t, reflect.TypeOf(corecontract.NextHopRevision{}))
	for _, test := range []struct {
		value   reflect.Type
		methods []string
	}{
		{reflect.TypeOf(corecontract.MountKey{}), []string{"Valid"}},
		{reflect.TypeOf(corecontract.NextHopRevision{}), []string{"Equal"}},
		{reflect.TypeOf((*corecontract.MountTransport)(nil)).Elem(), []string{"Finish", "Prepare", "ReadDecision"}},
		{reflect.TypeOf((*corecontract.MountPeerPort)(nil)).Elem(), []string{"AuthorityRef", "Finish", "Prepare", "ReadDecision"}},
	} {
		var got []string
		for index := 0; index < test.value.NumMethod(); index++ {
			got = append(got, test.value.Method(index).Name)
		}
		if !reflect.DeepEqual(got, test.methods) {
			t.Fatalf("%s exposes %v, want %v", test.value, got, test.methods)
		}
	}
}

func TestFacetAndReadValuesMovedWithoutExpandingContract(t *testing.T) {
	want := []string{
		"type PublicFacetCommand", "type PublicFacetInspection", "type AttachmentControlRead",
		"var ErrAttachmentIndeterminate",
		"const FacetNone", "const FacetActive", "const FacetDetached", "const FacetRetired",
		"const PermissionFacetPropose", "const PermissionFacetCancel", "const PermissionFacetActivate", "const PermissionFacetDetach",
		"const PermissionFacetContactUpdate", "const PermissionFacetProposalRead", "const PermissionFacetAttachmentRead", "const PermissionObserveAttachment", "const PermissionFacetRelease",
		"var ErrAttachmentRevisionConflict", "var ErrAttachmentStateConflict", "var ErrProposalNotCurrent", "var ErrAcceptanceNotFound", "var ErrAttachmentChanged", "var ErrFacetContactRevisionConflict", "var ErrDispositionNotFound",
		"type FacetAttachmentState", "type FacetAttachment", "type FacetProposalRead", "type FacetSuccessorRead", "type FacetAttachmentRead", "type FacetAcceptanceEvidence", "type FacetAcceptanceSource", "type FacetAttachmentResult", "type FacetDispositionRead", "type FacetDispositionSource",
		"const ExistenceNotFound", "const ExistencePresent", "type ExistenceState", "type ElementRead", "type DirectPage", "type Projection",
	}
	sort.Strings(want)
	if got := exportedDeclarations(t, ".", "facet.go", "read.go"); !reflect.DeepEqual(got, want) {
		t.Fatalf("moved Facet/read contract surface: got %v, want %v", got, want)
	}
}

func TestObservationPermissionsRetainOriginalIdentifiers(t *testing.T) {
	if corecontract.PermissionObserveElement != "ucupa.core.element.observe" ||
		corecontract.PermissionObserveAttachment != "ucupa.core.element.facet-attachment-observe" {
		t.Fatal("moving permission declarations changed existing grants")
	}
}

func assertNoExportedStructFields(t *testing.T, value reflect.Type) {
	t.Helper()
	if value.Kind() != reflect.Struct {
		t.Fatalf("opaque type %s is not a struct", value)
	}
	for index := 0; index < value.NumField(); index++ {
		field := value.Field(index)
		if field.IsExported() {
			t.Fatalf("opaque type %s exports field %s", value, field.Name)
		}
	}
}

func exportedDeclarations(t *testing.T, directory string, selectedFiles ...string) []string {
	t.Helper()
	files, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	var declarations []string
	selected := map[string]bool{}
	for _, file := range selectedFiles {
		selected[file] = true
	}
	for _, entry := range files {
		if len(selected) != 0 && !selected[entry.Name()] {
			continue
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") ||
			strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, declaration := range parsed.Decls {
			switch value := declaration.(type) {
			case *ast.FuncDecl:
				if !value.Name.IsExported() {
					continue
				}
				if value.Recv == nil {
					declarations = append(declarations, "func "+value.Name.Name)
					continue
				}
				receiver, ok := exportedReceiverName(value.Recv.List[0].Type)
				if !ok {
					if receiver != "" {
						// Exported methods on an unexported adapter type are not
						// independently exported package declarations. Its public
						// interface is checked separately.
						continue
					}
					t.Fatalf("exported method %s has unsupported receiver in %s", value.Name.Name, path)
				}
				declarations = append(declarations, "method "+receiver+"."+value.Name.Name)
			case *ast.GenDecl:
				for _, specification := range value.Specs {
					switch item := specification.(type) {
					case *ast.TypeSpec:
						if item.Name.IsExported() {
							declarations = append(declarations, "type "+item.Name.Name)
						}
					case *ast.ValueSpec:
						for _, name := range item.Names {
							if name.IsExported() {
								declarations = append(declarations, strings.ToLower(value.Tok.String())+" "+name.Name)
							}
						}
					}
				}
			}
		}
	}
	sort.Strings(declarations)
	return declarations
}

func exportedReceiverName(expression ast.Expr) (string, bool) {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name, value.IsExported()
	case *ast.StarExpr:
		return exportedReceiverName(value.X)
	default:
		return "", false
	}
}
