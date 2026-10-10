package entry

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	corepb "ucupa/ucupa-core/bindings/go/protobuf"
	federationcontract "ucupa/ucupa-core/contract/federation"
	"ucupa/ucupa-core/transport/grpc/internal/admission"
	grpcsession "ucupa/ucupa-core/transport/grpc/internal/session"
)

const (
	entryTestAuthority values.FactAuthorityRef = "00000000-0000-4000-8000-000000000001"
	entryTestPeer      values.FactAuthorityRef = "00000000-0000-4000-8000-000000000002"
	entryTestAnchor    values.ElementID        = "00000000-0000-4000-8000-000000000003"
	entryTestElement   values.ElementID        = "00000000-0000-4000-8000-000000000004"
	entryTestNextHop   values.NextHop          = "https://127.0.0.1:1"
)

var entryTestCaller = values.CallerContext{
	Subject:       values.PrincipalRef{Realm: "entry-realm", Principal: "entry-caller"},
	Credential:    [16]byte{1},
	Establishment: [32]byte{2},
	Proof:         [32]byte{3},
}

type entryOwnerCall struct {
	caller     values.CallerContext
	request    []byte
	federation bool
}

type entryTestOwner struct {
	mu            sync.Mutex
	result        []byte
	err           error
	calls         []entryOwnerCall
	federatedCall func(context.Context, values.CallerContext, []byte, federationcontract.FederationConnections) ([]byte, error)
}

func (o *entryTestOwner) PublicCall(ctx context.Context, caller values.CallerContext, request []byte) ([]byte, error) {
	o.mu.Lock()
	o.calls = append(o.calls, entryOwnerCall{caller: caller, request: append([]byte(nil), request...)})
	result, err := append([]byte(nil), o.result...), o.err
	o.mu.Unlock()
	return result, err
}

func (o *entryTestOwner) PublicCallWithFederation(ctx context.Context, caller values.CallerContext, request []byte, connections federationcontract.FederationConnections) ([]byte, error) {
	o.mu.Lock()
	o.calls = append(o.calls, entryOwnerCall{caller: caller, request: append([]byte(nil), request...), federation: true})
	call := o.federatedCall
	result, err := append([]byte(nil), o.result...), o.err
	o.mu.Unlock()
	if call != nil {
		return call(ctx, caller, request, connections)
	}
	return result, err
}

func (o *entryTestOwner) setOutcome(result []byte, err error) {
	o.mu.Lock()
	o.result, o.err = append([]byte(nil), result...), err
	o.mu.Unlock()
}

func (o *entryTestOwner) setFederatedCall(call func(context.Context, values.CallerContext, []byte, federationcontract.FederationConnections) ([]byte, error)) {
	o.mu.Lock()
	o.federatedCall = call
	o.mu.Unlock()
}

func (o *entryTestOwner) snapshotCalls() []entryOwnerCall {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make([]entryOwnerCall, len(o.calls))
	for i, call := range o.calls {
		out[i] = call
		out[i].request = append([]byte(nil), call.request...)
	}
	return out
}

type entryTestAuthenticator struct{ caller values.CallerContext }

func (a entryTestAuthenticator) Establish(context.Context, values.RealmID, [16]byte, [32]byte) (values.CallerContext, error) {
	return a.caller, nil
}

func newEntryTestService(owner *entryTestOwner, responseLimit int) (*Service, *admission.Service) {
	gate := admission.New(owner, entryTestAuthenticator{caller: entryTestCaller}, admission.Config{
		SessionTTL:           time.Minute,
		MaxSessions:          4,
		MaxSessionsPerCaller: 4,
		MaxConcurrentRPCs:    1,
	})
	return New(owner, gate, Config{NextHop: entryTestNextHop, MaxResponseBytes: responseLimit}), gate
}

func entryTestToken(kind networkpb.TokenKind, seed byte) *networkpb.Token {
	return &networkpb.Token{
		Owner: proto.String(string(entryTestAuthority)),
		Kind:  kind.Enum(),
		Value: bytes.Repeat([]byte{seed}, 32),
	}
}

func entryTestReference() *networkpb.Reference {
	return &networkpb.Reference{
		Boundary: &networkpb.Boundary{Core: proto.String(string(entryTestAuthority)), Anchor: proto.String(string(entryTestAnchor))},
		NextHop:  proto.String(string(entryTestNextHop)),
	}
}

