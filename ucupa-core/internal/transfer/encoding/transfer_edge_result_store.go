package encoding

import (
	"encoding/json"
	"slices"
	"strings"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	storebytes "ucupa/ucupa-core/internal/store"
	storerecord "ucupa/ucupa-core/internal/store/record"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// Each admitted edge reserves its eventual receipt bytes before creating an
// obligation. The common parent and closure are already retained, not repeated.
const MaxEdgeResultBytes = 320

const maxTransferResultBytes = MaxTransferManifestRecords * MaxEdgeResultBytes

type StoredEdgeResult struct {
	Mount    values.ElementID            `json:"mount"`
	At       *storerecord.StoredMountCut `json:"at"`
	Revision uint64                      `json:"revision"`
}

func EncodeEdgeResults(closures []transferfact.EdgeClosure, installed transferfact.CoreInstallation, results []transferfact.EdgeInstallation) (string, error) {
	if len(results) != len(closures) {
		return "", contract.ErrMountCoordinationRequired
	}
	if len(results) == 0 {
		return "", nil
	}
	closed := make(map[values.ElementID]transferfact.EdgeClosure, len(closures))
	for _, c := range closures {
		closed[c.Fence.Mount] = c
	}
	ordered := slices.Clone(results)
	slices.SortFunc(ordered, func(a, b transferfact.EdgeInstallation) int {
		return strings.Compare(string(a.Closure.Fence.Mount), string(b.Closure.Fence.Mount))
	})
	stored := make([]StoredEdgeResult, 0, len(results))
	for _, e := range ordered {
		mount := e.Closure.Fence.Mount
		c, ok := closed[mount]
		if !ok || e.Closure != c || e.Parent != installed || !e.Valid() {
			return "", contract.ErrConflict
		}
		stored = append(stored, StoredEdgeResult{Mount: mount, At: storerecord.StoreMountCut(e.At), Revision: e.Revision})
		delete(closed, mount)
	}
	data, err := json.Marshal(stored)
	if err != nil {
		return "", err
	}
	if len(data) > len(results)*MaxEdgeResultBytes {
		return "", values.PublicResourceExhausted
	}
	return string(data), nil
}

func ValidateEdgeResults(closures []transferfact.EdgeClosure, installed transferfact.CoreInstallation, raw string) error {
	if len(raw) == 0 || len(raw) > len(closures)*MaxEdgeResultBytes {
		return contract.ErrInvalid
	}
	var stored []StoredEdgeResult
	if err := storebytes.DecodeCanonicalJSON([]byte(raw), &stored); err != nil || len(stored) != len(closures) {
		return contract.ErrInvalid
	}
	closed := make(map[values.ElementID]transferfact.EdgeClosure, len(closures))
	for _, c := range closures {
		closed[c.Fence.Mount] = c
	}
	results := make([]transferfact.EdgeInstallation, 0, len(stored))
	for _, e := range stored {
		c, ok := closed[e.Mount]
		if !ok {
			return contract.ErrConflict
		}
		at, err := e.At.Cut()
		if err != nil {
			return err
		}
		results = append(results, transferfact.EdgeInstallation{Parent: installed, Closure: c, At: at, Revision: e.Revision})
	}
	canonical, err := EncodeEdgeResults(closures, installed, results)
	if err != nil || canonical != raw {
		return contract.ErrInvalid
	}
	return nil
}
