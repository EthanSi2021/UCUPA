package wire

import (
	"encoding/binary"

	"google.golang.org/protobuf/proto"

	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
)

func MountRevision(w *pb.ServiceRevision, element string) (corecontract.ServiceRevision, error) {
	if w == nil || len(w.Incarnation) != 32 || w.GetSequence() == 0 || !values.ElementID(element).Valid() {
		return corecontract.ServiceRevision{}, corecontract.ErrInvalid
	}
	e := corecontract.ServiceRevision{Incarnation: [32]byte(w.Incarnation), Element: values.ElementID(element), Sequence: w.GetSequence()}
	if e.Incarnation == ([32]byte{}) {
		return corecontract.ServiceRevision{}, corecontract.ErrInvalid
	}
	return e, nil
}

func MountRevisionWire(e corecontract.ServiceRevision) *pb.ServiceRevision {
	if e == (corecontract.ServiceRevision{}) {
		return nil
	}
	return &pb.ServiceRevision{Incarnation: append([]byte(nil), e.Incarnation[:]...), Sequence: proto.Uint64(e.Sequence)}
}

func MountCutWire(c corecontract.CoreReadCut) *pb.ServiceCoreCut {
	if c == (corecontract.CoreReadCut{}) {
		return nil
	}
	b := c.Evidence().Bytes()
	return &pb.ServiceCoreCut{Incarnation: append([]byte(nil), b[:32]...), Sequence: proto.Uint64(binary.BigEndian.Uint64(b[32:]))}
}

func MountCut(w *pb.ServiceCoreCut) (corecontract.CoreReadCut, error) {
	if w == nil {
		return corecontract.CoreReadCut{}, nil
	}
	if len(w.Incarnation) != 32 || w.GetSequence() == 0 {
		return corecontract.CoreReadCut{}, corecontract.ErrInvalid
	}
	b := make([]byte, 40)
	copy(b, w.Incarnation)
	binary.BigEndian.PutUint64(b[32:], w.GetSequence())
	e, err := values.NewCoreCutEvidence(b)
	if err != nil {
		return corecontract.CoreReadCut{}, corecontract.ErrInvalid
	}
	return corecontract.CoreReadCutFromEvidence(e)
}

func MountKey(w *pb.ServiceMountKey) (corecontract.MountKey, error) {
	if w == nil {
		return corecontract.MountKey{}, corecontract.ErrInvalid
	}
	e, err := MountRevision(w.Proposal, w.GetElement())
	if err != nil {
		return corecontract.MountKey{}, err
	}
	r, err := e.MountRevision()
	k := corecontract.MountKey{ParentAuthority: values.FactAuthorityRef(w.GetParentCore()), CarrierAuthority: values.FactAuthorityRef(w.GetCarrierCore()), MountAnchorUUID: values.ElementID(w.GetElement()), ProposalRevision: r}
	if err != nil || !k.Valid() {
		return corecontract.MountKey{}, corecontract.ErrInvalid
	}
	return k, nil
}

func MountKeyWire(k corecontract.MountKey) *pb.ServiceMountKey {
	return &pb.ServiceMountKey{ParentCore: proto.String(string(k.ParentAuthority)), CarrierCore: proto.String(string(k.CarrierAuthority)), Element: proto.String(string(k.MountAnchorUUID)), Proposal: MountRevisionWire(k.ProposalRevision.ServiceEvidence())}
}

func MountAdmission(w *pb.ServiceMountAdmission) (corecontract.MountAdmissionKey, error) {
	if w == nil {
		return corecontract.MountAdmissionKey{}, nil
	}
	k, err := MountKey(w.Key)
	if err != nil || len(w.OperationId) == 0 || len(w.OperationId) > 128 {
		return corecontract.MountAdmissionKey{}, corecontract.ErrInvalid
	}
	return corecontract.MountAdmissionKey{MountKey: k, OperationID: corecontract.OperationID(w.OperationId)}, nil
}

func MountAdmissionWire(a corecontract.MountAdmissionKey) *pb.ServiceMountAdmission {
	if a == (corecontract.MountAdmissionKey{}) {
		return nil
	}
	return &pb.ServiceMountAdmission{Key: MountKeyWire(a.MountKey), OperationId: []byte(a.OperationID)}
}

