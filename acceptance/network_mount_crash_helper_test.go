package acceptance_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ucupa/composition"
	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
	coregrpc "ucupa/ucupa-core/transport/grpc"
	realminstance "ucupa/ucupa-realm/instance"
)

type mountCrashConfig struct {
	Dir, Store, Phase, PeerCA string
	Expected, Peer            values.FactAuthorityRef
	PeerPin                   [32]byte
	NextHop                   values.NextHop
	RealmNextHop              values.NextHop
}
type crashMountOwner struct {
	coregrpc.MountControl
	phase string
}

func (o crashMountOwner) PrepareApprovedMountControl(ctx context.Context, c values.CallerContext, source values.FactAuthorityRef, r corecontract.MountPrepareRequest, view values.AuthorizationView, digest [32]byte) (corecontract.MountPrepared, error) {
	if o.phase == "before-prepare" {
		os.Exit(41)
	}
	result, err := o.MountControl.PrepareApprovedMountControl(ctx, c, source, r, view, digest)
	if err == nil && result.Prepared && o.phase == "after-prepare" {
		os.Exit(41)
	}
	return result, err
}
func (o crashMountOwner) MountPeer() corecontract.MountPeerPort {
	return crashMountPeer{MountPeerPort: o.MountControl.MountPeer(), phase: o.phase}
}

type crashMountPeer struct {
	corecontract.MountPeerPort
	phase string
}

func (p crashMountPeer) Finish(ctx context.Context, source values.FactAuthorityRef, r corecontract.MountFinishRequest) (corecontract.MountClosed, error) {
	if p.phase == "before-finish" {
		os.Exit(41)
	}
	result, err := p.MountPeerPort.Finish(ctx, source, r)
	if err == nil && p.phase == "after-finish" {
		os.Exit(41)
	}
	return result, err
}

// This subprocess uses the production owner/authenticator/gRPC transport. The
// only test substitution is abrupt exit immediately around durable transitions;
// it neither implements a second protocol nor fabricates prepared evidence.
func TestNetworkMountCrashHelper(t *testing.T) {
	path := os.Getenv("UCUPA_MOUNT_CRASH_CONFIG")
	if path == "" {
		t.Skip("subprocess only")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var c mountCrashConfig
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	r, err := realminstance.OpenDurable(filepath.Join(c.Dir, "realm.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	links := &composition.LocalRealmConnections{}
	if err = links.BindEndpoint(c.RealmNextHop, r); err != nil {
		t.Fatal(err)
	}
	o, err := core.OpenDurable(links, c.Store, corecontract.Config{PublicAccessPolicy: &values.PublicAccessPolicy{ContextTTL: time.Minute, ObservationTTL: time.Minute}})
	if err != nil {
		t.Fatal(err)
	}
	defer o.Close()
	cert, err := tls.LoadX509KeyPair(filepath.Join(c.Dir, "tls.pem"), filepath.Join(c.Dir, "tls.key"))
	if err != nil {
		t.Fatal(err)
	}
	ca, err := os.ReadFile(c.PeerCA)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		t.Fatal("invalid test CA")
	}
	config := coregrpc.Config{Expected: c.Expected, NextHop: c.NextHop, Certificate: cert, SessionTTL: 30 * time.Second, RPCDeadline: 5 * time.Second, HandshakeTimeout: time.Second, ConnectionLifetime: time.Minute, ShutdownTimeout: time.Second, MaxSessions: 16, MaxSessionsPerCaller: 8, MaxConnections: 16, MaxConcurrentRPCs: 8, MaxStreams: 8, MaxMetadataBytes: 8192, MaxResponseBytes: 8 << 20,
		ReadDelegation: &coregrpc.ReadDelegationConfig{ClientRoots: roots, Peers: map[values.FactAuthorityRef][32]byte{c.Peer: c.PeerPin}, MaxTTL: 30 * time.Second, MaxGrants: 8, MaxPerCaller: 6}, MountApprovals: &coregrpc.MountApprovalConfig{MaxTTL: 30 * time.Second, MaxApprovals: 8, MaxPerCaller: 6}}
	config.MountControl = crashMountOwner{MountControl: o.Mounts(), phase: c.Phase}
	s, err := coregrpc.New(o.Access(), composition.RealmNetworkAuthentication{Connections: links, Owner: o.Access()}, config)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
	l, err := net.Listen("tcp", strings.TrimPrefix(string(c.NextHop), "https://"))
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("mount-crash: ready")
	if err := s.Serve(l); err != nil {
		t.Fatal(err)
	}
}

func startMountCrashProduct(t *testing.T, c mountCrashConfig) func() {
	t.Helper()
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(c.Dir, "crash.json")
	writeNetworkFile(t, path, raw)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^TestNetworkMountCrashHelper$", "-test.count=1")
	cmd.Env = append(os.Environ(), "UCUPA_MOUNT_CRASH_CONFIG="+path)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	finished := false
	t.Cleanup(func() {
		if !finished {
			_ = cmd.Process.Kill()
			<-done
			finished = true
		}
	})
	ready := make(chan string, 1)
	go func() {
		scan := bufio.NewScanner(stdout)
		if scan.Scan() {
			ready <- scan.Text()
		} else {
			ready <- ""
		}
	}()
	select {
	case line := <-ready:
		if line != "mount-crash: ready" {
			t.Fatalf("crash helper not ready: %s", line)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("crash helper startup timeout")
	}
	return func() {
		t.Helper()
		select {
		case err := <-done:
			finished = true
			if err == nil || cmd.ProcessState.ExitCode() != 41 {
				t.Fatalf("expected injected abrupt exit, got %v: %s", err, stderr.String())
			}
		case <-time.After(10 * time.Second):
			t.Fatal("fault point not reached")
		}
	}
}
