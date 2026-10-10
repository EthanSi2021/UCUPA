package acceptance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	corepb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
	facetpb "ucupa/ucupa-facet/bindings/go/protobuf"
	facetsdk "ucupa/ucupa-facet/sdk/go"
)

func setWriterProductConfig(t *testing.T, dir string, writer map[string]any) {
	t.Helper()
	file := filepath.Join(dir, "service.json")
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var c map[string]any
	if err = json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	c["writer"] = writer
	raw, err = json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	writeNetworkFile(t, file, raw)
}

func rejectWriterProductStartup(t *testing.T, dir, program string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		program += ".exe"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, filepath.Join(dir, program), "-config", filepath.Join(dir, "service.json")).CombinedOutput()
	if err == nil || ctx.Err() != nil {
		t.Fatalf("unqualified startup did not fail closed: %v %s", err, out)
	}
}

func prepareWriterInstall(t *testing.T, dir, program string, i values.WriterInstallation) {
	t.Helper()
	// An ordinary product Open must not acquire a fenced Store.
	rejectWriterProductStartup(t, dir, program)
	secret := filepath.Join(dir, "writer-secret")
	wrong := i.Secret
	wrong[0] ^= 1
	writeNetworkFile(t, secret, wrong[:])
	setWriterProductConfig(t, dir, map[string]any{"handoff": true, "install_operation": []byte(i.Operation), "install_secret_file": secret})
	rejectWriterProductStartup(t, dir, program)
	writeNetworkFile(t, secret, i.Secret[:])
}

func writerRPCRequest(t *testing.T) (values.WriterInstallation, *netpb.WriterHandoffRequest) {
	t.Helper()
	i, err := values.NewWriterInstallation(string([]byte{255, 0, 128}) + "product-handoff")
	if err != nil {
		t.Fatal(err)
	}
	hash := i.Commitment()
	return i, &netpb.WriterHandoffRequest{Operation: []byte(i.Operation), ExpectedEpoch: proto.Uint64(1), Target: hash[:]}
}

func TestWriterCoreIndependentProduct(t *testing.T) {
	f := newNetworkFixtureWriter(t, false, "writer-product", &values.WriterStartup{Handoff: true})
	for _, p := range []values.PermissionID{corecontract.PermissionInspectWriter, corecontract.PermissionHandoffWriter} {
		f.grant(t, p, values.AuthorityScope(f.c.AuthorityRef()), false)
	}
	start := prepareNetworkProduct(t, f, map[string]any{"writer": map[string]any{"handoff": true}})
	stop := start()
	f.connect(t)
	ctx := context.Background()
	readReq := &corepb.ReadElementRequest{Handle: f.enter(t, corepb.Intent_READ), Address: &netpb.Address{}, PrepareObserve: proto.Bool(true)}
	read, err := f.client.Core().ReadElement(ctx, readReq)
	if err != nil || read.GetResult() == nil {
		t.Fatalf("before read %v %v", read, err)
	}
	observation, err := f.client.Core().BeginObserve(ctx, &corepb.BeginObserveRequest{Preparation: read.GetResult().GetPreparation()})
	if err != nil || observation.GetResult() == nil {
		t.Fatalf("prepare observation %v %v", observation, err)
	}
	request := &corepb.CreateAnchorRequest{Handle: f.enter(t, corepb.Intent_CREATE_ANCHOR), Element: proto.String(networkEntity), OperationId: []byte("before-fence"), Final: networkTuple(10)}
	written, err := f.client.Core().CreateAnchor(ctx, request)
	if err != nil || written.GetResult() == nil {
		t.Fatalf("write %v %v", written, err)
	}
	i, handoff := writerRPCRequest(t)
	done, err := f.client.Writer().HandoffWriter(ctx, handoff)
	if err != nil || !done.GetResult().GetPending() {
		t.Fatalf("handoff %v %v", done, err)
	}
	again, err := f.client.Writer().HandoffWriter(ctx, handoff)
	if err != nil || !proto.Equal(done, again) {
		t.Fatalf("exact retry %v %v", again, err)
	}
	old, err := f.client.Core().ReadElement(ctx, readReq)
	if err == nil && old.GetResult() != nil {
		t.Fatal("fenced Core disclosed old read")
	}
	changes, err := f.client.Core().PollChanges(ctx, &corepb.PollChangesRequest{Cursor: observation.GetResult().GetCursor(), Limit: proto.Uint32(1)})
	if err == nil && changes.GetResult() != nil {
		t.Fatal("fenced Core disclosed old observation")
	}
	writeAgain, err := f.client.Core().CreateAnchor(ctx, request)
	if err == nil && writeAgain.GetResult() != nil {
		t.Fatal("fenced Core disclosed old mutation")
	}
	stop()
	prepareWriterInstall(t, f.dir, "ucupa-core", i)
	stop = start()
	defer stop()
	if _, err = f.client.Writer().InspectWriter(ctx, &netpb.WriterInspectRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("old session: %v", err)
	}
	_ = f.client.Close()
	f.connect(t)
	installed, err := f.client.Writer().InspectWriter(ctx, &netpb.WriterInspectRequest{Operation: []byte(i.Operation)})
	if err != nil || installed.GetResult().GetEpoch() != 2 || !installed.GetResult().GetDecision().GetInstalled() {
		t.Fatalf("install %v %v", installed, err)
	}
	if !bytes.Equal(installed.GetResult().GetDecision().GetFinalCut(), done.GetResult().GetDecision().GetFinalCut()) {
		t.Fatal("final cut changed")
	}
	recovered, err := f.client.Core().RecoverOutcome(ctx, &corepb.RecoverOutcomeRequest{Reference: f.reference(), OperationId: request.OperationId, Request: &corepb.MutationCommand{Command: &corepb.MutationCommand_CreateAnchor{CreateAnchor: &corepb.CreateAnchorCommand{Element: request.Element, OperationId: request.OperationId, Final: request.Final}}}})
	if err != nil || !proto.Equal(written.GetResult(), recovered.GetResult()) {
		t.Fatalf("original result %v %v", recovered, err)
	}
	stop()
	setWriterProductConfig(t, f.dir, map[string]any{"handoff": true})
	stop = start()
	defer stop()
	_ = f.client.Close()
	f.connect(t)
	installed, err = f.client.Writer().InspectWriter(ctx, &netpb.WriterInspectRequest{})
	if err != nil || installed.GetResult().GetEpoch() != 2 {
		t.Fatalf("restart advanced epoch %v %v", installed, err)
	}
}

