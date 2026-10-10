package contract

import "ucupa/ucupa-core/internal/token"

// Each alias exposes the original value and its existing safe methods, not
// internal raw reconstruction, private fields, or additional authority.
type Revision = token.Revision
type MountRevision = token.MountRevision
type ServiceRevision = token.ServiceRevision
type FacetContactRevision = token.FacetContactRevision
type ListContinuation = token.ListContinuation
