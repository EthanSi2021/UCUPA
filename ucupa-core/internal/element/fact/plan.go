package fact

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/geometry"
	"ucupa/ucupa-core/internal/token"
)

// Input is the fact-only portion of a mutation binding.
type Input struct {
	Kind     OperationKind
	Element  values.ElementID
	Final    contract.SpatialTuple
	Expected contract.Revision
}

// ExternalParent describes the one admitted parent outside the local fact
// tables. Range is the selected range used for the containment check.
type ExternalParent struct {
	Present  bool
	ID       values.ElementID
	Range    contract.BoundedSphere
	Admitted bool
}

// ParentContext supplies the finite parent and mounted-content facts needed by
// deterministic element rules. It is not a callback or a state handle.
type ParentContext struct {
	External                 ExternalParent
	ParentHasMountedContent  bool
	ElementHasMountedContent bool
}

// Outcome is the value result of planning or applying one mutation. It has no
// cut, authorization, journal, mount, or attachment state.
type Outcome struct {
	Status    contract.MutationStatus
	Rejection Rejection
	Kind      contract.ElementKind
	Anchor    Anchor
	Entity    Entity
	Tombstone contract.Revision
}

// Plan is an immutable, state-private deterministic decision. A plan never
// carries an issued revision or a writable state reference.
type Plan struct {
	input   Input
	outcome Outcome
}

// Outcome returns a value copy of the plan's decision.
func (p Plan) Outcome() Outcome {
	return p.outcome
}

func rejectedPlan(rejection Rejection) Plan {
	return Plan{outcome: Outcome{
		Status:    contract.MutationRejected,
		Rejection: rejection,
	}}
}

func unchangedAnchorPlan(anchor Anchor) Plan {
	return Plan{outcome: Outcome{
		Status: contract.MutationUnchanged,
		Kind:   contract.ElementAnchor,
		Anchor: anchor,
	}}
}

func unchangedEntityPlan(entity Entity) Plan {
	return Plan{outcome: Outcome{
		Status: contract.MutationUnchanged,
		Kind:   contract.ElementEntity,
		Entity: entity,
	}}
}

func committedPlan(input Input, kind contract.ElementKind) Plan {
	return Plan{
		input: input,
		outcome: Outcome{
			Status: contract.MutationCommitted,
			Kind:   kind,
		},
	}
}

// Plan evaluates one element mutation without changing the state. Its order
// follows the existing Core mutation decision rules.
func (s State) Plan(input Input, context ParentContext) (Plan, error) {
	switch input.Kind {
	case OperationCreateRoot:
		if _, retired := s.RetiredRoot(); retired {
			return rejectedPlan(Conflict), nil
		}
		if s.Claimed(input.Element) || s.RootExists() || context.External.Present {
			return rejectedPlan(Conflict), nil
		}
		return committedPlan(input, contract.ElementAnchor), nil

	case OperationCreateAnchor, OperationCreateEntity:
		if rejection := s.parentRejection(input.Final.Parent, context); rejection != NoError {
			return rejectedPlan(rejection), nil
		}
		if s.Claimed(input.Element) ||
			(context.External.Present && context.External.ID == input.Element) {
			return rejectedPlan(Conflict), nil
		}
		inside, err := s.Contains(input.Final, context.External)
		if err != nil {
			return Plan{}, err
		}
		if !inside {
			return rejectedPlan(Conflict), nil
		}
		if input.Kind == OperationCreateAnchor {
			return committedPlan(input, contract.ElementAnchor), nil
		}
		return committedPlan(input, contract.ElementEntity), nil

	case OperationUpdateAnchor:
		anchor, ok := s.anchors[input.Element]
		if !ok {
			if s.Kind(input.Element) != 0 {
				return rejectedPlan(Conflict), nil
			}
			return rejectedPlan(NotFound), nil
		}
		if anchor.Revision != input.Expected {
			return rejectedPlan(RevisionConflict), nil
		}
		if anchor.Root() {
			return rejectedPlan(Conflict), nil
		}
		if AnchorTupleEqual(anchor, input.Final) {
			return unchangedAnchorPlan(anchor), nil
		}
		if rejection := s.parentRejection(input.Final.Parent, context); rejection != NoError {
			return rejectedPlan(rejection), nil
		}
		if input.Element == input.Final.Parent ||
			s.DescendsFrom(input.Final.Parent, input.Element) {
			return rejectedPlan(Conflict), nil
		}
		inside, err := s.Contains(input.Final, context.External)
		if err != nil {
			return Plan{}, err
		}
		if !inside {
			return rejectedPlan(Conflict), nil
		}
		candidateRange := geometry.BoundedRange(input.Final.Range)
		candidateSphere, bounded := candidateRange.BoundedSphere()
		if !bounded {
			return Plan{}, contract.ErrStore
		}
		covers, err := s.RangeContainsDirectContent(input.Element, candidateSphere)
		if err != nil {
			return Plan{}, err
		}
		if !covers {
			return rejectedPlan(Conflict), nil
		}
		return committedPlan(input, contract.ElementAnchor), nil

	case OperationUpdateEntity:
		entity, ok := s.entities[input.Element]
		if !ok {
			if s.Kind(input.Element) != 0 {
				return rejectedPlan(Conflict), nil
			}
			return rejectedPlan(NotFound), nil
		}
		if entity.Revision != input.Expected {
			return rejectedPlan(RevisionConflict), nil
		}
		if EntityTupleEqual(entity, input.Final) {
			return unchangedEntityPlan(entity), nil
		}
		if rejection := s.parentRejection(input.Final.Parent, context); rejection != NoError {
			return rejectedPlan(rejection), nil
		}
		inside, err := s.Contains(input.Final, context.External)
		if err != nil {
			return Plan{}, err
		}
		if !inside {
			return rejectedPlan(Conflict), nil
		}
		return committedPlan(input, contract.ElementEntity), nil

	case OperationDeleteAnchor:
		anchor, ok := s.anchors[input.Element]
		if !ok {
			if s.Kind(input.Element) != 0 {
				return rejectedPlan(Conflict), nil
			}
			return rejectedPlan(NotFound), nil
		}
		if anchor.Revision != input.Expected {
			return rejectedPlan(RevisionConflict), nil
		}
		if anchor.Root() || s.HasDirectContent(input.Element) ||
			context.ElementHasMountedContent {
			return rejectedPlan(Conflict), nil
		}
		return committedPlan(input, 0), nil

	case OperationDeleteEntity:
		entity, ok := s.entities[input.Element]
		if !ok {
			if s.Kind(input.Element) != 0 {
				return rejectedPlan(Conflict), nil
			}
			return rejectedPlan(NotFound), nil
		}
		if entity.Revision != input.Expected {
			return rejectedPlan(RevisionConflict), nil
		}
		return committedPlan(input, 0), nil

	default:
		return Plan{}, contract.ErrStore
	}
}