func entryTestAddress() *networkpb.Address {
	return &networkpb.Address{Origin: proto.String(string(entryTestAnchor)), Up: proto.Uint64(0), Down: &networkpb.ElementPath{Items: []string{string(entryTestElement)}}}
}

func entryTestTuple() *corepb.SpatialTuple {
	return &corepb.SpatialTuple{
		Parent: proto.String(string(entryTestAnchor)),
		Pose: &corepb.Pose{
			Translation: &networkpb.Vector3{Items: []float64{1, 2, 3}},
			Rotation:    &corepb.Rotation{Pivot: proto.Uint32(1), Ratios: &networkpb.Vector3{Items: []float64{0, 0, 0}}},
		},
		Range: &corepb.Range{Kind: corepb.RangeKind_BOUNDED_SPHERE.Enum(), Radius: proto.Float64(4)},
	}
}

func entryTestRecoverRequest() *corepb.MutationCommand {
	return &corepb.MutationCommand{Command: &corepb.MutationCommand_DeleteEntity{DeleteEntity: &corepb.DeleteEntityCommand{
		Element:     proto.String(string(entryTestElement)),
		OperationId: []byte("original-delete"),
		Expected:    entryTestToken(networkpb.TokenKind_RECORD_REVISION, 9),
	}}}
}

type entryFailureResponse interface {
	GetFailure() networkpb.Failure
}

