package registry

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	accessobserve "ucupa/ucupa-core/internal/access/observe"
)

// ChangeGroup is the finite detached representation of one public
// invalidation group retained by a local/source queue.
type ChangeGroup struct {
	ID         values.PublicToken
	Basis      values.PublicToken
	Categories []string
	Terminal   *contract.TransitionTerminal
}

type observationQueue struct {
	active     bool
	gap        bool
	terminal   bool
	pending    bool
	selection  accessobserve.Selection
	groups     []ChangeGroup
	pendingIDs []values.ElementID
	base       uint64
}

// PreparationUse exposes only immutable authorization/binding facts and the
// opaque links needed to continue an observation.
type PreparationUse struct {
	Ref          Ref
	Token        values.PublicToken
	View         values.AuthorizationView
	Requirements []values.Requirement
	Receipt      Ref
	HasReceipt   bool
	Source       Ref
	HasSource    bool
	Initial      values.PublicToken
	HasQueue     bool
	Gap          bool
	Active       bool
	Base         uint64
	Selection    accessobserve.Selection
	ControlKey   values.FacetAcceptanceKey
}

// CursorUse is one caller-bound position in a preparation.
type CursorUse struct {
	Ref         Ref
	Token       values.PublicToken
	Preparation Ref
	Position    uint64
}

// PrepareLocalLocked creates a local queue parented by the space handle.
func (r *Registry) PrepareLocalLocked(space Ref, requirements []values.Requirement, view values.AuthorizationView, cut contract.CoreReadCut, exact values.ElementID, attachment bool) (PreparationUse, error) {
	if !r.presentLocked(space) || space.entry.token.Kind != "space-handle" {
		return PreparationUse{}, values.PublicInvalidContext
	}
	return r.prepareQueueLocked(space, Ref{}, requirements, view, cut, accessobserve.Selection{
		Boundary: space.entry.boundary, Exact: exact, IncludeAttachment: attachment,
	}, false)
}

// PrepareSourceLocked creates the source-local queue parented by its receipt.
func (r *Registry) PrepareSourceLocked(read Ref, requirements []values.Requirement, view values.AuthorizationView, cut contract.CoreReadCut, attachment bool) (PreparationUse, error) {
	if !r.presentLocked(read) || read.entry.read == nil || read.entry.read.source == nil {
		return PreparationUse{}, values.PublicInvalidContext
	}
	selection := accessobserve.Selection{
		Boundary: read.entry.boundary, IncludeAttachment: attachment,
		Federated: true, FederatedRead: read.entry.read.source.request.Intent == "read",
	}
	prepared, err := r.prepareQueueLocked(read, read, requirements, view, cut, selection, false)
	if err == nil {
		read.entry.read.preparation = prepared.Ref.entry
	}
	return prepared, err
}

// PrepareControlLocked creates the service observation queue. Releasing this
// preparation also retires its invisible parent receipt.
func (r *Registry) PrepareControlLocked(read Ref, requirements []values.Requirement, view values.AuthorizationView, cut contract.CoreReadCut, key values.FacetAcceptanceKey) (PreparationUse, error) {
	if !r.presentLocked(read) || read.entry.read == nil {
		return PreparationUse{}, values.PublicInvalidContext
	}
	prepared, err := r.prepareQueueLocked(read, Ref{}, requirements, view, cut, accessobserve.Selection{
		Boundary: read.entry.boundary, Exact: read.entry.boundary,
		IncludeAttachment: true, Control: true,
	}, true)
	if err == nil {
		prepared.Ref.entry.preparation.controlKey = key
		read.entry.read.preparation = prepared.Ref.entry
		prepared = preparationUse(prepared.Ref.entry)
	}
	return prepared, err
}

