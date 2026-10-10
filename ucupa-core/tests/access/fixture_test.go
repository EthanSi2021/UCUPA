package access_test

import (
	"fmt"
	"testing"
	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
)

var actor = values.PrincipalRef{Realm: "test-realm", Principal: "builder"}

func id(n uint64) values.ElementID {
	return values.ElementID(fmt.Sprintf("00000000-0000-4000-8000-%012x", n))
}
func createRoot(t *testing.T, c *core.Core, v values.ElementID, op corecontract.OperationID) corecontract.ElementRecord {
	t.Helper()
	r, e := c.Elements().CreateRoot(actor, v, op)
	if e != nil || r.Status != corecontract.MutationCommitted || r.Record == nil {
		t.Fatalf("create root: %#v %v", r, e)
	}
	return *r.Record
}
