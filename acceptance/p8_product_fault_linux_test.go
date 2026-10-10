//go:build linux

package acceptance_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	netpb "ucupa/shared/bindings/go/protobuf"
	facetpb "ucupa/ucupa-facet/bindings/go/protobuf"
	facetsdk "ucupa/ucupa-facet/sdk/go"
)

// This opt-in fault test may fill only a separately mounted, bounded volume.
// It is never enabled by the ordinary repository verifier.
func verifyP8IsolatedDiskFull(t *testing.T, facetDir, coreDir string, client *facetsdk.Client, access *facetpb.Access, typ []byte, previous *netpb.Token, restartFacet func()) {
	t.Helper()
	mount := os.Getenv("UCUPA_P8_FAULT_MOUNT")
	if mount == "" {
		t.Fatal("UCUPA_P8_DISK_FULL requires UCUPA_P8_FAULT_MOUNT")
	}
	mount, err := filepath.EvalSymlinks(mount)
	if err != nil {
		t.Fatal(err)
	}
	mount, err = filepath.Abs(mount)
	if err != nil {
		t.Fatal(err)
	}
	volume := p8Device(t, mount)
	if volume == p8Device(t, filepath.Dir(mount)) || volume == p8Device(t, "/") {
		t.Fatal("fault mount is not an isolated filesystem")
	}
	for _, dir := range []string{facetDir, coreDir, os.TempDir()} {
		resolved, err := filepath.EvalSymlinks(dir)
		if err != nil {
			t.Fatal(err)
		}
		rel, err := filepath.Rel(mount, resolved)
		if err != nil || rel == ".." || filepath.IsAbs(rel) || len(rel) >= 3 && rel[:3] == "../" || p8Device(t, dir) != volume {
			t.Fatalf("fault fixture is outside isolated mount: %q", dir)
		}
	}
	var stats syscall.Statfs_t
	if err := syscall.Statfs(mount, &stats); err != nil {
		t.Fatal(err)
	}
	capacity := stats.Blocks * uint64(stats.Bsize)
	if capacity == 0 || capacity > 512<<20 {
		t.Fatalf("fault filesystem is not bounded to 512 MiB: %d bytes", capacity)
	}

	// Force a real ENOSPC before issuing a fresh public mutation. The filler is
	// disposable and is removed before the original owner Store is reopened.
	fillerPath := filepath.Join(facetDir, "p8-isolated-disk-filler")
	filler, err := os.OpenFile(fillerPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(fillerPath) })
	block := make([]byte, 1<<20)
	var written uint64
	full := false
	for written <= capacity {
		n, writeErr := filler.Write(block)
		written += uint64(n)
		if errors.Is(writeErr, syscall.ENOSPC) {
			full = true
			break
		}
		if writeErr != nil {
			t.Fatalf("fill isolated volume: %v", writeErr)
		}
	}
	if err := filler.Close(); err != nil && !errors.Is(err, syscall.ENOSPC) {
		t.Fatal(err)
	}
	if !full {
		t.Fatal("isolated volume did not return ENOSPC")
	}
	t.Logf("P8 isolated volume returned ENOSPC after %d filler bytes", written)

	ctx := context.Background()
	payload := bytes.Repeat([]byte{'p'}, 1<<20)
	request := &facetpb.PutRequest{Access: access, TypeRef: typ, Expected: &facetpb.Expected{Condition: &facetpb.Expected_Revision{Revision: previous}}, OperationId: []byte("p8-disk-full-put"), Payload: payload}
	out, err := client.Facet().Put(ctx, request)
	if err == nil && out.GetResult() != nil {
		t.Fatal("public Put committed despite verified ENOSPC; fault did not exercise a Store write")
	}
	if err := os.Remove(fillerPath); err != nil {
		t.Fatalf("release isolated volume: %v", err)
	}
	p8ResolveFaultedPut(t, "p8-disk-full", client, request, restartFacet)
}

