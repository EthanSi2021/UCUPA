package collect

import (
	"context"
	"slices"
	"sort"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	federationcontract "ucupa/ucupa-core/contract/federation"
)

type row struct {
	id     values.ElementID
	record *federationcontract.FederationRecord
	match  *federationcontract.FederationMatch
	cost   uint64
}

func (r *Read) page(ctx context.Context, after values.ElementID, limit int) (Page, error) {
	if r.operation == "read" {
		page, err := r.target.part().Project(ctx, federationcontract.FederationProjectionRequest{Kind: "read", Element: r.target.id()}, nil, 1)
		if err != nil {
			return Page{}, err
		}
		if len(page.Records) != 1 || page.Records[0].ID != r.target.id() || page.Records[0].Attachment == nil {
			return Page{}, values.PublicInvalidContext
		}
		record := cloneRecord(page.Records[0])
		return Page{Record: &record}, nil
	}
	rows := make([]row, 0, limit+1)
	for _, plan := range r.plans {
		partRows, err := r.scan(ctx, plan, after, limit+1)
		if err != nil {
			return Page{}, err
		}
		rows = append(rows, partRows...)
		var working uint64
		for _, row := range rows {
			working += row.cost
		}
		if working > 8<<20 {
			return Page{}, values.PublicResourceExhausted
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].id < rows[j].id })
		for i := 1; i < len(rows); i++ {
			if rows[i].id == rows[i-1].id {
				return Page{}, values.PublicConflict
			}
		}
		if len(rows) > limit+1 {
			rows = rows[:limit+1]
		}
	}
	more := len(rows) > limit
	var last values.ElementID
	if more {
		rows = rows[:limit]
		last = rows[len(rows)-1].id
	}
	result := Page{Partial: more || r.gap != corecontract.QueryNoGap, Gap: r.gap, Scope: r.scope, ActualUpDepth: r.actual, Last: last}
	if r.operation == "query" {
		result.Matches = make([]federationcontract.FederationMatch, 0, len(rows))
		for _, row := range rows {
			result.Matches = append(result.Matches, *row.match)
		}
	} else {
		result.Records = make([]federationcontract.FederationRecord, 0, len(rows))
		for _, row := range rows {
			result.Records = append(result.Records, *row.record)
		}
	}
	return result, nil
}

func (r *Read) scan(ctx context.Context, plan projection, after values.ElementID, maximum int) ([]row, error) {
	rows := make([]row, 0, maximum)
	var cursor *values.PublicToken
	var previous values.ElementID
	var working uint64
	for pages := 0; pages < 4096; pages++ {
		page, err := plan.part.Project(ctx, plan.request, cursor, 256)
		if err != nil {
			return nil, err
		}
		if page.Basis != plan.part.Part().Basis || page.Purpose != r.sources.request.Purpose {
			return nil, values.PublicInvalidContext
		}
		if plan.request.Kind == "query" && (plan.coverage == ([32]byte{}) || page.Coverage != plan.coverage || !slices.Equal(page.Parents, plan.parents)) {
			return nil, values.PublicInvalidContext
		}
		if page.Gap > r.gap {
			return nil, values.PublicInvalidContext
		}
		add := func(id values.ElementID, path []values.ElementID) ([]values.ElementID, uint64, bool, error) {
			if id <= previous {
				return nil, 0, false, values.PublicInvalidContext
			}
			previous = id
			if id <= after || len(rows) == maximum {
				return nil, 0, false, nil
			}
			cost := uint64(len(path)+len(r.prefix[plan.part.Part().Basis.Value]))*128 + 4096
			if cost > 4<<20 || working > (4<<20)-cost {
				return nil, 0, false, values.PublicResourceExhausted
			}
			working += cost
			full, err := r.fullPath(plan.part, path)
			return full, cost, true, err
		}
		if r.operation == "query" {
			for _, source := range page.Matches {
				full, cost, include, err := add(source.ID, source.Path)
				if err != nil {
					return nil, err
				}
				if include {
					match := source
					match.Path = full
					rows = append(rows, row{id: source.ID, match: &match, cost: cost})
				}
			}
		} else {
			for _, source := range page.Records {
				full, cost, include, err := add(source.ID, source.Path)
				if err != nil {
					return nil, err
				}
				if include {
					record := cloneRecord(source)
					record.Path = full
					rows = append(rows, row{id: source.ID, record: &record, cost: cost})
				}
			}
		}
		if len(rows) == maximum || page.Continuation == nil {
			return rows, nil
		}
		if cursor != nil && *cursor == *page.Continuation || len(page.Matches)+len(page.Records) == 0 {
			return nil, values.PublicInvalidContext
		}
		copy := *page.Continuation
		cursor = &copy
	}
	return nil, values.PublicResourceExhausted
}

func cloneRecord(source federationcontract.FederationRecord) federationcontract.FederationRecord {
	source.Path = append([]values.ElementID(nil), source.Path...)
	if source.Pose != nil {
		copy := *source.Pose
		source.Pose = &copy
	}
	if source.Attachment != nil {
		copy := *source.Attachment
		source.Attachment = &copy
	}
	if source.Mount != nil {
		copy := *source.Mount
		source.Mount = &copy
	}
	return source
}
