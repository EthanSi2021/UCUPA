package namespace

import (
	"crypto/sha256"
	"encoding/json"

	"ucupa/shared/values"
)

const fixtureFacetOwner values.FactAuthorityRef = "00000000-0000-4000-8000-000000009901"

// fixtureType derives an arbitrary opaque TypeRef from text; no artifact exists.
func fixtureType(text string) values.FacetTypeRef {
	digest := sha256.Sum256([]byte("ucupa-facet-test:" + text))
	ref, err := values.NewFacetTypeRef(digest[:])
	if err != nil {
		panic(err)
	}
	return ref
}
func fixturePayload(text string) []byte {
	raw, err := values.CanonicalJSONValue(text)
	if err != nil {
		panic(err)
	}
	return raw
}
func fixtureText(raw []byte) string {
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		panic(err)
	}
	return text
}
