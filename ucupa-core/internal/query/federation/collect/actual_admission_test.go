package collect

import (
	"context"
	"errors"
	"testing"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	federationcontract "ucupa/ucupa-core/contract/federation"
)

type queuedConnections struct {
	parts []federationcontract.FederationReadConnection
	opens int
}

func (connections *queuedConnections) OpenFederationRead(context.Context, values.CallerContext, values.NextHop, values.FactAuthorityRef, federationcontract.FederationReadRequest) (federationcontract.FederationReadConnection, error) {
	if connections.opens >= len(connections.parts) {
		return nil, values.PublicUnavailable
	}
	part := connections.parts[connections.opens]
	connections.opens++
	return part, nil
}

// This is the source-vector half of the former root
// TestFederationSourceVectorRejectsMismatchedAdmissionAndLimits. The actual
// query-level test separately proves that an optional rejected source becomes
// unresolved coverage rather than entering the published vector.
func TestSourcesFollowRejectsMismatchedAdmissionAndLimits(t *testing.T) {
	request := federationcontract.FederationReadRequest{Boundary: testRoot, Intent: "query", Purpose: [32]byte{9}}
	outgoing, incoming := testRelation(t, testOwner, testOwner2, testMount1, 1)

	limitedRoot := newTestConnection(t, testOwner, 151, false)
	limitedChild := newTestConnection(t, testOwner2, 161, false)
	limitedChild.part.Boundary = testMount1
	limitedChild.part.Incoming = &incoming
	limitedConnections := &queuedConnections{parts: []federationcontract.FederationReadConnection{limitedChild}}
	limited, err := newSources(Config{Caller: testCaller, Source: testOwner, PartRequest: request, Local: limitedRoot, Connections: limitedConnections, SourceLimit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := limited.follow(context.Background(), limitedRoot, outgoing); !errors.Is(err, values.PublicResourceExhausted) || limitedConnections.opens != 0 {
		t.Fatal("capacity checked after dialing", err)
	}
	if err := limited.release(context.Background()); err != nil || limitedRoot.releases != 1 || limitedChild.releases != 0 {
		t.Fatal("limited vector release changed", err)
	}

	root := newTestConnection(t, testOwner, 171, false)
	rejected := newTestConnection(t, testOwner2, 181, false)
	rejected.part.Boundary = testMount1
	badIncoming := incoming
	badIncoming.Range, _ = corecontract.NewBoundedSphere(1)
	rejected.part.Incoming = &badIncoming
	accepted := newTestConnection(t, testOwner2, 191, false)
	accepted.part.Boundary = testMount1
	accepted.part.Incoming = &incoming
	connections := &queuedConnections{parts: []federationcontract.FederationReadConnection{rejected, accepted}}
	sourceSet, err := newSources(Config{Caller: testCaller, Source: testOwner, PartRequest: request, Local: root, Connections: connections, SourceLimit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sourceSet.follow(context.Background(), root, outgoing); !errors.Is(err, values.PublicInvalidContext) {
		t.Fatal("joined different ranges", err)
	}
	if connections.opens != 1 || len(sourceSet.parts) != 1 || len(sourceSet.owners) != 1 || len(sourceSet.graph) != 1 || rejected.releases != 1 {
		t.Fatal("rejected edge entered the vector or was not released")
	}
	child, err := sourceSet.follow(context.Background(), root, outgoing)
	if err != nil || child != accepted || connections.opens != 2 || len(sourceSet.parts) != 2 {
		t.Fatal("corrected source was not admitted", err)
	}
	if _, err := sourceSet.follow(context.Background(), root, outgoing); !errors.Is(err, values.PublicInvalidContext) || connections.opens != 2 || len(sourceSet.parts) != 2 {
		t.Fatal("duplicate target was dialed or admitted", err)
	}
	if child.Part().Incoming == nil || child.Part().Incoming.Key != outgoing.Key {
		t.Fatal("admitted source did not retain the exact mount key")
	}
	if err := sourceSet.release(context.Background()); err != nil || root.releases != 1 || rejected.releases != 1 || accepted.releases != 1 {
		t.Fatal("source release count changed", err)
	}
}
