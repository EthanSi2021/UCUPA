package fact

import (
	"ucupa/ucupa-core/contract"
	commitbinding "ucupa/ucupa-core/internal/commit/binding"
)

func ValidMountAdmissionKey(key contract.MountAdmissionKey) bool {
	return key.MountKey.Valid() && commitbinding.ValidOperationID(key.OperationID)
}
