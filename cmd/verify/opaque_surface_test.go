package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Compile outside both owner directories. This tests Go's actual import and
// field visibility, rather than only trusting our dependency-checker model.
func TestOpaqueContractsDoNotExposeConstructionOrOwnerInternals(t *testing.T) {
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root, err := repositoryRoot(cwd)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, source, want string
	}{
		{"public-relationship-contract", `package main
import c "ucupa/ucupa-core/contract"
import v "ucupa/shared/values"
func main() {
 _ = c.PublicMountCommand{}
 _ = c.PublicMountInspection{}
 _ = c.PublicMountContact{}
 _ = c.PublicFacetCommand{}
 _ = c.PublicFacetInspection{}
 _ = c.AttachmentControlRead{}
 var _ c.MountControlTransport
 _, _ = c.CoreReadCutFromEvidence(v.CoreCutEvidence{})
}`, ""},
		{"public-relationship-no-root-alias", `package main
import c "ucupa/ucupa-core"
var _ = c.PublicFacetCommand{}
func main() {}`, "package ucupa/ucupa-core is not in std"},
		{"public-relationship-no-admission-method", `package main
import c "ucupa/ucupa-core/contract"
var _ = c.PublicFacetCommand{}.Valid
func main() {}`, "undefined"},
		{"mount-connection-public-constructor", `package main
import c "ucupa/ucupa-core/transport/grpc/mountconn"
func main() { _ = c.NewMountConnections; var p *c.MountConnections; _ = p.ConfiguredFor }`, ""},
		{"mount-connection-no-root-alias", `package main
import c "ucupa/ucupa-core/transport/grpc"
func main() { _ = c.NewMountConnections }`, "undefined"},
		{"mount-connection-private-state", `package main
import c "ucupa/ucupa-core/transport/grpc/mountconn"
var _ = c.MountConnections{source: "x"}
func main() {}`, "cannot refer to unexported field"},
		{"commit-binding-private", `package main
import _ "ucupa/ucupa-core/internal/commit/binding"
func main() {}`, "use of internal package"},
		{"access-value-private", `package main
import _ "ucupa/ucupa-core/internal/access/value"
func main() {}`, "use of internal package"},
		{"query-scan-private", `package main
import _ "ucupa/ucupa-core/internal/query/scan"
func main() {}`, "use of internal package"},
		{"query-contract-values", `package main
import c "ucupa/ucupa-core/contract"
func main() {
 _ = c.RegionQuery{UpDepth: -1, Selector: c.QueryBoth}
 _ = c.Path{Up: 1}
 var page c.QueryPage
 _ = page.Continuation
 _ = c.ConnectedDirectContinuation{}
 _ = c.QueryCurrentVerified
}`, ""},
		{"query-position-no-fields", `package main
import c "ucupa/ucupa-core/contract"
var _ = c.QueryContinuation{last: "x"}
func main() {}`, "cannot refer to unexported field"},
		{"query-position-no-method", `package main
import c "ucupa/ucupa-core/contract"
var _ = c.QueryContinuation{}.QueryLast
func main() {}`, "undefined"},
		{"query-position-no-constructor", `package main
import c "ucupa/ucupa-core/contract"
var _ = c.QueryContinuationFromParts
func main() {}`, "undefined"},
		{"query-request-private", `package main
import _ "ucupa/ucupa-core/internal/query/request"
func main() {}`, "use of internal package"},
		{"query-no-root-alias", `package main
import c "ucupa/ucupa-core"
var _ c.RegionQuery
func main() {}`, "package ucupa/ucupa-core is not in std"},
		{"facet-instance-namespace", `package main
import f "ucupa/ucupa-facet/instance"
func main() {
 var i *f.Instance
 _ = i.AuthorityRef
 _ = i.Close
 _ = i.Namespace.Get
 _ = i.Namespace.Put
 _ = i.Namespace.AcceptAttachment
 _ = i.Namespace.PublicCall
}`, ""},
		{"facet-instance-no-business-forwarder", `package main
import f "ucupa/ucupa-facet/instance"
var i *f.Instance
var _ = i.Put
func main() {}`, "undefined"},
		{"facet-namespace-no-lifecycle-method", `package main
import f "ucupa/ucupa-facet/instance"
var i *f.Instance
var _ = i.Namespace.Close
func main() {}`, "undefined"},
		{"facet-namespace-private", `package main
import _ "ucupa/ucupa-facet/internal/namespace"
func main() {}`, "use of internal package"},
		{"core-element-facts-private", `package main
import _ "ucupa/ucupa-core/internal/element/fact"
func main() {}`, "use of internal package"},
		{"read-identifiers-public", `package main
import c "ucupa/ucupa-core/contract"
func main() {
 var snapshot c.CoreReadSnapshot
 _ = snapshot == snapshot
 var cut c.ConnectedCut
 _ = cut.Equal(cut)
}`, ""},
		{"read-identifier-private-field", `package main
import c "ucupa/ucupa-core/contract"
var _ = c.CoreReadSnapshot{id: 1}
func main() {}`, "cannot refer to unexported field"},
		{"read-identifier-no-constructor", `package main
import c "ucupa/ucupa-core/contract"
var _ = c.CoreReadSnapshotFromParts
func main() {}`, "undefined"},
		{"connected-identifier-no-getter", `package main
import c "ucupa/ucupa-core/contract"
var _ = c.ConnectedCut{}.ConnectedID
func main() {}`, "undefined"},
		{"connected-opaque-ports", `package main
import (
 "context"
 c "ucupa/ucupa-core/contract/connected"
 v "ucupa/shared/values"
)
type peer struct{}
func (peer) AuthorityRef() v.FactAuthorityRef { return "" }
func (peer) ReadConnected(context.Context, v.FactAuthorityRef, c.Request) (c.Reply, error) {
 return c.Reply{}, nil
}
type transport struct{}
func (transport) ReadConnected(context.Context, v.NextHop, v.FactAuthorityRef, c.Request) (c.Reply, error) {
 return c.Reply{}, nil
}
func main() {
 var _ c.PeerPort = peer{}
 var _ c.Transport = transport{}
 _ = c.Request{}
 _ = c.Reply{}
}`, ""},
		{"connected-request-private-field", `package main
import c "ucupa/ucupa-core/contract/connected"
var _ = c.Request{data: 1}
func main() {}`, "cannot refer to unexported field"},
		{"connected-request-private-data", `package main
import c "ucupa/ucupa-core/contract/connected"
func main() { var request c.Request; _ = request.data }`, "cannot refer to unexported field"},
		{"connected-no-payload-constructor", `package main
import c "ucupa/ucupa-core/contract/connected"
var _ = c.NewRequest
func main() {}`, "undefined"},
		{"connected-no-payload-reader", `package main
import c "ucupa/ucupa-core/contract/connected"
var _ = c.ReadReply
func main() {}`, "undefined"},
		{"connected-exchange-private", `package main
import _ "ucupa/ucupa-core/internal/query/exchange"
func main() {}`, "use of internal package"},
		{"connected-no-old-root-request", `package main
import c "ucupa/ucupa-core"
var _ c.ConnectedPeerRequest
func main() {}`, "package ucupa/ucupa-core is not in std"},
		{"connected-no-old-root-transport", `package main
import c "ucupa/ucupa-core"
var _ c.ConnectedTransport
func main() {}`, "package ucupa/ucupa-core is not in std"},
		{"read-identifiers-no-root-alias", `package main
import c "ucupa/ucupa-core"
var _ c.ConnectedCut
func main() {}`, "package ucupa/ucupa-core is not in std"},
		{"public-values", `package main
import (
 c "ucupa/ucupa-core/contract"
 f "ucupa/ucupa-facet/contract"
)
func main() {
 cut := c.CoreReadCut{}
 _ = cut.Equal(cut)
 _ = cut.Evidence()
 rev := f.Revision{}
 _ = rev.Equal(rev)
 contact := f.ContactRevision{}
 _ = contact.Equal(contact)
 _ = f.ExpectAbsent()
 _ = f.ExpectRevision(rev)
}`, ""},
		{"core-spatial-values", `package main
import c "ucupa/ucupa-core/contract"
func main() {
 v := c.Vec3{X: 1, Y: 2, Z: 3}
 r, err := c.NewRotation(1, 0, 0, 0)
 if err != nil { panic(err) }
 p, err := c.NewPose(v, r)
 if err != nil { panic(err) }
 s, err := c.NewBoundedSphere(2)
 if err != nil { panic(err) }
 var empty c.Range
 _ = v.X
 _ = p.Translation
 _ = p.Rotation
 _ = r.Pivot()
 _ = r.Ratios()
 _ = s.Radius()
 _ = empty.Kind()
 _, _ = empty.BoundedSphere()
}`, ""},
		{"core-revision-values", `package main
import c "ucupa/ucupa-core/contract"
func main() {
 r := c.Revision{}
 m := c.MountRevision{}
 _ = r.Equal(r)
 _ = m.Equal(m)
 _, _ = r.ServiceEvidence().ElementRevision()
 _, _ = m.ServiceEvidence().MountRevision()
 _ = c.FacetContactRevision{}.Equal(c.FacetContactRevision{})
 _ = c.ListContinuation{}
}`, ""},
		{"core-element-values", `package main
import c "ucupa/ucupa-core/contract"
func main() {
 var operation c.OperationID = "operation"
 var tuple c.SpatialTuple
 var anchor c.AnchorRecord
 var entity c.EntityRecord
 record := c.ElementRecord{Kind: c.ElementAnchor, Anchor: &anchor}
 result := c.MutationResult{Status: c.MutationCommitted, Record: &record}
 _ = operation
 _ = tuple
 _ = entity
 _ = result
 _ = []error{c.ErrDenied, c.ErrNotFound, c.ErrConflict, c.ErrRevisionConflict,
 c.ErrOperationConflict, c.ErrReadCutExpired, c.ErrStore}
}`, ""},
		{"core-element-no-root-alias", `package main
import c "ucupa/ucupa-core"
var _ c.ElementRecord
func main() {}`, "package ucupa/ucupa-core is not in std"},
		{"core-transport-wire-private", `package main
import _ "ucupa/ucupa-core/transport/grpc/internal/wire"
func main() {}`, "use of internal package"},
		{"core-revision-field", `package main
import c "ucupa/ucupa-core/contract"
var _ = c.Revision{serial: 1}
func main() {}`, "unexported field"},
		{"core-mount-public-values", `package main
import c "ucupa/ucupa-core/contract"
func main() {
 key := c.MountKey{}
 _ = key.Valid()
 revision := c.NextHopRevision{}
 _ = revision.Equal(revision)
 var transport c.MountTransport
 var peer c.MountPeerPort
 _, _ = transport, peer
}`, ""},
		{"core-mount-contact-field", `package main
import c "ucupa/ucupa-core/contract"
var _ = c.NextHopRevision{serial: 1}
func main() {}`, "unexported field"},
		{"core-mount-contact-constructor", `package main
import c "ucupa/ucupa-core/contract"
var _ = c.NextHopRevisionFromParts
func main() {}`, "undefined"},
		{"core-mount-contact-getter", `package main
import c "ucupa/ucupa-core/contract"
var _ = c.NextHopKey
func main() {}`, "undefined"},
		{"core-mount-no-root-alias", `package main
import c "ucupa/ucupa-core"
var _ c.MountKey
func main() {}`, "package ucupa/ucupa-core is not in std"},
		{"core-mount-revision-field", `package main
import c "ucupa/ucupa-core/contract"
var _ = c.MountRevision{serial: 1}
func main() {}`, "unexported field"},
		{"core-contact-revision-field", `package main
import c "ucupa/ucupa-core/contract"
var _ = c.FacetContactRevision{serial: 1}
func main() {}`, "unexported field"},
		{"core-list-continuation-field", `package main
import c "ucupa/ucupa-core/contract"
var _ = c.ListContinuation{parent: "forged"}
func main() {}`, "unexported field"},
		{"core-revision-constructor", `package main
import c "ucupa/ucupa-core/contract"
var _ = c.RevisionFromParts
func main() {}`, "undefined"},
		{"core-revision-scope", `package main
import c "ucupa/ucupa-core/contract"
var _ = c.RevisionScope
func main() {}`, "undefined"},
		{"core-contact-constructor", `package main
import c "ucupa/ucupa-core/contract"
var _ = c.FacetContactRevisionFromParts
func main() {}`, "undefined"},
		{"core-list-constructor", `package main
import c "ucupa/ucupa-core/contract"
var _ = c.ListContinuationFromParts
func main() {}`, "undefined"},
		{"core-private-view-port", `package main
import c "ucupa/ucupa-core/contract"
var _ c.PublicAuthorizer
func main() {}`, "undefined"},
		{"core-field", `package main
import c "ucupa/ucupa-core/contract"
var _ = c.CoreReadCut{serial: 1}
func main() {}`, "unexported field"},
		{"core-spatial-rotation-field", `package main
import c "ucupa/ucupa-core/contract"
var _ = c.Rotation{pivot: c.PivotW}
func main() {}`, "unexported field"},
		{"core-spatial-sphere-field", `package main
import c "ucupa/ucupa-core/contract"
var _ = c.BoundedSphere{radius: 1}
func main() {}`, "unexported field"},
		{"core-spatial-raw-function", `package main
import c "ucupa/ucupa-core/contract"
var _ = c.RotationFromComponents
func main() {}`, "undefined"},
		{"core-spatial-raw-sphere-function", `package main
import c "ucupa/ucupa-core/contract"
var _ = c.SphereFromStored
func main() {}`, "undefined"},
		{"core-spatial-work", `package main
import c "ucupa/ucupa-core/contract"
var _ c.Work
func main() {}`, "undefined"},
		{"core-spatial-exact-pose", `package main
import c "ucupa/ucupa-core/contract"
var _ c.ExactPose
func main() {}`, "undefined"},
		{"facet-private-witness", `package main
import f "ucupa/ucupa-facet/contract"
var _ f.ContractWitness
func main() {}`, "undefined"},
		{"facet-private-view-port", `package main
import f "ucupa/ucupa-facet/contract"
var _ f.PublicAuthorizer
func main() {}`, "undefined"},
		{"facet-private-control-port", `package main
import f "ucupa/ucupa-facet/contract"
var _ f.CoreControlSource
func main() {}`, "undefined"},
		{"facet-private-proposal-port", `package main
import f "ucupa/ucupa-facet/contract"
var _ f.CoreProposalSource
func main() {}`, "undefined"},
		{"facet-field", `package main
import f "ucupa/ucupa-facet/contract"
var _ = f.Revision{serial: 1}
func main() {}`, "unexported field"},
		{"facet-contact-field", `package main
import f "ucupa/ucupa-facet/contract"
var _ = f.ContactRevision{serial: 1}
func main() {}`, "unexported field"},
		{"facet-precondition-field", `package main
import f "ucupa/ucupa-facet/contract"
var _ = f.PutPrecondition{absent: true}
func main() {}`, "unexported field"},
		{"core-internal", `package main
import _ "ucupa/ucupa-core/internal/token"
func main() {}`, "use of internal package"},
		{"facet-internal", `package main
import _ "ucupa/ucupa-facet/internal/token"
func main() {}`, "use of internal package"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFixture(t, dir, "main.go", tc.source+"\n")
			cmd := exec.Command(goTool, "build", "-o", filepath.Join(dir, "consumer"), filepath.Join(dir, "main.go"))
			cmd.Dir = root
			cmd.Env = commandEnvironment(false)
			out, err := cmd.CombinedOutput()
			if tc.want == "" {
				if err != nil {
					t.Fatalf("valid public consumer failed: %v\n%s", err, out)
				}
				return
			}
			if err == nil || !strings.Contains(string(out), tc.want) {
				t.Fatalf("expected compiler rejection %q, got %v\n%s", tc.want, err, out)
			}
		})
	}
}

