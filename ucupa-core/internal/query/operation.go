package query

import (
	"time"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	commitpkg "ucupa/ucupa-core/internal/commit"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	queryrequest "ucupa/ucupa-core/internal/query/request"
	queryscan "ucupa/ucupa-core/internal/query/scan"
	"ucupa/ucupa-core/internal/token"
)

// resolveLocalScope privately resolves only authorization context. The
// caller must gate every state-derived failure before returning it.

func resolveLocalScope(elements elementfact.View, mounts mountfact.View, start values.ElementID, depth int64) (values.ElementID, uint64, error) {
	current := start
	for n := uint64(0); ; n++ {
		if mountfact.IsExternalParent(mounts, current) {
			return "", 0, contract.ErrMountContextRequired
		}
		anchor, ok := elements.Anchor(current)
		if !ok {
			return "", 0, contract.ErrInvalid
		}
		if n > uint64(elements.Counts().Anchors) {
			return "", 0, contract.ErrStore
		}
		if anchor.Root() || (depth >= 0 && n == uint64(depth)) {
			return current, n, nil
		}
		current = anchor.Parent
	}
}

// QueryRegion fixes Scope, query and cut for the page chain. Off means current
// verification, not disabled pagination; Retained alone survives later writes.
func (core *Operations) QueryRegion(actor values.PrincipalRef, query contract.RegionQuery, at *contract.CoreReadCut, limit uint16, continuation *contract.QueryContinuation) (contract.QueryPage, error) {
	query, valid := queryrequest.CanonicalRegionQuery(query)
	if core == nil || !valid || !token.ValidOptionalCut(at) || limit == 0 || limit > 256 ||
		(continuation != nil && (at != nil || !queryrequest.ValidQueryContinuation(*(continuation)))) {
		return contract.QueryPage{}, contract.ErrInvalid
	}
	root, depth, cut, decision := core.initialQueryScope(query, continuation)
	permissionID := root
	if !permissionID.Valid() {
		permissionID = query.StartAnchorID
	}
	permission := core.requirement(contract.PermissionQueryRegion, permissionID)
	approval, err := core.authorize(actor, permission)
	if err != nil {
		return contract.QueryPage{}, err
	}
	if decision != nil {
		approval, err = core.authorize(actor, permission)
		if err != nil {
			return contract.QueryPage{}, err
		}
		if err := core.checkQueryCurrent(cut, false, approval); err != nil {
			return contract.QueryPage{}, err
		}
		return contract.QueryPage{}, decision
	}
	mode := core.Mode()
	if continuation != nil {
		if queryrequest.QueryActor(*(continuation)) != actor || queryrequest.QueryQuery(*(continuation)) != query || queryrequest.QueryOwner(*(continuation)) != core.authority {
			decision = contract.ErrInvalid
		} else if queryrequest.QueryRun(*(continuation)) != core.reads.run {
			decision = contract.ErrReadCutExpired
		}
		mode = queryrequest.QueryMode(*(continuation))
	} else if at != nil {
		if token.Scope(*at) != token.Scope(cut) {
			decision = contract.ErrInvalid
		} else if *at != cut {
			decision = contract.ErrReadCutExpired
		}
	}
	if decision != nil {
		approval, err = core.authorize(actor, permission)
		if err != nil {
			return contract.QueryPage{}, err
		}
		return contract.QueryPage{}, decision
	}
	var borrow *Lease
	if continuation != nil && mode == contract.QueryRetained {
		borrow, decision = core.Borrow(actor, *queryrequest.QuerySnapshot(*(continuation)))
	} else {
		borrow, decision = core.Capture(actor, root, &cut, mode)
	}
	keep := false
	if borrow != nil {
		defer borrow.Finish()
		if continuation == nil && borrow.snapshot != nil {
			defer func() {
				if !keep {
					_ = core.ReleaseReadSnapshot(actor, *borrow.snapshot)
				}
			}()
		}
	}
	var page contract.QueryPage
	if decision == nil {
		decision = validateReadView(borrow.view)
	}
	if decision == nil {
		resolved, actual, err := resolveLocalScope(borrow.view.projection.Elements, borrow.view.projection.Mount, query.StartAnchorID, query.UpDepth)
		if err != nil {
			decision = err
		} else if resolved != root || actual != depth {
			decision = contract.ErrInvalid
		}
	}
	if decision == nil {
		scan := query
		scan.StartAnchorID = root
		scan.UpDepth = 0
		page, decision = queryscan.Coverage(borrow.view.projection.Elements, borrow.view.projection.Mount, borrow.view.projection.Transfers, borrow.view.projection.Scope, scan, cut, limit, continuation, false)
		if decision == nil {
			gap, err := queryscan.TransferGap(borrow.view.projection.Elements, borrow.view.projection.Transfers, scan)
			decision = err
			if gap {
				page.Status, page.Gap = contract.QueryPartial, contract.QueryUnresolvedCoverage
			}
		}
		if decision == nil {
			page.ScopeAnchorID, page.ActualUpDepth, page.ReadMode = root, depth, mode
			if borrow.snapshot != nil {
				token := *borrow.snapshot
				page.Snapshot = &token
			}
			if page.Continuation != nil {
				*page.Continuation = queryrequest.BindQueryContinuation(*page.Continuation, core.authority, query, actor, core.reads.run, mode, root, depth, page.Snapshot)
			}
		}
	}
	approval, err = core.authorize(actor, permission)
	if err != nil {
		return contract.QueryPage{}, err
	}
	if borrow != nil {
		core.owner.RLock()
		if !core.settled(approval) {
			core.owner.RUnlock()
			return contract.QueryPage{}, contract.ErrUnavailable
		}
		if err := core.CheckLocked(borrow, ""); err != nil {
			core.owner.RUnlock()
			return contract.QueryPage{}, err
		}
		core.owner.RUnlock()
	} else if err := core.checkQueryCurrent(cut, continuation != nil, approval); err != nil {
		return contract.QueryPage{}, err
	}
	if decision != nil {
		return contract.QueryPage{}, decision
	}
	keep = true
	return page, nil
}

