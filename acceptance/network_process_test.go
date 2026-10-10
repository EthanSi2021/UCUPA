package acceptance_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	networkpb "ucupa/shared/bindings/go/protobuf"
	corepb "ucupa/ucupa-core/bindings/go/protobuf"
)

func TestNetworkIndependentProcessReopen(t *testing.T) {
	f := newNetworkFixture(t, false)
	start := prepareNetworkProduct(t, f, nil)
	stop := start()
	f.connect(t)
	runNetworkClosure(t, f)
	// The response and its original operation binding survive process death.
	request := &corepb.CreateAnchorRequest{Handle: f.enter(t, corepb.Intent_CREATE_ANCHOR), Element: proto.String("00000000-0000-4000-8000-000000000003"), OperationId: []byte("process-recovery"), Final: networkTuple(10)}
	written, err := f.client.Core().CreateAnchor(context.Background(), request)
	if err != nil || written.GetResult() == nil {
		t.Fatalf("create: %v %v", written, err)
	}
	stop()
	_ = f.client.Close()
	_ = start()
	f.connect(t)
	recovered, err := f.client.Core().RecoverOutcome(context.Background(), &corepb.RecoverOutcomeRequest{Reference: f.reference(), OperationId: request.OperationId, Request: &corepb.MutationCommand{Command: &corepb.MutationCommand_CreateAnchor{CreateAnchor: &corepb.CreateAnchorCommand{Element: request.Element, OperationId: request.OperationId, Final: request.Final}}}})
	if err != nil || !proto.Equal(written.GetResult(), recovered.GetResult()) {
		t.Fatalf("reopen recovery: %v %v", recovered, err)
	}
}

func TestNetworkReadDelegationProductProcess(t *testing.T) {
	target, actor := newNetworkFixture(t, false), newNetworkFixture(t, false)
	caFile := filepath.Join(target.dir, "actor-ca.pem")
	writeNetworkFile(t, caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: actor.config.Certificate.Certificate[1]}))
	pin := actor.clientConfig.SPKISHA256
	start := prepareNetworkProduct(t, target, map[string]any{"read_delegation": map[string]any{
		"client_ca_file": caFile, "client_spki_sha256": map[string]string{string(actor.config.Expected): base64.RawURLEncoding.EncodeToString(pin[:])},
		"max_ttl": "10s", "max_grants": 4, "max_per_caller": 3,
	}})
	stop := start()
	target.connect(t)
	grant := issueReadDelegation(t, target, actor, true, 10, corepb.Intent_READ, corepb.Intent_QUERY)
	client := delegatedClient(t, target, actor, grant)
	read, err := client.Core().ReadElement(context.Background(), &corepb.ReadElementRequest{Handle: grant.Handle, Address: &networkpb.Address{}, PrepareObserve: proto.Bool(true)})
	if err != nil || read.GetResult() == nil {
		t.Fatalf("delegated product read: %v %v", read, err)
	}
	stop()
	_ = start()
	if _, err := client.Core().ReadElement(context.Background(), &corepb.ReadElementRequest{Handle: grant.Handle, Address: &networkpb.Address{}, PrepareObserve: proto.Bool(false)}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("product reopened with old delegation: %v", err)
	}
	_ = target.client.Close()
	target.connect(t)
	next := issueReadDelegation(t, target, actor, false, 10, corepb.Intent_READ)
	fresh := delegatedClient(t, target, actor, next)
	read, err = fresh.Core().ReadElement(context.Background(), &corepb.ReadElementRequest{Handle: next.Handle, Address: &networkpb.Address{}, PrepareObserve: proto.Bool(false)})
	if err != nil || read.GetResult() == nil {
		t.Fatalf("fresh product delegation: %v %v", read, err)
	}
}

// This launches the product binary, not a test-only server. Both tests retain
// independent durable owners and consume only generated public RPCs thereafter.
func prepareNetworkProduct(t *testing.T, f *networkFixture, extra map[string]any) func() func() {
	start := prepareNetworkProductControl(t, f, extra)
	return func() func() { return start().stop }
}

