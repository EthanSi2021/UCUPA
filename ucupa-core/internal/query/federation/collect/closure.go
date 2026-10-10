package collect

import (
	"ucupa/shared/values"
	federationcontract "ucupa/ucupa-core/contract/federation"
	"ucupa/ucupa-core/internal/query/federation/transcript"
)

// closureCommitment binds the sealed private graph and query coverage. It is
// not an offline proof, source list or credential.
func (r *Read) closureCommitment() ([32]byte, error) {
	if r.closed || r.sources.closed || !r.sources.sealed {
		return [32]byte{}, values.PublicInvalidContext
	}
	if r.operation == "query" && len(r.plans) == 0 {
		return [32]byte{}, values.PublicInvalidContext
	}
	if err := r.sources.checkEvidenceGraph(); err != nil {
		return [32]byte{}, err
	}
	v := r.sources
	w := transcript.HMACSHA256([]byte(v.parts[0].Part().Basis.Value))
	w.Bytes([]byte("ucupa.core.aggregate-closure"))
	w.Bytes(v.request.Purpose[:])
	w.Text(string(v.caller.Subject.Realm), string(v.caller.Subject.Principal))
	w.Bytes(v.caller.Establishment[:])
	w.Text(r.operation, string(r.boundary), string(r.scope))
	w.Number(r.actual)
	w.Number(uint64(r.gap))
	for _, connection := range v.parts {
		node := v.graph[connection.Part().Basis.Value]
		p := node.part
		w.Token(p.Basis)
		w.Text(string(p.Boundary), string(p.Subject.Realm), string(p.Subject.Principal))
		w.Bytes(p.Purpose[:])
		w.Bytes(p.CutStamp[:])
		w.Bytes(p.AuthorizationStamp[:])
		w.Number(uint64(p.ReadMode))
		if p.Hidden {
			w.Number(1)
		} else {
			w.Number(0)
		}
		w.Token(node.parent)
		w.Relation(&node.outgoing)
		w.Relation(p.Incoming)
		if p.Preparation == nil {
			w.Number(0)
		} else {
			w.Number(1)
			w.Token(*p.Preparation)
		}
	}
	var result [32]byte
	for _, plan := range r.plans {
		if plan.request.Kind != "query" {
			continue
		}
		if plan.coverage == ([32]byte{}) {
			return [32]byte{}, values.PublicInvalidContext
		}
		w.Token(plan.part.Part().Basis)
		w.Bytes(plan.coverage[:])
		w.Number(uint64(len(plan.parents)))
		for i, p := range plan.parents {
			if !p.Parent.Valid() || p.Adjacency == ([32]byte{}) || i > 0 && plan.parents[i-1].Parent >= p.Parent {
				return [32]byte{}, values.PublicInvalidContext
			}
			w.Text(string(p.Parent))
			w.Bytes(p.Adjacency[:])
		}
	}
	copy(result[:], w.Sum())
	return result, nil
}

// sourceGraph projects only independently disclosable source evidence. Hidden
// nodes retain no placeholder, owner, count or contact in the result.
func (r *Read) sourceGraph() (SourceGraph, error) {
	if _, err := r.closureCommitment(); err != nil {
		return SourceGraph{}, err
	}
	v := r.sources
	result := SourceGraph{Purpose: v.request.Purpose, Operation: r.operation}
	visible := make(map[values.FactAuthorityRef]bool)
	for _, connection := range v.parts {
		part := connection.Part()
		if part.Hidden {
			continue
		}
		node := v.graph[part.Basis.Value]
		out := SourceEvidence{
			Owner:              part.Basis.Owner,
			Boundary:           part.Boundary,
			Subject:            part.Subject,
			CutStamp:           part.CutStamp,
			AuthorizationStamp: part.AuthorizationStamp,
			ReadMode:           part.ReadMode,
		}
		if node.parent.Valid() {
			if !visible[node.parent.Owner] || part.Incoming == nil {
				return SourceGraph{}, values.PublicInvalidContext
			}
			stamp := func(domain string, relation *federationcontract.FederationMountRelation) [32]byte {
				w := transcript.HMACSHA256([]byte(v.parts[0].Part().Basis.Value))
				w.Text(domain)
				w.Bytes(v.request.Purpose[:])
				w.Relation(relation)
				var result [32]byte
				copy(result[:], w.Sum())
				return result
			}
			out.Parent = &ParentEvidence{
				Owner:      node.parent.Owner,
				Anchor:     node.outgoing.Key.MountAnchorUUID,
				Delegation: stamp("ucupa.core.disclosed-delegation", &node.outgoing),
				Acceptance: stamp("ucupa.core.disclosed-acceptance", part.Incoming),
			}
		}
		for _, plan := range r.plans {
			if plan.part.Part().Basis != part.Basis || plan.request.Kind != "query" {
				continue
			}
			out.Query = &QueryEvidence{Scope: plan.request.Query.Scope, Coverage: plan.coverage, Parents: append([]federationcontract.FederationParentEvidence(nil), plan.parents...)}
		}
		visible[part.Basis.Owner] = true
		result.Sources = append(result.Sources, out)
	}
	return result, nil
}