func (r *Registry) prepareQueueLocked(parent, source Ref, requirements []values.Requirement, view values.AuthorizationView, cut contract.CoreReadCut, selection accessobserve.Selection, ownsParent bool) (PreparationUse, error) {
	if !r.presentLocked(parent) {
		return PreparationUse{}, values.PublicInvalidContext
	}
	item, err := r.issueLocked(parent.entry.caller, "observation-preparation", parent.entry.boundary, parent.entry)
	if err != nil {
		return PreparationUse{}, err
	}
	r.shortenObservationLocked(item)
	item.preparation = &preparationState{
		view: view, requirements: cloneRequirements(requirements), cut: cut,
		queue: &observationQueue{selection: cloneSelection(selection)}, ownsParent: ownsParent,
	}
	if source.entry != nil {
		item.preparation.source = source.entry
	}
	return preparationUse(item), nil
}

// PrepareCompositeLocked creates the public preparation which owns the
// composite receipt and delegates its actual queue to collect.Read.
func (r *Registry) PrepareCompositeLocked(read Ref) (PreparationUse, error) {
	if !r.presentLocked(read) || read.entry.read == nil || read.entry.read.composite == nil {
		return PreparationUse{}, values.PublicInvalidContext
	}
	item, err := r.issueLocked(read.entry.caller, "observation-preparation", read.entry.boundary, read.entry)
	if err != nil {
		return PreparationUse{}, err
	}
	r.shortenObservationLocked(item)
	item.preparation = &preparationState{receipt: read.entry, ownsReceipt: true}
	read.entry.read.preparation = item
	return preparationUse(item), nil
}

// PrepareHiddenLocked creates a preparation and its single fixed zero-position
// cursor. Hidden observations never allocate a queue or disclose changes.
func (r *Registry) PrepareHiddenLocked(read Ref) (PreparationUse, CursorUse, error) {
	if !r.presentLocked(read) || read.entry.read == nil || read.entry.read.source == nil || read.entry.read.source.denial == nil {
		return PreparationUse{}, CursorUse{}, values.PublicInvalidContext
	}
	item, err := r.issueLocked(read.entry.caller, "observation-preparation", read.entry.boundary, read.entry)
	if err != nil {
		return PreparationUse{}, CursorUse{}, err
	}
	item.preparation = &preparationState{source: read.entry}
	read.entry.read.preparation = item
	cursor, err := r.issueCursorLocked(item, 0)
	if err != nil {
		r.removeLocked(item)
		return PreparationUse{}, CursorUse{}, err
	}
	item.preparation.initial = cursor
	return preparationUse(item), cursorUse(cursor), nil
}

// ResolvePreparationLocked resolves a caller-bound preparation. Missing or
// expired preparations produce GAP, as in the original public contract.
func (r *Registry) ResolvePreparationLocked(caller values.CallerContext, token values.PublicToken) (PreparationUse, error) {
	item, err := r.resolveLocked(caller, token, "observation-preparation")
	if err != nil {
		return PreparationUse{}, err
	}
	if item.preparation == nil {
		return PreparationUse{}, values.PublicGap
	}
	return preparationUse(item), nil
}

// PreparationLocked revalidates a preparation after a lock-free collector
// operation.
func (r *Registry) PreparationLocked(ref Ref) (PreparationUse, bool) {
	if r == nil || !r.presentLocked(ref) || ref.entry.preparation == nil {
		return PreparationUse{}, false
	}
	return preparationUse(ref.entry), true
}

func preparationUse(item *entry) PreparationUse {
	state := item.preparation
	result := PreparationUse{
		Ref: Ref{item}, Token: item.token,
		View: state.view, Requirements: cloneRequirements(state.requirements),
	}
	if state.receipt != nil {
		result.Receipt = Ref{state.receipt}
		result.HasReceipt = true
	}
	if state.source != nil {
		result.Source = Ref{state.source}
		result.HasSource = true
	}
	if state.initial != nil {
		result.Initial = state.initial.token
	}
	if state.queue != nil {
		result.HasQueue = true
		result.Gap = state.queue.gap
		result.Active = state.queue.active
		result.Base = state.queue.base
		result.Selection = cloneSelection(state.queue.selection)
	}
	result.ControlKey = state.controlKey
	return result
}