func prepareNetworkProductControl(t *testing.T, f *networkFixture, extra map[string]any) func() *serviceProductProcess {
	t.Helper()
	// Provision through real owner APIs, then close all in-process owners before
	// launching the actual product host. The caller thereafter uses public bytes.
	if err := f.c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.r.Close(); err != nil {
		t.Fatal(err)
	}
	certPath, keyPath := filepath.Join(f.dir, "tls.pem"), filepath.Join(f.dir, "tls.key")
	writeNetworkFile(t, certPath, f.certPEM)
	writeNetworkFile(t, keyPath, f.keyPEM)
	configuration := map[string]any{
		"listen": strings.TrimPrefix(string(f.config.NextHop), "https://"), "next_hop": f.config.NextHop, "expected_core": f.config.Expected, "realm_id": f.actor.Realm, "realm_next_hop": f.realmAddress,
		"core_store": filepath.Join(f.dir, "core.db"), "realm_store": filepath.Join(f.dir, "realm.db"), "certificate_file": certPath, "private_key_file": keyPath,
		"session_ttl": "30s", "rpc_deadline": "5s", "handshake_timeout": "1s", "connection_lifetime": "1m", "shutdown_timeout": "1s",
		"max_sessions": 16, "max_sessions_per_caller": 8, "max_connections": 16, "max_concurrent_rpcs": 8, "max_streams": 8, "max_metadata_bytes": 8192, "max_response_bytes": 8 << 20,
		"context_ttl": "1m", "observation_ttl": "1m",
	}
	for name, value := range extra {
		configuration[name] = value
	}
	if configuration["realm_profile"] == "network" {
		delete(configuration, "realm_store")
		delete(configuration, "realm_next_hop")
	}
	return prepareServiceProductControl(t, f.dir, "ucupa-core", configuration)
}

func prepareServiceProduct(t *testing.T, dir, program string, configuration map[string]any) func() func() {
	start := prepareServiceProductControl(t, dir, program, configuration)
	return func() func() { return start().stop }
}

type serviceProductProcess struct {
	t       *testing.T
	cmd     *exec.Cmd
	stderr  *bytes.Buffer
	stopped bool
}

func (p *serviceProductProcess) stop() {
	if !p.stopped {
		p.stopped = true
		_ = p.cmd.Process.Kill()
		_ = p.cmd.Wait()
	}
}

func (p *serviceProductProcess) gracefulStop() {
	p.t.Helper()
	if p.stopped {
		return
	}
	if err := interruptProductProcess(p.cmd); err != nil {
		p.stop()
		p.t.Fatalf("send product orderly stop: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- p.cmd.Wait() }()
	select {
	case err := <-done:
		p.stopped = true
		if err != nil {
			p.t.Fatalf("product orderly stop: %v %s", err, p.stderr.String())
		}
	case <-time.After(5 * time.Second):
		_ = p.cmd.Process.Kill()
		<-done
		p.stopped = true
		p.t.Fatal("product orderly stop timed out")
	}
}

func prepareServiceProductControl(t *testing.T, dir, program string, configuration map[string]any) func() *serviceProductProcess {
	t.Helper()
	raw, err := json.Marshal(configuration)
	if err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(dir, "service.json")
	writeNetworkFile(t, cfgPath, raw)
	goTool := filepath.Join(runtime.GOROOT(), "bin", "go")
	exe := filepath.Join(dir, program)
	if runtime.GOOS == "windows" {
		goTool += ".exe"
		exe += ".exe"
	}
	buildCtx, cancelBuild := context.WithTimeout(context.Background(), time.Minute)
	defer cancelBuild()
	build := exec.CommandContext(buildCtx, goTool, "build", "-o", exe, "./cmd/"+program)
	build.Dir = ".."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build host: %v %s", err, out)
	}
	return func() *serviceProductProcess {
		startupLimit := time.Minute
		if os.Getenv("UCUPA_P8_MEASURE") == "1" {
			startupLimit = 5 * time.Minute
		}
		startupCtx, cancelStartup := context.WithTimeout(context.Background(), startupLimit)
		defer cancelStartup()
		cmd := exec.Command(exe, "-config", cfgPath)
		configureProductProcess(cmd)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		ready := make(chan string, 1)
		go func() {
			scanner := bufio.NewScanner(stdout)
			if scanner.Scan() {
				ready <- scanner.Text()
			} else {
				ready <- ""
			}
		}()
		process := &serviceProductProcess{t: t, cmd: cmd, stderr: &stderr}
		t.Cleanup(process.stop)
		select {
		case line := <-ready:
			if !strings.HasPrefix(line, program+": ready ") {
				process.stop()
				t.Fatalf("host not ready: %s %s", line, stderr.String())
			}
		case <-startupCtx.Done():
			process.stop()
			t.Fatalf("host startup timeout: %s", stderr.String())
		}
		return process
	}
}
