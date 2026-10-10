package encoding

import (
	"encoding/json"
	"slices"
	"strings"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/geometry"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	storebytes "ucupa/ucupa-core/internal/store"
	storerecord "ucupa/ucupa-core/internal/store/record"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

const MaxTransferClosureBytes = 1 << 20

// One common fence lives in the acceptance event. Per-edge records reuse the
// original admission, cut and float codecs, without duplicating source inputs
// or retaining any peer credentials. The event owns the canonical bytes.
type StoredEdgeClosure struct {
	Mount         values.ElementID                  `json:"mount"`
	Previous      *storerecord.StoredMountAdmission `json:"previous"`
	PreviousRange string                            `json:"previousRange"`
	PreviousCut   *storerecord.StoredMountCut       `json:"previousCut"`
	Range         string                            `json:"range"`
	At            *storerecord.StoredMountCut       `json:"at"`
	Revision      uint64                            `json:"revision"`
}

func EncodeEdgeClosures(f transferfact.EdgeFence, closures []transferfact.EdgeClosure) (string, error) {
	if len(closures) == 0 {
		return "", nil
	}
	if len(closures) > MaxTransferManifestRecords {
		return "", values.PublicResourceExhausted
	}
	if !f.Valid() {
		return "", contract.ErrInvalid
	}
	ordered := slices.Clone(closures)
	slices.SortFunc(ordered, func(a, b transferfact.EdgeClosure) int {
		return strings.Compare(string(a.Fence.Mount), string(b.Fence.Mount))
	})
	stored := make([]StoredEdgeClosure, 0, len(ordered))
	operationBytes := 0
	for n, c := range ordered {
		operationBytes += len(c.Previous.Key.OperationID)
		if operationBytes > MaxTransferClosureBytes {
			return "", values.PublicResourceExhausted
		}
		exact := f
		exact.Mount = c.Fence.Mount
		if c.Fence != exact || !c.Valid(c.Previous.Key.MountKey.CarrierAuthority) || (n > 0 && ordered[n-1].Fence.Mount == c.Fence.Mount) {
			return "", contract.ErrConflict
		}
		stored = append(stored, StoredEdgeClosure{Mount: c.Fence.Mount, Previous: storerecord.StoreMountAdmission(c.Previous.Key),
			PreviousRange: storebytes.EncodeFloatBits(c.Previous.RangeValue.Radius()), PreviousCut: storerecord.StoreMountCut(c.Previous.DecisionCut),
			Range: storebytes.EncodeFloatBits(c.Range.Radius()), At: storerecord.StoreMountCut(c.At), Revision: c.Revision})
	}
	data, err := json.Marshal(stored)
	if err != nil {
		return "", err
	}
	if len(data) > MaxTransferClosureBytes {
		return "", values.PublicResourceExhausted
	}
	return string(data), nil
}

func decodeEdgeClosures(f transferfact.EdgeFence, raw string) ([]transferfact.EdgeClosure, error) {
	if raw == "" {
		return nil, nil
	}
	if len(raw) > MaxTransferClosureBytes {
		return nil, values.PublicResourceExhausted
	}
	var stored []StoredEdgeClosure
	if err := storebytes.DecodeCanonicalJSON([]byte(raw), &stored); err != nil || len(stored) == 0 || len(stored) > MaxTransferManifestRecords {
		return nil, contract.ErrInvalid
	}
	closures := make([]transferfact.EdgeClosure, 0, len(stored))
	for _, e := range stored {
		key, err := e.Previous.Admission()
		if err != nil {
			return nil, err
		}
		oldCut, err := e.PreviousCut.Cut()
		if err != nil {
			return nil, err
		}
		at, err := e.At.Cut()
		if err != nil {
			return nil, err
		}
		oldRange, err := storebytes.DecodeFloatBits(e.PreviousRange)
		if err != nil {
			return nil, err
		}
		rangeValue, err := storebytes.DecodeFloatBits(e.Range)
		if err != nil {
			return nil, err
		}
		exact := f
		exact.Mount = e.Mount
		closures = append(closures, transferfact.EdgeClosure{Fence: exact, Previous: mountfact.Admission{Key: key, RangeValue: geometry.SphereFromStored(oldRange), DecisionCut: oldCut}, Range: geometry.SphereFromStored(rangeValue), At: at, Revision: e.Revision})
	}
	canonical, err := EncodeEdgeClosures(f, closures)
	if err != nil || canonical != raw {
		return nil, contract.ErrInvalid
	}
	return closures, nil
}

func acceptanceEdgeFence(e TransferEvent) transferfact.EdgeFence {
	if e.Kind == TransferCarrierAccepted {
		return transferfact.EdgeFence{Carrier: e.CarrierFence, Mount: e.CarrierFence.Permit.Reservation.Input.Mount}
	}
	return transferfact.EdgeFence{Source: e.Fence, Mount: e.Fence.Input.Boundary}
}

func AcceptedEdges(slice transferfact.Slice, e TransferEvent) (transferfact.Slice, error) {
	f := acceptanceEdgeFence(e)
	closures, err := decodeEdgeClosures(f, e.Closures)
	if err != nil {
		return transferfact.Slice{}, err
	}
	return slice.WithClosedEdges(f, closures)
}
