package acceptance_test

import (
	"context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"testing"
	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	"ucupa/ucupa-core/contract"
)

// This is a real source product process, not a complete two-owner transfer.
// INTENT and cancellation must survive process loss without fencing content.
func TestTransitionSourceIndependentProduct(t *testing.T) {
	f := newNetworkFixture(t, false)
	for _, permission := range []values.PermissionID{contract.PermissionTransitionExport, contract.PermissionTransitionInspect} {
		f.grant(t, permission, values.CoreElementScope(f.c.AuthorityRef(), values.ElementID(networkEntity)), false)
	}
	start := prepareNetworkProduct(t, f, nil)
	stop := start()
	defer stop()
	f.connect(t)
	ctx := context.Background()
	written, err := f.client.Core().CreateAnchor(ctx, &pb.CreateAnchorRequest{Handle: f.enter(t, pb.Intent_CREATE_ANCHOR), OperationId: []byte("element"), Element: proto.String(networkEntity), Final: networkTuple(4)})
	if err != nil || written.GetResult() == nil {
		t.Fatalf("create: %v %v", written, err)
	}
	address := &netpb.Address{Down: &netpb.ElementPath{Items: []string{networkEntity}}}
	read, err := f.client.Core().ReadElement(ctx, &pb.ReadElementRequest{Handle: f.enter(t, pb.Intent_READ), Address: address})
	if err != nil || read.GetResult().GetRecord() == nil {
		t.Fatalf("read: %v %v", read, err)
	}
	request := &pb.BeginTransitionRequest{OperationId: []byte{255, 0, 128, 1}, Kind: pb.TransitionKind_TRANSITION_SLICE.Enum(), Target: proto.String("00000000-0000-4000-8000-000000000031"), Boundary: proto.String(networkEntity), Expected: read.GetResult().GetRecord().GetRevision(), Final: networkTuple(4), SourceAddress: proto.String(string(f.config.NextHop)), TargetAddress: proto.String("https://target.example:443")}
	intent, err := f.client.Transitions().BeginTransition(ctx, request)
	if err != nil || intent.GetResult().GetProgress() != pb.TransitionProgress_TRANSITION_INTENT {
		t.Fatalf("begin: %v %v", intent, err)
	}
	retry, err := f.client.Transitions().BeginTransition(ctx, request)
	if err != nil || !proto.Equal(intent, retry) {
		t.Fatalf("retry: %v %v", retry, err)
	}
	wrong := proto.Clone(request).(*pb.BeginTransitionRequest)
	wrong.TargetAddress = proto.String("https://other.example:443")
	if r, e := f.client.Transitions().BeginTransition(ctx, wrong); e != nil || r.GetError() != netpb.Failure_CONFLICT {
		t.Fatalf("changed input: %v %v", r, e)
	}
	inspect := &pb.InspectTransitionRequest{OperationId: request.OperationId, Boundary: request.Boundary}
	stop()
	stop = start()
	defer stop()
	if _, err = f.client.Transitions().InspectTransition(ctx, inspect); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("old session after restart: %v", err)
	}
	_ = f.client.Close()
	f.connect(t)
	retry, err = f.client.Transitions().BeginTransition(ctx, request)
	if err != nil || !proto.Equal(intent, retry) {
		t.Fatalf("restart original retry: %v %v", retry, err)
	}
	cancel := &pb.CancelTransitionRequest{OperationId: request.OperationId, Boundary: request.Boundary, Expected: intent.GetResult().GetRevision()}
	ended, err := f.client.Transitions().CancelTransition(ctx, cancel)
	if err != nil || ended.GetResult().GetProgress() != pb.TransitionProgress_TRANSITION_CANCELLED {
		t.Fatalf("cancel: %v %v", ended, err)
	}
	stop()
	stop = start()
	defer stop()
	_ = f.client.Close()
	f.connect(t)
	retry, err = f.client.Transitions().CancelTransition(ctx, cancel)
	if err != nil || !proto.Equal(ended, retry) {
		t.Fatalf("restart exact cancel: %v %v", retry, err)
	}
	retry, err = f.client.Transitions().BeginTransition(ctx, request)
	if err != nil || !proto.Equal(ended, retry) {
		t.Fatalf("cancelled ID revived: %v %v", retry, err)
	}
	read, err = f.client.Core().ReadElement(ctx, &pb.ReadElementRequest{Handle: f.enter(t, pb.Intent_READ), Address: address})
	if err != nil || read.GetResult().GetRecord() == nil || !proto.Equal(request.Expected, read.GetResult().GetRecord().GetRevision()) {
		t.Fatalf("intent/cancel changed source: %v %v", read, err)
	}
}