func TestFederationExternalSurfaceIsClosed(t *testing.T) {
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root, err := repositoryRoot(cwd)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, source, want string
	}{
		{"federation-public-values-and-ports", `package main
import (
 "context"
 c "ucupa/ucupa-core/contract"
 f "ucupa/ucupa-core/contract/federation"
 v "ucupa/shared/values"
)
type readConnection struct{}
func (readConnection) Part() f.FederationReadPart { return f.FederationReadPart{} }
func (readConnection) Project(context.Context, f.FederationProjectionRequest, *v.PublicToken, uint16) (f.FederationProjectionPage, error) { return f.FederationProjectionPage{}, nil }
func (readConnection) Check(context.Context, bool) error { return nil }
func (readConnection) Observe(context.Context, f.FederationObserveRequest) (f.FederationObservationPage, error) { return f.FederationObservationPage{}, nil }
func (readConnection) Release(context.Context) error { return nil }
type connections struct{}
func (connections) OpenFederationRead(context.Context, v.CallerContext, v.NextHop, v.FactAuthorityRef, f.FederationReadRequest) (f.FederationReadConnection, error) { return readConnection{}, nil }
func main() {
 var _ f.FederationConnections = connections{}
 var _ f.FederationReadConnection = readConnection{}
 _ = f.FederationReadRequest{}
 _ = f.FederationMountRelation{}
 _ = f.FederationReadPart{}
 _ = f.FederationRecord{}
 _ = f.FederationPoseStep{}
 _ = f.FederationPathRequest{}
 _ = f.FederationSpineNode{}
 _ = f.FederationFrontier{}
 _ = f.FederationPathResult{}
 _ = f.FederationQueryRequest{}
 _ = f.FederationProjectionRequest{}
 _ = f.FederationMatch{}
 _ = f.FederationProjectionPage{}
 _ = f.FederationParentEvidence{}
 _ = f.FederationObserveRequest{}
 _ = f.FederationChangeGroup{}
 _ = f.FederationObservationPage{}
 _, _ = f.NewFederationRange(c.RangeUnboundedRoot, c.BoundedSphere{})
}`, ""},
		{"federation-private-traversal-field", `package main
import f "ucupa/ucupa-core/contract/federation"
func main() { var page f.FederationProjectionPage; _ = page.traversal }`, "cannot refer to unexported field"},
		{"federation-no-traversal-reader", `package main
import f "ucupa/ucupa-core/contract/federation"
var _ = f.ProjectionTraversal
func main() {}`, "undefined"},
		{"federation-no-traversal-setter", `package main
import f "ucupa/ucupa-core/contract/federation"
var _ = f.SetProjectionTraversal
func main() {}`, "undefined"},
		{"federation-no-internal-value", `package main
import _ "ucupa/ucupa-core/internal/query/federation/value"
func main() {}`, "use of internal package"},
		{"federation-no-old-root-value", `package main
import c "ucupa/ucupa-core"
var _ c.FederationReadRequest
func main() {}`, "package ucupa/ucupa-core is not in std"},
		{"federation-no-old-root-range-constructor", `package main
import c "ucupa/ucupa-core"
var _ = c.NewFederationRange
func main() {}`, "package ucupa/ucupa-core is not in std"},
		{"federation-no-old-root-port", `package main
import c "ucupa/ucupa-core"
var _ c.FederationConnections
func main() {}`, "package ucupa/ucupa-core is not in std"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFixture(t, dir, "main.go", tc.source+"\n")
			cmd := exec.Command(goTool, "build", "-o", filepath.Join(dir, "consumer"), filepath.Join(dir, "main.go"))
			cmd.Dir = root
			cmd.Env = commandEnvironment(false)
			out, err := cmd.CombinedOutput()
			if tc.want == "" {
				if err != nil {
					t.Fatalf("valid Federation consumer failed: %v\n%s", err, out)
				}
				return
			}
			if err == nil || !strings.Contains(string(out), tc.want) {
				t.Fatalf("expected compiler rejection %q, got %v\n%s", tc.want, err, out)
			}
		})
	}
}

