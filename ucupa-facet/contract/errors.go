package contract

import "errors"

var (
	ErrInvalid                  = errors.New("invalid Facet input")
	ErrDenied                   = errors.New("Facet authorization denied")
	ErrUnavailable              = errors.New("Facet source unavailable")
	ErrStore                    = errors.New("Facet Store failure")
	ErrIndeterminate            = errors.New("Facet commit outcome indeterminate")
	ErrOperationConflict        = errors.New("Facet operation conflict")
	ErrOperationNotFound        = errors.New("Facet operation not found")
	ErrProposalNotCurrent       = errors.New("Facet proposal not current")
	ErrSubjectAuthorityConflict = errors.New("Facet subject authority conflict")
	ErrAcceptanceNotFound       = errors.New("Facet acceptance not found")
	ErrContactRevisionConflict  = errors.New("Facet contact revision conflict")
	ErrElementNotFound          = errors.New("Core Element not found")
	ErrNamespaceNone            = errors.New("Core Facet namespace none")
	ErrNamespaceDetached        = errors.New("Core Facet namespace detached")
	ErrNamespaceRetired         = errors.New("Facet namespace retired")
	ErrNotCurrentOwner          = errors.New("Facet not current owner")
	ErrAttachmentChanged        = errors.New("Core Facet attachment changed")
	ErrKeyNotFound              = errors.New("Facet key not found")
	ErrRevisionConflict         = errors.New("Facet revision conflict")
	ErrCapacity                 = errors.New("Facet capacity exceeded")
	ErrSnapshotChanged          = errors.New("Facet snapshot changed during Core check")
)
