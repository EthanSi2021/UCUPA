package wire

import (
	federationcontract "ucupa/ucupa-core/contract/federation"

	"google.golang.org/protobuf/proto"

	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"

	pb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
)

func FederationPoseWire(p corecontract.Pose) *pb.Pose {
	r := p.Rotation.Ratios()
	return &pb.Pose{Translation: &netpb.Vector3{Items: []float64{p.Translation.X, p.Translation.Y, p.Translation.Z}}, Rotation: &pb.Rotation{Pivot: proto.Uint32(uint32(p.Rotation.Pivot())), Ratios: &netpb.Vector3{Items: []float64{r[0], r[1], r[2]}}}}
}

func FederationPose(w *pb.Pose) (corecontract.Pose, error) {
	if w == nil || w.Translation == nil || len(w.Translation.Items) != 3 || w.Rotation == nil || w.Rotation.Ratios == nil || len(w.Rotation.Ratios.Items) != 3 || w.Rotation.GetPivot() < 1 || w.Rotation.GetPivot() > 4 {
		return corecontract.Pose{}, corecontract.ErrInvalid
	}
	var q [4]float64
	pivot, next := int(w.Rotation.GetPivot())-1, 0
	for i := range q {
		if i == pivot {
			q[i] = 1
		} else {
			q[i] = w.Rotation.Ratios.Items[next]
			next++
		}
	}
	r, err := corecontract.NewRotation(q[0], q[1], q[2], q[3])
	if err != nil || uint32(r.Pivot()) != w.Rotation.GetPivot() || r.Ratios() != [3]float64(w.Rotation.Ratios.Items) {
		return corecontract.Pose{}, corecontract.ErrInvalid
	}
	t := w.Translation.Items
	return corecontract.NewPose(corecontract.Vec3{X: t[0], Y: t[1], Z: t[2]}, r)
}

func FederationPathWire(path []values.ElementID) *netpb.ElementPath {
	w := &netpb.ElementPath{}
	for _, id := range path {
		w.Items = append(w.Items, string(id))
	}
	return w
}

func FederationPathValues(w *netpb.ElementPath) ([]values.ElementID, error) {
	if w == nil || len(w.Items) > 4096 {
		return nil, corecontract.ErrInvalid
	}
	path := make([]values.ElementID, len(w.Items))
	for i, id := range w.Items {
		path[i] = values.ElementID(id)
		if !path[i].Valid() {
			return nil, corecontract.ErrInvalid
		}
	}
	return path, nil
}

func FederationStepsWire(steps []federationcontract.FederationPoseStep) []*pb.FederationPoseStep {
	w := make([]*pb.FederationPoseStep, len(steps))
	for i, s := range steps {
		w[i] = &pb.FederationPoseStep{Pose: FederationPoseWire(s.Pose), Inverse: proto.Bool(s.Inverse)}
	}
	return w
}

func FederationSteps(w []*pb.FederationPoseStep) ([]federationcontract.FederationPoseStep, error) {
	if len(w) > 1024 {
		return nil, corecontract.ErrInvalid
	}
	steps := make([]federationcontract.FederationPoseStep, len(w))
	for i, s := range w {
		if s == nil || s.Inverse == nil {
			return nil, corecontract.ErrInvalid
		}
		p, err := FederationPose(s.Pose)
		if err != nil {
			return nil, err
		}
		steps[i] = federationcontract.FederationPoseStep{Pose: p, Inverse: s.GetInverse()}
	}
	return steps, nil
}

func FederationProjectionRequestWire(r federationcontract.FederationProjectionRequest) *pb.FederationProjectionRequest {
	w := &pb.FederationProjectionRequest{Kind: proto.String(r.Kind)}
	if r.Element != "" {
		w.Element = proto.String(string(r.Element))
	}
	if p := r.Path; p != nil {
		w.Path = &pb.FederationPathRequest{Origin: proto.String(string(p.Origin)), Up: proto.Uint64(p.Up), Down: FederationPathWire(p.Down)}
	}
	if q := r.Query; q != nil {
		w.Query = &pb.FederationQueryRequest{Scope: proto.String(string(q.Scope)), Sphere: &pb.Sphere{Center: &netpb.Vector3{Items: []float64{q.Sphere.Center.X, q.Sphere.Center.Y, q.Sphere.Center.Z}}, Radius: proto.Float64(q.Sphere.Radius)}, Selector: pb.Selector(q.Selector).Enum(), ToBasis: FederationStepsWire(q.ToBasis)}
	}
	return w
}

