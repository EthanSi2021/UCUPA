package source

import (
	federationcontract "ucupa/ucupa-core/contract/federation"
	accesspkg "ucupa/ucupa-core/internal/access"
	accessresolve "ucupa/ucupa-core/internal/access/resolve"
	federationprojection "ucupa/ucupa-core/internal/query/federation/projection"

	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"math"
	querypkg "ucupa/ucupa-core/internal/query"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	accessregistry "ucupa/ucupa-core/internal/access/registry"
	accessvalue "ucupa/ucupa-core/internal/access/value"
	"ucupa/ucupa-core/internal/geometry"
	federationdenial "ucupa/ucupa-core/internal/query/federation/denial"
	queryrequest "ucupa/ucupa-core/internal/query/request"
)

// ProjectFederationReadPart executes only the original operation's disclosure
// at its fixed cut. A cursor also binds the actual projection, not just Purpose.
func (core *Operations) ProjectFederationReadPart(ctx context.Context, caller values.CallerContext, source values.FactAuthorityRef, token values.PublicToken, request federationcontract.FederationProjectionRequest, continuation *values.PublicToken, limit uint16) (federationcontract.FederationProjectionPage, error) {
	finish, err := core.enterFederationCall(ctx)
	if err != nil {
		return federationcontract.FederationProjectionPage{}, err
	}
	defer finish()
	conditions, err := federationProjectionConditions(request, limit)
	if err != nil {
		return federationcontract.FederationProjectionPage{}, err
	}
	// The retained basis view, or its captured denial, is rechecked online
	// outside the fact lock; every original check then runs again under one
	// lock guarded by the permit that recheck minted.
	core.owner.Lock()
	basis, err := accessresolve.ReadLocked(core.owner, core.registry, caller, accessvalue.TokenJSON(token))
	core.owner.Unlock()
	if err != nil {
		return federationcontract.FederationProjectionPage{}, err
	}
	var permit accesspkg.Permit
	var denialPermit accesspkg.DenialPermit
	if basis.Source.Denial != nil {
		view, requirement := federationdenial.Authorization(*basis.Source.Denial)
		denialPermit, err = core.admission.RecheckDenial(ctx, caller, view, requirement)
	} else {
		permit, err = core.admission.Recheck(ctx, caller, basis.View, basis.Requirements, false)
	}
	if err != nil {
		return federationcontract.FederationProjectionPage{}, err
	}
	core.owner.Lock()
	basis, err = accessresolve.ReadLocked(core.owner, core.registry, caller, accessvalue.TokenJSON(token))
	if err == nil {
		err = core.checkFederationBasisLocked(ctx, caller, source, basis, permit, denialPermit)
	}
	if err == nil && basis.Source.Denial != nil {
		err = values.PublicDenied
	}
	if err == nil && request.Kind != "path" && request.Kind != "read-at" && request.Kind != basis.Source.Request.Intent {
		err = values.PublicDenied
	}
	var after values.ElementID
	if err == nil && continuation != nil {
		var page accessregistry.ContinuationUse
		page, err = accessresolve.ContinuationLocked(core.owner, core.registry, caller, accessvalue.TokenJSON(*continuation))
		if err == nil {
			if page.Receipt != basis.Ref || page.Conditions != conditions {
				err = values.PublicInvalidContext
			} else {
				after = page.Last
			}
		}
	}
	core.owner.Unlock()
	if err != nil {
		return federationcontract.FederationProjectionPage{}, err
	}
	// A discovery receipt identifies a cut, not permission to read full facts.
	// read-at is a separate exact read against that cut. Keep its authorization
	// view until disclosure; do not alter the original query/list grant or its
	// observation preparation, and do not recapture a newer spatial state.
	var readView values.AuthorizationView
	retainedReadView := true
	var readRequirements []values.Requirement
	var readPermit accesspkg.Permit
	defer func() {
		if !retainedReadView {
			core.admission.ReleasePermit(ctx, readPermit)
		}
	}()
	if request.Kind == "read-at" {
		readRequirements = accesspkg.ClosureRequirements(core.owner.AuthorityRef(), basis.Boundary, corecontract.PermissionReadElement)
		core.owner.Lock()
		var bound bool
		readView, bound = core.registry.SourceReadViewLocked(basis.Ref, request.Element)
		core.owner.Unlock()
		if bound {
			readPermit, err = core.admission.Recheck(ctx, caller, readView, readRequirements, false)
		} else {
			readView, readPermit, err = core.admission.Authorize(ctx, caller, readRequirements)
			retainedReadView = false
		}
		if err != nil {
			return federationcontract.FederationProjectionPage{}, err
		}
	}
	var borrow *querypkg.Lease
	if basis.Source.Mode == corecontract.QueryRetained {
		borrow, err = core.queries.Borrow(caller.Subject, *basis.Snapshot)
	} else {
		borrow, err = core.queries.Capture(caller.Subject, basis.Boundary, &basis.Cut, corecontract.QueryCurrentVerified)
	}
	if err != nil {
		return federationcontract.FederationProjectionPage{}, federationprojection.ReadError(err)
	}
	defer borrow.Finish()
	projection := borrow.Projection()
	core.owner.RLock()
	if !core.owner.AvailableLocked() {
		core.owner.RUnlock()
		return federationcontract.FederationProjectionPage{}, values.PublicUnavailable
	}
	signer := core.owner.SigningLocked()
	core.owner.RUnlock()
	facts := federationprojection.New(projection.Elements, projection.Mount, projection.Attachments, projection.Transfers, signer)
	// Account for closure/projection work as well as the bounded response before
	// allocating it. A retained view can be larger than today's current state.
	working := uint64(1<<20) + uint64(projection.Elements.Counts().Anchors+projection.Elements.Counts().Entities)*512
	core.owner.Lock()
	workingReservation, reserveErr := core.registry.ReserveLocked(working)
	core.owner.Unlock()
	if reserveErr != nil {
		return federationcontract.FederationProjectionPage{}, reserveErr
	}
	defer core.registry.Release(workingReservation)
	result, last, dependencies, err := facts.Project(ctx, basis.Boundary, request, after, limit)
	if err != nil {
		return federationcontract.FederationProjectionPage{}, err
	}
	core.owner.Lock()
	defer core.owner.Unlock()
	var exact values.ElementID
	if request.Kind == "read" || request.Kind == "read-at" {
		exact = request.Element
	}
	if err := core.queries.CheckLocked(borrow, exact); err != nil {
		return federationcontract.FederationProjectionPage{}, federationprojection.ReadError(err)
	}
	current, err := accessresolve.ReadLocked(core.owner, core.registry, caller, accessvalue.TokenJSON(token))
	if err != nil || current.Ref != basis.Ref {
		return federationcontract.FederationProjectionPage{}, values.PublicInvalidContext
	}
	if err := core.checkFederationBasisLocked(ctx, caller, source, basis, permit, denialPermit); err != nil {
		return federationcontract.FederationProjectionPage{}, err
	}
	if request.Kind == "read-at" {
		// Concurrent exact reads cannot replace the first retained Read view.
		if bound, ok := core.registry.SourceReadViewLocked(basis.Ref, request.Element); ok {
			readView = bound
		}
		if err := core.admission.Validate(ctx, caller, readView, readRequirements, false, readPermit); err != nil {
			return federationcontract.FederationProjectionPage{}, err
		}
		if _, ok := core.registry.SourceReadViewLocked(basis.Ref, request.Element); !ok {
			if _, err := core.registry.BindSourceReadViewLocked(basis.Ref, request.Element, readView); err != nil {
				return federationcontract.FederationProjectionPage{}, err
			}
			retainedReadView = core.registry.OwnAuthorizationReleaseLocked(basis.Ref, core.admission.PermitRelease(readPermit))
		}
	}
	if err := core.bindFederationObservationLocked(basis, facts, request, result); err != nil {
		return federationcontract.FederationProjectionPage{}, err
	}
	if err := core.registry.BindSourceDependenciesLocked(basis.Ref, core.owner.FactsLocked().Mount, dependencies); err != nil {
		return federationcontract.FederationProjectionPage{}, err
	}
	if ctx.Err() != nil {
		return federationcontract.FederationProjectionPage{}, values.PublicUnavailable
	}
	result.Basis, result.Purpose = token, basis.Source.Request.Purpose
	if request.Kind == "query" {
		result.Coverage = federationCoverageStamp(basis, conditions, result)
		for i := range result.Parents {
			p := &result.Parents[i]
			p.Adjacency = federationStamp(basis, "ucupa.core.parent-adjacency", result.Coverage[:], []byte(p.Parent), p.Adjacency[:])
		}
	}
	if last != "" {
		page, err := core.registry.IssueContinuationLocked(basis.Ref, last, conditions)
		if err != nil {
			return federationcontract.FederationProjectionPage{}, err
		}
		t := page.Token
		result.Continuation = &t
	}
	return result, nil
}

