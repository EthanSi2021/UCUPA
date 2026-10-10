package fact

import (
	"strings"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	coretoken "ucupa/ucupa-core/internal/token"
)

func TestValidMountAdmissionKey(t *testing.T) {
	validMount := coretoken.MountKey{
		ParentAuthority:  values.FactAuthorityRef("00000000-0000-4000-8000-000000000001"),
		MountAnchorUUID:  values.ElementID("00000000-0000-4000-8000-000000000002"),
		CarrierAuthority: values.FactAuthorityRef("00000000-0000-4000-8000-000000000003"),
		ProposalRevision: coretoken.MountRevisionFromParts([32]byte{1}, values.ElementID("00000000-0000-4000-8000-000000000002"), 1),
	}
	tests := []struct {
		name string
		key  contract.MountAdmissionKey
		want bool
	}{
		{name: "empty key"},
		{name: "empty operation", key: contract.MountAdmissionKey{MountKey: validMount}},
		{name: "128 bytes", key: contract.MountAdmissionKey{MountKey: validMount, OperationID: contract.OperationID(strings.Repeat("a", 128))}, want: true},
		{name: "129 bytes", key: contract.MountAdmissionKey{MountKey: validMount, OperationID: contract.OperationID(strings.Repeat("a", 129))}},
		{name: "multi-byte within byte limit", key: contract.MountAdmissionKey{MountKey: validMount, OperationID: contract.OperationID(strings.Repeat("界", 42))}, want: true},
		{name: "multi-byte beyond byte limit", key: contract.MountAdmissionKey{MountKey: validMount, OperationID: contract.OperationID(strings.Repeat("界", 43))}},
		{name: "valid key", key: contract.MountAdmissionKey{MountKey: validMount, OperationID: "operation"}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidMountAdmissionKey(tt.key); got != tt.want {
				t.Fatalf("ValidMountAdmissionKey() = %v, want %v", got, tt.want)
			}
		})
	}
}
