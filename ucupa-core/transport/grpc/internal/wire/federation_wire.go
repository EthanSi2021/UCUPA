package wire

import (
	federationcontract "ucupa/ucupa-core/contract/federation"

	"google.golang.org/protobuf/proto"

	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"

	pb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
)

func FederationToken(w *netpb.Token, kind netpb.TokenKind, literal string) (values.PublicToken, error) {
	if w == nil || w.GetKind() != kind || !values.FactAuthorityRef(w.GetOwner()).Valid() || len(w.Value) != 32 {
		return values.PublicToken{}, corecontract.ErrInvalid
	}
	return values.PublicToken{Owner: values.FactAuthorityRef(w.GetOwner()), Kind: literal, Value: values.EncodePublicBytes(w.Value)}, nil
}
func FederationRequest(w *pb.FederationReadRequest) (federationcontract.FederationReadRequest, error) {
	if w == nil || !values.ElementID(w.GetBoundary()).Valid() || len(w.Purpose) != 32 || [32]byte(w.Purpose) == ([32]byte{}) || (w.Mode != nil && w.GetMode() != pb.ReadMode_CURRENT_VERIFIED) {
		return federationcontract.FederationReadRequest{}, corecontract.ErrInvalid
	}
	r := federationcontract.FederationReadRequest{Boundary: values.ElementID(w.GetBoundary()), PrepareObserve: w.GetPrepareObserve(), Purpose: [32]byte(w.Purpose), Mode: corecontract.QueryReadMode(w.GetMode())}
	switch w.GetIntent() {
	case pb.Intent_READ:
		r.Intent = "read"
	case pb.Intent_LIST:
		r.Intent = "list"
	case pb.Intent_QUERY:
		r.Intent = "query"
	default:
		return r, corecontract.ErrInvalid
	}
	if w.Incoming != nil {
		var err error
		r.Incoming, err = MountKey(w.Incoming)
		if err != nil || r.Incoming.MountAnchorUUID != r.Boundary {
			return r, corecontract.ErrInvalid
		}
	}
	return r, nil
}
func FederationRequestWire(r federationcontract.FederationReadRequest) *pb.FederationReadRequest {
	intent := map[string]pb.Intent{"read": pb.Intent_READ, "list": pb.Intent_LIST, "query": pb.Intent_QUERY}[r.Intent]
	w := &pb.FederationReadRequest{Boundary: proto.String(string(r.Boundary)), Intent: intent.Enum(), PrepareObserve: proto.Bool(r.PrepareObserve), Purpose: append([]byte(nil), r.Purpose[:]...)}
	if r.Incoming.Valid() {
		w.Incoming = MountKeyWire(r.Incoming)
	}
	if r.Mode != 0 {
		w.Mode = pb.ReadMode(r.Mode).Enum()
	}
	return w
}
func FederationRelationWire(r federationcontract.FederationMountRelation) *pb.FederationMountRelation {
	return &pb.FederationMountRelation{Incoming: proto.Bool(r.Incoming), State: proto.Uint32(uint32(r.State)), Key: MountKeyWire(r.Key), Admission: MountAdmissionWire(r.Admission), Revision: ControlToken(r.Revision, netpb.TokenKind_MOUNT_REVISION), Contact: ControlToken(r.Contact, netpb.TokenKind_CONTACT_REVISION), NextHop: proto.String(string(r.NextHop)), Range: &pb.Range{Kind: pb.RangeKind_BOUNDED_SPHERE.Enum(), Radius: proto.Float64(r.Range.Radius())}, WriteGateOpen: proto.Bool(r.WriteGateOpen)}
}
func FederationRelation(w *pb.FederationMountRelation) (federationcontract.FederationMountRelation, error) {
	r := federationcontract.FederationMountRelation{}
	if w == nil || w.GetState() < uint32(corecontract.MountEstablishing) || w.GetState() > uint32(corecontract.MountDetached) || !values.NextHop(w.GetNextHop()).Valid() || w.Range == nil || w.Range.GetKind() != pb.RangeKind_BOUNDED_SPHERE || w.Range.Radius == nil {
		return r, corecontract.ErrInvalid
	}
	var err error
	r.Incoming, r.State, r.NextHop, r.WriteGateOpen = w.GetIncoming(), corecontract.MountState(w.GetState()), values.NextHop(w.GetNextHop()), w.GetWriteGateOpen()
	if r.Key, err = MountKey(w.Key); err != nil {
		return r, err
	}
	if r.Admission, err = MountAdmission(w.Admission); err != nil || r.Admission.MountKey != r.Key {
		return r, corecontract.ErrInvalid
	}
	if r.Revision, err = FederationToken(w.Revision, netpb.TokenKind_MOUNT_REVISION, "mount-revision"); err != nil {
		return r, err
	}
	if r.Contact, err = FederationToken(w.Contact, netpb.TokenKind_CONTACT_REVISION, "contact-revision"); err != nil {
		return r, err
	}
	r.Range, err = corecontract.NewBoundedSphere(w.Range.GetRadius())
	return r, err
}
func FederationPartWire(r federationcontract.FederationReadPart) *pb.FederationReadPart {
	w := &pb.FederationReadPart{Basis: ControlToken(r.Basis, netpb.TokenKind_READ_RECEIPT), Boundary: proto.String(string(r.Boundary)), Purpose: append([]byte(nil), r.Purpose[:]...), ReadMode: pb.ReadMode(r.ReadMode).Enum()}
	w.Subject = MountPrincipalWire(r.Subject)
	w.Hidden = proto.Bool(r.Hidden)
	w.CutStamp = append([]byte(nil), r.CutStamp[:]...)
	w.AuthorizationStamp = append([]byte(nil), r.AuthorizationStamp[:]...)
	if r.Incoming != nil {
		w.Incoming = FederationRelationWire(*r.Incoming)
	}
	if r.Preparation != nil {
		w.Preparation = ControlToken(*r.Preparation, netpb.TokenKind_OBSERVATION_PREPARATION)
	}
	return w
}
func FederationPart(w *pb.FederationReadPart) (federationcontract.FederationReadPart, error) {
	r := federationcontract.FederationReadPart{}
	if w == nil || w.Hidden == nil || w.GetHidden() && w.Incoming == nil {
		return r, corecontract.ErrInvalid
	}
	r.Hidden = w.GetHidden()
	if w == nil || !values.ElementID(w.GetBoundary()).Valid() || len(w.Purpose) != 32 || [32]byte(w.Purpose) == ([32]byte{}) || (w.GetReadMode() != pb.ReadMode_CURRENT_VERIFIED && w.GetReadMode() != pb.ReadMode_RETAINED) {
		return r, corecontract.ErrInvalid
	}
	var err error
	if len(w.CutStamp) != 32 || [32]byte(w.CutStamp) == ([32]byte{}) || len(w.AuthorizationStamp) != 32 || [32]byte(w.AuthorizationStamp) == ([32]byte{}) {
		return r, corecontract.ErrInvalid
	}
	if r.Subject, err = MountPrincipal(w.Subject); err != nil {
		return r, err
	}
	r.CutStamp, r.AuthorizationStamp = [32]byte(w.CutStamp), [32]byte(w.AuthorizationStamp)
	r.Boundary, r.Purpose, r.ReadMode = values.ElementID(w.GetBoundary()), [32]byte(w.Purpose), corecontract.QueryReadMode(w.GetReadMode())
	if r.Basis, err = FederationToken(w.Basis, netpb.TokenKind_READ_RECEIPT, "read-receipt"); err != nil {
		return r, err
	}
	if w.Incoming != nil {
		rel, e := FederationRelation(w.Incoming)
		if e != nil {
			return r, e
		}
		r.Incoming = &rel
	}
	if w.Preparation != nil {
		t, e := FederationToken(w.Preparation, netpb.TokenKind_OBSERVATION_PREPARATION, "observation-preparation")
		if e != nil {
			return r, e
		}
		r.Preparation = &t
	}
	return r, nil
}
