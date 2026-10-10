package projection

import (
	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	federationvalue "ucupa/ucupa-core/internal/query/federation/value"
	coretoken "ucupa/ucupa-core/internal/token"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func (s *Projection) incoming() federationvalue.FederationMountRelation {
	return Incoming(s.mount, s.signer)
}
func (s *Projection) outgoing(id values.ElementID) federationvalue.FederationMountRelation {
	return Outgoing(s.elements, s.mount, s.transfers, id, s.signer)
}
func Incoming(mounts mountfact.View, signer coretoken.PublicSigner) federationvalue.FederationMountRelation {
	in := mounts.Incoming()
	return federationvalue.FederationMountRelation{Incoming: true, State: corecontract.MountActive, Key: in.Key,
		Admission: in.Admission.Key, Revision: signer.MountRevision(in.Key.ProposalRevision),
		Contact: signer.MountContact(in.Contact.Revision), NextHop: in.Contact.NextHop,
		Range: in.Admission.RangeValue, WriteGateOpen: in.Gate == mountfact.GateOpen && !in.Detached}
}

func Outgoing(elements elementfact.View, mounts mountfact.View, transfers transferfact.Visibility, id values.ElementID, signer coretoken.PublicSigner) federationvalue.FederationMountRelation {
	mount := mounts.Record(id)
	anchor, _ := elements.Anchor(id)
	rangeValue, _ := anchor.Range.BoundedSphere()
	return federationvalue.FederationMountRelation{State: mount.State, Key: mount.Key, Admission: mount.Admission,
		Revision: signer.MountRevision(mount.Revision), Contact: signer.MountContact(mount.Contact.Revision),
		NextHop: mount.Contact.NextHop, Range: rangeValue,
		WriteGateOpen: transfers.OutgoingReadable(id, mount)}
}