func verifyFacetWriterProduct(t *testing.T, dir string, client *facetsdk.Client, start func() func(), stop func(), login func(), get *facetpb.GetRequest, written *facetpb.PutResponse) {
	t.Helper()
	ctx := context.Background()
	i, request := writerRPCRequest(t)
	done, err := client.Writer().HandoffWriter(ctx, request)
	if err != nil || !done.GetResult().GetPending() {
		t.Fatalf("Facet fence %v %v", done, err)
	}
	again, err := client.Writer().HandoffWriter(ctx, request)
	if err != nil || !proto.Equal(done, again) {
		t.Fatalf("Facet exact retry %v %v", again, err)
	}
	if old, err := client.Facet().Get(ctx, get); err == nil && old.GetResult() != nil {
		t.Fatal("fenced Facet disclosed content")
	}
	stop()
	prepareWriterInstall(t, dir, "ucupa-facet", i)
	stop = start()
	defer stop()
	if _, err := client.Writer().InspectWriter(ctx, &netpb.WriterInspectRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("old Facet session %v", err)
	}
	login()
	installed, err := client.Writer().InspectWriter(ctx, &netpb.WriterInspectRequest{Operation: []byte(i.Operation)})
	if err != nil || installed.GetResult().GetEpoch() != 2 || !installed.GetResult().GetDecision().GetInstalled() {
		t.Fatalf("Facet install %v %v", installed, err)
	}
	read, err := client.Facet().Get(ctx, get)
	if err != nil || string(read.GetResult().GetPayload()) != `"durable"` {
		t.Fatalf("Facet data %v %v", read, err)
	}
	put := &facetpb.PutRequest{Access: get.Access, TypeRef: get.TypeRef, Expected: &facetpb.Expected{Condition: &facetpb.Expected_Absent{Absent: true}}, OperationId: []byte("product-put"), Payload: []byte(`"durable"`)}
	recovered, err := client.Facet().RecoverOutcome(ctx, &facetpb.RecoverOutcomeRequest{Access: get.Access, OperationId: put.OperationId, Request: &facetpb.MutationCommand{Command: &facetpb.MutationCommand_Put{Put: put}}})
	if err != nil || !proto.Equal(written.GetResult(), recovered.GetResult()) {
		t.Fatalf("Facet original result %v %v", recovered, err)
	}
}
