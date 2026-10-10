package namespace

import (
	"context"
	"errors"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-facet/contract"
)

func TestNetworkNamespacePeersRejectCancellationDuringAuthorization(t *testing.T) {
	for _, step := range []string{"check-export", "fence", "refuse", "settle", "read-source", "check-import", "stage", "accept", "install", "read-import"} {
		t.Run(step, func(t *testing.T) {
			x, p := networkAcceptanceFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			p.authorized = cancel
			other, err := values.NewFactAuthorityRef()
			if err != nil {
				t.Fatal(err)
			}
			var callErr error
			switch step {
			case "check-export":
				_, callErr = x.f.CheckNamespaceExport(ctx, x.caller, x.key, "cancelled")
			case "fence":
				_, callErr = x.f.FenceNamespace(ctx, x.caller, x.key, "cancelled", other)
			case "refuse":
				callErr = x.f.RefuseNamespaceFence(ctx, x.caller, x.key, "cancelled", other)
			case "settle":
				callErr = x.f.SettleNamespace(ctx, x.caller, x.key, "cancelled", other)
			case "read-source":
				_, callErr = x.f.ReadNamespaceSource(ctx, x.caller, x.key, "cancelled", fixtureType("cancelled"))
			case "check-import":
				manifest := contract.NamespaceManifest{Element: x.key.Element, Source: other, FinalCut: 1}
				_, callErr = x.f.CheckNamespaceImport(ctx, x.caller, x.key, "cancelled", manifest, "local:core")
			case "stage":
				callErr = x.f.StageNamespaceEntry(ctx, x.caller, x.key, "cancelled", fixtureType("cancelled"), []byte("payload"))
			case "accept":
				_, callErr = x.f.AcceptNamespace(ctx, x.caller, x.key, "cancelled", "local:core")
			case "install":
				callErr = x.f.InstallNamespace(ctx, x.caller, x.key, "cancelled", other, 1)
			case "read-import":
				_, callErr = x.f.ReadNamespaceImport(ctx, x.caller, x.key, "cancelled")
			}
			if !errors.Is(callErr, contract.ErrUnavailable) || len(x.f.events) != 0 || len(x.f.staged) != 0 || p.locked.Load() || p.released.Load() != 1 {
				t.Fatalf("err=%v events=%d staged=%d locked=%v releases=%d", callErr, len(x.f.events), len(x.f.staged), p.locked.Load(), p.released.Load())
			}
		})
	}
}
