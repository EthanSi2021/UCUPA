package collect

import (
	"context"

	"ucupa/shared/values"
	federationcontract "ucupa/ucupa-core/contract/federation"
)

// ReadAt derives an exact basis through this Read's already-sealed source set.
// It cannot open a source which was absent from the original collection.
func (r *Read) ReadAt(ctx context.Context, address Address, prior []*ExactBasis) (ExactResult, *ExactBasis, *Lease, error) {
	if r == nil || ctx == nil {
		return ExactResult{}, nil, nil, values.PublicInvalid
	}
	r.mu.Lock()
	keep := false
	defer func() {
		if !keep {
			r.mu.Unlock()
		}
	}()
	if r.closed {
		return ExactResult{}, nil, nil, values.PublicInvalidContext
	}
	if err := r.sources.check(ctx); err != nil {
		return ExactResult{}, nil, nil, err
	}
	if err := r.checkExactBases(ctx, prior); err != nil {
		return ExactResult{}, nil, nil, err
	}
	resolver := &Read{sources: r.sources, prefix: r.prefix, required: make(map[string]bool), boundary: r.boundary}
	target, err := resolver.resolve(ctx, address)
	if err != nil {
		return ExactResult{}, nil, nil, err
	}
	page, err := target.part().Project(ctx, federationcontract.FederationProjectionRequest{Kind: "read-at", Element: target.id()}, nil, 1)
	if err != nil {
		return ExactResult{}, nil, nil, err
	}
	if page.Basis != target.part().Part().Basis || len(page.Records) != 1 || page.Records[0].ID != target.id() || page.Records[0].Attachment == nil {
		return ExactResult{}, nil, nil, values.PublicInvalidContext
	}
	if err := r.sources.check(ctx); err != nil {
		return ExactResult{}, nil, nil, err
	}
	if err := r.checkExactBases(ctx, prior); err != nil {
		return ExactResult{}, nil, nil, err
	}
	closure, err := r.closureCommitment()
	if err != nil {
		return ExactResult{}, nil, nil, err
	}
	graph, err := r.sourceGraph()
	if err != nil {
		return ExactResult{}, nil, nil, err
	}
	result := ExactResult{Record: cloneRecord(page.Records[0]), Closure: closure, SourceGraph: graph, ReadMode: r.sources.parts[0].Part().ReadMode}
	basis := &ExactBasis{read: r, part: target.part(), element: target.id()}
	keep = true
	return result, basis, &Lease{read: r}, nil
}

func (r *Read) checkExactBases(ctx context.Context, bases []*ExactBasis) error {
	for _, basis := range bases {
		if basis == nil || basis.read != r || basis.part == nil || !basis.element.Valid() {
			return values.PublicInvalidContext
		}
		if _, err := basis.part.Project(ctx, federationcontract.FederationProjectionRequest{Kind: "read-at", Element: basis.element}, nil, 1); err != nil {
			return err
		}
	}
	return nil
}