func TestTransitionNetworkPermissionIsNotCRUD(t *testing.T) {
	f := newNetworkFixture(t, true)
	ctx := context.Background()
	read, err := f.client.Core().ReadElement(ctx, &pb.ReadElementRequest{Handle: f.enter(t, pb.Intent_READ), Address: &netpb.Address{}})
	if err != nil || read.GetResult().GetRecord() == nil {
		t.Fatalf("read root: %v %v", read, err)
	}
	request := &pb.BeginTransitionRequest{OperationId: []byte("root-transfer"), Kind: pb.TransitionKind_TRANSITION_ROOT.Enum(), Target: proto.String("00000000-0000-4000-8000-000000000031"), Boundary: proto.String(string(networkRoot)), Expected: read.GetResult().GetRecord().GetRevision(), SourceAddress: proto.String(string(f.config.NextHop)), TargetAddress: proto.String("https://target.example:443")}
	if r, e := f.client.Transitions().BeginTransition(ctx, request); e != nil || r.GetError() != netpb.Failure_DENIED {
		t.Fatalf("CRUD authorized export: %v %v", r, e)
	}
	scope := values.CoreElementScope(f.c.AuthorityRef(), networkRoot)
	f.grant(t, contract.PermissionTransitionExport, scope, false)
	intent, err := f.client.Transitions().BeginTransition(ctx, request)
	if err != nil || intent.GetResult() == nil {
		t.Fatalf("explicit export: %v %v", intent, err)
	}
	inspect := &pb.InspectTransitionRequest{OperationId: request.OperationId, Boundary: request.Boundary}
	if r, e := f.client.Transitions().InspectTransition(ctx, inspect); e != nil || r.GetError() != netpb.Failure_DENIED {
		t.Fatalf("export authorized inspection: %v %v", r, e)
	}
	f.grant(t, contract.PermissionTransitionInspect, scope, false)
	f.grant(t, contract.PermissionTransitionExport, scope, true)
	cancel := &pb.CancelTransitionRequest{OperationId: request.OperationId, Boundary: request.Boundary, Expected: intent.GetResult().GetRevision()}
	if r, e := f.client.Transitions().CancelTransition(ctx, cancel); e != nil || r.GetError() != netpb.Failure_DENIED {
		t.Fatalf("revoked export cancelled: %v %v", r, e)
	}
	if r, e := f.client.Transitions().InspectTransition(ctx, inspect); e != nil || !proto.Equal(intent, r) {
		t.Fatalf("denied cancel mutated: %v %v", r, e)
	}
}