func TestServiceMapsEveryCoreMethodToItsPublicOperation(t *testing.T) {
	owner := &entryTestOwner{err: values.PublicDenied}
	service, _ := newEntryTestService(owner, 1<<20)
	ctx := grpcsession.WithCaller(context.Background(), entryTestCaller)

	tests := []struct {
		name   string
		op     string
		invoke func(context.Context) (entryFailureResponse, error)
	}{
		{"Enter", "enter", func(ctx context.Context) (entryFailureResponse, error) {
			return service.Enter(ctx, &corepb.EnterRequest{Reference: entryTestReference(), Intent: corepb.Intent_READ.Enum()})
		}},
		{"Close", "close", func(ctx context.Context) (entryFailureResponse, error) {
			return service.Close(ctx, &corepb.CloseRequest{Handle: entryTestToken(networkpb.TokenKind_SPACE_HANDLE, 1)})
		}},
		{"ReadElement", "read", func(ctx context.Context) (entryFailureResponse, error) {
			return service.ReadElement(ctx, &corepb.ReadElementRequest{Handle: entryTestToken(networkpb.TokenKind_SPACE_HANDLE, 1), Address: entryTestAddress()})
		}},
		{"ListDirect", "list", func(ctx context.Context) (entryFailureResponse, error) {
			return service.ListDirect(ctx, &corepb.ListDirectRequest{Handle: entryTestToken(networkpb.TokenKind_SPACE_HANDLE, 1), Address: entryTestAddress()})
		}},
		{"QueryRegion", "query", func(ctx context.Context) (entryFailureResponse, error) {
			return service.QueryRegion(ctx, &corepb.QueryRegionRequest{
				Handle: entryTestToken(networkpb.TokenKind_SPACE_HANDLE, 1), Start: entryTestAddress(), Basis: entryTestAddress(),
				Sphere: &corepb.Sphere{Center: &networkpb.Vector3{Items: []float64{0, 0, 0}}, Radius: proto.Float64(5)},
			})
		}},
		{"BeginObserve", "begin-observe", func(ctx context.Context) (entryFailureResponse, error) {
			return service.BeginObserve(ctx, &corepb.BeginObserveRequest{Preparation: entryTestToken(networkpb.TokenKind_OBSERVATION_PREPARATION, 2)})
		}},
		{"PollChanges", "poll", func(ctx context.Context) (entryFailureResponse, error) {
			return service.PollChanges(ctx, &corepb.PollChangesRequest{Cursor: entryTestToken(networkpb.TokenKind_OBSERVATION_CURSOR, 3)})
		}},
		{"ReleaseObserve", "release-observe", func(ctx context.Context) (entryFailureResponse, error) {
			return service.ReleaseObserve(ctx, &corepb.ReleaseObserveRequest{Context: &corepb.ReleaseObserveRequest_Preparation{Preparation: entryTestToken(networkpb.TokenKind_OBSERVATION_PREPARATION, 2)}})
		}},
		{"CreateAnchor", "create-anchor", func(ctx context.Context) (entryFailureResponse, error) {
			return service.CreateAnchor(ctx, &corepb.CreateAnchorRequest{Element: proto.String(string(entryTestElement)), OperationId: []byte("create-anchor"), Final: entryTestTuple(), Handle: entryTestToken(networkpb.TokenKind_SPACE_HANDLE, 1)})
		}},
		{"CreateEntity", "create-entity", func(ctx context.Context) (entryFailureResponse, error) {
			return service.CreateEntity(ctx, &corepb.CreateEntityRequest{Element: proto.String(string(entryTestElement)), OperationId: []byte("create-entity"), Final: entryTestTuple(), Handle: entryTestToken(networkpb.TokenKind_SPACE_HANDLE, 1)})
		}},
		{"UpdateAnchor", "update-anchor", func(ctx context.Context) (entryFailureResponse, error) {
			return service.UpdateAnchor(ctx, &corepb.UpdateAnchorRequest{Element: proto.String(string(entryTestElement)), OperationId: []byte("update-anchor"), Final: entryTestTuple(), Expected: entryTestToken(networkpb.TokenKind_RECORD_REVISION, 4), Handle: entryTestToken(networkpb.TokenKind_SPACE_HANDLE, 1)})
		}},
		{"UpdateEntity", "update-entity", func(ctx context.Context) (entryFailureResponse, error) {
			return service.UpdateEntity(ctx, &corepb.UpdateEntityRequest{Element: proto.String(string(entryTestElement)), OperationId: []byte("update-entity"), Final: entryTestTuple(), Expected: entryTestToken(networkpb.TokenKind_RECORD_REVISION, 4), Handle: entryTestToken(networkpb.TokenKind_SPACE_HANDLE, 1)})
		}},
		{"DeleteAnchor", "delete-anchor", func(ctx context.Context) (entryFailureResponse, error) {
			return service.DeleteAnchor(ctx, &corepb.DeleteAnchorRequest{Element: proto.String(string(entryTestElement)), OperationId: []byte("delete-anchor"), Expected: entryTestToken(networkpb.TokenKind_RECORD_REVISION, 4), Handle: entryTestToken(networkpb.TokenKind_SPACE_HANDLE, 1)})
		}},
		{"DeleteEntity", "delete-entity", func(ctx context.Context) (entryFailureResponse, error) {
			return service.DeleteEntity(ctx, &corepb.DeleteEntityRequest{Element: proto.String(string(entryTestElement)), OperationId: []byte("delete-entity"), Expected: entryTestToken(networkpb.TokenKind_RECORD_REVISION, 4), Handle: entryTestToken(networkpb.TokenKind_SPACE_HANDLE, 1)})
		}},
		{"RecoverOutcome", "recover-outcome", func(ctx context.Context) (entryFailureResponse, error) {
			return service.RecoverOutcome(ctx, &corepb.RecoverOutcomeRequest{Reference: entryTestReference(), OperationId: []byte("recover"), Request: entryTestRecoverRequest()})
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			before := len(owner.snapshotCalls())
			out, err := test.invoke(ctx)
			if err != nil {
				t.Fatalf("method error = %v", err)
			}
			if out == nil {
				t.Fatal("method returned a nil response")
			}
			if out.GetFailure() != networkpb.Failure_DENIED {
				t.Fatalf("failure = %v, want %v", out.GetFailure(), networkpb.Failure_DENIED)
			}
			calls := owner.snapshotCalls()
			if len(calls) != before+1 {
				t.Fatalf("owner calls = %d, want %d", len(calls), before+1)
			}
			call := calls[len(calls)-1]
			if call.federation || call.caller != entryTestCaller {
				t.Fatalf("owner call = %+v, want ordinary call by original caller", call)
			}
			value, err := values.ParseCanonicalJSON(call.request, values.PublicJSONLimits())
			if err != nil {
				t.Fatalf("owner request is not canonical JSON: %v", err)
			}
			object, ok := value.(map[string]any)
			if !ok || object["op"] != test.op {
				t.Fatalf("public operation = %v, want %q", object["op"], test.op)
			}
		})
	}
}
