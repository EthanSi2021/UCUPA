package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"ucupa/composition"
	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	facetcontract "ucupa/ucupa-facet/contract"
	realm "ucupa/ucupa-realm/contract"
	realminstance "ucupa/ucupa-realm/instance"
)

const (
	worldID       values.ElementID = "00000000-0000-4000-8000-000000003001"
	siteID        values.ElementID = "00000000-0000-4000-8000-000000003002"
	destinationID values.ElementID = "00000000-0000-4000-8000-000000003003"
	localEntityID values.ElementID = "00000000-0000-4000-8000-000000003004"
)

var (
	operator     = values.PrincipalRef{Realm: "realm-p", Principal: "operator"}
	facetService = values.PrincipalRef{Realm: "realm-p", Principal: "facet-service"}
)

func main() {
	if err := run(os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "ucupa: FAIL: %v\n", err)
		os.Exit(1)
	}
}

func run(output io.Writer) error {
	cluster, err := newCluster()
	if err != nil {
		return err
	}
	defer cluster.Core().Close()
	identity, err := corecontract.NewRotation(1, 0, 0, 0)
	if err != nil {
		return err
	}
	quarterTurn, err := corecontract.NewRotation(1, 0, 0, 1)
	if err != nil {
		return err
	}
	sitePose, err := corecontract.NewPose(corecontract.Vec3{X: 10}, identity)
	if err != nil {
		return err
	}
	destinationPose, err := corecontract.NewPose(corecontract.Vec3{X: 5}, quarterTurn)
	if err != nil {
		return err
	}
	entityPose, err := corecontract.NewPose(corecontract.Vec3{X: 3}, identity)
	if err != nil {
		return err
	}
	movedPose, err := corecontract.NewPose(corecontract.Vec3{Y: 2}, identity)
	if err != nil {
		return err
	}
	siteRange, err := corecontract.NewBoundedSphere(20)
	if err != nil {
		return err
	}
	destinationRange, err := corecontract.NewBoundedSphere(10)
	if err != nil {
		return err
	}
	entityRange, err := corecontract.NewBoundedSphere(1)
	if err != nil {
		return err
	}

	if _, err := cluster.Core().Elements().CreateRoot(operator, worldID, "runner-core-create-root"); err != nil {
		return fmt.Errorf("create root: %w", err)
	}
	if _, err := cluster.Core().Elements().CreateAnchor(operator, siteID, corecontract.SpatialTuple{
		Parent: worldID, LocalPose: sitePose, Range: siteRange,
	}, "runner-core-create-site"); err != nil {
		return fmt.Errorf("create site: %w", err)
	}
	if _, err := cluster.Core().Elements().CreateAnchor(operator, destinationID, corecontract.SpatialTuple{
		Parent: worldID, LocalPose: destinationPose, Range: destinationRange,
	}, "runner-core-create-destination"); err != nil {
		return fmt.Errorf("create destination: %w", err)
	}
	created, err := cluster.Core().Elements().CreateEntity(operator, localEntityID, corecontract.SpatialTuple{
		Parent: siteID, LocalPose: entityPose, Range: entityRange,
	}, "runner-core-create-entity")

	if err != nil || created.Record == nil || created.Record.Entity == nil {
		return fmt.Errorf("create entity: result %#v, error %v", created, err)
	}

	localFacetType := demoType()
	key := values.FacetKey{Element: localEntityID, Type: localFacetType}
	access, err := attachDemo(cluster)
	if err != nil {
		return err
	}
	payload := []byte(`"local-facet"`)
	if _, err := cluster.Facets().Namespace.Put(context.Background(), operator, access, key.Type, payload, facetcontract.ExpectAbsent(), "runner-local-put"); err != nil {
		return fmt.Errorf("put facet: %w", err)
	}
	moved, err := cluster.Core().Elements().UpdateEntity(operator, localEntityID, corecontract.SpatialTuple{
		Parent: destinationID, LocalPose: movedPose, Range: entityRange,
	}, created.Record.Entity.Revision, "runner-core-update")

	if err != nil || moved.Record == nil || moved.Record.Entity == nil {
		return fmt.Errorf("update entity: result %#v, error %v", moved, err)
	}
	projection, err := cluster.Core().Elements().ProjectToAncestor(operator, localEntityID, worldID, nil)
	if err != nil {
		return fmt.Errorf("project entity to root: %w", err)
	}
	// The probe is in site coordinates, but its Scope is the complete local
	// root. The moved Entity is now in the sibling destination branch. Its
	// center is outside the query sphere; its Range is exactly tangent.
	page, err := cluster.Core().Queries().QueryRegion(operator, corecontract.RegionQuery{
		StartAnchorID: worldID,
		BasisAnchorID: siteID,
		Sphere:        corecontract.QuerySphere{Radius: 6},
		Selector:      corecontract.QueryEntities,
	}, &projection.Cut, 1, nil)

	if err != nil || page.Status != corecontract.QueryComplete || page.Gap != corecontract.QueryNoGap ||
		page.Continuation != nil || len(page.Matches) != 1 {
		return fmt.Errorf("query sibling branch: page %#v, error %v", page, err)
	}
	match := page.Matches[0]
	if match.ElementID != localEntityID || match.Kind != corecontract.ElementEntity ||
		len(match.Path) != 2 || match.Path[0] != destinationID || match.Path[1] != localEntityID {
		return fmt.Errorf("query sibling path: %#v", match)
	}
	read, err := cluster.Core().Elements().ReadElement(operator, match.ElementID, &page.Cut)
	if err != nil || read.State != corecontract.ExistencePresent || read.Record == nil ||
		read.Record.Entity == nil || !read.Record.Entity.Revision.Equal(match.Revision) || !read.Cut.Equal(page.Cut) {
		return fmt.Errorf("same-cut query read: result %#v, error %v", read, err)
	}
	target, err := cluster.FacetConnections().Connect(read.Attachment.NextHop, read.Attachment.Key.Facet)
	if err != nil {
		return err
	}
	if got, err := target.Namespace.Get(context.Background(), operator, read.Attachment.Key, key.Type); err != nil || !bytes.Equal(got.Payload, payload) {
		return fmt.Errorf("read facet: payload %q, error %v", got, err)
	}
	if _, err := cluster.Core().Elements().DeleteEntity(operator, localEntityID, moved.Record.Entity.Revision, "runner-core-delete"); err != nil {
		return fmt.Errorf("delete entity: %w", err)
	}
	if _, err := target.Namespace.Get(context.Background(), operator, access, key.Type); !errors.Is(err, facetcontract.ErrElementNotFound) {
		return fmt.Errorf("facet after delete: got %v, want %v", err, facetcontract.ErrElementNotFound)
	}

	var connectedOutput bytes.Buffer
	if err := runConnected(&connectedOutput); err != nil {
		return err
	}
	_, err = fmt.Fprintf(output,
		"UCUPA: PASS\ncluster: %s/%s owner-local\nelement: %s root-relative=(%.9g,%.9g,%.9g) cross-branch query+same-cut read; identity+facet preserved; deleted subject hidden\n",
		cluster.ID(), cluster.RealmID(), localEntityID,
		projection.Pose.Translation.X, projection.Pose.Translation.Y, projection.Pose.Translation.Z,
	)
	if err != nil {
		return err
	}
	_, err = io.Copy(output, &connectedOutput)
	if err != nil {
		return err
	}
	return runPublic(output)
}

