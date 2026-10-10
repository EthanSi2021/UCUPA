package transition_test

import (
	"reflect"
	"testing"

	"ucupa/ucupa-core/contract/transition"
)

func TestTransferEvidenceIsOpaqueToPublicConsumers(t *testing.T) {
	for _, v := range []any{transition.Frozen{}, transition.Acceptance{}, transition.Decision{}, transition.Completion{},
		transition.CarrierPlan{}, transition.CarrierFrozen{}, transition.CarrierAcceptance{}, transition.CarrierDecision{}} {
		typeOf := reflect.TypeOf(v)
		for i := 0; i < typeOf.NumField(); i++ {
			if typeOf.Field(i).IsExported() {
				t.Fatalf("%s exposes %s", typeOf, typeOf.Field(i).Name)
			}
		}
		if typeOf.NumMethod() != 0 || reflect.PointerTo(typeOf).NumMethod() != 0 {
			t.Fatalf("%s exposes extraction or construction", typeOf)
		}
	}
}
