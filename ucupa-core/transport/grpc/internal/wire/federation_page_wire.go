package wire

import (
	federationcontract "ucupa/ucupa-core/contract/federation"

	"google.golang.org/protobuf/proto"

	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"

	pb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
)

func FederationRecordWire(r federationcontract.FederationRecord) *pb.FederationRecord {
	w := &pb.FederationRecord{Element: proto.String(string(r.ID)), Kind: pb.ElementKind(r.Kind).Enum(), Range: &pb.Range{Kind: pb.RangeKind(r.Range.Kind()).Enum()}, Revision: ControlToken(r.Revision, netpb.TokenKind_RECORD_REVISION), Path: &netpb.ElementPath{}}
	if r.Parent.Valid() {
		w.Parent = proto.String(string(r.Parent))
	}
	if s, bounded := r.Range.BoundedSphere(); bounded {
		w.Range.Radius = proto.Float64(s.Radius())
	}
	if r.Pose != nil {
		p := r.Pose
		r := p.Rotation.Ratios()
		w.Pose = &pb.Pose{Translation: &netpb.Vector3{Items: []float64{p.Translation.X, p.Translation.Y, p.Translation.Z}}, Rotation: &pb.Rotation{Pivot: proto.Uint32(uint32(p.Rotation.Pivot())), Ratios: &netpb.Vector3{Items: []float64{r[0], r[1], r[2]}}}}
	}
	for _, id := range r.Path {
		w.Path.Items = append(w.Path.Items, string(id))
	}
	if r.Attachment != nil {
		w.Attachment = PublicFacetInspectionWire(*r.Attachment)
	}
	if r.Mount != nil {
		w.Mount = FederationRelationWire(*r.Mount)
	}
	return w
}
func FederationPageWire(r federationcontract.FederationProjectionPage) *pb.FederationProjectionPage {
	w := &pb.FederationProjectionPage{Basis: ControlToken(r.Basis, netpb.TokenKind_READ_RECEIPT), Purpose: append([]byte(nil), r.Purpose[:]...), Complete: proto.Bool(r.Complete), Gap: proto.Uint32(uint32(r.Gap))}
	if r.Coverage != ([32]byte{}) {
		w.Coverage = append([]byte(nil), r.Coverage[:]...)
	}
	for _, p := range r.Parents {
		w.Parents = append(w.Parents, &pb.ParentAdjacencyEvidence{Parent: proto.String(string(p.Parent)), Adjacency: append([]byte(nil), p.Adjacency[:]...)})
	}
	for _, record := range r.Records {
		w.Records = append(w.Records, FederationRecordWire(record))
	}
	for _, m := range r.Matches {
		w.Matches = append(w.Matches, &pb.FederationMatch{Element: proto.String(string(m.ID)), Kind: pb.ElementKind(m.Kind).Enum(), Revision: ControlToken(m.Revision, netpb.TokenKind_RECORD_REVISION), Path: FederationPathWire(m.Path)})
	}
	for _, f := range r.Frontiers {
		w.Frontiers = append(w.Frontiers, FederationFrontierWire(f))
	}
	if r.Path != nil {
		w.Path = FederationPathResultWire(*r.Path)
	}
	if r.Continuation != nil {
		w.Continuation = ControlToken(*r.Continuation, netpb.TokenKind_QUERY_CONTINUATION)
	}
	return w
}
func FederationRecord(w *pb.FederationRecord) (federationcontract.FederationRecord, error) {
	r := federationcontract.FederationRecord{}
	if w == nil || !values.ElementID(w.GetElement()).Valid() || (w.GetKind() != pb.ElementKind_ANCHOR && w.GetKind() != pb.ElementKind_ENTITY) || w.Range == nil || w.Path == nil {
		return r, corecontract.ErrInvalid
	}
	r.ID, r.Kind, r.Parent = values.ElementID(w.GetElement()), corecontract.ElementKind(w.GetKind()), values.ElementID(w.GetParent())
	var err error
	if r.Revision, err = FederationToken(w.Revision, netpb.TokenKind_RECORD_REVISION, "record-revision"); err != nil {
		return r, err
	}
	sphere := corecontract.BoundedSphere{}
	switch w.Range.GetKind() {
	case pb.RangeKind_BOUNDED_SPHERE:
		if w.Range.Radius == nil {
			return r, corecontract.ErrInvalid
		}
		sphere, err = corecontract.NewBoundedSphere(w.Range.GetRadius())
		if err != nil {
			return r, err
		}
	case pb.RangeKind_UNBOUNDED_ROOT:
		if w.Range.Radius != nil {
			return r, corecontract.ErrInvalid
		}
	default:
		return r, corecontract.ErrInvalid
	}
	if r.Range, err = federationcontract.NewFederationRange(corecontract.RangeKind(w.Range.GetKind()), sphere); err != nil {
		return r, err
	}
	if w.Pose != nil {
		t, e := MountTuple(&pb.SpatialTuple{Parent: proto.String(string(r.Parent)), Pose: w.Pose, Range: w.Range})
		if e != nil {
			return r, e
		}
		r.Pose = &t.LocalPose
	} else if r.Parent != "" || w.Range.GetKind() != pb.RangeKind_UNBOUNDED_ROOT || r.Kind != corecontract.ElementAnchor {
		return r, corecontract.ErrInvalid
	}
	for _, id := range w.Path.Items {
		if !values.ElementID(id).Valid() {
			return r, corecontract.ErrInvalid
		}
		r.Path = append(r.Path, values.ElementID(id))
	}
	if w.Mount != nil {
		m, e := FederationRelation(w.Mount)
		if e != nil || m.Incoming || m.Key.MountAnchorUUID != r.ID {
			return r, corecontract.ErrInvalid
		}
		r.Mount = &m
	}
	if w.Attachment != nil {
		a, e := FederationAttachment(w.Attachment)
		if e != nil || a.Element != r.ID {
			return r, corecontract.ErrInvalid
		}
		r.Attachment = &a
	}
	return r, nil
}
func FederationAttachment(w *pb.PublicFacetInspection) (corecontract.PublicFacetInspection, error) {
	a := corecontract.PublicFacetInspection{Element: values.ElementID(w.GetElement()), State: corecontract.FacetAttachmentState(w.GetState()), Target: values.FactAuthorityRef(w.GetTarget()), KeyCore: values.FactAuthorityRef(w.GetKeyCore()), NextHop: values.NextHop(w.GetNextHop())}
	if w.PendingProposal != nil || w.PendingTarget != nil {
		return a, corecontract.ErrInvalid
	}
	var err error
	if a.Revision, err = FederationToken(w.Revision, netpb.TokenKind_ATTACHMENT_REVISION, "attachment-revision"); err != nil {
		return a, err
	}
	if w.Proposal != nil {
		if a.Proposal, err = FederationToken(w.Proposal, netpb.TokenKind_ATTACHMENT_REVISION, "attachment-revision"); err != nil {
			return a, err
		}
	}
	if w.ContactRevision != nil {
		if a.ContactRevision, err = FederationToken(w.ContactRevision, netpb.TokenKind_CONTACT_REVISION, "contact-revision"); err != nil {
			return a, err
		}
	}
	return a, nil
}
func FederationPage(w *pb.FederationProjectionPage) (federationcontract.FederationProjectionPage, error) {
	r := federationcontract.FederationProjectionPage{}
	if w == nil || len(w.Purpose) != 32 || [32]byte(w.Purpose) == ([32]byte{}) || w.GetGap() > 2 || w.GetComplete() && (w.Continuation != nil || w.GetGap() != 0) || !w.GetComplete() && w.Continuation == nil && w.GetGap() == 0 || len(w.Records) > 256 || len(w.Matches) > 256 || len(w.Frontiers) > 256 {
		return r, corecontract.ErrInvalid
	}
	r.Purpose, r.Complete = [32]byte(w.Purpose), w.GetComplete()
	if len(w.Parents) > 4096 {
		return r, corecontract.ErrInvalid
	}
	for i, p := range w.Parents {
		if p == nil || !values.ElementID(p.GetParent()).Valid() || len(p.Adjacency) != 32 || [32]byte(p.Adjacency) == ([32]byte{}) || i > 0 && w.Parents[i-1].GetParent() >= p.GetParent() {
			return r, corecontract.ErrInvalid
		}
		r.Parents = append(r.Parents, federationcontract.FederationParentEvidence{Parent: values.ElementID(p.GetParent()), Adjacency: [32]byte(p.Adjacency)})
	}
	if w.Coverage != nil {
		if len(w.Coverage) != 32 || [32]byte(w.Coverage) == ([32]byte{}) {
			return r, corecontract.ErrInvalid
		}
		r.Coverage = [32]byte(w.Coverage)
	}
	r.Gap = corecontract.QueryGap(w.GetGap())
	var err error
	if r.Basis, err = FederationToken(w.Basis, netpb.TokenKind_READ_RECEIPT, "read-receipt"); err != nil {
		return r, err
	}
	if w.Continuation != nil {
		t, e := FederationToken(w.Continuation, netpb.TokenKind_QUERY_CONTINUATION, "query-continuation")
		if e != nil {
			return r, e
		}
		r.Continuation = &t
	}
	for _, record := range w.Records {
		x, e := FederationRecord(record)
		if e != nil {
			return r, e
		}
		r.Records = append(r.Records, x)
	}
	for _, m := range w.Matches {
		if m == nil || !values.ElementID(m.GetElement()).Valid() || m.GetKind() < pb.ElementKind_ANCHOR || m.GetKind() > pb.ElementKind_ENTITY {
			return r, corecontract.ErrInvalid
		}
		path, e := FederationPathValues(m.Path)
		if e != nil {
			return r, e
		}
		revision, e := FederationToken(m.Revision, netpb.TokenKind_RECORD_REVISION, "record-revision")
		if e != nil {
			return r, e
		}
		r.Matches = append(r.Matches, federationcontract.FederationMatch{ID: values.ElementID(m.GetElement()), Kind: corecontract.ElementKind(m.GetKind()), Revision: revision, Path: path})
	}
	for _, w := range w.Frontiers {
		f, e := FederationFrontier(w)
		if e != nil {
			return r, e
		}
		r.Frontiers = append(r.Frontiers, f)
	}
	if w.Path != nil {
		p, e := FederationPathResult(w.Path)
		if e != nil {
			return r, e
		}
		r.Path = &p
	}
	return r, nil
}
