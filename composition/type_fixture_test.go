package composition_test

import (
	"crypto/sha256"

	"ucupa/shared/values"
)

func fixtureType(text string) values.FacetTypeRef {
	digest := sha256.Sum256([]byte("ucupa-facet-test:" + text))
	ref, err := values.NewFacetTypeRef(digest[:])
	if err != nil {
		panic(err)
	}
	return ref
}
func fixturePayload(text string) []byte {
	return []byte(text)
}
