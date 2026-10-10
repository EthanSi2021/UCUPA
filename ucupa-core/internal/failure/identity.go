// Package failure owns the two error identities shared by pure Core value
// construction and service-evidence parsing. It contains no business policy.
package failure

import "errors"

var (
	ErrInvalid         = errors.New("invalid Core input")
	ErrUnrepresentable = errors.New("Core result is unrepresentable")
)
