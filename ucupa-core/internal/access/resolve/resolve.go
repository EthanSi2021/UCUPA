// Package resolve performs the shared owner/token/registry lookup sequence.
// Every function requires the caller to hold the original commit owner lock.
// It owns no state and neither admits nor authorizes an operation.
package resolve

import (
	"ucupa/shared/values"
	accessregistry "ucupa/ucupa-core/internal/access/registry"
	accessvalue "ucupa/ucupa-core/internal/access/value"
	"ucupa/ucupa-core/internal/commit"
)

func tokenLocked(owner *commit.Owner, raw any, kind string) (values.PublicToken, error) {
	if owner.ClosedLocked() || !owner.AvailableLocked() {
		return values.PublicToken{}, values.PublicUnavailable
	}
	token, err := accessvalue.Token(raw, kind)
	if err != nil || token.Owner != owner.AuthorityRef() {
		return values.PublicToken{}, values.PublicInvalid
	}
	return token, nil
}

func SpaceLocked(owner *commit.Owner, resources *accessregistry.Registry, caller values.CallerContext, raw any) (accessregistry.SpaceUse, error) {
	token, err := tokenLocked(owner, raw, "space-handle")
	if err != nil {
		return accessregistry.SpaceUse{}, err
	}
	return resources.ResolveSpaceLocked(caller, token)
}

func ReadLocked(owner *commit.Owner, resources *accessregistry.Registry, caller values.CallerContext, raw any) (accessregistry.ReadUse, error) {
	token, err := tokenLocked(owner, raw, "read-receipt")
	if err != nil {
		return accessregistry.ReadUse{}, err
	}
	return resources.ResolveReadLocked(caller, token)

}

func ContinuationLocked(owner *commit.Owner, resources *accessregistry.Registry, caller values.CallerContext, raw any) (accessregistry.ContinuationUse, error) {
	token, err := tokenLocked(owner, raw, "query-continuation")
	if err != nil {
		return accessregistry.ContinuationUse{}, err
	}
	return resources.ResolveContinuationLocked(caller, token)
}

func PreparationLocked(owner *commit.Owner, resources *accessregistry.Registry, caller values.CallerContext, raw any) (accessregistry.PreparationUse, error) {
	token, err := tokenLocked(owner, raw, "observation-preparation")
	if err != nil {
		return accessregistry.PreparationUse{}, err
	}
	return resources.ResolvePreparationLocked(caller, token)
}

func CursorLocked(owner *commit.Owner, resources *accessregistry.Registry, caller values.CallerContext, raw any) (accessregistry.CursorUse, error) {
	token, err := tokenLocked(owner, raw, "observation-cursor")
	if err != nil {
		return accessregistry.CursorUse{}, err
	}
	return resources.ResolveCursorLocked(caller, token)
}