func MountTuple(w *pb.SpatialTuple) (corecontract.SpatialTuple, error) {
	if w == nil || !values.ElementID(w.GetParent()).Valid() || w.Pose == nil || w.Pose.Translation == nil || w.Pose.Rotation == nil || w.Pose.Rotation.Ratios == nil || w.Range == nil || w.Range.GetKind() != pb.RangeKind_BOUNDED_SPHERE || w.Range.Radius == nil {
		return corecontract.SpatialTuple{}, corecontract.ErrInvalid
	}
	r := w.Pose.Rotation
	if r.GetPivot() < 1 || r.GetPivot() > 4 || len(r.Ratios.Items) != 3 || len(w.Pose.Translation.Items) != 3 {
		return corecontract.SpatialTuple{}, corecontract.ErrInvalid
	}
	q := [4]float64{}
	a := r.Ratios.Items
	for i, j := 0, 0; i < 4; i++ {
		if i == int(r.GetPivot())-1 {
			q[i] = 1
		} else {
			q[i] = a[j]
			j++
		}
	}
	rotation, err := corecontract.NewRotation(q[0], q[1], q[2], q[3])
	if err != nil || uint32(rotation.Pivot()) != r.GetPivot() || rotation.Ratios() != ([3]float64{a[0], a[1], a[2]}) {
		return corecontract.SpatialTuple{}, corecontract.ErrInvalid
	}
	t := w.Pose.Translation
	pose, err := corecontract.NewPose(corecontract.Vec3{X: t.Items[0], Y: t.Items[1], Z: t.Items[2]}, rotation)
	if err != nil {
		return corecontract.SpatialTuple{}, err
	}
	sphere, err := corecontract.NewBoundedSphere(w.Range.GetRadius())
	if err != nil {
		return corecontract.SpatialTuple{}, err
	}
	return corecontract.SpatialTuple{Parent: values.ElementID(w.GetParent()), LocalPose: pose, Range: sphere}, nil
}

func MountTupleWire(t corecontract.SpatialTuple) *pb.SpatialTuple {
	if t == (corecontract.SpatialTuple{}) {
		return nil
	}
	r, p := t.LocalPose.Rotation.Ratios(), t.LocalPose.Translation
	return &pb.SpatialTuple{Parent: proto.String(string(t.Parent)), Pose: &pb.Pose{Translation: &networkpb.Vector3{Items: []float64{p.X, p.Y, p.Z}}, Rotation: &pb.Rotation{Pivot: proto.Uint32(uint32(t.LocalPose.Rotation.Pivot())), Ratios: &networkpb.Vector3{Items: []float64{r[0], r[1], r[2]}}}}, Range: &pb.Range{Kind: pb.RangeKind_BOUNDED_SPHERE.Enum(), Radius: proto.Float64(t.Range.Radius())}}
}

func MountCommand(w *pb.ServiceMountCommand) (corecontract.MountCommand, error) {
	if w == nil || !values.ElementID(w.GetElement()).Valid() {
		return corecontract.MountCommand{}, corecontract.ErrInvalid
	}
	c := corecontract.MountCommand{Kind: corecontract.MountCommandKind(w.GetKind()), MountAnchorUUID: values.ElementID(w.GetElement()), CarrierAuthority: values.FactAuthorityRef(w.GetCarrierCore()), CarrierNextHop: values.NextHop(w.GetCarrierNextHop()), ParentNextHop: values.NextHop(w.GetParentNextHop())}
	if w.ExpectedElement != nil {
		e, err := MountRevision(w.ExpectedElement, w.GetElement())
		if err != nil {
			return corecontract.MountCommand{}, err
		}
		c.ExpectedElementRevision, err = e.ElementRevision()
		if err != nil {
			return corecontract.MountCommand{}, err
		}
	}
	if w.ExpectedMount != nil {
		e, err := MountRevision(w.ExpectedMount, w.GetElement())
		if err != nil {
			return corecontract.MountCommand{}, err
		}
		c.ExpectedMountRevision, err = e.MountRevision()
		if err != nil {
			return corecontract.MountCommand{}, err
		}
	}
	if w.Final != nil {
		var err error
		c.Final, err = MountTuple(w.Final)
		if err != nil {
			return corecontract.MountCommand{}, err
		}
	}
	// Tagged shape is enforced before the owner sees the intent. Optional
	// fields may not hide an irrelevant explicitly-present empty value.
	switch c.Kind {
	case corecontract.MountEstablish:
		if w.ExpectedElement == nil || w.ExpectedMount != nil || w.Final != nil || !c.CarrierAuthority.Valid() || !c.CarrierNextHop.Valid() || !c.ParentNextHop.Valid() {
			return corecontract.MountCommand{}, corecontract.ErrInvalid
		}
	case corecontract.MountUpdate:
		if w.ExpectedElement == nil || w.ExpectedMount == nil || w.Final == nil || w.CarrierCore != nil || w.CarrierNextHop != nil || w.ParentNextHop != nil {
			return corecontract.MountCommand{}, corecontract.ErrInvalid
		}
	case corecontract.MountRevoke:
		if w.ExpectedElement != nil || w.ExpectedMount == nil || w.Final != nil || w.CarrierCore != nil || w.CarrierNextHop != nil || w.ParentNextHop != nil {
			return corecontract.MountCommand{}, corecontract.ErrInvalid
		}
	default:
		return corecontract.MountCommand{}, corecontract.ErrInvalid
	}
	return c, nil
}