func FederationProjectionRequest(w *pb.FederationProjectionRequest) (federationcontract.FederationProjectionRequest, error) {
	r := federationcontract.FederationProjectionRequest{Kind: w.GetKind(), Element: values.ElementID(w.GetElement())}
	if w == nil {
		return r, corecontract.ErrInvalid
	}
	switch r.Kind {
	case "path":
		p := w.Path
		if p == nil || p.Up == nil || !values.ElementID(p.GetOrigin()).Valid() || w.Element != nil || w.Query != nil {
			return r, corecontract.ErrInvalid
		}
		down, err := FederationPathValues(p.Down)
		if err != nil {
			return r, err
		}
		r.Path = &federationcontract.FederationPathRequest{Origin: values.ElementID(p.GetOrigin()), Up: p.GetUp(), Down: down}
	case "read", "read-at", "list":
		if !r.Element.Valid() || w.Path != nil || w.Query != nil {
			return r, corecontract.ErrInvalid
		}
	case "query":
		q := w.Query
		if q == nil || w.Path != nil || w.Element != nil || !values.ElementID(q.GetScope()).Valid() || q.Sphere == nil || q.Sphere.Center == nil || len(q.Sphere.Center.Items) != 3 || q.Sphere.Radius == nil || q.GetSelector() < pb.Selector_ANCHORS || q.GetSelector() > pb.Selector_BOTH {
			return r, corecontract.ErrInvalid
		}
		steps, err := FederationSteps(q.ToBasis)
		if err != nil {
			return r, err
		}
		c := q.Sphere.Center.Items
		r.Query = &federationcontract.FederationQueryRequest{Scope: values.ElementID(q.GetScope()), Sphere: corecontract.QuerySphere{Center: corecontract.Vec3{X: c[0], Y: c[1], Z: c[2]}, Radius: q.Sphere.GetRadius()}, Selector: corecontract.QuerySelector(q.GetSelector()), ToBasis: steps}
	default:
		return r, corecontract.ErrInvalid
	}
	return r, nil
}

func FederationFrontierWire(f federationcontract.FederationFrontier) *pb.FederationFrontier {
	return &pb.FederationFrontier{Relation: FederationRelationWire(f.Relation), Path: FederationPathWire(f.Path), ToScope: FederationStepsWire(f.ToScope)}
}

func FederationFrontier(w *pb.FederationFrontier) (federationcontract.FederationFrontier, error) {
	var f federationcontract.FederationFrontier
	if w == nil {
		return f, corecontract.ErrInvalid
	}
	var err error
	if f.Relation, err = FederationRelation(w.Relation); err != nil {
		return f, err
	}
	if f.Path, err = FederationPathValues(w.Path); err != nil {
		return f, err
	}
	if f.ToScope, err = FederationSteps(w.ToScope); err != nil {
		return f, err
	}
	if f.Relation.Incoming && (len(f.Path) != 0 || len(f.ToScope) != 0) {
		return f, corecontract.ErrInvalid
	}
	return f, nil
}

func FederationPathResultWire(p federationcontract.FederationPathResult) *pb.FederationPathResult {
	w := &pb.FederationPathResult{Element: proto.String(string(p.Element)), Kind: pb.ElementKind(p.Kind).Enum(), ActualUp: proto.Uint64(p.ActualUp), RemainingUp: proto.Uint64(p.RemainingUp), RemainingDown: FederationPathWire(p.RemainingDown)}
	for _, n := range p.Spine {
		s := &pb.FederationSpineNode{Element: proto.String(string(n.ID))}
		if n.Parent != "" {
			s.Parent = proto.String(string(n.Parent))
		}
		if n.Pose != nil {
			s.Pose = FederationPoseWire(*n.Pose)
		}
		w.Spine = append(w.Spine, s)
	}
	if p.Frontier != nil {
		w.Frontier = FederationFrontierWire(*p.Frontier)
	}
	return w
}

func FederationPathResult(w *pb.FederationPathResult) (federationcontract.FederationPathResult, error) {
	p := federationcontract.FederationPathResult{Element: values.ElementID(w.GetElement()), Kind: corecontract.ElementKind(w.GetKind()), ActualUp: w.GetActualUp(), RemainingUp: w.GetRemainingUp()}
	if w == nil || !p.Element.Valid() || (p.Kind != corecontract.ElementAnchor && p.Kind != corecontract.ElementEntity) || len(w.Spine) > 4097 {
		return p, corecontract.ErrInvalid
	}
	var err error
	if p.RemainingDown, err = FederationPathValues(w.RemainingDown); err != nil {
		return p, err
	}
	for _, n := range w.Spine {
		if n == nil || !values.ElementID(n.GetElement()).Valid() || (n.Parent != nil && !values.ElementID(n.GetParent()).Valid()) {
			return p, corecontract.ErrInvalid
		}
		s := federationcontract.FederationSpineNode{ID: values.ElementID(n.GetElement()), Parent: values.ElementID(n.GetParent())}
		if n.Pose != nil {
			pose, e := FederationPose(n.Pose)
			if e != nil {
				return p, e
			}
			s.Pose = &pose
		}
		p.Spine = append(p.Spine, s)
	}
	if w.Frontier != nil {
		f, e := FederationFrontier(w.Frontier)
		if e != nil {
			return p, e
		}
		p.Frontier = &f
	}
	return p, nil
}
