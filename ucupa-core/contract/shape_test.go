package contract_test

import (
	"reflect"
	"testing"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
)

func TestGreenfieldCoreExportedTypeShapes(t *testing.T) {
	floatType := reflect.TypeOf(float64(0))
	elementIDType := reflect.TypeOf(values.ElementID(""))
	assertExactPublicStruct(t, reflect.TypeOf(contract.Vec3{}), []publicField{
		{name: "X", typ: floatType},
		{name: "Y", typ: floatType},
		{name: "Z", typ: floatType},
	})
	assertExactPublicStruct(t, reflect.TypeOf(contract.Pose{}), []publicField{
		{name: "Translation", typ: reflect.TypeOf(contract.Vec3{})},
		{name: "Rotation", typ: reflect.TypeOf(contract.Rotation{})},
	})
	assertExactPublicStruct(t, reflect.TypeOf(contract.SpatialTuple{}), []publicField{
		{name: "Parent", typ: elementIDType},
		{name: "LocalPose", typ: reflect.TypeOf(contract.Pose{})},
		{name: "Range", typ: reflect.TypeOf(contract.BoundedSphere{})},
	})
	assertExactPublicStruct(t, reflect.TypeOf(contract.AnchorRecord{}), []publicField{
		{name: "ID", typ: elementIDType},
		{name: "ParentAnchorID", typ: reflect.TypeOf((*values.ElementID)(nil))},
		{name: "LocalPose", typ: reflect.TypeOf((*contract.Pose)(nil))},
		{name: "Range", typ: reflect.TypeOf(contract.Range{})},
		{name: "Revision", typ: reflect.TypeOf(contract.Revision{})},
	})
	assertExactPublicStruct(t, reflect.TypeOf(contract.EntityRecord{}), []publicField{
		{name: "ID", typ: elementIDType},
		{name: "ContainingAnchorID", typ: elementIDType},
		{name: "LocalPose", typ: reflect.TypeOf(contract.Pose{})},
		{name: "Range", typ: reflect.TypeOf(contract.BoundedSphere{})},
		{name: "Revision", typ: reflect.TypeOf(contract.Revision{})},
	})
	assertExactPublicStruct(t, reflect.TypeOf(contract.ElementRecord{}), []publicField{
		{name: "Kind", typ: reflect.TypeOf(contract.ElementKind(0))},
		{name: "Anchor", typ: reflect.TypeOf((*contract.AnchorRecord)(nil))},
		{name: "Entity", typ: reflect.TypeOf((*contract.EntityRecord)(nil))},
	})
	assertExactPublicStruct(t, reflect.TypeOf(contract.MutationResult{}), []publicField{
		{name: "Status", typ: reflect.TypeOf(contract.MutationStatus(0))},
		{name: "Record", typ: reflect.TypeOf((*contract.ElementRecord)(nil))},
		{name: "TombstoneRevision", typ: reflect.TypeOf(contract.Revision{})},
		{name: "OutcomeCut", typ: reflect.TypeOf(contract.CoreReadCut{})},
	})
	assertExactPublicStruct(t, reflect.TypeOf(contract.ElementRead{}), []publicField{
		{name: "Authority", typ: reflect.TypeOf(values.FactAuthorityRef(""))},
		{name: "Element", typ: reflect.TypeOf(values.ElementID(""))},
		{name: "Attachment", typ: reflect.TypeOf(contract.FacetAttachment{})},
		{name: "State", typ: reflect.TypeOf(contract.ExistenceState(0))},
		{name: "Record", typ: reflect.TypeOf((*contract.ElementRecord)(nil))},
		{name: "Cut", typ: reflect.TypeOf(contract.CoreReadCut{})},
	})
	assertExactPublicStruct(t, reflect.TypeOf(contract.DirectPage{}), []publicField{
		{name: "Records", typ: reflect.TypeOf([]contract.ElementRecord(nil))},
		{name: "Cut", typ: reflect.TypeOf(contract.CoreReadCut{})},
		{name: "Continuation", typ: reflect.TypeOf((*contract.ListContinuation)(nil))},
	})
	assertExactPublicStruct(t, reflect.TypeOf(contract.Projection{}), []publicField{
		{name: "Pose", typ: reflect.TypeOf(contract.Pose{})},
		{name: "SubjectRevision", typ: reflect.TypeOf(contract.Revision{})},
		{name: "Cut", typ: reflect.TypeOf(contract.CoreReadCut{})},
	})
	assertExactPublicStruct(t, reflect.TypeOf(contract.QuerySphere{}), []publicField{
		{name: "Center", typ: reflect.TypeOf(contract.Vec3{})},
		{name: "Radius", typ: floatType},
	})
	assertExactPublicStruct(t, reflect.TypeOf(contract.RegionQuery{}), []publicField{
		{name: "StartAnchorID", typ: elementIDType},
		{name: "UpDepth", typ: reflect.TypeOf(int64(0))},
		{name: "BasisAnchorID", typ: elementIDType},
		{name: "Sphere", typ: reflect.TypeOf(contract.QuerySphere{})},
		{name: "Selector", typ: reflect.TypeOf(contract.QuerySelector(0))},
	})
	assertExactPublicStruct(t, reflect.TypeOf(contract.QueryMatch{}), []publicField{
		{name: "ElementID", typ: elementIDType},
		{name: "Kind", typ: reflect.TypeOf(contract.ElementKind(0))},
		{name: "Revision", typ: reflect.TypeOf(contract.Revision{})},
		{name: "Path", typ: reflect.TypeOf([]values.ElementID(nil))},
	})
	assertExactPublicStruct(t, reflect.TypeOf(contract.QueryPage{}), []publicField{
		{name: "Matches", typ: reflect.TypeOf([]contract.QueryMatch(nil))},
		{name: "Cut", typ: reflect.TypeOf(contract.CoreReadCut{})},
		{name: "ReadMode", typ: reflect.TypeOf(contract.QueryReadMode(0))},
		{name: "Snapshot", typ: reflect.TypeOf((*contract.CoreReadSnapshot)(nil))},
		{name: "ScopeAnchorID", typ: elementIDType},
		{name: "ActualUpDepth", typ: reflect.TypeOf(uint64(0))},
		{name: "Status", typ: reflect.TypeOf(contract.QueryStatus(0))},
		{name: "Gap", typ: reflect.TypeOf(contract.QueryGap(0))},
		{name: "Continuation", typ: reflect.TypeOf((*contract.QueryContinuation)(nil))},
	})

	for _, opaque := range []reflect.Type{
		reflect.TypeOf(contract.BoundedSphere{}),
		reflect.TypeOf(contract.CoreReadCut{}),
		reflect.TypeOf(contract.ListContinuation{}),
		reflect.TypeOf(contract.QueryContinuation{}),
		reflect.TypeOf(contract.Range{}),
		reflect.TypeOf(contract.Revision{}),
		reflect.TypeOf(contract.Rotation{}),
	} {
		assertNoExportedStructFields(t, opaque)
	}

}

type publicField struct {
	name string
	typ  reflect.Type
}

func assertExactPublicStruct(t *testing.T, value reflect.Type, want []publicField) {
	t.Helper()
	if value.Kind() != reflect.Struct || value.NumField() != len(want) {
		t.Fatalf("%s fields: got %d want %d", value, value.NumField(), len(want))
	}
	for index, expected := range want {
		field := value.Field(index)
		if field.Name != expected.name || field.Type != expected.typ ||
			field.PkgPath != "" || field.Anonymous || field.Tag != "" {
			t.Fatalf("%s field %d: got %#v want name=%s type=%v exported non-embedded untagged", value, index, field, expected.name, expected.typ)
		}
	}
}
