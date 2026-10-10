package fact

import (
	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
)

// The external parent is context, never a claimed Element or synthetic root.
// Before initial activation there is no admitted content and the gate is shut;
// the accepted proposal still determines the sole allowed external boundary.

func IsExternalParent(mounts View, id values.ElementID) bool {
	incoming := mounts.Incoming()
	return incoming.Key.Valid() && id == incoming.Key.MountAnchorUUID
}

func GuardLocalList(mounts View, parent values.ElementID) error {
	if IsExternalParent(mounts, parent) || HasContent(mounts, parent) {
		return corecontract.ErrMountContextRequired
	}
	return nil
}
