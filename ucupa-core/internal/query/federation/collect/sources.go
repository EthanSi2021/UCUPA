package collect

import (
	"context"
	"errors"
	"time"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	federationcontract "ucupa/ucupa-core/contract/federation"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
)

// ErrMixedRetention tells the outer access owner to rebuild the complete
// source set in current-verified mode before publishing any result.
var ErrMixedRetention = errors.New("federation must recollect all sources in current mode")

// A sources vector is owned by one Read. Read.mu serializes all use; no Core
// owner mutex is held while a source method is called.
type sources struct {
	caller      values.CallerContext
	source      values.FactAuthorityRef
	request     federationcontract.FederationReadRequest
	connections federationcontract.FederationConnections
	parts       []federationcontract.FederationReadConnection
	owners      map[values.FactAuthorityRef]struct{}
	limit       int
	sealed      bool
	closed      bool
	graph       map[string]evidenceNode
}

func newSources(config Config) (*sources, error) {
	if !config.Caller.Subject.Valid() || !config.Source.Valid() || config.Local == nil || config.Connections == nil || config.SourceLimit < 1 || config.SourceLimit > 256 {
		return nil, values.PublicInvalid
	}
	part := config.Local.Part()
	if !partMatches(part, config.PartRequest, config.Source) || part.Subject != config.Caller.Subject || part.Hidden {
		return nil, values.PublicInvalidContext
	}
	return &sources{
		caller:      config.Caller,
		source:      config.Source,
		request:     config.PartRequest,
		connections: config.Connections,
		parts:       []federationcontract.FederationReadConnection{config.Local},
		owners:      map[values.FactAuthorityRef]struct{}{config.Source: {}},
		limit:       config.SourceLimit,
		graph:       map[string]evidenceNode{part.Basis.Value: {part: freezeEvidence(part)}},
	}, nil
}

// follow accepts only an outgoing edge projected by a registered source.
// Repeated owners are rejected rather than joined through two mount paths.
func (v *sources) follow(ctx context.Context, parent federationcontract.FederationReadConnection, edge federationcontract.FederationMountRelation) (federationcontract.FederationReadConnection, error) {
	if v.closed || v.sealed {
		return nil, values.PublicInvalidContext
	}
	registered := false
	for _, part := range v.parts {
		if parent != nil && part.Part().Basis == parent.Part().Basis {
			registered = true
			break
		}
	}
	if !registered || edge.Incoming || !edge.Key.Valid() || !mountfact.ValidMountAdmissionKey(edge.Admission) || edge.Admission.MountKey != edge.Key ||
		edge.State != corecontract.MountActive || !edge.WriteGateOpen || !edge.NextHop.Valid() || edge.Key.ParentAuthority != parent.Part().Basis.Owner {
		return nil, values.PublicInvalidContext
	}
	target := edge.Key.CarrierAuthority
	if _, exists := v.owners[target]; exists {
		return nil, values.PublicInvalidContext
	}
	if len(v.parts) >= v.limit {
		return nil, values.PublicResourceExhausted
	}
	request := v.request
	request.Boundary, request.Incoming = edge.Key.MountAnchorUUID, edge.Key
	part, err := v.connections.OpenFederationRead(ctx, v.caller, edge.NextHop, target, request)
	if err != nil {
		return nil, err
	}
	if part == nil {
		return nil, values.PublicUnavailable
	}
	info := part.Part()
	if info.Subject != v.caller.Subject || !partMatches(info, request, target) || !edgeMatches(edge, info.Incoming) {
		_ = retirePart(part)
		return nil, values.PublicInvalidContext
	}
	v.parts = append(v.parts, part)
	v.owners[target] = struct{}{}
	v.graph[info.Basis.Value] = evidenceNode{part: freezeEvidence(info), parent: parent.Part().Basis, outgoing: edge}
	return part, nil
}

func partMatches(part federationcontract.FederationReadPart, request federationcontract.FederationReadRequest, owner values.FactAuthorityRef) bool {
	if part.Hidden && (!request.Incoming.Valid() || request.Intent == "read" || part.Incoming == nil) {
		return false
	}
	return part.Subject.Valid() && part.CutStamp != ([32]byte{}) && part.AuthorizationStamp != ([32]byte{}) && part.Basis.Valid() && part.Basis.Kind == "read-receipt" && part.Basis.Owner == owner &&
		part.Boundary == request.Boundary && part.Purpose == request.Purpose &&
		(part.ReadMode == corecontract.QueryRetained || part.ReadMode == corecontract.QueryCurrentVerified) &&
		(request.Mode != corecontract.QueryCurrentVerified || part.ReadMode == corecontract.QueryCurrentVerified) &&
		(request.PrepareObserve == (part.Preparation != nil)) &&
		(part.Preparation == nil || part.Preparation.Valid() && part.Preparation.Owner == owner && part.Preparation.Kind == "observation-preparation")
}

func edgeMatches(out federationcontract.FederationMountRelation, in *federationcontract.FederationMountRelation) bool {
	return in != nil && in.Incoming && in.State == corecontract.MountActive && in.WriteGateOpen &&
		in.Key == out.Key && in.Admission == out.Admission && in.Range == out.Range
}

func (v *sources) seal(ctx context.Context) error {
	if v.closed || v.sealed {
		return values.PublicInvalidContext
	}
	if err := v.checkEvidenceGraph(); err != nil {
		return err
	}
	mode := v.parts[0].Part().ReadMode
	for _, part := range v.parts {
		if part.Part().ReadMode != mode {
			return ErrMixedRetention
		}
	}
	// Every capture precedes every check. If all checks succeed, all cuts were
	// current at the instant the first check began; no global clock is needed.
	for _, part := range v.parts {
		if err := part.Check(ctx, true); err != nil {
			return err
		}
	}
	if err := v.checkEvidenceGraph(); err != nil {
		return err
	}
	v.sealed = true
	return nil
}

func (v *sources) check(ctx context.Context) error {
	if v.closed || !v.sealed {
		return values.PublicInvalidContext
	}
	if err := v.checkEvidenceGraph(); err != nil {
		return err
	}
	for _, part := range v.parts {
		if err := part.Check(ctx, false); err != nil {
			return err
		}
	}
	return v.checkEvidenceGraph()
}

func (v *sources) release(ctx context.Context) error {
	if v.closed {
		return nil
	}
	v.closed = true
	var failure error
	for _, part := range v.parts {
		if err := part.Release(ctx); err != nil && failure == nil {
			failure = err
		}
	}
	v.parts = nil
	v.owners = nil
	v.graph = nil
	return failure
}

func retirePart(part federationcontract.FederationReadConnection) error {
	if part == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return part.Release(ctx)
}

func (v *sources) retire() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return v.release(ctx)
}
