package contract

import "ucupa/ucupa-core/internal/token"

// CoreReadSnapshot is the existing opaque, runtime-only retained view handle.
// It is not a durable revision or authorization credential.
type CoreReadSnapshot = token.CoreReadSnapshot

// ConnectedCut names the existing bounded, caller-bound read context.
type ConnectedCut = token.ConnectedCut
