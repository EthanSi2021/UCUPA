package collect

import (
	"context"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
)

// New captures and seals one complete source set. On failure it releases every
// source it accepted; no partial recipe escapes.
func New(ctx context.Context, config Config) (*Read, error) {
	if ctx == nil || config.Request.Operation != config.PartRequest.Intent {
		return nil, values.PublicInvalid
	}
	sourceSet, err := newSources(config)
	if err != nil {
		return nil, err
	}
	r := &Read{
		sources:   sourceSet,
		boundary:  config.PartRequest.Boundary,
		operation: config.Request.Operation,
		prefix:    map[string][]values.ElementID{sourceSet.parts[0].Part().Basis.Value: nil},
		required:  map[string]bool{sourceSet.parts[0].Part().Basis.Value: true},
	}
	keep := false
	defer func() {
		if !keep {
			_ = sourceSet.retire()
		}
	}()
	switch r.operation {
	case "query":
		if config.Request.Query == nil {
			return nil, values.PublicInvalid
		}
		err = r.planQuery(ctx, *config.Request.Query)
	case "read", "list":
		r.target, err = r.resolve(ctx, config.Request.Address)
		if err == nil && r.operation == "list" {
			err = r.planList(ctx)
		}
	default:
		err = values.PublicInvalid
	}
	if err != nil {
		return nil, err
	}
	if err = sourceSet.seal(ctx); err != nil {
		return nil, err
	}
	keep = true
	return r, nil
}

// Collect executes one bounded page against the sealed source set. It performs
// source checks before and after projection; the outer owner must still repeat
// its public context and authorization checks before disclosure.
func (r *Read) Collect(ctx context.Context, after values.ElementID, limit int, prepare bool) (Page, *Lease, error) {
	if r == nil || ctx == nil || limit < 1 {
		return Page{}, nil, values.PublicInvalid
	}
	r.mu.Lock()
	keep := false
	defer func() {
		if !keep {
			r.mu.Unlock()
		}
	}()
	if r.closed {
		return Page{}, nil, values.PublicInvalidContext
	}
	if err := r.sources.check(ctx); err != nil {
		return Page{}, nil, err
	}
	page, err := r.page(ctx, after, limit)
	if err != nil {
		return Page{}, nil, err
	}
	if err := r.sources.check(ctx); err != nil {
		return Page{}, nil, err
	}
	if prepare {
		if r.gap != corecontract.QueryNoGap {
			return Page{}, nil, values.PublicUnavailable
		}
		if _, err := r.beginObservation(ctx); err != nil {
			return Page{}, nil, err
		}
	}
	page.Closure, err = r.closureCommitment()
	if err != nil {
		return Page{}, nil, err
	}
	page.SourceGraph, err = r.sourceGraph()
	if err != nil {
		return Page{}, nil, err
	}
	page.ReadMode = r.sources.parts[0].Part().ReadMode
	keep = true
	return page, &Lease{read: r}, nil
}
