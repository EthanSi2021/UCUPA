package encoding

import (
	"encoding/json"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	storebytes "ucupa/ucupa-core/internal/store"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// This proof is charged to the existing 8 MiB retained-evidence allowance,
// separate from the fixed control envelope, leaving terminal journal room.
const MaxTransferParentProofBytes = 2 * MaxTransferIntentBytes

func EncodeTargetParentHold(h transferfact.ParentHold) (string, error) {
	if h == (transferfact.ParentHold{}) {
		return "", nil
	}
	if !h.Valid() || h.Input.Role != transferfact.TargetParent {
		return "", contract.ErrInvalid
	}
	if _, err := EncodeTransferParentIntent(h.Input); err != nil {
		return "", err
	}
	b, err := json.Marshal(storeTransferParentHold(h))
	if len(b) > MaxTransferParentProofBytes {
		return "", values.PublicResourceExhausted
	}
	return string(b), err
}

func DecodeTargetParentHold(raw string) (transferfact.ParentHold, error) {
	if raw == "" {
		return transferfact.ParentHold{}, nil
	}
	if len(raw) > MaxTransferParentProofBytes {
		return transferfact.ParentHold{}, values.PublicResourceExhausted
	}
	var stored StoredTransferParentHold
	if err := storebytes.DecodeCanonicalJSON([]byte(raw), &stored); err != nil {
		return transferfact.ParentHold{}, contract.ErrInvalid
	}
	h, err := stored.hold()
	if err != nil {
		return transferfact.ParentHold{}, err
	}
	canonical, err := EncodeTargetParentHold(h)
	if err != nil || canonical != raw {
		return transferfact.ParentHold{}, contract.ErrInvalid
	}
	return h, nil
}
