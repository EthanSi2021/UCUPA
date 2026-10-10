package transition

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	"ucupa/ucupa-core/contract"
	port "ucupa/ucupa-core/contract/transition"
	"ucupa/ucupa-core/transport/grpc/internal/admission"
	"ucupa/ucupa-core/transport/grpc/internal/session"
)

const ownerID values.FactAuthorityRef = "00000000-0000-4000-8000-000000000011"
const targetID values.FactAuthorityRef = "00000000-0000-4000-8000-000000000012"
const boundary values.ElementID = "00000000-0000-4000-8000-000000000013"

// These ports exercise only RPC failure classification, not transfer proof.
type targetFailurePorts struct {
	port.TargetPeer
	port.SourceTransport
	readErr, writeErr, authErr error
	reads, writes              int
}

func (*targetFailurePorts) AuthorityRef() values.FactAuthorityRef { return targetID }
func (p *targetFailurePorts) CheckTransitionPeer(context.Context, values.FactAuthorityRef) error {
	return p.authErr
}
func (p *targetFailurePorts) ReadTransitionFence(context.Context, values.NextHop, values.FactAuthorityRef, contract.TransitionApprovalBinding) (port.Frozen, error) {
	p.reads++
	return port.Frozen{}, p.readErr
}
func (p *targetFailurePorts) AcceptFrozenTransition(context.Context, contract.TransitionApprovalBinding, []byte, port.Frozen) (port.Acceptance, error) {
	p.writes++
	return port.Acceptance{}, p.writeErr
}
func (p *targetFailurePorts) InstallTransition(context.Context, values.FactAuthorityRef, contract.TransitionApprovalBinding, port.SourceTransport) (port.Completion, error) {
	p.writes++
	return port.Completion{}, p.writeErr
}

func TestTargetRPCDoesNotClaimStoreFailureMeansNotCommitted(t *testing.T) {
	b := &pb.TransitionApprovalBinding{Source: proto.String(string(ownerID)), Command: beginRequest()}
	for _, code := range []codes.Code{codes.Unauthenticated, codes.Unavailable, codes.Unimplemented} {
		p := &targetFailurePorts{writeErr: status.Error(code, "admission no longer available")}
		s := NewTarget(p, targetID, p, p, nil, nil, nil, nil)
		if _, err := s.AcceptTransition(context.Background(), &pb.AcceptTransitionRequest{Binding: b, ApprovalToken: make([]byte, 32)}); status.Code(err) != code {
			t.Fatalf("admission failure %v became %v", code, err)
		}
	}
	for _, afterDispatch := range []bool{false, true} {
		p := &targetFailurePorts{}
		want := netpb.Failure_UNAVAILABLE
		if afterDispatch {
			p.writeErr = contract.ErrStore
			want = netpb.Failure_INDETERMINATE
		} else {
			p.readErr = contract.ErrStore
		}
		s := NewTarget(p, targetID, p, p, nil, nil, nil, nil)
		r, err := s.AcceptTransition(context.Background(), &pb.AcceptTransitionRequest{Binding: b, ApprovalToken: make([]byte, 32)})
		if err != nil || r.GetFailure() != want {
			t.Fatalf("accept dispatch=%v: %v %v", afterDispatch, r, err)
		}
		if p.reads != 1 || p.writes != map[bool]int{false: 0, true: 1}[afterDispatch] {
			t.Fatal("unexpected dispatch")
		}
	}
	p := &targetFailurePorts{writeErr: contract.ErrStore}
	s := NewTarget(p, targetID, p, p, nil, nil, nil, nil)
	r, err := s.InstallTransition(context.Background(), &pb.InstallTransitionRequest{Binding: b})
	if err != nil || r.GetFailure() != netpb.Failure_INDETERMINATE {
		t.Fatalf("install failure: %v %v", r, err)
	}
	p.authErr = status.Error(codes.Unauthenticated, "wrong peer")
	p.reads, p.writes = 0, 0
	if _, err := s.AcceptTransition(context.Background(), &pb.AcceptTransitionRequest{Binding: b, ApprovalToken: make([]byte, 32)}); status.Code(err) != codes.Unauthenticated || p.reads != 0 || p.writes != 0 {
		t.Fatal("unauthenticated dispatch", err)
	}
}

