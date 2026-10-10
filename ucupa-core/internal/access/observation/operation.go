// Package observation executes the one local/source observation state machine.
// It owns no queues, capacity, snapshots or facts: those remain in the shared
// Registry and commit owner. The caller has already admitted the public request.
package observation

import (
	"ucupa/ucupa-core/internal/access"
	"ucupa/ucupa-core/internal/access/registry"
	"ucupa/ucupa-core/internal/commit"
)

type Operations struct {
	owner     *commit.Owner
	admission *access.Admission
	registry  *registry.Registry
}

func New(owner *commit.Owner, admission *access.Admission, resources *registry.Registry) *Operations {
	return &Operations{owner: owner, admission: admission, registry: resources}
}
