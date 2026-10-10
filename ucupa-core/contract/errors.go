package contract

import "errors"

// These are the existing public Core error identities. The actual permission,
// state, retention and storage decisions remain in the responsible operation.
var (
	ErrDenied            = errors.New("Core authorization denied")
	ErrNotFound          = errors.New("Core Element not found")
	ErrConflict          = errors.New("Core state conflict")
	ErrRevisionConflict  = errors.New("Core revision conflict")
	ErrOperationConflict = errors.New("Core operation conflict")
	ErrCapacity          = errors.New("Core capacity exceeded")
	ErrReadCutExpired    = errors.New("Core read cut expired")
	ErrStore             = errors.New("Core Store failure")
)
