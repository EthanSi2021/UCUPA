// Package coordinator bridges one admitted public request to the existing
// cross-source collector. It owns no source graph, contexts, queues or snapshots.
package coordinator

import (
	"ucupa/ucupa-core/internal/access"
	"ucupa/ucupa-core/internal/access/observation"
	"ucupa/ucupa-core/internal/access/registry"
	"ucupa/ucupa-core/internal/commit"
	"ucupa/ucupa-core/internal/query/federation/source"
)

type Operations struct {
	owner        *commit.Owner
	admission    *access.Admission
	registry     *registry.Registry
	sources      *source.Operations
	observations *observation.Operations
}

// New wires the same components already used by local source operations.
// Read and Observe are entered only after the public envelope's admission.
func New(owner *commit.Owner, admission *access.Admission, resources *registry.Registry, sources *source.Operations, observations *observation.Operations) *Operations {
	return &Operations{owner: owner, admission: admission, registry: resources, sources: sources, observations: observations}
}
