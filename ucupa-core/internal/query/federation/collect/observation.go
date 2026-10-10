package collect

import (
	"context"
	"errors"

	"ucupa/shared/values"
	federationcontract "ucupa/ucupa-core/contract/federation"
)

// observation positions are coordinator-local, never WorldRevisions. Source
// committed order remains independent and source cursors stay private.
type observation struct {
	sources  *sources
	cursors  []values.PublicToken
	bases    []values.PublicToken
	position uint64
	start    int
	pending  *observationBatch
	gap      bool
}

type observationBatch struct {
	position uint64
	limit    uint16
	groups   []federationcontract.FederationChangeGroup
	cursors  []values.PublicToken
	start    int
}

type observationDelivery struct {
	base           uint64
	sourcePosition uint64
	groups         []federationcontract.FederationChangeGroup
}

func (r *Read) beginObservation(ctx context.Context) (*observation, error) {
	if ctx == nil {
		return nil, values.PublicInvalid
	}
	if r.closed || r.sources == nil {
		return nil, values.PublicInvalidContext
	}
	if r.observation != nil {
		return r.observation, r.observation.check(ctx)
	}
	cost := uint64(len(r.sources.parts))*4096 + 65536
	if err := r.charge(cost); err != nil {
		return nil, err
	}
	o, err := beginObservation(ctx, r.sources)
	if err != nil {
		r.used -= cost
		return nil, err
	}
	r.observation = o
	return o, nil
}

func beginObservation(ctx context.Context, sourceSet *sources) (*observation, error) {
	if ctx == nil {
		return nil, values.PublicInvalid
	}
	if sourceSet == nil || sourceSet.closed || !sourceSet.sealed || !sourceSet.request.PrepareObserve || len(sourceSet.parts) == 0 || len(sourceSet.parts) > 256 {
		return nil, values.PublicInvalidContext
	}
	o := &observation{sources: sourceSet}
	for _, part := range sourceSet.parts {
		page, err := part.Observe(ctx, federationcontract.FederationObserveRequest{Operation: "begin"})
		if err != nil {
			return nil, err
		}
		if !validObservationPage(part, page, 0) {
			return nil, values.PublicInvalidContext
		}
		o.cursors = append(o.cursors, page.Cursor)
		o.bases = append(o.bases, part.Part().Basis)
	}
	if err := o.check(ctx); err != nil {
		return nil, err
	}
	return o, nil
}

func (o *observation) check(ctx context.Context) error {
	if o.gap || o.sources.closed || !o.sources.sealed || len(o.bases) != len(o.sources.parts) {
		return values.PublicGap
	}
	if ctx.Err() != nil {
		return values.PublicUnavailable
	}
	if err := o.sources.checkEvidenceGraph(); err != nil {
		return o.failure(err)
	}
	for index, part := range o.sources.parts {
		if part.Part().Basis != o.bases[index] {
			o.gap = true
			return values.PublicGap
		}
		page, err := part.Observe(ctx, federationcontract.FederationObserveRequest{Operation: "begin"})
		if err != nil {
			return o.failure(err)
		}
		if !validObservationPage(part, page, 0) {
			o.gap = true
			return values.PublicGap
		}
	}
	if ctx.Err() != nil {
		return values.PublicUnavailable
	}
	if err := o.sources.checkEvidenceGraph(); err != nil {
		return o.failure(err)
	}
	return nil
}

func (o *observation) failure(err error) error {
	if errors.Is(err, values.PublicGap) || errors.Is(err, values.PublicInvalidContext) || errors.Is(err, values.PublicDenied) {
		o.gap = true
	}
	return err
}

func (o *observation) poll(ctx context.Context, position uint64, limit uint16) (observationBatch, error) {
	if ctx == nil {
		return observationBatch{}, values.PublicInvalid
	}
	if limit == 0 || limit > 256 {
		return observationBatch{}, values.PublicInvalid
	}
	if err := o.check(ctx); err != nil {
		return observationBatch{}, err
	}
	if position != o.position {
		if o.pending == nil || position != o.pending.position {
			return observationBatch{}, values.PublicGap
		}
		o.cursors = append([]values.PublicToken(nil), o.pending.cursors...)
		o.position, o.start = position, o.pending.start
		o.pending = nil
	}
	if o.pending != nil {
		if limit != o.pending.limit {
			return observationBatch{}, values.PublicInvalidContext
		}
		return cloneObservationBatch(*o.pending), nil
	}
	batch := observationBatch{position: o.position, limit: limit, cursors: append([]values.PublicToken(nil), o.cursors...), start: o.start}
	for step := 0; step < len(o.sources.parts) && len(batch.groups) < int(limit); step++ {
		index := (o.start + step) % len(o.sources.parts)
		part := o.sources.parts[index]
		page, err := part.Observe(ctx, federationcontract.FederationObserveRequest{Operation: "poll", Cursor: &o.cursors[index], Limit: 1})
		if err != nil {
			return observationBatch{}, o.failure(err)
		}
		if !validObservationPage(part, page, 1) || (len(page.Groups) > 0) == (page.Cursor == o.cursors[index]) {
			o.gap = true
			return observationBatch{}, values.PublicGap
		}
		batch.cursors[index] = page.Cursor
		batch.groups = append(batch.groups, page.Groups...)
		batch.start = (index + 1) % len(o.sources.parts)
	}
	if err := o.check(ctx); err != nil {
		return observationBatch{}, err
	}
	if len(batch.groups) > 0 {
		if o.position == ^uint64(0) {
			o.gap = true
			return observationBatch{}, values.PublicGap
		}
		batch.position++
		saved := cloneObservationBatch(batch)
		o.pending = &saved
	}
	return cloneObservationBatch(batch), nil
}

