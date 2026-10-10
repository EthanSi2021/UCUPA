package contract

import "ucupa/shared/values"

// QueryAccess exposes the three existing local consumer operations, not the
// query implementation's private cross-responsibility read leases.
type QueryAccess interface {
	QueryRegion(values.PrincipalRef, RegionQuery, *CoreReadCut, uint16, *QueryContinuation) (QueryPage, error)
	ReadElementSnapshot(values.PrincipalRef, values.ElementID, CoreReadSnapshot) (ElementRead, error)
	ReleaseReadSnapshot(values.PrincipalRef, CoreReadSnapshot) error
}
