package facet_test

import (
	"context"
	"encoding/binary"
	"errors"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
)

type dispositionSource func(context.Context, values.NextHop, values.FactAuthorityRef, values.ElementID) (contract.FacetDispositionRead, error)

func (f dispositionSource) ReadFacetDisposition(ctx context.Context, hop values.NextHop, facet values.FactAuthorityRef, element values.ElementID) (contract.FacetDispositionRead, error) {
	return f(ctx, hop, facet, element)
}

func retiredDisposition(t *testing.T, key values.FacetAcceptanceKey) contract.FacetDispositionRead {
	t.Helper()
	var raw [40]byte
	raw[0] = 11
	binary.BigEndian.PutUint64(raw[32:], 6)
	cut, err := values.NewFacetReadCut(raw[:])
	if err != nil {
		t.Fatal(err)
	}
	return contract.FacetDispositionRead{Found: true, Facet: key.Facet, Element: key.Element, Key: key, Cut: cut}
}

// releaseFixture drives the public control path to a DETACHED relationship.
func releaseFixture(t *testing.T, f fixture) contract.PublicFacetInspection {
	t.Helper()
	pending := publicPropose(t, f, "release-propose")
	if err := f.facets.SetFacetAcceptanceSource(acceptanceSource(func(_ context.Context, _ values.NextHop, key values.FacetAcceptanceKey) (contract.FacetAcceptanceEvidence, error) {
		return accepted(key), nil
	})); err != nil {
		t.Fatal(err)
	}
	_, active, err := f.facets.ApplyFacetControl(context.Background(), publicCaller, "release-activate", contract.PublicFacetCommand{
		Kind: "activate", Element: facetEntity, Expected: pending.Revision,
	})
	if err != nil || active.State != contract.FacetActive {
		t.Fatalf("release activate = %#v, %v", active, err)
	}
	_, detached, err := f.facets.ApplyFacetControl(context.Background(), publicCaller, "release-detach", contract.PublicFacetCommand{
		Kind: "detach", Element: facetEntity, Expected: active.Revision,
	})
	if err != nil || detached.State != contract.FacetDetached || detached.Target == "" {
		t.Fatalf("release detach = %#v, %v", detached, err)
	}
	return detached
}

// Release is the control owner's occupancy release: the trusted disposition
// source alone supplies the final-disposition evidence, a proven absence stays
// a journaled refusal, and the committed release replays its exact result.
func TestFacetReleaseControlBindsDispositionSource(t *testing.T) {
	for _, mode := range []string{"absent", "malformed", "denied", "committed"} {
		t.Run(mode, func(t *testing.T) {
			f := publicFixture(t, false, nil)
			detached := releaseFixture(t, f)
			key := facts(t, f.owner, facetEntity).attachment.Key
			if err := f.facets.SetFacetDispositionSource(dispositionSource(func(_ context.Context, hop values.NextHop, facet values.FactAuthorityRef, element values.ElementID) (contract.FacetDispositionRead, error) {
				if hop != "local:retired-facet" || facet != key.Facet || element != facetEntity {
					t.Fatalf("disposition read = %q %q %q", hop, facet, element)
				}
				switch mode {
				case "absent":
					return contract.FacetDispositionRead{}, nil
				case "malformed":
					return contract.FacetDispositionRead{Found: true, Facet: facet, Element: element}, nil
				case "denied":
					f.auth.validate = values.AuthorizationDenied
					return retiredDisposition(t, key), nil
				default:
					return retiredDisposition(t, key), nil
				}
			})); err != nil {
				t.Fatal(err)
			}
			command := contract.PublicFacetCommand{Kind: "release", Element: facetEntity, Expected: detached.Revision, Target: detached.Target, NextHop: "local:retired-facet"}
			status, retired, err := f.facets.ApplyFacetControl(context.Background(), publicCaller, "release", command)
			switch mode {
			case "absent":
				if !errors.Is(err, contract.ErrDispositionNotFound) || status != contract.MutationRejected {
					t.Fatalf("absent disposition = %#v %v", retired, err)
				}
				if current := facts(t, f.owner, facetEntity); current.attachment.State != contract.FacetDetached {
					t.Fatal("refused release changed the record", current)
				}
				// The refusal is journaled: an exact retry resolves its receipt.
				retryStatus, retry, retryErr := f.facets.ApplyFacetControl(context.Background(), publicCaller, "release", command)
				if retryErr == nil || retryStatus != contract.MutationRejected || !errors.Is(retryErr, contract.ErrDispositionNotFound) {
					t.Fatalf("refusal retry = %#v %v", retry, retryErr)
				}
			case "malformed":
				if !errors.Is(err, contract.ErrUnavailable) {
					t.Fatalf("malformed disposition = %v", err)
				}
				if current := facts(t, f.owner, facetEntity); current.attachment.State != contract.FacetDetached {
					t.Fatal("unavailable release changed the record", current)
				}
			case "denied":
				if !errors.Is(err, values.PublicDenied) {
					t.Fatalf("revoked release = %v", err)
				}
				if current := facts(t, f.owner, facetEntity); current.attachment.State != contract.FacetDetached {
					t.Fatal("denied release changed the record", current)
				}
			default:
				if err != nil || status != contract.MutationCommitted || retired.State != contract.FacetRetired {
					t.Fatalf("committed release = %#v %v %v", status, retired, err)
				}
				retryStatus, retry, retryErr := f.facets.ApplyFacetControl(context.Background(), publicCaller, "release", command)
				if retryErr != nil || retryStatus != contract.MutationCommitted || retry != retired {
					t.Fatalf("release retry = %#v %v", retry, retryErr)
				}
			}
		})
	}
}