func (core *Operations) checkQueryCurrent(cut contract.CoreReadCut, issued bool, approval contract.RealmAttachment) error {
	core.owner.RLock()
	defer core.owner.RUnlock()
	if core.owner.ClosedLocked() {
		if issued {
			return contract.ErrReadCutExpired
		}
		return contract.ErrStore
	}
	if !core.owner.AvailableLocked() {
		return contract.ErrStore
	}
	if !core.settled(approval) {
		return contract.ErrUnavailable
	}
	if core.owner.CurrentCutLocked() != cut {
		return contract.ErrReadCutExpired
	}
	return nil
}

func (core *Operations) initialQueryScope(query contract.RegionQuery, c *contract.QueryContinuation) (values.ElementID, uint64, contract.CoreReadCut, error) {
	if c != nil {
		return queryrequest.QueryRoot(*(c)), queryrequest.QueryDepth(*(c)), queryrequest.QueryCut(*(c)), nil
	}
	core.owner.RLock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		core.owner.RUnlock()
		return "", 0, contract.CoreReadCut{}, contract.ErrStore
	}
	cut := core.owner.CurrentCutLocked()
	visibility := core.owner.SourceVisibilityLocked()
	if visibility.Inactive(query.StartAnchorID) || visibility.Inactive(query.BasisAnchorID) {
		core.owner.RUnlock()
		return "", 0, cut, contract.ErrUnavailable
	}
	// Only the immutable parent map and external-parent descriptor are needed
	// for private Scope resolution. Even a deep walk runs outside the Operations lock
	// and does not retain the state's operation or commit journals.
	elements := core.owner.AnchorProjectionLocked()
	mounts := mountfact.IncomingOnly(core.owner.FactsLocked().Mount.Incoming()).View
	core.owner.RUnlock()
	root, depth, err := resolveLocalScope(elements, mounts, query.StartAnchorID, query.UpDepth)
	return root, depth, cut, err
}

// Operations owns local query execution and the one bounded snapshot registry.
// It borrows immutable facts from the original commit owner, never a second state.
type Operations struct {
	owner      *commitpkg.Owner
	authority  values.FactAuthorityRef
	authorizer contract.Authorizer
	reads      *snapshotRegistry
}

func New(owner *commitpkg.Owner, authorizer contract.Authorizer, run [32]byte, settings Settings) *Operations {
	return &Operations{owner: owner, authority: owner.AuthorityRef(), authorizer: authorizer, reads: newSnapshotRegistry(run, settings)}
}

// authorize resolves one typed requirement set through the owner's current
// durable Realm contact. An unreachable or unverifiable Realm is never
// compressed into a denial. The returned approval is the contact generation
// captured before the authorization: consuming it at a later final read or
// commit boundary requires the same generation to still be current.
func (core *Operations) authorize(actor values.PrincipalRef, requirements ...values.Requirement) (contract.RealmAttachment, error) {
	if core == nil || core.authorizer == nil {
		return contract.RealmAttachment{}, contract.ErrStore
	}
	if !actor.Valid() {
		return contract.RealmAttachment{}, contract.ErrDenied
	}
	approval, ok := core.owner.CurrentContact()
	if !ok {
		return contract.RealmAttachment{}, contract.ErrUnavailable
	}
	switch core.authorizer.AuthorizePrincipal(actor, requirements...) {
	case values.AuthorizationValid:
		return approval, nil
	case values.AuthorizationDenied:
		return contract.RealmAttachment{}, contract.ErrDenied
	default:
		return contract.RealmAttachment{}, contract.ErrUnavailable
	}
}

// settled reports whether an authorization approval's contact generation is
// still the current one at a final read or commit boundary.
func (core *Operations) settled(approval contract.RealmAttachment) bool {
	current, ok := core.owner.CurrentContact()
	return ok && current == approval
}
func (core *Operations) requirement(permission values.PermissionID, element values.ElementID) values.Requirement {
	return values.Requirement{Permission: permission, Scope: values.CoreElementScope(core.authority, element)}
}
func (core *Operations) Run() [32]byte      { return core.reads.run }
func (core *Operations) Settings() Settings { return core.reads.settings }
func (core *Operations) Now() time.Time     { return core.reads.now() }

// Close releases query resources after the shared commit owner stops admission.
// It does not close that owner or the Store.
func (core *Operations) Close() { core.reads.close() }
func (core *Operations) Mode() contract.QueryReadMode {
	if core.reads.settings.Retention > 0 {
		return contract.QueryRetained
	}
	return contract.QueryCurrentVerified
}