func (s State) parentRejection(parent values.ElementID, context ParentContext) Rejection {
	if context.ParentHasMountedContent {
		return Conflict
	}
	if _, ok := s.anchors[parent]; ok ||
		(context.External.Present && context.External.ID == parent) {
		return NoError
	}
	if s.Kind(parent) != 0 {
		return Conflict
	}
	return NotFound
}

// Apply commits a previously planned mutation using the already-issued
// revision. It does not issue revisions and does not own cut or journal state.
// The aggregate must plan and apply to the same candidate without intervening
// changes, under its existing commit lock; Apply does not re-run the decision.
func (s *State) Apply(plan Plan, revision contract.Revision) (Outcome, error) {
	if s == nil || plan.outcome.Status != contract.MutationCommitted {
		return Outcome{}, contract.ErrInvalid
	}
	if !token.ValidRevision(revision) ||
		token.RevisionElement(revision) != plan.input.Element {
		return Outcome{}, contract.ErrInvalid
	}
	s.ensure()

	switch plan.input.Kind {
	case OperationCreateRoot:
		anchor := Anchor{
			ID:        plan.input.Element,
			LocalPose: contract.Pose{},
			Range:     geometry.UnboundedRootRange(),
			Revision:  revision,
		}
		s.claimed[anchor.ID] = struct{}{}
		s.anchors[anchor.ID] = anchor
		return Outcome{
			Status: contract.MutationCommitted,
			Kind:   contract.ElementAnchor,
			Anchor: anchor,
		}, nil

	case OperationCreateAnchor:
		anchor := Anchor{
			ID:        plan.input.Element,
			Parent:    plan.input.Final.Parent,
			LocalPose: plan.input.Final.LocalPose,
			Range:     geometry.BoundedRange(plan.input.Final.Range),
			Revision:  revision,
		}
		s.claimed[anchor.ID] = struct{}{}
		s.anchors[anchor.ID] = anchor
		return Outcome{
			Status: contract.MutationCommitted,
			Kind:   contract.ElementAnchor,
			Anchor: anchor,
		}, nil

	case OperationCreateEntity:
		entity := Entity{
			ID:        plan.input.Element,
			Parent:    plan.input.Final.Parent,
			LocalPose: plan.input.Final.LocalPose,
			Range:     plan.input.Final.Range,
			Revision:  revision,
		}
		s.claimed[entity.ID] = struct{}{}
		s.entities[entity.ID] = entity
		return Outcome{
			Status: contract.MutationCommitted,
			Kind:   contract.ElementEntity,
			Entity: entity,
		}, nil

	case OperationUpdateAnchor:
		anchor, ok := s.anchors[plan.input.Element]
		if !ok {
			return Outcome{}, contract.ErrStore
		}
		anchor.Parent = plan.input.Final.Parent
		anchor.LocalPose = plan.input.Final.LocalPose
		anchor.Range = geometry.BoundedRange(plan.input.Final.Range)
		anchor.Revision = revision
		s.anchors[plan.input.Element] = anchor
		return Outcome{
			Status: contract.MutationCommitted,
			Kind:   contract.ElementAnchor,
			Anchor: anchor,
		}, nil

	case OperationUpdateEntity:
		entity, ok := s.entities[plan.input.Element]
		if !ok {
			return Outcome{}, contract.ErrStore
		}
		entity.Parent = plan.input.Final.Parent
		entity.LocalPose = plan.input.Final.LocalPose
		entity.Range = plan.input.Final.Range
		entity.Revision = revision
		s.entities[plan.input.Element] = entity
		return Outcome{
			Status: contract.MutationCommitted,
			Kind:   contract.ElementEntity,
			Entity: entity,
		}, nil

	case OperationDeleteAnchor:
		if _, ok := s.anchors[plan.input.Element]; !ok {
			return Outcome{}, contract.ErrStore
		}
		delete(s.anchors, plan.input.Element)
		s.tombstones[plan.input.Element] = revision
		return Outcome{
			Status:    contract.MutationCommitted,
			Tombstone: revision,
		}, nil

	case OperationDeleteEntity:
		if _, ok := s.entities[plan.input.Element]; !ok {
			return Outcome{}, contract.ErrStore
		}
		delete(s.entities, plan.input.Element)
		s.tombstones[plan.input.Element] = revision
		return Outcome{
			Status:    contract.MutationCommitted,
			Tombstone: revision,
		}, nil

	default:
		return Outcome{}, contract.ErrInvalid
	}
}
