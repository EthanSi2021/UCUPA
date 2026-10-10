package commit

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/geometry"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	"ucupa/ucupa-core/internal/token"
)

func (state *coreState) publicMountCommand(c contract.MountCommand) contract.PublicMountCommand {
	return contract.PublicMountCommand{Kind: c.Kind, Element: c.MountAnchorUUID,
		ExpectedElement: token.PublicElementRevision(state.publicKey, state.authority, c.ExpectedElementRevision),
		ExpectedMount:   token.PublicMountRevision(state.publicKey, state.authority, c.ExpectedMountRevision),
		Carrier:         c.CarrierAuthority, CarrierNextHop: c.CarrierNextHop,
		ParentNextHop: c.ParentNextHop, Final: c.Final}
}

// Shape validation and authorization precede this deterministic comparison.
// The retained map remains part of the one commit aggregate; the control flow
// receives only the exact resolved command it needs to reserve or retry.
func (state *coreState) resolvePublicMount(actor values.PrincipalRef, id contract.OperationID, input contract.PublicMountCommand, command contract.MountCommand) (contract.MountCommand, error) {
	if old, exists := state.mount.operations[id]; exists {
		// Do not disclose or reuse another user's retained operation even when
		// the new user has identical current permission.
		if old.Binding.Actor != actor || state.publicMountCommand(old.Binding.Command) != input {
			return contract.MountCommand{}, values.PublicConflict
		}
		command = old.Binding.Command
	} else {
		anchor, exists := state.elements.Anchor(input.Element)
		if !exists {
			return contract.MountCommand{}, values.PublicNotFound
		}
		command.ExpectedElementRevision = contract.Revision{}
		command.ExpectedMountRevision = contract.MountRevision{}
		if input.Kind != contract.MountRevoke {
			command.ExpectedElementRevision = anchor.Revision
		}
		if input.Kind != contract.MountEstablish {
			command.ExpectedMountRevision = state.mount.facts.Record(input.Element).Revision
		}
		if state.publicMountCommand(command) != input {
			return contract.MountCommand{}, values.PublicConflict
		}
		if _, valid := CanonicalMountCommand(command); !valid {
			return contract.MountCommand{}, values.PublicConflict
		}
	}
	return command, nil
}

func (state *coreState) mountContactAt(element values.ElementID, incoming bool) (contract.MountKey, mountfact.Contact) {
	if incoming {
		if state.mount.facts.Incoming().Key.MountAnchorUUID != element {
			return contract.MountKey{}, mountfact.Contact{}
		}
		return state.mount.facts.Incoming().Key, state.mount.facts.Incoming().Contact
	}
	r := state.mount.facts.Record(element)
	return r.Key, r.Contact
}

// Contact recovery resolves one retained binding, never a lookup address or a
// new permission. The caller still revalidates its view before and after commit.
func (state *coreState) resolvePublicMountContact(actor values.PrincipalRef, id contract.OperationID, element values.ElementID, incoming bool, expected values.PublicToken, nextHop values.NextHop) (NextHopBinding, error) {
	key, contact := state.mountContactAt(element, incoming)
	if retained, exists := state.mount.addresses[id]; exists {
		b := retained.binding
		if b.Actor != actor || b.Element != element || token.NextHopIncoming(b.Expected) != incoming || b.NextHop != nextHop || token.PublicMountContact(state.publicKey, state.authority, b.Expected) != expected {
			return NextHopBinding{}, values.PublicConflict
		}
		key, contact.Revision = b.Key, b.Expected
	} else if !key.Valid() {
		return NextHopBinding{}, values.PublicNotFound
	} else if token.PublicMountContact(state.publicKey, state.authority, contact.Revision) != expected {
		return NextHopBinding{}, values.PublicConflict
	}
	return NextHopBinding{Actor: actor, Element: element, Key: key, Expected: contact.Revision, NextHop: nextHop}, nil
}

type MountControlPlan struct {
	Carrier  values.FactAuthorityRef
	Address  values.NextHop
	Contact  contract.NextHopRevision
	Peer     values.PrincipalRef
	Approval bool
}

func (state *coreState) mountControlPlan(id contract.OperationID, command contract.MountCommand) MountControlPlan {
	record := state.mount.facts.Record(command.MountAnchorUUID)
	p := MountControlPlan{Carrier: record.Key.CarrierAuthority, Address: record.Contact.NextHop, Contact: record.Contact.Revision}
	if command.Kind == contract.MountEstablish {
		p.Carrier, p.Address = command.CarrierAuthority, command.CarrierNextHop
		p.Approval = true
	}
	if command.Kind == contract.MountUpdate {
		if anchor, ok := state.elements.Anchor(command.MountAnchorUUID); ok && !anchor.Root() {
			if sphere, bounded := anchor.Range.BoundedSphere(); bounded {
				p.Approval = !geometry.EqualSphere(sphere, command.Final.Range)
			}
		}
	}
	if old, ok := state.mount.operations[id]; ok {
		if old.Request.Key.Valid() {
			p.Carrier, p.Address = old.Request.Key.CarrierAuthority, record.Contact.NextHop
		}
		p.Peer = old.Binding.PeerActor
		p.Approval = old.Decision == contract.MountPending && old.Binding.PeerActor.Valid()
	}
	return p
}

func (owner *Owner) MountControlPlanLocked(id contract.OperationID, command contract.MountCommand) MountControlPlan {
	return owner.state.mountControlPlan(id, command)
}
func (owner *Owner) ResolvePublicMountLocked(actor values.PrincipalRef, id contract.OperationID, input contract.PublicMountCommand, command contract.MountCommand) (contract.MountCommand, error) {
	return owner.state.resolvePublicMount(actor, id, input, command)
}
func (owner *Owner) ResolvePublicMountContactLocked(actor values.PrincipalRef, id contract.OperationID, element values.ElementID, incoming bool, expected values.PublicToken, address values.NextHop) (NextHopBinding, error) {
	return owner.state.resolvePublicMountContact(actor, id, element, incoming, expected, address)
}
func (owner *Owner) MountContactAtLocked(element values.ElementID, incoming bool) (contract.MountKey, mountfact.Contact) {
	return owner.state.mountContactAt(element, incoming)
}
