package coresdk

import (
	"bytes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"math"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
)

func badFederation() error { return status.Error(codes.DataLoss, "invalid federation evidence") }
func validateFederationRelation(r *pb.FederationMountRelation, owner values.FactAuthorityRef) error {
	if r == nil || r.Key == nil || r.Admission == nil || !proto.Equal(r.Key, r.Admission.Key) || len(r.Admission.OperationId) == 0 || len(r.Admission.OperationId) > 128 || r.GetState() < 1 || r.GetState() > 4 || r.Range == nil || r.Range.GetKind() != pb.RangeKind_BOUNDED_SPHERE || r.Range.Radius == nil || r.Range.GetRadius() < 0 || !values.NextHop(r.GetNextHop()).Valid() {
		return badFederation()
	}
	if (r.GetIncoming() && r.Key.GetCarrierCore() != string(owner)) || (!r.GetIncoming() && r.Key.GetParentCore() != string(owner)) || r.Key.GetParentCore() == r.Key.GetCarrierCore() {
		return badFederation()
	}
	return nil
}
func validateFederationPart(p *pb.FederationReadPart, owner values.FactAuthorityRef) error {
	if p == nil || p.Hidden == nil || p.GetHidden() && p.Incoming == nil {
		return badFederation()
	}
	if p == nil || !values.ElementID(p.GetBoundary()).Valid() || len(p.Purpose) != 32 || [32]byte(p.Purpose) == ([32]byte{}) || (p.GetReadMode() != pb.ReadMode_CURRENT_VERIFIED && p.GetReadMode() != pb.ReadMode_RETAINED) {
		return badFederation()
	}
	if p.Subject == nil || !(values.PrincipalRef{Realm: values.RealmID(p.Subject.GetRealm()), Principal: values.PrincipalID(p.Subject.GetPrincipal())}).Valid() || len(p.CutStamp) != 32 || [32]byte(p.CutStamp) == ([32]byte{}) || len(p.AuthorizationStamp) != 32 || [32]byte(p.AuthorizationStamp) == ([32]byte{}) {
		return badFederation()
	}
	if p.Incoming != nil {
		if err := validateFederationRelation(p.Incoming, owner); err != nil {
			return err
		}
		if !p.Incoming.GetIncoming() {
			return badFederation()
		}
	}
	return nil
}
func validateFederationPage(p *pb.FederationProjectionPage, part *pb.FederationReadPart, owner values.FactAuthorityRef) error {
	if part.GetHidden() {
		return badFederation()
	}
	if p.GetGap() > 2 || p.GetComplete() && (p.Continuation != nil || p.GetGap() != 0) || !p.GetComplete() && p.Continuation == nil && p.GetGap() == 0 || len(p.Purpose) != 32 || !bytes.Equal(p.Purpose, part.Purpose) || len(p.Frontiers) > 256 {
		return badFederation()
	}
	seen := map[string]bool{}
	for _, r := range p.Records {
		if r == nil || seen[r.GetElement()] || !values.ElementID(r.GetElement()).Valid() || r.Path == nil || r.Range == nil {
			return badFederation()
		}
		seen[r.GetElement()] = true
		if r.Range.GetKind() == pb.RangeKind_UNBOUNDED_ROOT {
			if r.Pose != nil || r.Parent != nil || r.Range.Radius != nil || r.GetKind() != pb.ElementKind_ANCHOR || r.GetElement() != part.GetBoundary() {
				return badFederation()
			}
		} else if r.Range.GetKind() == pb.RangeKind_BOUNDED_SPHERE {
			if r.Range.Radius == nil || r.Range.GetRadius() < 0 || r.Pose == nil || !values.ElementID(r.GetParent()).Valid() || !validFederationPose(r.Pose) {
				return badFederation()
			}
		} else {
			return badFederation()
		}
		path := r.Path.Items
		if r.GetElement() == part.GetBoundary() {
			if len(path) != 0 {
				return badFederation()
			}
		} else {
			if len(path) == 0 || path[len(path)-1] != r.GetElement() {
				return badFederation()
			}
			if len(path) == 1 {
				if r.GetParent() != part.GetBoundary() {
					return badFederation()
				}
			} else if r.GetParent() != path[len(path)-2] {
				return badFederation()
			}
		}
		if r.Mount != nil {
			if err := validateFederationRelation(r.Mount, owner); err != nil {
				return err
			}
			if r.Mount.GetIncoming() || r.Mount.Key.GetElement() != r.GetElement() || r.GetKind() != pb.ElementKind_ANCHOR {
				return badFederation()
			}
		}
		if a := r.Attachment; a != nil {
			if a.GetElement() != r.GetElement() || a.PendingProposal != nil || a.PendingTarget != nil {
				return badFederation()
			}
			if err := validateTokenOwners(a.ProtoReflect(), owner); err != nil {
				return err
			}
		}
	}
	return nil
}
func validFederationPose(p *pb.Pose) bool {
	if p.Translation == nil || len(p.Translation.Items) != 3 || p.Rotation == nil || p.Rotation.Ratios == nil || len(p.Rotation.Ratios.Items) != 3 || p.Rotation.GetPivot() < 1 || p.Rotation.GetPivot() > 4 {
		return false
	}
	for _, n := range p.Translation.Items {
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return false
		}
	}
	for _, n := range p.Rotation.Ratios.Items {
		if math.IsNaN(n) || math.IsInf(n, 0) || math.Abs(n) > 1 {
			return false
		}
	}
	return true
}