// ResolveCursorLocked resolves a caller-bound observation cursor.
func (r *Registry) ResolveCursorLocked(caller values.CallerContext, token values.PublicToken) (CursorUse, error) {
	item, err := r.resolveLocked(caller, token, "observation-cursor")
	if err != nil {
		return CursorUse{}, err
	}
	if item.cursor == nil || item.cursor.preparation == nil {
		return CursorUse{}, values.PublicGap
	}
	return cursorUse(item), nil
}

// CursorLocked revalidates a cursor after a lock-free collector operation.
func (r *Registry) CursorLocked(ref Ref) (CursorUse, bool) {
	if r == nil || !r.presentLocked(ref) || ref.entry.cursor == nil {
		return CursorUse{}, false
	}
	return cursorUse(ref.entry), true
}

func cursorUse(item *entry) CursorUse {
	return CursorUse{
		Ref: Ref{item}, Token: item.token,
		Preparation: Ref{item.cursor.preparation}, Position: item.cursor.position,
	}
}

func (r *Registry) issueCursorLocked(preparation *entry, position uint64) (*entry, error) {
	item, err := r.issueLocked(preparation.caller, "observation-cursor", preparation.boundary, preparation)
	if err != nil {
		return nil, err
	}
	item.cursor = &cursorState{preparation: preparation, position: position}
	return item, nil
}

// BeginLocalLocked creates or reuses the one initial cursor of a local/source
// queue.
func (r *Registry) BeginLocalLocked(preparation Ref) (values.PublicToken, error) {
	if !r.presentLocked(preparation) || preparation.entry.preparation == nil || preparation.entry.preparation.queue == nil {
		return values.PublicToken{}, values.PublicGap
	}
	state := preparation.entry.preparation
	if state.queue.gap {
		return values.PublicToken{}, values.PublicGap
	}
	if state.initial == nil {
		cursor, err := r.issueCursorLocked(preparation.entry, 0)
		if err != nil {
			return values.PublicToken{}, err
		}
		state.initial = cursor
		state.queue.active = true
	}
	return state.initial.token, nil
}

// PollLocalLocked advances a local/source queue and returns detached groups.
func (r *Registry) PollLocalLocked(cursor Ref, limit int) ([]ChangeGroup, values.PublicToken, error) {
	if !r.presentLocked(cursor) || cursor.entry.cursor == nil {
		return nil, values.PublicToken{}, values.PublicGap
	}
	position := cursor.entry.cursor.position
	prep := cursor.entry.cursor.preparation
	if !r.presentLocked(Ref{prep}) || prep.preparation == nil || prep.preparation.queue == nil {
		return nil, values.PublicToken{}, values.PublicGap
	}
	queue := prep.preparation.queue
	if queue.gap || !queue.active || position < queue.base || position > queue.base+uint64(len(queue.groups)) {
		return nil, values.PublicToken{}, values.PublicGap
	}
	// A terminal queue retains its unique evidence for rereading at the same
	// cursor; once the retained window is exhausted it ends in GAP, never an
	// empty steady page pretending the old scope still continues.
	if queue.terminal && position == queue.base+uint64(len(queue.groups)) {
		return nil, values.PublicToken{}, values.PublicGap
	}
	if consumed := position - queue.base; consumed > 0 {
		queue.groups = append([]ChangeGroup(nil), queue.groups[consumed:]...)
		queue.base = position
		r.bytes -= consumed * 512
		for _, old := range r.entries {
			if old.cursor != nil && old.cursor.preparation == prep && old.cursor.position < queue.base {
				r.removeLocked(old)
			}
		}
	}
	count := min(limit, len(queue.groups))
	groups := cloneGroups(queue.groups[:count])
	next := cursor.entry
	if count > 0 {
		nextPosition := position + uint64(count)
		for _, candidate := range r.entries {
			if candidate.cursor != nil && candidate.cursor.preparation == prep && candidate.cursor.position == nextPosition {
				next = candidate
				break
			}
		}
		if next == cursor.entry {
			var err error
			next, err = r.issueCursorLocked(prep, nextPosition)
			if err != nil {
				return nil, values.PublicToken{}, err
			}
		}
	}
	return groups, next.token, nil
}

