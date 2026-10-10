package federation_test

import (
	"reflect"
	"testing"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/contract/federation"
	federationdenial "ucupa/ucupa-core/internal/query/federation/denial"
	federationvalue "ucupa/ucupa-core/internal/query/federation/value"
)

func TestFederationRangeServiceValues(t *testing.T) {
	// This is the public contract edge formerly checked in the Core root. The
	// value package retains its own leaf validation test; this one proves the
	// exported constructor neither broadens nor remaps the five exact forms.
	sphere, err := corecontract.NewBoundedSphere(12)
	if err != nil {
		t.Fatal(err)
	}
	bounded, err := federation.NewFederationRange(corecontract.RangeBoundedSphere, sphere)
	if got, ok := bounded.BoundedSphere(); err != nil || bounded.Kind() != corecontract.RangeBoundedSphere || !ok || got != sphere {
		t.Fatal("bounded range", bounded, ok, err)
	}
	unbounded, err := federation.NewFederationRange(corecontract.RangeUnboundedRoot, corecontract.BoundedSphere{})
	if got, ok := unbounded.BoundedSphere(); err != nil || unbounded.Kind() != corecontract.RangeUnboundedRoot || ok || got != (corecontract.BoundedSphere{}) {
		t.Fatal("unbounded range", unbounded, ok, err)
	}
	for _, input := range []struct {
		kind   corecontract.RangeKind
		sphere corecontract.BoundedSphere
	}{
		{},
		{kind: corecontract.RangeBoundedSphere},
		{kind: corecontract.RangeUnboundedRoot, sphere: sphere},
	} {
		if got, err := federation.NewFederationRange(input.kind, input.sphere); err != values.PublicInvalid || got != (corecontract.Range{}) {
			t.Fatal("malformed range accepted", got, err)
		}
	}
}

func TestFederationContractAliasesTheUniqueValueRepresentation(t *testing.T) {
	cases := []struct {
		name  string
		left  any
		right any
	}{
		{"FederationReadRequest", federation.FederationReadRequest{}, federationvalue.FederationReadRequest{}},
		{"FederationMountRelation", federation.FederationMountRelation{}, federationvalue.FederationMountRelation{}},
		{"FederationReadPart", federation.FederationReadPart{}, federationvalue.FederationReadPart{}},
		{"FederationRecord", federation.FederationRecord{}, federationvalue.FederationRecord{}},
		{"FederationPoseStep", federation.FederationPoseStep{}, federationvalue.FederationPoseStep{}},
		{"FederationPathRequest", federation.FederationPathRequest{}, federationvalue.FederationPathRequest{}},
		{"FederationSpineNode", federation.FederationSpineNode{}, federationvalue.FederationSpineNode{}},
		{"FederationFrontier", federation.FederationFrontier{}, federationvalue.FederationFrontier{}},
		{"FederationPathResult", federation.FederationPathResult{}, federationvalue.FederationPathResult{}},
		{"FederationQueryRequest", federation.FederationQueryRequest{}, federationvalue.FederationQueryRequest{}},
		{"FederationProjectionRequest", federation.FederationProjectionRequest{}, federationvalue.FederationProjectionRequest{}},
		{"FederationMatch", federation.FederationMatch{}, federationvalue.FederationMatch{}},
		{"FederationProjectionPage", federation.FederationProjectionPage{}, federationvalue.FederationProjectionPage{}},
		{"FederationParentEvidence", federation.FederationParentEvidence{}, federationvalue.FederationParentEvidence{}},
		{"FederationObserveRequest", federation.FederationObserveRequest{}, federationvalue.FederationObserveRequest{}},
		{"FederationChangeGroup", federation.FederationChangeGroup{}, federationvalue.FederationChangeGroup{}},
		{"FederationObservationPage", federation.FederationObservationPage{}, federationvalue.FederationObservationPage{}},
		{"FederationDenial", federation.FederationDenial{}, federationdenial.Evidence{}},
	}
	for _, test := range cases {
		if reflect.TypeOf(test.left) != reflect.TypeOf(test.right) {
			t.Fatalf("%s is a second representation", test.name)
		}
	}
}

