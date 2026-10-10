// Package source executes owner-local federation reading. Facts, retained
// snapshots, admission and public lifetimes each remain in their one owner.
package source

import (
	"context"
	"ucupa/shared/values"
	accesspkg "ucupa/ucupa-core/internal/access"
	"ucupa/ucupa-core/internal/access/observation"
	accessregistry "ucupa/ucupa-core/internal/access/registry"
	"ucupa/ucupa-core/internal/commit"
	"ucupa/ucupa-core/internal/query"
)

type Operations struct {
	owner        *commit.Owner
	admission    *accesspkg.Admission
	registry     *accessregistry.Registry
	queries      *query.Operations
	observations *observation.Operations
}

func New(owner *commit.Owner, admission *accesspkg.Admission, resources *accessregistry.Registry, queries *query.Operations, observations *observation.Operations) *Operations {
	return &Operations{owner: owner, admission: admission, registry: resources, queries: queries, observations: observations}
}

func (core *Operations) AuthorityRef() values.FactAuthorityRef {
	if core == nil {
		return ""
	}
	return core.owner.AuthorityRef()
}

func (core *Operations) enterFederationCall(ctx context.Context) (func(), error) {
	if core == nil || core.registry == nil {
		return nil, values.PublicUnavailable
	}
	if ctx == nil {
		return nil, values.PublicInvalid
	}
	if ctx.Err() != nil {
		return nil, values.PublicUnavailable
	}
	if accesspkg.HasAdmission(ctx, core.admission) {
		return func() {}, nil
	}
	if err := core.admission.Acquire(); err != nil {
		return nil, err
	}
	return core.admission.Release, nil
}