func TestFederationExportedDeclarationsRemainClosed(t *testing.T) {
	working, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root, err := repositoryRoot(working)
	if err != nil {
		t.Fatal(err)
	}
	checkFederationExports(t, root, "ucupa-core/internal/query/federation/value", []string{"value.go", "observe.go"}, map[string]bool{
		"FederationReadRequest":       true,
		"FederationMountRelation":     true,
		"FederationReadPart":          true,
		"FederationRecord":            true,
		"FederationPoseStep":          true,
		"FederationPathRequest":       true,
		"FederationSpineNode":         true,
		"FederationFrontier":          true,
		"FederationPathResult":        true,
		"FederationQueryRequest":      true,
		"FederationProjectionRequest": true,
		"FederationMatch":             true,
		"FederationProjectionPage":    true,
		"FederationParentEvidence":    true,
		"FederationObserveRequest":    true,
		"FederationChangeGroup":       true,
		"FederationObservationPage":   true,
	}, map[string]bool{"NewFederationRange": true, "ProjectionTraversal": true, "SetProjectionTraversal": true})
	checkFederationExports(t, root, "ucupa-core/contract/federation", []string{"federation.go"}, map[string]bool{
		"FederationReadRequest":       true,
		"FederationMountRelation":     true,
		"FederationReadPart":          true,
		"FederationRecord":            true,
		"FederationPoseStep":          true,
		"FederationPathRequest":       true,
		"FederationSpineNode":         true,
		"FederationFrontier":          true,
		"FederationPathResult":        true,
		"FederationQueryRequest":      true,
		"FederationProjectionRequest": true,
		"FederationMatch":             true,
		"FederationProjectionPage":    true,
		"FederationParentEvidence":    true,
		"FederationObserveRequest":    true,
		"FederationChangeGroup":       true,
		"FederationObservationPage":   true,
		"FederationDenial":            true,
		"FederationConnections":       true,
		"FederationReadConnection":    true,
	}, map[string]bool{"NewFederationRange": true})
}

