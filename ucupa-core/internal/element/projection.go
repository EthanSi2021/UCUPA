package element

import (
	"errors"
	mountfact "ucupa/ucupa-core/internal/mount/fact"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	commitpkg "ucupa/ucupa-core/internal/commit"
	"ucupa/ucupa-core/internal/geometry"
	"ucupa/ucupa-core/internal/token"
)

type projectionSnapshot struct {
	cut          contract.CoreReadCut
	requirements []values.Requirement
	poses        []contract.Pose
	revision     contract.Revision
	decision     error
}

func (core *Operations) ProjectToAncestor(
	actor values.PrincipalRef,
	subject values.ElementID,
	ancestor values.ElementID,
	at *contract.CoreReadCut,
) (contract.Projection, error) {
	if core == nil || !subject.Valid() || !ancestor.Valid() || !token.ValidOptionalCut(at) {
		return contract.Projection{}, contract.ErrInvalid
	}
	// Gate all state access with the exact scopes already present in the
	// request. Path discovery happens only after this first authorization.
	approval, err := core.authorize(actor, core.readRequirements([]values.ElementID{subject, ancestor})...)
	if err != nil {
		return contract.Projection{}, err
	}

	core.owner.RLock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		core.owner.RUnlock()
		return contract.Projection{}, contract.ErrStore
	}
	if !core.settled(approval) {
		core.owner.RUnlock()
		return contract.Projection{}, contract.ErrUnavailable
	}
	current := core.owner.CurrentCutLocked()
	if at != nil && token.Scope(*at) != token.Scope(current) {
		core.owner.RUnlock()
		return contract.Projection{}, contract.ErrInvalid
	}
	if at != nil && *at != current {
		core.owner.RUnlock()
		return contract.Projection{}, contract.ErrReadCutExpired
	}
	snapshot := core.provisionalProjectionLocked(subject, ancestor)
	core.owner.RUnlock()
	if snapshot.decision != nil && len(snapshot.requirements) == 0 {
		return contract.Projection{}, snapshot.decision
	}

	// Re-authorize the complete resolved path as one all-of decision. This call
	// remains outside the Core lock; the cut check below prevents a mixed path.
	if len(snapshot.requirements) != 0 {
		approval, err = core.authorize(actor, snapshot.requirements...)
		if err != nil {
			return contract.Projection{}, err
		}
	}
	core.owner.RLock()
	defer core.owner.RUnlock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		return contract.Projection{}, contract.ErrStore
	}
	if !core.settled(approval) {
		return contract.Projection{}, contract.ErrUnavailable
	}
	if core.owner.CurrentCutLocked() != snapshot.cut {
		return contract.Projection{}, contract.ErrReadCutExpired
	}
	if snapshot.decision != nil {
		return contract.Projection{}, snapshot.decision
	}
	pose, err := geometry.ComposePath(snapshot.poses)
	if err != nil {
		if errors.Is(err, contract.ErrUnrepresentable) {
			return contract.Projection{}, contract.ErrUnrepresentable
		}
		return contract.Projection{}, contract.ErrStore
	}
	return contract.Projection{Pose: pose, SubjectRevision: snapshot.revision, Cut: snapshot.cut}, nil
}

func (core *Operations) provisionalProjectionLocked(
	subject values.ElementID,
	ancestor values.ElementID,
) projectionSnapshot {
	snapshot := projectionSnapshot{cut: core.owner.CurrentCutLocked()}
	if mountfact.IsExternalParent(core.owner.FactsLocked().Mount, subject) || mountfact.IsExternalParent(core.owner.FactsLocked().Mount, ancestor) {
		snapshot.decision = contract.ErrMountContextRequired
		return snapshot
	}
	if core.owner.FactsLocked().Elements.Kind(subject) == 0 {
		snapshot.decision = contract.ErrNotFound
		return snapshot
	}
	if _, exists := core.owner.FactsLocked().Elements.Anchor(ancestor); !exists {
		if core.owner.FactsLocked().Elements.Kind(ancestor) != 0 {
			snapshot.decision = contract.ErrConflict
		} else {
			snapshot.decision = contract.ErrNotFound
		}
		return snapshot
	}
	ids, poses, revision, err := subjectPathToFrameBoundary(core.owner.FactsLocked(), subject)
	if err != nil {
		// A broken or cyclic chain can still expose which existing records were
		// traversed before the corruption was found. Preserve those provisional
		// path scopes so the error is disclosed only after the same all-of path
		// authorization and cut recheck as every other resolved-path decision.
		snapshot.requirements = core.readRequirements(append(ids, ancestor))
		snapshot.decision = err
		return snapshot
	}
	position := -1
	for index, id := range ids {
		if id == ancestor {
			position = index
			break
		}
	}
	if position < 0 {
		snapshot.requirements = core.readRequirements(append(ids, ancestor))
		snapshot.decision = contract.ErrConflict
		return snapshot
	}
	snapshot.requirements = core.readRequirements(ids[:position+1])
	snapshot.poses = append([]contract.Pose(nil), poses[:position]...)
	snapshot.revision = revision
	return snapshot
}

func (core *Operations) readRequirements(ids []values.ElementID) []values.Requirement {
	requirements := make([]values.Requirement, 0, len(ids))
	seen := make(map[values.ElementID]struct{}, len(ids))
	for _, id := range ids {
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		requirements = append(requirements, core.requirement(contract.PermissionReadElement, id))
	}
	return requirements
}

func subjectPathToFrameBoundary(state commitpkg.Facts,
	subject values.ElementID,
) ([]values.ElementID, []contract.Pose, contract.Revision, error) {
	ids := make([]values.ElementID, 0, 8)
	poses := make([]contract.Pose, 0, 8)
	var revision contract.Revision
	var current values.ElementID
	if anchor, exists := state.Elements.Anchor(subject); exists {
		revision = anchor.Revision
		current = subject
	} else if entity, exists := state.Elements.Entity(subject); exists {
		revision = entity.Revision
		ids = append(ids, subject)
		poses = append(poses, entity.LocalPose)
		current = entity.Parent
	} else {
		return []values.ElementID{subject}, nil, contract.Revision{}, contract.ErrNotFound
	}
	seen := make(map[values.ElementID]struct{}, state.Elements.Counts().Anchors)
	for current != "" {
		if !current.Valid() {
			return ids, poses, contract.Revision{}, contract.ErrStore
		}
		if _, duplicate := seen[current]; duplicate {
			return ids, poses, contract.Revision{}, contract.ErrStore
		}
		seen[current] = struct{}{}
		// Record a canonical referenced scope before looking it up. A missing
		// parent is itself state learned from the preceding record and must
		// participate in the authorization that gates broken-chain disclosure.
		ids = append(ids, current)
		if mountfact.IsExternalParent(state.Mount, current) {
			// End at the declared boundary, without inventing its record or Pose.
			// A local ancestor projection only consumes the owned prefix above.
			return ids, poses, revision, nil
		}
		anchor, exists := state.Elements.Anchor(current)
		if !exists {
			return ids, poses, contract.Revision{}, contract.ErrStore
		}
		if anchor.Root() {
			return ids, poses, revision, nil
		}
		poses = append(poses, anchor.LocalPose)
		current = anchor.Parent
	}
	return ids, poses, contract.Revision{}, contract.ErrStore
}
