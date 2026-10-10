// Package local executes public read/list/query against one Core's fixed read
// basis. It consumes the existing snapshot and context owners; it owns no facts,
// registry, retained cuts, admission counter or mutex.
package local

import (
	"ucupa/ucupa-core/internal/access"
	"ucupa/ucupa-core/internal/access/registry"
	"ucupa/ucupa-core/internal/commit"
	"ucupa/ucupa-core/internal/query"
)

type Operations struct {
	owner     *commit.Owner
	admission *access.Admission
	access    *registry.Registry
	queries   *query.Operations
}

func New(owner *commit.Owner, admission *access.Admission, resources *registry.Registry, queries *query.Operations) *Operations {
	return &Operations{owner: owner, admission: admission, access: resources, queries: queries}
}
