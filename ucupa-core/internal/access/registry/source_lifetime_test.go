package registry_test

import (
	"context"
	"errors"
	"testing"
	"time"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	federationcontract "ucupa/ucupa-core/contract/federation"
	"ucupa/ucupa-core/internal/access/registry"
)

// The old root matrix's source Open/Check/Release paths remain real. Exact
// registry accounting belongs here; exact snapshot expiry and byte accounting
// remain in query's own tests, without importing its test-only hooks.
func TestFederationPartContextCapacityAndCancellation(t *testing.T) {
	for _, scenario := range []string{"context-expiry", "public-capacity", "snapshot-capacity", "cancelled", "bad-mode", "missing-purpose"} {
		t.Run(scenario, func(t *testing.T) {
			ttl := time.Hour
			config := contract.Config{SnapshotRetention: &ttl, PublicAccessPolicy: &values.PublicAccessPolicy{ContextTTL: ttl, ObservationTTL: ttl}}
			if scenario == "public-capacity" {
				one := uint64(1)
				config.PublicAccessPolicy.MaxPublicBytes = &one
			}
			if scenario == "snapshot-capacity" {
				config.SnapshotBytes = 1
			}
			f := newResourceComponents(t, config)
			root := resourceID(93000)
			if _, err := f.elements.CreateRoot(resourceActor, root, "federation-root"); err != nil {
				t.Fatal(err)
			}
			caller := values.CallerContext{Subject: resourceActor}
			request := federationcontract.FederationReadRequest{Boundary: root, Intent: "read", Purpose: [32]byte{1}}
			ctx := context.Background()
			want := error(values.PublicInvalidContext)
			switch scenario {
			case "public-capacity", "snapshot-capacity":
				want = values.PublicResourceExhausted
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
				want = values.PublicUnavailable
			case "bad-mode":
				request.Mode = contract.QueryRetained
				want = values.PublicInvalid
			case "missing-purpose":
				request.Purpose = [32]byte{}
				want = values.PublicInvalid
			}
			part, err := f.sources.OpenFederationReadPart(ctx, caller, f.sources.AuthorityRef(), request)
			if scenario == "context-expiry" {
				if err != nil {
					t.Fatal(err)
				}
				if !registry.SetPublicResourceScenarioForTest(f.resources, "expired") {
					t.Fatal("missing expiry fault")
				}
				err = f.sources.CheckFederationReadPart(ctx, caller, f.sources.AuthorityRef(), part.Basis, false)
				if releaseErr := f.sources.ReleaseFederationReadPart(ctx, caller, f.sources.AuthorityRef(), part.Basis); releaseErr != nil {
					t.Fatal(releaseErr)
				}
			}
			if !errors.Is(err, want) {
				t.Fatalf("got %v, want %v", err, want)
			}
			if count, bytes := registry.PublicResourceCountsForTest(f.resources); count != 0 || bytes != 0 {
				t.Fatalf("source failure leaked %d contexts/%d bytes", count, bytes)
			}
		})
	}
}
