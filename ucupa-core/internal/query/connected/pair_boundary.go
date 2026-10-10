package connected

import (
	corecontract "ucupa/ucupa-core/contract"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
)

// connectedPairSupported bounds the existing two-source Connected reader,
// not the legal Core topology. The public multilevel coordinator must account
// for every relation separately; selecting an arbitrary first outgoing mount
// here would erase other possible sources and could falsely report COMPLETE.
func connectedPairSupported(mounts mountfact.View) bool {
	outgoing := 0
	for _, relation := range mounts.Outgoings() {
		if relation.State != corecontract.MountNone {
			outgoing++
		}
	}
	return outgoing <= 1 && (outgoing == 0 || !mounts.Incoming().Key.Valid())
}
