package contract_test

import (
	"reflect"
	"testing"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
)

func TestConnectedReadPublicShapes(t *testing.T) {
	elementID := reflect.TypeOf(values.ElementID(""))
	elementIDs := reflect.TypeOf([]values.ElementID(nil))
	path := reflect.TypeOf(corecontract.Path{})
	source := reflect.TypeOf(corecontract.ConnectedSource{})
	cut := reflect.TypeOf(corecontract.ConnectedCut{})
	readMode := reflect.TypeOf(corecontract.QueryReadMode(0))
	status := reflect.TypeOf(corecontract.QueryStatus(0))
	gap := reflect.TypeOf(corecontract.QueryGap(0))

	assertExactPublicContractStruct(t, corecontract.Path{}, []publicContractField{
		{"OriginAnchorID", elementID},
		{"Up", reflect.TypeOf(uint64(0))},
		{"Down", elementIDs},
	})
	assertExactPublicContractStruct(t, corecontract.PathRegionQuery{}, []publicContractField{
		{"StartPath", path},
		{"UpDepth", reflect.TypeOf(int64(0))},
		{"BasisPath", path},
		{"Sphere", reflect.TypeOf(corecontract.QuerySphere{})},
		{"Selector", reflect.TypeOf(corecontract.QuerySelector(0))},
	})
	assertExactPublicContractStruct(t, corecontract.ConnectedSource{}, []publicContractField{
		{"Authority", reflect.TypeOf(values.FactAuthorityRef(""))},
		{"Cut", reflect.TypeOf(corecontract.CoreReadCut{})},
	})
	assertExactPublicContractStruct(t, corecontract.ConnectedMatch{}, []publicContractField{
		{"ElementID", elementID},
		{"Kind", reflect.TypeOf(corecontract.ElementKind(0))},
		{"Revision", reflect.TypeOf(corecontract.Revision{})},
		{"Path", elementIDs},
		{"Source", source},
	})
	assertExactPublicContractStruct(t, corecontract.ConnectedRecord{}, []publicContractField{
		{"Record", reflect.TypeOf(corecontract.ElementRecord{})},
		{"Source", source},
	})
	assertExactPublicContractStruct(t, corecontract.PathRead{}, []publicContractField{
		{"Attachment", reflect.TypeOf(corecontract.FacetAttachment{})},
		{"State", reflect.TypeOf(corecontract.ExistenceState(0))},
		{"Record", reflect.TypeOf((*corecontract.ElementRecord)(nil))},
		{"Source", source},
		{"Path", path},
		{"VerifiedPrefix", path},
		{"AbsenceSource", source},
		{"Cut", cut},
		{"ReadMode", readMode},
	})
	assertExactPublicContractStruct(t, corecontract.ConnectedQueryPage{}, []publicContractField{
		{"Matches", reflect.TypeOf([]corecontract.ConnectedMatch(nil))},
		{"Cut", cut},
		{"ReadMode", readMode},
		{"ResolvedScopePath", path},
		{"ActualUpDepth", reflect.TypeOf(uint64(0))},
		{"Status", status},
		{"Gap", gap},
		{"Continuation", reflect.TypeOf((*corecontract.ConnectedQueryContinuation)(nil))},
	})
	assertExactPublicContractStruct(t, corecontract.ConnectedDirectPage{}, []publicContractField{
		{"Records", reflect.TypeOf([]corecontract.ConnectedRecord(nil))},
		{"Cut", cut},
		{"ReadMode", readMode},
		{"Status", status},
		{"Gap", gap},
		{"Continuation", reflect.TypeOf((*corecontract.ConnectedDirectContinuation)(nil))},
	})
}