func newCluster() (*composition.Cluster, error) {
	localFacetType := demoType()
	key := values.FacetKey{Element: localEntityID, Type: localFacetType}
	policy, err := realminstance.New(realm.Seed{
		ID:              "realm-p",
		LocalPrincipals: []values.PrincipalID{operator.Principal, facetService.Principal},
		Bindings: []realm.PermissionBinding{
			binding(operator, realm.PermissionPutBinding, values.RealmScope(operator.Realm)),
		},
	})
	if err != nil {
		return nil, err
	}
	cluster, err := composition.NewCluster(composition.Seed{
		ID: "cluster-p", Realm: policy, RealmNextHop: "local:realm", FacetServiceActor: facetService, CoreServiceActor: facetService, CoreNextHop: "local:core", FacetNextHop: "local:facet",
	})
	if err != nil {
		return nil, err
	}
	owner := cluster.Core().AuthorityRef()
	bindings := []realm.PermissionBinding{
		binding(operator, facetcontract.PermissionPut, values.FacetKeyScope(cluster.Facets().AuthorityRef(), key.Element, key.Type)),
		binding(operator, facetcontract.PermissionGet, values.FacetKeyScope(cluster.Facets().AuthorityRef(), key.Element, key.Type)),
		binding(operator, corecontract.PermissionFacetPropose, values.CoreElementScope(owner, key.Element)),
		binding(operator, corecontract.PermissionFacetActivate, values.CoreElementScope(owner, key.Element)),
		binding(operator, facetcontract.PermissionAccept, values.FacetElementScope(cluster.Facets().AuthorityRef(), key.Element)),
		binding(facetService, facetcontract.PermissionReadAcceptance, values.FacetElementScope(cluster.Facets().AuthorityRef(), key.Element)),
		binding(operator, corecontract.PermissionCreateRoot, values.CoreElementScope(owner, worldID)),
		binding(operator, corecontract.PermissionCreateAnchor, values.CoreElementScope(owner, worldID)),
		binding(operator, corecontract.PermissionCreateEntity, values.CoreElementScope(owner, siteID)),
		binding(operator, corecontract.PermissionReadElement, values.CoreElementScope(owner, worldID)),
		binding(operator, corecontract.PermissionQueryRegion, values.CoreElementScope(owner, worldID)),
		binding(operator, corecontract.PermissionReadElement, values.CoreElementScope(owner, destinationID)),
		binding(operator, corecontract.PermissionReadElement, values.CoreElementScope(owner, localEntityID)),
		binding(operator, corecontract.PermissionUpdateElement, values.CoreElementScope(owner, localEntityID)),
		binding(operator, corecontract.PermissionPlaceElement, values.CoreElementScope(owner, destinationID)),
		binding(operator, corecontract.PermissionDeleteEntity, values.CoreElementScope(owner, localEntityID)),
		binding(facetService, corecontract.PermissionFacetProposalRead, values.CoreElementScope(owner, localEntityID)),
		binding(facetService, corecontract.PermissionFacetAttachmentRead, values.CoreElementScope(owner, localEntityID)),
	}
	for index, grant := range bindings {
		if _, err := policy.PutBinding(operator, grant, policy.BindingSetRevision(), realm.OperationID(fmt.Sprintf("runner-bootstrap-grant-%d", index))); err != nil {
			return nil, fmt.Errorf("grant Core access: %w", err)
		}
	}
	return cluster, nil
}

func binding(subject values.PrincipalRef, permission values.PermissionID, scope values.OperationScope) realm.PermissionBinding {
	return realm.PermissionBinding{Subject: subject, Permission: permission, Scope: scope}
}
