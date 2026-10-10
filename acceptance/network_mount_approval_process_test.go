package acceptance_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/pem"
	"path/filepath"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"ucupa/shared/values"
	corepb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
	coresdk "ucupa/ucupa-core/sdk/go"
)

func TestNetworkMountApprovalProductRestart(t *testing.T) {
	target, actor := newNetworkFixture(t, false), newNetworkFixture(t, false)
	target.grant(t, corecontract.PermissionMountAccept, values.CoreElementScope(target.c.AuthorityRef(), values.ElementID(networkEntity)), false)
	caFile := filepath.Join(target.dir, "actor-ca.pem")
	writeNetworkFile(t, caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: actor.config.Certificate.Certificate[1]}))
	pin := actor.clientConfig.SPKISHA256
	start := prepareNetworkProduct(t, target, map[string]any{
		"read_delegation": map[string]any{"client_ca_file": caFile, "client_spki_sha256": map[string]string{string(actor.config.Expected): base64.RawURLEncoding.EncodeToString(pin[:])}, "max_ttl": "10s", "max_grants": 4, "max_per_caller": 3},
		"mount_approvals": map[string]any{"max_ttl": "10s", "max_approvals": 4, "max_per_caller": 3},
	})
	stop := start()
	target.connect(t)
	digest := sha256.Sum256([]byte("product command commitment"))
	binding := &corepb.MountApprovalBinding{Actor: proto.String(string(actor.c.AuthorityRef())), Element: proto.String(networkEntity), OperationId: []byte("process-mount"), CommandDigest: digest[:]}
	approval := createMountApproval(t, target, binding, 10)
	cfg := target.clientConfig
	cfg.Actor, cfg.ActorCertificate = actor.c.AuthorityRef(), &actor.config.Certificate
	service, err := coresdk.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	check := &corepb.CheckMountApprovalRequest{Binding: binding, Token: approval.Token}
	out, err := service.MountApprovals().CheckMountApproval(context.Background(), check)
	if err != nil || out.GetResult().GetPrincipal() != string(target.actor.Principal) {
		t.Fatalf("product approval: %v %v", out, err)
	}
	stop()
	_ = start()
	if _, err := service.MountApprovals().CheckMountApproval(context.Background(), check); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("approval survived restart: %v", err)
	}
	_ = target.client.Close()
	target.connect(t)
	fresh := createMountApproval(t, target, binding, 10)
	if string(fresh.Token) == string(approval.Token) {
		t.Fatal("reused old approval")
	}
	out, err = service.MountApprovals().CheckMountApproval(context.Background(), &corepb.CheckMountApprovalRequest{Binding: binding, Token: fresh.Token})
	if err != nil || out.GetResult() == nil {
		t.Fatalf("fresh approval: %v %v", out, err)
	}
}