type witness struct {
	alive   bool
	calls   int
	after   func()
	err     error
	caller  values.CallerContext
	result  contract.TransitionStatus
	carrier contract.CarrierStatus
}

func (w *witness) AuthorityRef() values.FactAuthorityRef { return ownerID }
func (w *witness) BindEntrySession(context.Context, values.CallerContext) (admission.EntrySession, error) {
	if !w.alive {
		return admission.EntrySession{}, errors.New("expired")
	}
	return admission.EntrySession{}, nil
}
func (w *witness) CheckEntrySession(admission.EntrySession) error {
	if !w.alive {
		return errors.New("expired")
	}
	return nil
}
func (w *witness) EntryContext(ctx context.Context, _ admission.EntrySession) (context.Context, context.CancelFunc, error) {
	child, cancel := context.WithCancel(ctx)
	return child, cancel, nil
}
func (w *witness) RecoverTransition(_ context.Context, c values.CallerContext, _ contract.TransitionApprovalBinding, _ port.TargetTransport) (contract.TransitionStatus, error) {
	return w.call(c)
}
func (w *witness) RecoverCarrierTransition(_ context.Context, c values.CallerContext, _ contract.CarrierApprovalBinding, _ port.CarrierTransport) (contract.CarrierStatus, error) {
	_, err := w.call(c)
	return w.carrier, err
}
func (w *witness) call(c values.CallerContext) (contract.TransitionStatus, error) {
	w.calls++
	w.caller = c
	if w.after != nil {
		w.after()
	}
	return w.result, w.err
}
func (w *witness) BeginTransition(_ context.Context, c values.CallerContext, _ contract.OperationID, _ contract.TransitionCommand) (contract.TransitionStatus, error) {
	return w.call(c)
}
func (w *witness) InspectTransition(_ context.Context, c values.CallerContext, _ contract.OperationID, _ values.ElementID) (contract.TransitionStatus, error) {
	return w.call(c)
}
func (w *witness) CancelTransition(_ context.Context, c values.CallerContext, _ contract.OperationID, _ values.ElementID, _ values.PublicToken) (contract.TransitionStatus, error) {
	return w.call(c)
}

func (w *witness) BeginCarrierTransition(_ context.Context, c values.CallerContext, _ contract.OperationID, _ contract.CarrierCommand) (contract.CarrierStatus, error) {
	_, err := w.call(c)
	return w.carrier, err
}
func (w *witness) InspectCarrierTransition(_ context.Context, c values.CallerContext, _ contract.OperationID, _ values.ElementID) (contract.CarrierStatus, error) {
	_, err := w.call(c)
	return w.carrier, err
}
func (w *witness) CancelCarrierTransition(_ context.Context, c values.CallerContext, _ contract.OperationID, _ values.ElementID, _ values.PublicToken) (contract.CarrierStatus, error) {
	_, err := w.call(c)
	return w.carrier, err
}
func fixture() (*witness, *Service, context.Context) {
	w := &witness{alive: true, result: contract.TransitionStatus{Source: ownerID, Operation: contract.OperationID("\xff\x00intent"), Kind: contract.TransitionRoot, Target: targetID, Boundary: boundary, Progress: contract.TransitionIntent, Revision: values.PublicToken{Owner: ownerID, Kind: "transition-revision", Value: values.EncodePublicBytes(make([]byte, 32))}}}
	c := values.CallerContext{Subject: values.PrincipalRef{Realm: "realm", Principal: "operator"}}
	return w, New(w, ownerID, w, nil, nil), session.WithCaller(context.Background(), c)
}
func beginRequest() *pb.BeginTransitionRequest {
	return &pb.BeginTransitionRequest{OperationId: []byte("\xff\x00intent"), Kind: pb.TransitionKind_TRANSITION_ROOT.Enum(), Target: proto.String(string(targetID)), Boundary: proto.String(string(boundary)), Expected: &netpb.Token{Owner: proto.String(string(ownerID)), Kind: netpb.TokenKind_RECORD_REVISION.Enum(), Value: make([]byte, 32)}, SourceAddress: proto.String("https://source.example:443"), TargetAddress: proto.String("https://target.example:443")}
}

