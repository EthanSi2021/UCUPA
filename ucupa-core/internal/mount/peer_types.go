package mount

import (
	"reflect"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
)

type localMountPeer struct{ core *Operations }

func (core *Operations) MountPeer() contract.MountPeerPort { return localMountPeer{core: core} }

func (peer localMountPeer) AuthorityRef() values.FactAuthorityRef { return peer.core.AuthorityRef() }

func (core *Operations) ConfigureMountTransport(transport contract.MountTransport) error {
	if core == nil {
		return contract.ErrInvalid
	}
	if transport != nil {
		value := reflect.ValueOf(transport)
		switch value.Kind() {
		case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
			if value.IsNil() {
				return contract.ErrInvalid
			}
		}
	}
	core.owner.Lock()
	defer core.owner.Unlock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		return contract.ErrStore
	}
	core.mountTransport = transport
	return nil
}