// Two actual ucupa-core executables own their Stores. No test-side owner is
// called to fence, accept, commit, install, or settle the transferred slice.
func TestTransitionRecoveryIndependentProducts(t *testing.T) {
	target, source, b := transitionApprovalSetup(t)
	scope := values.CoreElementScope(source.c.AuthorityRef(), values.ElementID(networkEntity))
	for _, permission := range []values.PermissionID{contract.PermissionTransitionExport, contract.PermissionTransitionCommit, contract.PermissionTransitionInspect} {
		source.grant(t, permission, scope, false)
	}
	target.grant(t, contract.PermissionTransitionImport, values.CoreElementScope(target.c.AuthorityRef(), values.ElementID(networkEntity)), false)
	source.server.Stop()
	_ = source.client.Close()
	configuration := func(host, peer *networkFixture) map[string]any {
		return map[string]any{
			"read_delegation":        identityTrustFiles(t, host, peer),
			"transition_connections": map[string]any{"peers": mountProductConnections(t, host, peer), "max_concurrent_pulls": 4, "pull_deadline": "5s"},
			"transition_approvals":   map[string]any{"max_ttl": "30s", "max_approvals": 8, "max_per_caller": 6},
		}
	}
	startSource := prepareNetworkProduct(t, source, configuration(source, target))
	startTarget := prepareNetworkProduct(t, target, configuration(target, source))
	stopSource, stopTarget := startSource(), startTarget()
	defer func() { stopSource(); stopTarget() }()
	source.connect(t)
	target.connect(t)
	ctx := context.Background()
	// Hold a public source observation across the final cut. A completed
	// transfer must end this old reading context rather than silently serving
	// the departed element as an ordinary source change.
	oldHandle := source.enter(t, pb.Intent_READ)
	oldRead, err := source.client.Core().ReadElement(ctx, &pb.ReadElementRequest{
		Handle:         oldHandle,
		Address:        &netpb.Address{Down: &netpb.ElementPath{Items: []string{networkEntity}}},
		PrepareObserve: proto.Bool(true),
	})
	if err != nil || oldRead.GetResult() == nil || oldRead.GetResult().Preparation == nil {
		t.Fatalf("prepare source observation: %v %v", oldRead, err)
	}
	oldObservation, err := source.client.Core().BeginObserve(ctx, &pb.BeginObserveRequest{Preparation: oldRead.GetResult().Preparation})
	if err != nil || oldObservation.GetResult() == nil {
		t.Fatalf("begin source observation: %v %v", oldObservation, err)
	}
	intent, err := source.client.Transitions().BeginTransition(ctx, b.Command)
	if err != nil || intent.GetResult().GetProgress() != pb.TransitionProgress_TRANSITION_INTENT {
		t.Fatalf("intent: %v %v", intent, err)
	}
	approval := createTransitionApproval(t, target, b, 30)
	recover := &pb.RecoverTransitionRequest{Binding: b, ApprovalToken: approval.Token}
	wrong := proto.Clone(recover).(*pb.RecoverTransitionRequest)
	wrong.Binding.Command.Final = networkTuple(5)
	if r, err := source.client.Transitions().RecoverTransition(ctx, wrong); err == nil && r.GetResult() != nil {
		t.Fatalf("changed input recovered: %v", r)
	}
	inspect := &pb.InspectTransitionRequest{OperationId: b.Command.OperationId, Boundary: b.Command.Boundary}
	if r, err := source.client.Transitions().InspectTransition(ctx, inspect); err != nil || !proto.Equal(r, intent) {
		t.Fatalf("changed binding fenced source: %v %v", r, err)
	}
	// First acceptance needs the original live target approval, not just CRUD
	// rights or an authenticated service certificate.
	if _, err := target.client.TransitionApprovals().RevokeTransitionApproval(ctx, &pb.RevokeTransitionApprovalRequest{Token: approval.Token}); err != nil {
		t.Fatal(err)
	}
	if r, err := source.client.Transitions().RecoverTransition(ctx, recover); err == nil && r.GetResult() != nil {
		t.Fatalf("revoked preflight advanced: %v", r)
	}
	if r, err := source.client.Transitions().InspectTransition(ctx, inspect); err != nil || !proto.Equal(r, intent) {
		t.Fatalf("revoked preflight fenced source: %v %v", r, err)
	}
	approval = createTransitionApproval(t, target, b, 30)
	recover.ApprovalToken = approval.Token
	finished, err := source.client.Transitions().RecoverTransition(ctx, recover)
	if err != nil || finished.GetResult().GetProgress() != pb.TransitionProgress_TRANSITION_SETTLED {
		t.Fatalf("recover: %v %v", finished, err)
	}
	// The settled transfer must not silently GAP the old observation. While the
	// original authorization is alive and its retention window holds, the old
	// stream discloses the one terminal successor evidence of the ended scope
	// with the transition's specified contact, then ends.
	terminalPoll, err := source.client.Core().PollChanges(ctx, &pb.PollChangesRequest{Cursor: oldObservation.GetResult().Cursor})
	if err != nil || terminalPoll.GetResult() == nil || len(terminalPoll.GetResult().Groups) != 1 {
		t.Fatalf("old observation lost terminal evidence: %v %v", terminalPoll, err)
	}
	terminalGroup := terminalPoll.GetResult().GetGroups()[0]
	if len(terminalGroup.GetCategory()) != 1 || terminalGroup.GetCategory()[0] != pb.Category_TERMINAL || terminalGroup.GetTerminal() == nil {
		t.Fatalf("old observation missed the exclusive terminal group: %v", terminalGroup)
	}
	statement := terminalGroup.GetTerminal()
	if string(statement.GetOperationId()) != string(b.Command.GetOperationId()) ||
		statement.GetSource() != string(source.c.AuthorityRef()) ||
		statement.GetTarget() != b.Command.GetTarget() ||
		statement.GetFinalCut() == 0 || !statement.GetSettled() ||
		statement.GetNextHop() != b.Command.GetTargetAddress() {
		t.Fatalf("terminal successor evidence mismatch: %v vs %v", statement, b.Command)
	}
	reread, err := source.client.Core().PollChanges(ctx, &pb.PollChangesRequest{Cursor: oldObservation.GetResult().Cursor})
	if err != nil || !proto.Equal(terminalPoll, reread) {
		t.Fatalf("terminal evidence not idempotent at its cursor: %v %v", reread, err)
	}
	exhausted, err := source.client.Core().PollChanges(ctx, &pb.PollChangesRequest{Cursor: terminalPoll.GetResult().NextCursor})
	if err != nil || exhausted.GetFailure() != netpb.Failure_GAP {
		t.Fatalf("exhausted terminal window kept streaming: %v %v", exhausted, err)
	}
	oldCurrent, err := source.client.Core().ReadElement(ctx, &pb.ReadElementRequest{
		Handle: oldHandle, Basis: oldRead.GetResult().Receipt,
		Address: &netpb.Address{Down: &netpb.ElementPath{Items: []string{networkEntity}}},
	})
	if err != nil || oldCurrent.GetFailure() != netpb.Failure_INVALID_CONTEXT {
		t.Fatalf("old read basis crossed source fence: %v %v", oldCurrent, err)
	}
	readTarget := func() *pb.ReadElementResponse {
		t.Helper()
		r, err := target.client.Core().ReadElement(ctx, &pb.ReadElementRequest{Handle: target.enter(t, pb.Intent_READ), Address: &netpb.Address{Down: &netpb.ElementPath{Items: []string{networkEntity}}}})
		if err != nil || r.GetResult().GetRecord() == nil {
			t.Fatalf("installed read: %v %v", r, err)
		}
		return r
	}
	installed := readTarget()
	newRead, err := target.client.Core().ReadElement(ctx, &pb.ReadElementRequest{
		Handle:         target.enter(t, pb.Intent_READ),
		Address:        &netpb.Address{Down: &netpb.ElementPath{Items: []string{networkEntity}}},
		PrepareObserve: proto.Bool(true),
	})
	if err != nil || newRead.GetResult() == nil || newRead.GetResult().Preparation == nil {
		t.Fatalf("prepare target observation: %v %v", newRead, err)
	}
	newObservation, err := target.client.Core().BeginObserve(ctx, &pb.BeginObserveRequest{Preparation: newRead.GetResult().Preparation})
	if err != nil || newObservation.GetResult() == nil {
		t.Fatalf("begin target observation: %v %v", newObservation, err)
	}
	updated, err := target.client.Core().UpdateAnchor(ctx, &pb.UpdateAnchorRequest{
		Handle: target.enter(t, pb.Intent_UPDATE_ANCHOR), Element: proto.String(networkEntity),
		Expected: installed.GetResult().GetRecord().Revision, Final: networkTuple(5), OperationId: []byte("after-transfer-update"),
	})
	if err != nil || updated.GetResult() == nil {
		t.Fatalf("target update: %v %v", updated, err)
	}
	newPoll, err := target.client.Core().PollChanges(ctx, &pb.PollChangesRequest{Cursor: newObservation.GetResult().Cursor})
	if err != nil || newPoll.GetResult() == nil || len(newPoll.GetResult().Groups) != 1 ||
		newPoll.GetResult().Groups[0].GetOwner() != string(target.c.AuthorityRef()) {
		t.Fatalf("target observation did not begin a fresh stream: %v %v", newPoll, err)
	}
	// A network loss surfaces as transport unavailability, never a fake GAP
	// or a revived old stream.
	stopSource()
	if r, err := source.client.Core().PollChanges(ctx, &pb.PollChangesRequest{Cursor: oldObservation.GetResult().Cursor}); status.Code(err) != codes.Unavailable {
		t.Fatalf("disconnected terminal poll: %v %v", r, err)
	}
	stopTarget()
	stopSource, stopTarget = startSource(), startTarget()
	_ = source.client.Close()
	_ = target.client.Close()
	source.connect(t)
	target.connect(t)
	// The in-memory observation queue does not survive a product restart: the
	// same old cursor resolves to GAP, never a replayed stream or evidence.
	if r, err := source.client.Core().PollChanges(ctx, &pb.PollChangesRequest{Cursor: oldObservation.GetResult().Cursor}); err != nil || r.GetFailure() != netpb.Failure_GAP {
		t.Fatalf("old cursor replayed after restart: %v %v", r, err)
	}
	// Sessions and approvals are gone, but the original completed obligation is
	// durable. Recovery must return its exact result, not run a new transfer.
	repeated, err := source.client.Transitions().RecoverTransition(ctx, recover)
	if err != nil || !proto.Equal(repeated, finished) {
		t.Fatalf("reopen original recovery: %v %v", repeated, err)
	}
	if r, err := source.client.Transitions().RecoverTransition(ctx, wrong); err != nil || r.GetError() != netpb.Failure_CONFLICT {
		t.Fatalf("completed operation accepted a changed binding: %v %v", r, err)
	}
	if current := readTarget(); !proto.Equal(updated.GetResult().GetRecord(), current.GetResult().GetRecord()) {
		t.Fatal("updated target record changed across product restart")
	}
}