type recoveryTargets struct {
	calls int
	err   error
	port.TargetTransport
}

func (p *recoveryTargets) Target(contract.TransitionApprovalBinding, []byte) (port.TargetTransport, error) {
	p.calls++
	return p, p.err
}

func TestRecoveryRPCUsesOriginalSessionAndMutationFailureSemantics(t *testing.T) {
	request := func() *pb.RecoverTransitionRequest {
		return &pb.RecoverTransitionRequest{Binding: &pb.TransitionApprovalBinding{Source: proto.String(string(ownerID)), Command: beginRequest()}, ApprovalToken: make([]byte, 32)}
	}
	for _, mode := range []string{"store failure", "release after dispatch", "missing session", "read grant", "wrong source", "missing token", "too large", "missing connections", "factory failure"} {
		t.Run(mode, func(t *testing.T) {
			w, s, ctx := fixture()
			targets := &recoveryTargets{}
			s.targets = targets
			r := request()
			wantCalls := 0
			switch mode {
			case "store failure":
				w.err = contract.ErrStore
				wantCalls = 1
			case "release after dispatch":
				w.after = func() { w.alive = false }
				wantCalls = 1
			case "missing session":
				w.alive = false
			case "read grant":
				ctx = session.WithGrant(ctx, [32]byte{1})
			case "wrong source":
				r.Binding.Source = proto.String(string(targetID))
			case "missing token":
				r.ApprovalToken = nil
			case "too large":
				r.Binding.Command.SourceAddress = proto.String(string(make([]byte, 4097)))
			case "missing connections":
				s.targets = nil
			case "factory failure":
				targets.err = contract.ErrUnavailable
			}
			out, err := s.RecoverTransition(ctx, r)
			if w.calls != wantCalls {
				t.Fatalf("owner calls %d want %d", w.calls, wantCalls)
			}
			if mode == "store failure" {
				if err != nil || out.GetError() != netpb.Failure_INDETERMINATE {
					t.Fatalf("mutation classified as no write: %v %v", out, err)
				}
				return
			}
			if mode == "factory failure" {
				if err != nil || out.GetError() != netpb.Failure_UNAVAILABLE {
					t.Fatalf("factory result: %v %v", out, err)
				}
				return
			}
			if err == nil || out != nil {
				t.Fatalf("invalid recovery disclosed result: %v %v", out, err)
			}
			if wantCalls == 0 && targets.calls != 0 {
				t.Fatal("invalid session/input acquired a connection")
			}
		})
	}
}
func TestControlRequiresOriginalSession(t *testing.T) {
	for _, end := range []string{"expired before", "expired after", "context cancelled", "read delegation", "missing caller"} {
		t.Run(end, func(t *testing.T) {
			w, s, ctx := fixture()
			wantCalls, wantCode := 1, codes.Unauthenticated
			switch end {
			case "expired before":
				w.alive = false
				wantCalls = 0
			case "expired after":
				w.after = func() { w.alive = false }
			case "context cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				defer cancel()
				w.after = cancel
				wantCode = codes.Canceled
			case "read delegation":
				ctx = session.WithGrant(ctx, [32]byte{1})
				wantCalls, wantCode = 0, codes.PermissionDenied
			case "missing caller":
				ctx = context.Background()
				wantCalls = 0
			}
			out, err := s.BeginTransition(ctx, beginRequest())
			if out != nil || status.Code(err) != wantCode || w.calls != wantCalls {
				t.Fatalf("out=%v error=%v calls=%d", out, err, w.calls)
			}
		})
	}
}
func TestControlFailureClassificationAndDisclosure(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want netpb.Failure
	}{
		{contract.ErrStore, netpb.Failure_INDETERMINATE}, {contract.ErrInvalid, netpb.Failure_INVALID}, {contract.ErrDenied, netpb.Failure_DENIED},
		{contract.ErrNotFound, netpb.Failure_NOT_FOUND}, {contract.ErrRevisionConflict, netpb.Failure_CONFLICT}, {contract.ErrOperationConflict, netpb.Failure_CONFLICT},
		{contract.ErrAttachmentStateConflict, netpb.Failure_CONFLICT}, {contract.ErrMountBusy, netpb.Failure_CONFLICT},
		{contract.ErrMountCoordinationRequired, netpb.Failure_UNAVAILABLE},
		{values.PublicResourceExhausted, netpb.Failure_RESOURCE_EXHAUSTED}, {values.PublicInvalidContext, netpb.Failure_INVALID_CONTEXT},
		{values.PublicUnavailable, netpb.Failure_UNAVAILABLE}, {values.PublicOutcomeUnknown, netpb.Failure_OUTCOME_UNKNOWN},
	} {
		w, s, ctx := fixture()
		w.err = tc.err
		r, err := s.BeginTransition(ctx, beginRequest())
		if err != nil || r.GetError() != tc.want || r.GetResult() != nil || w.calls != 1 {
			t.Fatalf("%v: %v %v", tc.err, r, err)
		}
		if w.caller.Subject != (values.PrincipalRef{Realm: "realm", Principal: "operator"}) {
			t.Fatal("changed caller")
		}
	}
	w, s, ctx := fixture()
	w.err = contract.ErrStore
	r, err := s.InspectTransition(ctx, &pb.InspectTransitionRequest{OperationId: beginRequest().OperationId, Boundary: beginRequest().Boundary})
	if err != nil || r.GetError() != netpb.Failure_UNAVAILABLE {
		t.Fatalf("read failure: %v %v", r, err)
	}
	w.err = errors.New("private failure")
	if r, err = s.BeginTransition(ctx, beginRequest()); r != nil || status.Code(err) != codes.Internal {
		t.Fatalf("private failure leak: %v %v", r, err)
	}
	w.err = nil
	w.result.Operation = "different"
	if r, err = s.BeginTransition(ctx, beginRequest()); r != nil || status.Code(err) != codes.Internal {
		t.Fatalf("wrong result: %v %v", r, err)
	}
}
func TestTransitionShapeRejectedBeforeOwner(t *testing.T) {
	for _, mutate := range []func(*pb.BeginTransitionRequest){
		func(r *pb.BeginTransitionRequest) { r.Kind = pb.TransitionKind_TRANSITION_KIND_UNSPECIFIED.Enum() },
		func(r *pb.BeginTransitionRequest) { r.OperationId = nil },
		func(r *pb.BeginTransitionRequest) { r.OperationId = make([]byte, 129) },
		func(r *pb.BeginTransitionRequest) { r.Final = &pb.SpatialTuple{} },
		func(r *pb.BeginTransitionRequest) { r.Kind = pb.TransitionKind_TRANSITION_SLICE.Enum() },
		func(r *pb.BeginTransitionRequest) { r.Target = proto.String(string(ownerID)) },
		func(r *pb.BeginTransitionRequest) { r.Expected.Owner = proto.String(string(targetID)) },
		func(r *pb.BeginTransitionRequest) { r.Expected.Kind = netpb.TokenKind_TRANSITION_REVISION.Enum() },
		func(r *pb.BeginTransitionRequest) { r.SourceAddress = proto.String("") },
	} {
		w, s, ctx := fixture()
		r := beginRequest()
		mutate(r)
		if out, err := s.BeginTransition(ctx, r); out != nil || status.Code(err) != codes.InvalidArgument || w.calls != 0 {
			t.Fatalf("%v %v calls=%d", out, err, w.calls)
		}
	}
}