func checkFederationExports(t *testing.T, root, directory string, files []string, wantTypes, wantFuncs map[string]bool) {
	t.Helper()
	foundTypes := map[string]bool{}
	foundFuncs := map[string]bool{}
	for _, name := range files {
		file := directory + "/" + name
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file)))
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file, data, parser.ParseComments)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		for _, declaration := range parsed.Decls {
			switch declaration := declaration.(type) {
			case *ast.GenDecl:
				switch declaration.Tok {
				case token.TYPE:
					for _, specification := range declaration.Specs {
						typeSpec, ok := specification.(*ast.TypeSpec)
						if ok && ast.IsExported(typeSpec.Name.Name) {
							foundTypes[typeSpec.Name.Name] = true
						}
					}
				case token.VAR, token.CONST:
					for _, specification := range declaration.Specs {
						valueSpec, ok := specification.(*ast.ValueSpec)
						if !ok {
							continue
						}
						for _, name := range valueSpec.Names {
							if ast.IsExported(name.Name) {
								t.Errorf("%s exports forbidden variable or constant %s", file, name.Name)
							}
						}
					}
				}
			case *ast.FuncDecl:
				if declaration.Name != nil && ast.IsExported(declaration.Name.Name) {
					foundFuncs[declaration.Name.Name] = true
				}
			}
		}
	}
	for name := range foundTypes {
		if !wantTypes[name] {
			t.Errorf("%s exports unexpected Federation type %s", directory, name)
		}
	}
	for name := range wantTypes {
		if !foundTypes[name] {
			t.Errorf("%s lost required Federation type %s", directory, name)
		}
	}
	for name := range foundFuncs {
		if !wantFuncs[name] {
			t.Errorf("%s exports unexpected Federation function %s", directory, name)
		}
	}
	for name := range wantFuncs {
		if !foundFuncs[name] {
			t.Errorf("%s lost required Federation function %s", directory, name)
		}
	}
}
