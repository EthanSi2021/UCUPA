package coordinator

import (
	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	accessvalue "ucupa/ucupa-core/internal/access/value"
	federationcollect "ucupa/ucupa-core/internal/query/federation/collect"
)

// decodeFederatedCollectRequest closes the JSON boundary before collection.
// Source authorization/opening still precedes this parse, preserving the
// established failure order of the public operation.
func decodeFederatedCollectRequest(operation string, boundary values.ElementID, request map[string]any) (federationcollect.Request, error) {
	result := federationcollect.Request{Operation: operation}
	if operation == "read" || operation == "list" {
		address, err := decodeFederatedAddress(request["address"], boundary)
		if err != nil {
			return federationcollect.Request{}, err
		}
		result.Address = address
		return result, nil
	}
	if operation != "query" {
		return federationcollect.Request{}, values.PublicInvalid
	}
	start, err := decodeFederatedAddress(request["start"], boundary)
	if err != nil {
		return federationcollect.Request{}, err
	}
	basis, err := decodeFederatedAddress(request["basis"], boundary)
	if err != nil {
		return federationcollect.Request{}, err
	}
	depth := int64(0)
	if raw, ok := request["upDepth"]; ok {
		text, valid := raw.(string)
		if !valid {
			return federationcollect.Request{}, values.PublicInvalid
		}
		depth, err = values.ParsePublicUpDepth(text)
		if err != nil {
			return federationcollect.Request{}, err
		}
	}
	sphere, err := accessvalue.Object(request["sphere"], "center", "radius")
	if err != nil {
		return federationcollect.Request{}, err
	}
	center, err := accessvalue.Vec3(sphere["center"])
	if err != nil {
		return federationcollect.Request{}, err
	}
	radius, valid := sphere["radius"].(float64)
	if !valid {
		return federationcollect.Request{}, values.PublicInvalid
	}
	selector := corecontract.QueryBoth
	switch request["selector"] {
	case "anchors":
		selector = corecontract.QueryAnchors
	case "entities":
		selector = corecontract.QueryEntities
	case nil, "both":
	default:
		return federationcollect.Request{}, values.PublicInvalid
	}
	result.Query = &federationcollect.Query{
		Start:    start,
		Basis:    basis,
		UpDepth:  depth,
		Sphere:   corecontract.QuerySphere{Center: center, Radius: radius},
		Selector: selector,
	}
	return result, nil
}
