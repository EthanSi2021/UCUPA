package coresdk

import (
	"bytes"
	"google.golang.org/protobuf/proto"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
)

func publicEvidenceStamp(v []byte) bool { return len(v) == 32 && !bytes.Equal(v, make([]byte, 32)) }

// A public source vector must form a rooted, non-repeating disclosed tree.
// It cannot claim hidden source placeholders or carry service grants/addresses.
// The authenticated entry attests the stamps; the SDK validates structure and
// provenance, not a fictitious offline verification of the underlying facts.
func validatePublicSourceGraph(g *pb.PublicSourceGraph, closure []byte, entry values.FactAuthorityRef) (map[values.FactAuthorityRef]bool, error) {
	owners := map[values.FactAuthorityRef]bool{}
	if g == nil {
		if closure != nil {
			return nil, badFederation()
		}
		owners[entry] = true
		return owners, nil
	}
	if !publicEvidenceStamp(closure) || !publicEvidenceStamp(g.Purpose) || len(g.Sources) < 1 || len(g.Sources) > 256 || (g.GetOperation() != "read" && g.GetOperation() != "query" && g.GetOperation() != "list") {
		return nil, badFederation()
	}
	scans := 0
	for i, s := range g.Sources {
		if s == nil {
			return nil, badFederation()
		}
		owner := values.FactAuthorityRef(s.GetOwner())
		if !owner.Valid() || owners[owner] || !values.ElementID(s.GetBoundary()).Valid() || s.Principal == nil || !(values.PrincipalRef{Realm: values.RealmID(s.Principal.GetRealm()), Principal: values.PrincipalID(s.Principal.GetPrincipal())}).Valid() || !publicEvidenceStamp(s.Cut) || !publicEvidenceStamp(s.Authorization) || (s.GetReadMode() != pb.ReadMode_CURRENT_VERIFIED && s.GetReadMode() != pb.ReadMode_RETAINED) {
			return nil, badFederation()
		}
		if i == 0 {
			if owner != entry || s.Parent != nil {
				return nil, badFederation()
			}
		} else {
			p := s.Parent
			if p == nil || !owners[values.FactAuthorityRef(p.GetOwner())] || p.GetAnchor() != s.GetBoundary() || !publicEvidenceStamp(p.Delegation) || !publicEvidenceStamp(p.Acceptance) || !proto.Equal(s.Principal, g.Sources[0].Principal) || s.GetReadMode() != g.Sources[0].GetReadMode() {
				return nil, badFederation()
			}
		}
		if (s.Coverage != nil) != (s.QueryScope != nil) {
			return nil, badFederation()
		}
		if s.Coverage != nil {
			if g.GetOperation() != "query" || !publicEvidenceStamp(s.Coverage) || !values.ElementID(s.GetQueryScope()).Valid() {
				return nil, badFederation()
			}
			scans++
			if err := validateParentEvidence(s.Parents, s.GetQueryScope(), false); err != nil {
				return nil, err
			}
		} else if len(s.Parents) != 0 {
			return nil, badFederation()
		}
		owners[owner] = true
	}
	if g.GetOperation() == "query" && scans == 0 {
		return nil, badFederation()
	}
	return owners, nil
}

func validateParentEvidence(parents []*pb.ParentAdjacencyEvidence, scope string, required bool) error {
	if len(parents) > 4096 || required && len(parents) == 0 {
		return badFederation()
	}
	found := false
	for i, p := range parents {
		if p == nil || !values.ElementID(p.GetParent()).Valid() || !publicEvidenceStamp(p.Adjacency) || i > 0 && parents[i-1].GetParent() >= p.GetParent() {
			return badFederation()
		}
		found = found || p.GetParent() == scope
	}
	if len(parents) != 0 && !found {
		return badFederation()
	}
	return nil
}
