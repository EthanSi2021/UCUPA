package projection

import (
	"errors"
	"fmt"
	"testing"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
)

func TestReadErrorPreservesPriorityAndUnknownIdentity(t *testing.T) {
	unknown := errors.New("unknown")
	for _, tc := range []struct{ in, want error }{
		{nil, nil}, {unknown, unknown},
		{contract.ErrReadCutExpired, values.PublicInvalidContext},
		{fmt.Errorf("wrapped: %w", contract.ErrReadCutExpired), values.PublicInvalidContext},
		{contract.ErrSnapshotCapacity, values.PublicResourceExhausted},
		{fmt.Errorf("wrapped: %w", contract.ErrSnapshotCapacity), values.PublicResourceExhausted},
		{contract.ErrStore, values.PublicUnavailable},
		{contract.ErrUnavailable, values.PublicUnavailable},
		{fmt.Errorf("wrapped: %w", contract.ErrUnavailable), values.PublicUnavailable},
		{fmt.Errorf("wrapped: %w", contract.ErrStore), values.PublicUnavailable},
		{errors.Join(contract.ErrStore, contract.ErrSnapshotCapacity, contract.ErrReadCutExpired), values.PublicInvalidContext},
		{errors.Join(contract.ErrStore, contract.ErrSnapshotCapacity), values.PublicResourceExhausted},
	} {
		if got := ReadError(tc.in); got != tc.want {
			t.Fatalf("input %v: got %v want %v", tc.in, got, tc.want)
		}
	}
	wrapped := fmt.Errorf("wrapped unknown: %w", unknown)
	if ReadError(wrapped) != wrapped {
		t.Fatal("unknown wrapper identity changed")
	}
}
