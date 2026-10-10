package acceptance_test

import (
	"context"
	"encoding/base64"
	"encoding/pem"
	"path/filepath"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	"ucupa/ucupa-core/contract"
)

// The real product loses runtime approval on restart; it does not persist a
// user session or confuse this preflight with a durable target acceptance.
func TestTransitionApprovalIndependentProductRestart(t *testing.T) {
	target, source, b := transitionApprovalSetup(t)
	target.grant(t, contract.PermissionTransitionImport, values.CoreElementScope(target.c.AuthorityRef(), values.ElementID(networkEntity)), false)
	caFile := filepath.Join(target.dir, "source-ca.pem")
	writeNetworkFile(t, caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: source.config.Certificate.Certificate[1]}))
	pin := source.clientConfig.SPKISHA256
	start := prepareNetworkProduct(t, target, map[string]any{
		"read_delegation":      map[string]any{"client_ca_file": caFile, "client_spki_sha256": map[string]string{string(source.config.Expected): base64.RawURLEncoding.EncodeToString(pin[:])}, "max_ttl": "10s", "max_grants": 4, "max_per_caller": 3},
		"transition_approvals": map[string]any{"max_ttl": "10s", "max_approvals": 4, "max_per_caller": 3},
	})
	stop := start()
	defer stop()
	target.connect(t)
	service := transitionApprovalService(t, target, source)
	a := createTransitionApproval(t, target, b, 10)
	check := &pb.CheckTransitionApprovalRequest{Binding: b, Token: a.Token}
	if out, err := service.TransitionApprovals().CheckTransitionApproval(context.Background(), check); err != nil || out.GetResult() == nil {
		t.Fatalf("product approval: %v %v", out, err)
	}
	stop()
	stop = start()
	defer stop()
	if _, err := service.TransitionApprovals().CheckTransitionApproval(context.Background(), check); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("runtime approval survived restart: %v", err)
	}
	_ = target.client.Close()
	target.connect(t)
	fresh := createTransitionApproval(t, target, b, 10)
	if string(fresh.Token) == string(a.Token) {
		t.Fatal("reused old approval")
	}
	if out, err := service.TransitionApprovals().CheckTransitionApproval(context.Background(), &pb.CheckTransitionApprovalRequest{Binding: b, Token: fresh.Token}); err != nil || out.GetResult() == nil {
		t.Fatalf("reauthenticated exact approval: %v %v", out, err)
	}
}