func TestFederationDenialRemainsOpaque(t *testing.T) {
	typ := reflect.TypeOf(federation.FederationDenial{})
	if typ.Kind() != reflect.Struct {
		t.Fatalf("denial kind: got %v, want struct", typ.Kind())
	}
	for index := 0; index < typ.NumField(); index++ {
		if field := typ.Field(index); field.IsExported() || field.Anonymous {
			t.Fatalf("denial field %d is externally constructible: %#v", index, field)
		}
	}
	if typ.NumMethod() != 0 || reflect.PointerTo(typ).NumMethod() != 0 {
		t.Fatal("denial gained public methods")
	}
}

func TestFederationProjectionTraversalRemainsHidden(t *testing.T) {
	typ := reflect.TypeOf(federation.FederationProjectionPage{})
	field, ok := typ.FieldByName("traversal")
	if !ok || field.IsExported() || field.PkgPath == "" {
		t.Fatalf("private traversal changed: %#v %v", field, ok)
	}
	if typ.NumMethod() != 0 {
		t.Fatal("projection page gained public behavior")
	}
}

func TestFederationValueFieldOrderRemainsClosed(t *testing.T) {
	cases := []struct {
		value any
		want  []string
	}{
		{federation.FederationReadRequest{}, []string{"Boundary", "Incoming", "Intent", "PrepareObserve", "Mode", "Purpose"}},
		{federation.FederationMountRelation{}, []string{"Incoming", "State", "Key", "Admission", "Revision", "Contact", "NextHop", "Range", "WriteGateOpen"}},
		{federation.FederationReadPart{}, []string{"Hidden", "Subject", "CutStamp", "AuthorizationStamp", "Basis", "Boundary", "Purpose", "ReadMode", "Incoming", "Preparation"}},
		{federation.FederationRecord{}, []string{"ID", "Kind", "Parent", "Pose", "Range", "Revision", "Path", "Attachment", "Mount"}},
		{federation.FederationPoseStep{}, []string{"Pose", "Inverse"}},
		{federation.FederationPathRequest{}, []string{"Origin", "Up", "Down"}},
		{federation.FederationSpineNode{}, []string{"ID", "Parent", "Pose"}},
		{federation.FederationFrontier{}, []string{"Relation", "Path", "ToScope"}},
		{federation.FederationPathResult{}, []string{"Element", "Kind", "ActualUp", "RemainingUp", "RemainingDown", "Spine", "Frontier"}},
		{federation.FederationQueryRequest{}, []string{"Scope", "Sphere", "Selector", "ToBasis"}},
		{federation.FederationProjectionRequest{}, []string{"Kind", "Path", "Element", "Query"}},
		{federation.FederationMatch{}, []string{"ID", "Kind", "Revision", "Path"}},
		{federation.FederationProjectionPage{}, []string{"Parents", "Coverage", "traversal", "Basis", "Purpose", "Records", "Matches", "Frontiers", "Path", "Gap", "Complete", "Continuation"}},
		{federation.FederationParentEvidence{}, []string{"Parent", "Adjacency"}},
		{federation.FederationObserveRequest{}, []string{"Operation", "Cursor", "Limit"}},
		{federation.FederationChangeGroup{}, []string{"ID", "Basis", "Categories", "Terminal"}},
		{federation.FederationObservationPage{}, []string{"Cursor", "Groups"}},
	}
	for _, test := range cases {
		typ := reflect.TypeOf(test.value)
		got := make([]string, typ.NumField())
		for index := range got {
			got[index] = typ.Field(index).Name
		}
		if !reflect.DeepEqual(got, test.want) {
			t.Fatalf("%s fields: got=%v want=%v", typ.Name(), got, test.want)
		}
	}
}
