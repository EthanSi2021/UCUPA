// Package request owns the stateless public request decoding shared by Facet
// admission and durable-history validation. It deliberately imports no Facet
// state, authorizer, Core port, or Store implementation.
package request

import "ucupa/shared/values"

// Object accepts one public JSON object and rejects fields outside the caller's
// allowlist. Required-field and cardinality checks remain with the operation
// that owns them; this preserves the existing wire-level validation order.
func Object(raw any, fields ...string) (map[string]any, error) {
	object, ok := raw.(map[string]any)
	if !ok {
		return nil, values.PublicInvalid
	}
	for key, value := range object {
		found := false
		for _, field := range fields {
			if key == field {
				found = true
				break
			}
		}
		if !found || value == nil {
			return nil, values.PublicInvalid
		}
	}
	return object, nil
}

// Token decodes one public token of the exact requested kind. Valid only
// checks token shape; authorization and owner relationship checks stay above
// this package.
func Token(raw any, kind string) (values.PublicToken, error) {
	o, err := Object(raw, "owner", "kind", "value")
	if err != nil {
		return values.PublicToken{}, err
	}
	owner, a := o["owner"].(string)
	actual, b := o["kind"].(string)
	value, c := o["value"].(string)
	token := values.PublicToken{Owner: values.FactAuthorityRef(owner), Kind: actual, Value: value}
	if !a || !b || !c || !token.Valid() || actual != kind {
		return values.PublicToken{}, values.PublicInvalid
	}
	return token, nil
}

// TokenJSON returns the existing public token object representation.
func TokenJSON(token values.PublicToken) map[string]any {
	return map[string]any{"owner": string(token.Owner), "kind": token.Kind, "value": token.Value}
}

// Type decodes one canonical public Facet type reference.
func Type(raw any) (values.FacetTypeRef, error) {
	text, ok := raw.(string)
	if !ok {
		return values.FacetTypeRef{}, values.PublicInvalid
	}
	bytes, err := values.DecodePublicBytes(text)
	if err != nil {
		return values.FacetTypeRef{}, values.PublicInvalid
	}
	typ, err := values.NewFacetTypeRef(bytes)
	if err != nil {
		return values.FacetTypeRef{}, values.PublicInvalid
	}
	return typ, nil
}
