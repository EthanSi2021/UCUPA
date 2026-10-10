// Package oracle preserves the private linear, all-or-nothing conformance scan.
// It borrows the sole commit owner under its original read lock; it is not a
// discovery endpoint, snapshot registry, or a second source of facts.
package oracle

import (
	"errors"
	"ucupa/ucupa-core/internal/commit"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	token "ucupa/ucupa-core/internal/token"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	"ucupa/ucupa-core/internal/geometry"
	queryscan "ucupa/ucupa-core/internal/query/scan"
)

// The linear oracle is deliberately module-private. It is a conformance
// reference for future discovery indexes, not a public Discover surface.
type Selector uint8

const (
	Anchors Selector = iota + 1
	Entities
	Both
)

type Result struct {
	Records []contract.ElementRecord
	Cut     contract.CoreReadCut
}

func Scan(
	owner *commit.Owner,
	boundaryAnchorID values.ElementID,
	queryBasisAnchorID values.ElementID,
	at *contract.CoreReadCut,
	query contract.QuerySphere,
	selector Selector,
) (Result, error) {
	return ScanMeasured(
		owner,
		boundaryAnchorID, queryBasisAnchorID, at, query, selector, nil,
	)
}

func ScanMeasured(
	owner *commit.Owner,
	boundaryAnchorID values.ElementID,
	queryBasisAnchorID values.ElementID,
	at *contract.CoreReadCut,
	query contract.QuerySphere,
	selector Selector,
	stats *queryscan.Stats,
) (Result, error) {
	if stats != nil {
		*stats = queryscan.Stats{}
	}
	if owner == nil || !boundaryAnchorID.Valid() || !queryBasisAnchorID.Valid() ||
		!token.ValidOptionalCut(at) || !geometry.Finite(query.Center.X) || !geometry.Finite(query.Center.Y) ||
		!geometry.Finite(query.Center.Z) || !geometry.Finite(query.Radius) || query.Radius < 0 ||
		(selector != Anchors && selector != Entities && selector != Both) {
		return Result{}, contract.ErrInvalid
	}
	queryRange, err := contract.NewBoundedSphere(query.Radius)
	if err != nil {
		return Result{}, contract.ErrInvalid
	}
	query.Center.X = geometry.CanonicalZero(query.Center.X)
	query.Center.Y = geometry.CanonicalZero(query.Center.Y)
	query.Center.Z = geometry.CanonicalZero(query.Center.Z)

	owner.RLock()
	defer owner.RUnlock()
	if owner.ClosedLocked() || !owner.AvailableLocked() {
		return Result{}, contract.ErrStore
	}
	// The oracle is a complete conformance reference, so it must not publish a
	// result from a malformed current snapshot even when the malformed record's
	// kind is excluded by selector. O(N) current validation and the scan share
	// this RLock and therefore the same owner-local cut. Retained history is
	// validated at Store creation/open/persist boundaries, not replayed per scan.
	if validationErr := owner.ValidateCurrentLocked(); validationErr != nil {
		return Result{}, contract.ErrStore
	}
	cut, err := owner.SelectCurrentCutLocked(at)
	if err != nil {
		return Result{}, err
	}
	if mountfact.IsExternalParent(owner.FactsLocked().Mount, boundaryAnchorID) {
		return Result{}, contract.ErrMountContextRequired
	}
	if _, exists := owner.FactsLocked().Elements.Anchor(boundaryAnchorID); !exists {
		if owner.FactsLocked().Elements.Kind(boundaryAnchorID) != 0 {
			return Result{}, contract.ErrConflict
		}
		return Result{}, contract.ErrNotFound
	}
	if _, exists := owner.FactsLocked().Elements.Anchor(queryBasisAnchorID); !exists {
		if owner.FactsLocked().Elements.Kind(queryBasisAnchorID) != 0 {
			return Result{}, contract.ErrConflict
		}
		return Result{}, contract.ErrNotFound
	}
	closure, err := queryscan.BoundaryClosure(owner.FactsLocked().Elements, boundaryAnchorID, stats)
	if err != nil {
		return Result{}, err
	}
	if _, basisInside := closure[queryBasisAnchorID]; !basisInside {
		return Result{}, contract.ErrConflict
	}
	if err := queryscan.GuardLocalClosure(owner.FactsLocked().Mount, closure); err != nil {
		return Result{}, err
	}
	projections := queryscan.NewProjectionMemo(owner.FactsLocked().Elements, boundaryAnchorID, stats)
	basisProjection, err := projections.Anchor(queryBasisAnchorID)
	if err != nil {
		return Result{}, err
	}
	basisWork := &geometry.Work{}
	basisInverse := basisWork.Inverse(basisProjection.Pose)
	if basisWork.Failed() || !geometry.PoseWithinWorkLimit(basisInverse) {
		return Result{}, contract.ErrUnrepresentable
	}

	ids := make([]values.ElementID, 0, len(closure)+owner.FactsLocked().Elements.Counts().Entities)
	for id := range closure {
		ids = append(ids, id)
	}
	for id, entity := range owner.FactsLocked().Elements.Entities() {
		if stats != nil {
			stats.EntityRecordsScanned++
		}
		if _, inside := closure[entity.Parent]; inside {
			ids = append(ids, id)
		}
	}
	elementfact.SortIDs(ids)

	records := make([]contract.ElementRecord, 0)
	for _, id := range ids {
		var (
			record       contract.ElementRecord
			radius       contract.BoundedSphere
			subjectExact geometry.ExactPose
		)
		if anchor, exists := owner.FactsLocked().Elements.Anchor(id); exists {
			if selector == Entities {
				continue
			}
			record = anchor.Record()
			if anchor.Root() {
				records = append(records, record)
				continue
			}
			var bounded bool
			radius, bounded = anchor.Range.BoundedSphere()
			if !bounded {
				return Result{}, contract.ErrStore
			}
			projection, projectionErr := projections.Anchor(id)
			if projectionErr != nil {
				return Result{}, projectionErr
			}
			subjectExact = projection.Pose
		} else if entity, exists := owner.FactsLocked().Elements.Entity(id); exists {
			if selector == Anchors {
				continue
			}
			record = entity.Record()
			radius = entity.Range
			projection, projectionErr := projections.Entity(entity)
			if projectionErr != nil {
				return Result{}, projectionErr
			}
			subjectExact = projection.Pose
		} else {
			return Result{}, contract.ErrStore
		}
		work := &geometry.Work{}
		relative := work.Compose(basisInverse, subjectExact)
		if work.Failed() || !geometry.PoseWithinWorkLimit(relative) {
			return Result{}, contract.ErrUnrepresentable
		}
		intersects, intersectionErr := geometry.CenterIntersects(relative, radius, query.Center, queryRange)
		if intersectionErr != nil {
			if errors.Is(intersectionErr, contract.ErrInvalid) {
				return Result{}, contract.ErrStore
			}
			return Result{}, intersectionErr
		}
		if intersects {
			records = append(records, record)
		}
	}
	return Result{Records: records, Cut: cut}, nil
}