// Observe owns collection and delivery state under Read.mu. The public access
// owner supplies and persists only the numeric position bound to its cursor.
func (r *Read) Observe(ctx context.Context, operation string, position uint64, limit int) (ObservationPage, *Lease, error) {
	if r == nil || ctx == nil || limit < 1 || limit > 256 {
		return ObservationPage{}, nil, values.PublicInvalid
	}
	r.mu.Lock()
	keep := false
	defer func() {
		if !keep {
			r.mu.Unlock()
		}
	}()
	if r.closed {
		return ObservationPage{}, nil, values.PublicGap
	}
	o, err := r.beginObservation(ctx)
	if err != nil {
		return ObservationPage{}, nil, err
	}
	if r.delivery == nil {
		r.delivery = &observationDelivery{}
	}
	d := r.delivery
	result := ObservationPage{}
	switch operation {
	case "begin-observe":
		// The initial public cursor is owned by the outer preparation context.
		result.Position = 0
		result.Base = d.base
	case "poll":
		if position < d.base || position > d.base+uint64(len(d.groups)) {
			return ObservationPage{}, nil, values.PublicGap
		}
		consumed := position - d.base
		d.groups = append([]federationcontract.FederationChangeGroup(nil), d.groups[consumed:]...)
		d.base = position
		if len(d.groups) == 0 {
			batch, err := o.poll(ctx, d.sourcePosition, 256)
			if err != nil {
				return ObservationPage{}, nil, err
			}
			d.groups, d.sourcePosition = batch.groups, batch.position
		}
		count := min(limit, len(d.groups))
		result.Groups = cloneGroups(d.groups[:count])
		result.Position = d.base + uint64(count)
		result.Base = d.base
		if result.Position < d.base {
			return ObservationPage{}, nil, values.PublicGap
		}
	default:
		return ObservationPage{}, nil, values.PublicInvalid
	}
	// Recheck cached batches too: a changed source view cannot disclose old
	// evidence, but this never acquires a new spatial cut.
	if err := o.check(ctx); err != nil {
		return ObservationPage{}, nil, err
	}
	keep = true
	return result, &Lease{read: r}, nil
}

func validObservationPage(part federationcontract.FederationReadConnection, page federationcontract.FederationObservationPage, limit int) bool {
	if part.Part().Hidden && len(page.Groups) != 0 {
		return false
	}
	owner := part.Part().Basis.Owner
	if !page.Cursor.Valid() || page.Cursor.Kind != "observation-cursor" || page.Cursor.Owner != owner || len(page.Groups) > limit {
		return false
	}
	for _, group := range page.Groups {
		if !group.ID.Valid() || group.ID.Kind != "change-id" || group.ID.Owner != owner || !group.Basis.Valid() || group.Basis.Kind != "change-basis" || group.Basis.Owner != owner || len(group.Categories) == 0 || len(group.Categories) > 2 {
			return false
		}
		spatial, attachment, terminal := false, false, false
		for _, category := range group.Categories {
			switch category {
			case "spatial":
				if spatial {
					return false
				}
				spatial = true
			case "attachment":
				if attachment {
					return false
				}
				attachment = true
			case "terminal":
				if terminal {
					return false
				}
				terminal = true
			default:
				return false
			}
		}
		// Terminal evidence is one exclusive group with its statement attached;
		// ordinary groups never assert terminal evidence.
		if terminal && (len(group.Categories) != 1 || group.Terminal == nil) || !terminal && group.Terminal != nil {
			return false
		}
	}
	return true
}

func cloneObservationBatch(batch observationBatch) observationBatch {
	batch.cursors = append([]values.PublicToken(nil), batch.cursors...)
	batch.groups = cloneGroups(batch.groups)
	return batch
}

func cloneGroups(groups []federationcontract.FederationChangeGroup) []federationcontract.FederationChangeGroup {
	result := append([]federationcontract.FederationChangeGroup(nil), groups...)
	for i := range result {
		result[i].Categories = append([]string(nil), result[i].Categories...)
	}
	return result
}
