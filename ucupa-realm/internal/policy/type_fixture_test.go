package policy

import (
	"crypto/sha256"

	"ucupa/shared/values"
)

const fixtureFacetOwner values.FactAuthorityRef = "00000000-0000-4000-8000-000000009901"

// fixtureType constructs exact fixture bytes; invalid fixture definitions fail immediately.
func fixtureType(text string) values.FacetTypeRef {
	digest := sha256.Sum256([]byte(text))
	ref, err := values.NewFacetTypeRef(digest[:])
	if err != nil {
		panic(err)
	}
	return ref
}