// CompositePositionLocked creates/reuses a public cursor for the collector's
// committed numeric position and reclaims positions older than its base.
func (r *Registry) CompositePositionLocked(preparation Ref, operation string, position, base uint64) (values.PublicToken, error) {
	if !r.presentLocked(preparation) || preparation.entry.preparation == nil || preparation.entry.preparation.receipt == nil {
		return values.PublicToken{}, values.PublicGap
	}
	state := preparation.entry.preparation
	if operation == "begin-observe" && state.initial != nil {
		return state.initial.token, nil
	}
	var next *entry
	for _, candidate := range r.entries {
		if candidate.cursor != nil && candidate.cursor.preparation == preparation.entry && candidate.cursor.position == position {
			next = candidate
			break
		}
	}
	if next == nil {
		var err error
		next, err = r.issueCursorLocked(preparation.entry, position)
		if err != nil {
			return values.PublicToken{}, err
		}
	}
	if operation == "begin-observe" {
		state.initial = next
		return next.token, nil
	}
	for _, old := range r.entries {
		if old.cursor != nil && old.cursor.preparation == preparation.entry && old.cursor.position < base {
			r.removeLocked(old)
		}
	}
	return next.token, nil
}

// GapLocked invalidates and refunds one local/source queue exactly once.
func (r *Registry) GapLocked(preparation Ref) {
	if !r.presentLocked(preparation) {
		return
	}
	r.gapLocked(preparation.entry)
}

func (r *Registry) gapLocked(preparation *entry) {
	if preparation == nil || preparation.preparation == nil || preparation.preparation.queue == nil {
		return
	}
	queue := preparation.preparation.queue
	if queue.gap {
		return
	}
	r.bytes -= uint64(len(queue.groups)) * 512
	r.bytes -= uint64(len(queue.pendingIDs)) * 128
	queue.groups = nil
	queue.pendingIDs = nil
	queue.pending = false
	queue.gap = true
}

// BindSourceObservationLocked extends a source read's observation selection
// at its original cut. State and charge changes remain partial and serial.
func (r *Registry) BindSourceObservationLocked(read Ref, ids map[values.ElementID]bool, exact values.ElementID, bindExact bool, current contract.CoreReadCut, max int) error {
	if !r.presentLocked(read) || read.entry.read == nil || read.entry.read.source == nil || read.entry.read.preparation == nil {
		return values.PublicGap
	}
	prep := read.entry.read.preparation
	if !r.presentLocked(Ref{prep}) || prep.preparation == nil || prep.preparation.queue == nil {
		return values.PublicGap
	}
	queue := prep.preparation.queue
	if queue.gap {
		return values.PublicGap
	}
	selection := &queue.selection
	added := 0
	for id := range ids {
		if !selection.Path[id] {
			added++
		}
	}
	if added == 0 && !bindExact {
		return nil
	}
	if queue.active || current != read.entry.read.cut || len(queue.groups) != 0 {
		return values.PublicInvalidContext
	}
	cost := uint64(added) * 128
	if len(selection.Path)+added > max || !r.canChargeLocked(cost) {
		return values.PublicResourceExhausted
	}
	if selection.Path == nil {
		selection.Path = make(map[values.ElementID]bool)
	}
	for id := range ids {
		selection.Path[id] = true
	}
	prep.bytes += cost
	r.bytes += cost
	if bindExact {
		selection.Exact = exact
	}
	return nil
}

func cloneSelection(selection accessobserve.Selection) accessobserve.Selection {
	result := selection
	if selection.Path != nil {
		result.Path = make(map[values.ElementID]bool, len(selection.Path))
		for id, present := range selection.Path {
			result.Path[id] = present
		}
	}
	return result
}

func cloneGroups(groups []ChangeGroup) []ChangeGroup {
	result := make([]ChangeGroup, len(groups))
	for i, group := range groups {
		// The terminal statement is an immutable published value; sharing its
		// pointer copies the evidence without a second mutable owner.
		result[i] = ChangeGroup{ID: group.ID, Basis: group.Basis, Categories: append([]string(nil), group.Categories...), Terminal: group.Terminal}
	}
	return result
}
