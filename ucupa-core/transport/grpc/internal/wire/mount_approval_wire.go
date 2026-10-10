package wire

import (
	"ucupa/shared/values"
	corepb "ucupa/ucupa-core/bindings/go/protobuf"
)

func ValidApprovalBinding(b *corepb.MountApprovalBinding) bool {
	return b != nil && values.FactAuthorityRef(b.GetActor()).Valid() && values.ElementID(b.GetElement()).Valid() &&
		len(b.OperationId) > 0 && len(b.OperationId) <= 128 && len(b.CommandDigest) == 32 && [32]byte(b.CommandDigest) != ([32]byte{})
}