// A separate controller attaches strace to the ready product PID and injects
// SIGKILL on its first pwrite64 after the gate opens. The test requires the
// resulting syscall trace before it treats the failed RPC as an in-flight kill.
func verifyP8InFlightCrash(t *testing.T, pid int, client *facetsdk.Client, access *facetpb.Access, typ []byte, previous *netpb.Token, restartFacet func()) {
	t.Helper()
	gate, trace := os.Getenv("UCUPA_P8_CRASH_GATE"), os.Getenv("UCUPA_P8_CRASH_TRACE")
	if gate == "" || trace == "" {
		t.Fatal("in-flight crash requires a gate and trace path")
	}
	for _, path := range []string{gate + ".ready", gate + ".go", trace} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("crash-control path must not exist before run: %s (%v)", path, err)
		}
	}
	ready, err := os.OpenFile(gate+".ready", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ready.WriteString(strconv.Itoa(pid)); err != nil {
		t.Fatal(err)
	}
	if err := ready.Close(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Remove(gate + ".ready")
		_ = os.Remove(gate + ".go")
	})
	t.Logf("P8 Facet product PID %d ready for in-flight pwrite64 SIGKILL", pid)
	deadline := time.Now().Add(2 * time.Minute)
	for {
		if _, err := os.Stat(gate + ".go"); err == nil {
			break
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		if time.Now().After(deadline) {
			t.Fatal("in-flight crash controller did not open gate")
		}
		time.Sleep(100 * time.Millisecond)
	}
	request := &facetpb.PutRequest{Access: access, TypeRef: typ, Expected: &facetpb.Expected{Condition: &facetpb.Expected_Revision{Revision: previous}}, OperationId: []byte("p8-inflight-crash-put"), Payload: bytes.Repeat([]byte{'c'}, 1<<20)}
	out, err := client.Facet().Put(context.Background(), request)
	if err == nil && out.GetResult() != nil {
		t.Fatal("in-flight crash request returned success")
	}
	deadline = time.Now().Add(10 * time.Second)
	for {
		data, err := os.ReadFile(trace)
		if err == nil && bytes.Contains(data, []byte("pwrite64(")) && bytes.Contains(data, []byte("facet.db>")) && bytes.Contains(data, []byte(strconv.Itoa(pid)+" +++ killed by SIGKILL +++")) {
			t.Logf("P8 observed SIGKILL on in-flight pwrite64 of Facet PID %d", pid)
			break
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("missing in-flight pwrite64 SIGKILL evidence in %s: %v", trace, err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	p8ResolveFaultedPut(t, "p8-inflight-crash", client, request, restartFacet)
}

func p8ResolveFaultedPut(t *testing.T, label string, client *facetsdk.Client, request *facetpb.PutRequest, restartFacet func()) {
	t.Helper()
	restartFacet()
	ctx := context.Background()
	access, typ, payload := request.Access, request.TypeRef, request.Payload
	recovered, err := client.Facet().RecoverOutcome(ctx, &facetpb.RecoverOutcomeRequest{Access: access, OperationId: request.OperationId,
		Request: &facetpb.MutationCommand{Command: &facetpb.MutationCommand_Put{Put: request}}})
	if err != nil {
		t.Fatalf("recover original operation from original Store: %v", err)
	}
	switch {
	case recovered.GetResult() != nil:
		t.Logf("P8 %s operation recovered as committed", label)
	case recovered.GetFailure() == netpb.Failure_OUTCOME_UNKNOWN:
		before, err := client.Facet().Get(ctx, &facetpb.GetRequest{Access: access, TypeRef: typ})
		if err != nil || before.GetResult() == nil || string(before.GetResult().Payload) != `"network"` {
			t.Fatalf("unknown outcome exposed partial or changed payload: %v %v", before, err)
		}
		retried, err := client.Facet().Put(ctx, request)
		if err != nil || retried.GetResult() == nil {
			t.Fatalf("retry exact original operation after %s: %v %v", label, retried, err)
		}
		recovered, err = client.Facet().RecoverOutcome(ctx, &facetpb.RecoverOutcomeRequest{Access: access, OperationId: request.OperationId,
			Request: &facetpb.MutationCommand{Command: &facetpb.MutationCommand_Put{Put: request}}})
		if err != nil || !proto.Equal(retried.GetResult(), recovered.GetResult()) {
			t.Fatalf("retried original receipt changed: %v %v", recovered, err)
		}
	default:
		t.Fatalf("unexpected %s operation outcome: %v", label, recovered)
	}
	read, err := client.Facet().Get(ctx, &facetpb.GetRequest{Access: access, TypeRef: typ})
	if err != nil || read.GetResult() == nil || !bytes.Equal(read.GetResult().Payload, payload) {
		t.Fatalf("recovered write is not fully visible: %v %v", read, err)
	}
	restore, err := client.Facet().Put(ctx, &facetpb.PutRequest{Access: access, TypeRef: typ, Expected: &facetpb.Expected{Condition: &facetpb.Expected_Revision{Revision: read.GetResult().Revision}}, OperationId: []byte(label + "-restore"), Payload: []byte(`"network"`)})
	if err != nil || restore.GetResult() == nil {
		t.Fatalf("restore fixture content: %v %v", restore, err)
	}
}

func p8Device(t *testing.T, path string) uint64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return uint64(info.Sys().(*syscall.Stat_t).Dev)
}