func federationProjectionConditions(r federationcontract.FederationProjectionRequest, limit uint16) (string, error) {
	if limit < 1 || limit > 256 {
		return "", values.PublicInvalid
	}
	switch r.Kind {
	case "path":
		if r.Path == nil || r.Element != "" || r.Query != nil || len(r.Path.Down) > 4096 || !r.Path.Origin.Valid() {
			return "", values.PublicInvalid
		}
		for _, id := range r.Path.Down {
			if !id.Valid() {
				return "", values.PublicInvalid
			}
		}
	case "read", "read-at", "list":
		if !r.Element.Valid() || r.Path != nil || r.Query != nil {
			return "", values.PublicInvalid
		}
	case "query":
		if r.Query == nil || r.Path != nil || r.Element != "" || !r.Query.Scope.Valid() || len(r.Query.ToBasis) > geometry.MaxExactProjectionEdges {
			return "", values.PublicInvalid
		}
		q := r.Query
		if _, ok := queryrequest.CanonicalRegionQuery(corecontract.RegionQuery{StartAnchorID: q.Scope, BasisAnchorID: q.Scope, Sphere: q.Sphere, Selector: q.Selector}); !ok {
			return "", values.PublicInvalid
		}
		for _, step := range q.ToBasis {
			if !geometry.ValidPose(step.Pose) {
				return "", values.PublicInvalid
			}
		}
	default:
		return "", values.PublicInvalid
	}
	encoded, err := json.Marshal(r)
	if err != nil {
		return "", values.PublicInvalid
	}
	// Rotation's representation is private and therefore absent from ordinary
	// JSON marshaling. Bind its exact canonical components explicitly as well.
	if r.Query != nil {
		for _, step := range r.Query.ToBasis {
			encoded = append(encoded, byte(step.Pose.Rotation.Pivot()))
			for _, ratio := range step.Pose.Rotation.Ratios() {
				encoded = binary.LittleEndian.AppendUint64(encoded, math.Float64bits(ratio))
			}
		}
	}
	hash := sha256.Sum256(encoded)
	return string(hash[:]), nil
}
