// Package federation exposes the one detached Federation value representation
// and its consumer-owned ports. Runtime flow, authorization and owner state
// remain with the actual Federation owner; aliases here create no second model.
package federation

import (
	"context"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	federationdenial "ucupa/ucupa-core/internal/query/federation/denial"
	federationvalue "ucupa/ucupa-core/internal/query/federation/value"
)

type FederationReadRequest = federationvalue.FederationReadRequest
type FederationMountRelation = federationvalue.FederationMountRelation
type FederationReadPart = federationvalue.FederationReadPart
type FederationRecord = federationvalue.FederationRecord
type FederationPoseStep = federationvalue.FederationPoseStep
type FederationPathRequest = federationvalue.FederationPathRequest
type FederationSpineNode = federationvalue.FederationSpineNode
type FederationFrontier = federationvalue.FederationFrontier
type FederationPathResult = federationvalue.FederationPathResult
type FederationQueryRequest = federationvalue.FederationQueryRequest
type FederationProjectionRequest = federationvalue.FederationProjectionRequest
type FederationMatch = federationvalue.FederationMatch
type FederationProjectionPage = federationvalue.FederationProjectionPage
type FederationParentEvidence = federationvalue.FederationParentEvidence
type FederationObserveRequest = federationvalue.FederationObserveRequest
type FederationChangeGroup = federationvalue.FederationChangeGroup
type FederationObservationPage = federationvalue.FederationObservationPage

// FederationDenial is trusted source-internal evidence, not public disclosure
// or an access grant. External consumers can only retain and return a value
// issued by the Core; its unique representation remains opaque.
type FederationDenial = federationdenial.Evidence

// FederationConnections is the coordinator's consumer-owned, authenticated
// source port. The caller is the original locally established requester.
// A target and address must come from the entry or an authorized relationship;
// this interface never asks an adapter to resolve a bare authority reference.
type FederationConnections interface {
	OpenFederationRead(context.Context, values.CallerContext, values.NextHop, values.FactAuthorityRef, FederationReadRequest) (FederationReadConnection, error)
}

// FederationReadConnection owns a bounded source-local read basis, not facts.
// It carries no independent write authority, caller substitution, or automatic
// query retry. Part returns a detached value copy; callers cannot mutate the
// connection's retained identity or source binding through a returned value.
type FederationReadConnection interface {
	Part() FederationReadPart
	Project(context.Context, FederationProjectionRequest, *values.PublicToken, uint16) (FederationProjectionPage, error)
	Check(context.Context, bool) error
	Observe(context.Context, FederationObserveRequest) (FederationObservationPage, error)
	Release(context.Context) error
}

// NewFederationRange validates the existing service value through the unique
// value implementation; it creates neither geometry nor persistence truth.
func NewFederationRange(kind corecontract.RangeKind, sphere corecontract.BoundedSphere) (corecontract.Range, error) {
	return federationvalue.NewFederationRange(kind, sphere)
}
