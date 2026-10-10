package fact

import (
	"slices"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/token"
)

const MaxLineageSteps = 4096

// LineageStep is the source owner's narrowed custody evidence for one member.
// It contains no other member, caller, payload or former spatial tuple. Digests
// bind the original decisions, not permission: the enclosing final manifest
// must come from the designated authenticated source, never from caller claims.
type LineageStep struct {
	Controller                   values.FactAuthorityRef
	Source, Target               values.FactAuthorityRef
	Operation                    contract.OperationID
	Kind                         contract.ElementKind
	SourceRevision               contract.Revision
	FinalCut, InstalledCut       contract.CoreReadCut
	Decision, Installation       [32]byte
	SourceAddress, TargetAddress values.NextHop
}

func (s Slice) Lineage(id values.ElementID) []LineageStep { return slices.Clone(s.lineage[id]) }

// WithLineage attaches only bounded, structurally continuous value evidence.
// Durable custody and any returning target's own anchor are checked by the
// original commit owner. This method cannot authorize adoption of a UUID.
func (s Slice) WithLineage(chains map[values.ElementID][]LineageStep) (Slice, error) {
	members := make(map[values.ElementID]Member, len(s.members))
	for _, member := range s.members {
		members[member.ID()] = member
	}
	out := make(map[values.ElementID][]LineageStep, len(chains))
	count := 0
	for id, chain := range chains {
		member, ok := members[id]
		if !ok || len(chain) == 0 {
			return Slice{}, contract.ErrInvalid
		}
		if len(chain) > MaxLineageSteps-count {
			return Slice{}, values.PublicResourceExhausted
		}
		count += len(chain)
		seen := make(map[TransferKey]bool, len(chain))
		for index, step := range chain {
			key := TransferKey{Controller: step.Controller, Operation: step.Operation}
			if !key.Valid() || !step.Source.Valid() || !step.Target.Valid() || step.Source == step.Target ||
				step.Kind != member.Kind || !token.ValidRevision(step.SourceRevision) || token.RevisionElement(step.SourceRevision) != id ||
				!token.Valid(step.FinalCut) || !token.Valid(step.InstalledCut) || token.Scope(step.FinalCut) == token.Scope(step.InstalledCut) ||
				token.RevisionScope(step.SourceRevision) != token.Scope(step.FinalCut) || step.Decision == ([32]byte{}) || step.Installation == ([32]byte{}) ||
				!step.SourceAddress.Valid() || !step.TargetAddress.Valid() {
				return Slice{}, contract.ErrConflict
			}
			if seen[key] {
				return Slice{}, contract.ErrConflict
			}
			seen[key] = true
			if index > 0 {
				prior := chain[index-1]
				if prior.Target != step.Source || token.Scope(prior.InstalledCut) != token.Scope(step.FinalCut) || token.Serial(prior.InstalledCut) > token.Serial(step.FinalCut) {
					return Slice{}, contract.ErrConflict
				}
			}
		}
		last := chain[len(chain)-1]
		if last.Target != s.source || token.Scope(last.InstalledCut) != token.Scope(s.cut) || token.Serial(last.InstalledCut) > token.Serial(s.cut) {
			return Slice{}, contract.ErrConflict
		}
		out[id] = slices.Clone(chain)
	}
	s.lineage = out
	return s, nil
}
