package contract

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/internal/query/request"
)

const PermissionQueryRegion values.PermissionID = "ucupa.core.anchor.query-region"

// The request leaf owns one representation; aliases do not expose its private
// canonicalization, binding, reconstruction or field access functions.
type QuerySphere = request.QuerySphere
type QuerySelector = request.QuerySelector
type RegionQuery = request.RegionQuery
type Path = request.Path
type PathRegionQuery = request.PathRegionQuery
type QueryReadMode = request.QueryReadMode
type QueryContinuation = request.QueryContinuation
type ConnectedQueryContinuation = request.ConnectedQueryContinuation
type ConnectedDirectContinuation = request.ConnectedDirectContinuation

const (
	QueryAnchors         = request.QueryAnchors
	QueryEntities        = request.QueryEntities
	QueryBoth            = request.QueryBoth
	QueryCurrentVerified = request.QueryCurrentVerified
	QueryRetained        = request.QueryRetained
)

type QueryMatch struct {
	ElementID values.ElementID
	Kind      ElementKind
	Revision  Revision
	Path      []values.ElementID
}
type QueryStatus uint8

const (
	QueryComplete QueryStatus = iota + 1
	QueryPartial
)

type QueryGap uint8

const (
	QueryNoGap              QueryGap = 0
	QueryExactWorkLimit     QueryGap = 1
	QueryUnresolvedCoverage QueryGap = 2
)

type QueryPage struct {
	Matches       []QueryMatch
	Cut           CoreReadCut
	ReadMode      QueryReadMode
	Snapshot      *CoreReadSnapshot
	ScopeAnchorID values.ElementID
	ActualUpDepth uint64
	Status        QueryStatus
	Gap           QueryGap
	Continuation  *QueryContinuation
}

type ConnectedSource struct {
	Authority values.FactAuthorityRef
	Cut       CoreReadCut
}

type ConnectedMatch struct {
	ElementID values.ElementID
	Kind      ElementKind
	Revision  Revision
	Path      []values.ElementID
	Source    ConnectedSource
}

type ConnectedRecord struct {
	Record ElementRecord
	Source ConnectedSource
}

type PathRead struct {
	Attachment     FacetAttachment
	State          ExistenceState
	Record         *ElementRecord
	Source         ConnectedSource
	Path           Path
	VerifiedPrefix Path
	AbsenceSource  ConnectedSource
	Cut            ConnectedCut
	ReadMode       QueryReadMode
}

type ConnectedQueryPage struct {
	Matches           []ConnectedMatch
	Cut               ConnectedCut
	ReadMode          QueryReadMode
	ResolvedScopePath Path
	ActualUpDepth     uint64
	Status            QueryStatus
	Gap               QueryGap
	Continuation      *ConnectedQueryContinuation
}

type ConnectedDirectPage struct {
	Records      []ConnectedRecord
	Cut          ConnectedCut
	ReadMode     QueryReadMode
	Status       QueryStatus
	Gap          QueryGap
	Continuation *ConnectedDirectContinuation
}
