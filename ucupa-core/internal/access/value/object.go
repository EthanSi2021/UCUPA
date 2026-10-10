// Package value preserves Core public value shapes, without sessions, authorization or protocol negotiation.
package value

import (
	"ucupa/shared/values"
)

func Object(value any, allowed ...string) (map[string]any, error) {
	object, ok := value.(map[string]any)
	if !ok {
		return nil, values.PublicInvalid
	}
	for key, v := range object {
		found := false
		for _, name := range allowed {
			if key == name {
				found = true
				break
			}
		}
		if !found || v == nil {
			return nil, values.PublicInvalid
		}
	}
	return object, nil
}

func Token(raw any, kind string) (values.PublicToken, error) {
	object, err := Object(raw, "owner", "kind", "value")
	if err != nil {
		return values.PublicToken{}, err
	}
	owner, a := object["owner"].(string)
	actualKind, b := object["kind"].(string)
	value, c := object["value"].(string)
	token := values.PublicToken{Owner: values.FactAuthorityRef(owner), Kind: actualKind, Value: value}
	if !a || !b || !c || !token.Valid() || actualKind != kind {
		return values.PublicToken{}, values.PublicInvalid
	}
	return token, nil
}

func TokenJSON(token values.PublicToken) map[string]any {
	return map[string]any{"owner": string(token.Owner), "kind": token.Kind, "value": token.Value}
}

// Limit preserves the existing optional public page-size rule; it is not an
// execution budget and does not coerce strings or integer Go values.
func Limit(raw any, defaultValue int) (int, error) {
	if raw == nil {
		return defaultValue, nil
	}
	n, ok := raw.(float64)
	if !ok || n < 1 || n > 256 || float64(int(n)) != n {
		return 0, values.PublicInvalid
	}
	return int(n), nil
}
