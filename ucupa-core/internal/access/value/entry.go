package value

import (
	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
)

// Intent maps the existing public operation name to its permission kinds.
// It is not an authorization decision and creates no scope or grant.
func Intent(intent string) []values.PermissionID {
	switch intent {
	case "read":
		return []values.PermissionID{corecontract.PermissionReadElement}
	case "list":
		return []values.PermissionID{corecontract.PermissionListDirect}
	case "query":
		return []values.PermissionID{corecontract.PermissionQueryRegion}
	case "create-anchor":
		return []values.PermissionID{corecontract.PermissionCreateAnchor}
	case "create-entity":
		return []values.PermissionID{corecontract.PermissionCreateEntity}
	case "update-anchor", "update-entity":
		return []values.PermissionID{corecontract.PermissionUpdateElement, corecontract.PermissionPlaceElement}
	case "delete-anchor":
		return []values.PermissionID{corecontract.PermissionDeleteAnchor}
	case "delete-entity":
		return []values.PermissionID{corecontract.PermissionDeleteEntity}
	default:
		return nil
	}
}

func Reference(authority values.FactAuthorityRef, raw any) (values.ElementID, error) {
	reference, err := Object(raw, "boundary", "nextHop", "trustHint")
	if err != nil {
		return "", err
	}
	boundary, err := Object(reference["boundary"], "core", "anchor")
	if err != nil {
		return "", err
	}
	owner, a := boundary["core"].(string)
	anchor, b := boundary["anchor"].(string)
	hop, c := reference["nextHop"].(string)
	if !a || !b || !c || !values.FactAuthorityRef(owner).Valid() || !values.ElementID(anchor).Valid() || !values.NextHop(hop).Valid() {
		return "", values.PublicInvalid
	}
	if hint, ok := reference["trustHint"]; ok {
		if _, ok := hint.(string); !ok {
			return "", values.PublicInvalid
		}
	}
	if values.FactAuthorityRef(owner) != authority {
		return "", values.PublicUnavailable
	}
	return values.ElementID(anchor), nil
}
