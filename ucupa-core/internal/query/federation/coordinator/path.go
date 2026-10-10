package coordinator

import (
	"ucupa/shared/values"
	accessvalue "ucupa/ucupa-core/internal/access/value"
	"ucupa/ucupa-core/internal/geometry"
	federationcollect "ucupa/ucupa-core/internal/query/federation/collect"
)

// decodeFederatedAddress is a public-representation adapter. The collector
// receives only the validated typed address and never imports JSON helpers.
func decodeFederatedAddress(raw any, boundary values.ElementID) (federationcollect.Address, error) {
	object, err := accessvalue.Object(raw, "origin", "up", "down")
	if err != nil {
		return federationcollect.Address{}, err
	}
	result := federationcollect.Address{Origin: boundary}
	if value, ok := object["origin"]; ok {
		text, valid := value.(string)
		if !valid || !values.ElementID(text).Valid() {
			return federationcollect.Address{}, values.PublicInvalid
		}
		result.Origin = values.ElementID(text)
	}
	if value, ok := object["up"]; ok {
		text, valid := value.(string)
		if !valid {
			return federationcollect.Address{}, values.PublicInvalid
		}
		result.Up, err = values.ParsePublicParentCount(text)
		if err != nil {
			return federationcollect.Address{}, err
		}
	}
	if value, ok := object["down"]; ok {
		ids, valid := value.([]any)
		if !valid || len(ids) > geometry.MaxExactProjectionEdges {
			return federationcollect.Address{}, values.PublicInvalid
		}
		for _, value := range ids {
			text, valid := value.(string)
			if !valid || !values.ElementID(text).Valid() {
				return federationcollect.Address{}, values.PublicInvalid
			}
			result.Down = append(result.Down, values.ElementID(text))
		}
	}
	return result, nil
}
