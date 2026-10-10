package instance

import (
	"crypto/rand"
	"errors"
)

func newTokenScope() ([32]byte, error) {
	var scope [32]byte
	if _, err := rand.Read(scope[:]); err != nil {
		return [32]byte{}, err
	}
	if scope == ([32]byte{}) {
		return [32]byte{}, errors.New("generated zero Core token scope")
	}
	return scope, nil
}
