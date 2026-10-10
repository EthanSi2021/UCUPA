package value

import "ucupa/shared/values"

// ReadConditions binds every field except page size and continuation using the
// existing canonical public JSON encoding. It is not an authorization proof.
func ReadConditions(request map[string]any) string {
	copy := make(map[string]any, len(request))
	for key, value := range request {
		if key != "limit" && key != "continuation" {
			copy[key] = value
		}
	}
	encoded, _ := values.CanonicalJSONValue(copy)
	return string(encoded)
}