func MountCommandWire(c corecontract.MountCommand) *pb.ServiceMountCommand {
	w := &pb.ServiceMountCommand{Kind: pb.MountCommandKind(c.Kind).Enum(), Element: proto.String(string(c.MountAnchorUUID)), ExpectedElement: MountRevisionWire(c.ExpectedElementRevision.ServiceEvidence()), ExpectedMount: MountRevisionWire(c.ExpectedMountRevision.ServiceEvidence()), Final: MountTupleWire(c.Final)}
	if c.Kind == corecontract.MountEstablish {
		w.CarrierCore = proto.String(string(c.CarrierAuthority))
		w.CarrierNextHop = proto.String(string(c.CarrierNextHop))
		w.ParentNextHop = proto.String(string(c.ParentNextHop))
	}
	return w
}

func MountPrincipal(w *pb.ServicePrincipal) (values.PrincipalRef, error) {
	if w == nil {
		return values.PrincipalRef{}, corecontract.ErrInvalid
	}
	p := values.PrincipalRef{Realm: values.RealmID(w.GetRealm()), Principal: values.PrincipalID(w.GetPrincipal())}
	if !p.Valid() {
		return values.PrincipalRef{}, corecontract.ErrInvalid
	}
	return p, nil
}

func MountPrincipalWire(p values.PrincipalRef) *pb.ServicePrincipal {
	if p == (values.PrincipalRef{}) {
		return nil
	}
	return &pb.ServicePrincipal{Realm: proto.String(string(p.Realm)), Principal: proto.String(string(p.Principal))}
}

func MountRequest(w *pb.ServiceMountPrepare) (corecontract.MountPrepareRequest, error) {
	if w == nil || len(w.OperationId) == 0 || len(w.OperationId) > 128 {
		return corecontract.MountPrepareRequest{}, corecontract.ErrInvalid
	}
	r := corecontract.MountPrepareRequest{OperationID: corecontract.OperationID(w.OperationId)}
	var err error
	if r.Key, err = MountKey(w.Key); err != nil {
		return r, err
	}
	if r.Command, err = MountCommand(w.Command); err != nil {
		return r, err
	}
	if r.ParentActor, err = MountPrincipal(w.ParentActor); err != nil {
		return r, err
	}
	if w.CarrierActor != nil || r.Command.Kind != corecontract.MountRevoke {
		if r.CarrierActor, err = MountPrincipal(w.CarrierActor); err != nil {
			return r, err
		}
	}
	if r.PreviousAdmission, err = MountAdmission(w.Previous); err != nil {
		return r, err
	}
	e, err := MountRevision(w.Reserved, w.Key.GetElement())
	if err != nil {
		return r, err
	}
	if r.ReservedRevision, err = e.MountRevision(); err != nil {
		return r, err
	}
	if r.Tuple, err = MountTuple(w.Tuple); err != nil {
		return r, err
	}
	return r, nil
}

func MountRequestWire(r corecontract.MountPrepareRequest) *pb.ServiceMountPrepare {
	return &pb.ServiceMountPrepare{Key: MountKeyWire(r.Key), OperationId: []byte(r.OperationID), Command: MountCommandWire(r.Command), ParentActor: MountPrincipalWire(r.ParentActor), CarrierActor: MountPrincipalWire(r.CarrierActor), Previous: MountAdmissionWire(r.PreviousAdmission), Reserved: MountRevisionWire(r.ReservedRevision.ServiceEvidence()), Tuple: MountTupleWire(r.Tuple)}
}

func MountDecision(w *pb.ServiceMountDecision) (corecontract.MountDecisionEvidence, error) {
	if w == nil || w.GetDecision() < pb.MountDecisionKind_MOUNT_PENDING || w.GetDecision() > pb.MountDecisionKind_MOUNT_ABORTED || w.GetRejection() < pb.MountRejection_MOUNT_NO_REJECTION || w.GetRejection() > pb.MountRejection_MOUNT_REVISION_CONFLICT {
		return corecontract.MountDecisionEvidence{}, corecontract.ErrInvalid
	}
	r, err := MountRequest(w.Request)
	if err != nil {
		return corecontract.MountDecisionEvidence{}, err
	}
	c, err := MountCut(w.Cut)
	if err != nil {
		return corecontract.MountDecisionEvidence{}, err
	}
	a, err := MountAdmission(w.Admission)
	if err != nil {
		return corecontract.MountDecisionEvidence{}, err
	}
	return corecontract.MountDecisionEvidence{Request: r, Decision: corecontract.MountDecision(w.GetDecision()), DecisionCut: c, AbortCause: corecontract.MountAbortCause(w.GetRejection() - 1), AdmissionKey: a}, nil
}

func MountDecisionWire(e corecontract.MountDecisionEvidence) *pb.ServiceMountDecision {
	return &pb.ServiceMountDecision{Request: MountRequestWire(e.Request), Decision: pb.MountDecisionKind(e.Decision).Enum(), Cut: MountCutWire(e.DecisionCut), Rejection: pb.MountRejection(e.AbortCause + 1).Enum(), Admission: MountAdmissionWire(e.AdmissionKey)}
}
