// Package value owns the detached Federation query value representation.
package value

import (
	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/geometry"
)

// FederationReadRequest selects one owner-local portion of a larger request.
// Purpose is the nonzero digest of the coordinator's normalized request.
// Mode zero selects local policy; QueryCurrentVerified explicitly prevents
// retention when a mixed-source request is reopened before publishing a page.
// Incoming is required only when Boundary is this Core's external mount anchor.
type FederationReadRequest struct {
	Boundary       values.ElementID
	Incoming       corecontract.MountKey
	Intent         string
	PrepareObserve bool
	Mode           corecontract.QueryReadMode
	Purpose        [32]byte
}

// FederationMountRelation is authenticated source evidence, not a public
// descendant address or authority directory. The coordinator must match both
// owners' exact Key/Admission/Range before accepting an edge. NextHop is the
// specified contact of this direction; changing it invalidates a read part.
type FederationMountRelation struct {
	Incoming      bool
	State         corecontract.MountState
	Key           corecontract.MountKey
	Admission     corecontract.MountAdmissionKey
	Revision      values.PublicToken
	Contact       values.PublicToken
	NextHop       values.NextHop
	Range         corecontract.BoundedSphere
	WriteGateOpen bool
}

// FederationReadPart is a finite runtime read basis. It is bound to the
// original caller, authenticated coordinator, purpose and exact local cut.
// Incoming is context only: the carrier never fabricates the parent's Element.
type FederationReadPart struct {
	Hidden             bool
	Subject            values.PrincipalRef
	CutStamp           [32]byte
	AuthorizationStamp [32]byte
	Basis              values.PublicToken
	Boundary           values.ElementID
	Purpose            [32]byte
	ReadMode           corecontract.QueryReadMode
	Incoming           *FederationMountRelation
	Preparation        *values.PublicToken
}

// FederationRecord contains detached value copies, never owner state or raw
// Element revisions. Attachment is from the same read cut and has no pending
// proposal. Path excludes the boundary and includes this Element's ID.
type FederationRecord struct {
	ID         values.ElementID
	Kind       corecontract.ElementKind
	Parent     values.ElementID
	Pose       *corecontract.Pose
	Range      corecontract.Range
	Revision   values.PublicToken
	Path       []values.ElementID
	Attachment *corecontract.PublicFacetInspection
	Mount      *FederationMountRelation
}

// FederationPoseStep is one exact coordinate recipe step. Steps multiply in
// slice order; they must never first be rounded into a cumulative public Pose.
type FederationPoseStep struct {
	Pose    corecontract.Pose
	Inverse bool
}

type FederationPathRequest struct {
	Origin values.ElementID
	Up     uint64
	Down   []values.ElementID
}

type FederationSpineNode struct {
	ID     values.ElementID
	Parent values.ElementID
	Pose   *corecontract.Pose
}

// FederationFrontier is service-only evidence of a necessary cross-owner
// traversal. ToScope transforms the frontier's anchor frame into this
// projection's Scope.
type FederationFrontier struct {
	Relation FederationMountRelation
	Path     []values.ElementID
	ToScope  []FederationPoseStep
}

type FederationPathResult struct {
	Element       values.ElementID
	Kind          corecontract.ElementKind
	ActualUp      uint64
	RemainingUp   uint64
	RemainingDown []values.ElementID
	Spine         []FederationSpineNode
	Frontier      *FederationFrontier
}

type FederationQueryRequest struct {
	Scope    values.ElementID
	Sphere   corecontract.QuerySphere
	Selector corecontract.QuerySelector
	ToBasis  []FederationPoseStep
}

// FederationProjectionRequest is a closed projection union. Path carries Path
// only; read/list carry Element only; query carries Query only. Path is
// available to all intents but never discloses attachment or non-path elements.
type FederationProjectionRequest struct {
	Kind    string
	Path    *FederationPathRequest
	Element values.ElementID
	Query   *FederationQueryRequest
}

type FederationMatch struct {
	ID       values.ElementID
	Kind     corecontract.ElementKind
	Revision values.PublicToken
	Path     []values.ElementID
}

// FederationProjectionPage Complete closes this bounded local projection, not
// remote frontiers. Query returns summaries only; read/list return records
// only; path returns Path only.
type FederationProjectionPage struct {
	Parents []FederationParentEvidence
	// Coverage is query-local authenticated scan evidence, independent of page
	// size or continuation. A nonzero Gap still means incomplete coverage.
	Coverage     [32]byte
	traversal    [32]byte
	Basis        values.PublicToken
	Purpose      [32]byte
	Records      []FederationRecord
	Matches      []FederationMatch
	Frontiers    []FederationFrontier
	Path         *FederationPathResult
	Gap          corecontract.QueryGap
	Complete     bool
	Continuation *values.PublicToken
}

// FederationParentEvidence closes the complete owner-local direct adjacency
// at the source cut. Only already disclosable query-path parents are named;
// children, page counts and private transcript bytes are not disclosed.
type FederationParentEvidence struct {
	Parent    values.ElementID
	Adjacency [32]byte
}

// NewFederationRange validates the discriminated service value when decoding a
// read part. It does not introduce a different geometry or persistence format.
func NewFederationRange(kind corecontract.RangeKind, sphere corecontract.BoundedSphere) (corecontract.Range, error) {
	switch kind {
	case corecontract.RangeBoundedSphere:
		if geometry.ValidSphere(sphere) {
			return geometry.BoundedRange(sphere), nil
		}
	case corecontract.RangeUnboundedRoot:
		if sphere == (corecontract.BoundedSphere{}) {
			return geometry.UnboundedRootRange(), nil
		}
	}
	return corecontract.Range{}, values.PublicInvalid
}

func ProjectionTraversal(page FederationProjectionPage) [32]byte { return page.traversal }

func SetProjectionTraversal(page *FederationProjectionPage, traversal [32]byte) {
	page.traversal = traversal
}
