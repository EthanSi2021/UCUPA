package element

import (
	"context"
	"errors"
	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	accesspkg "ucupa/ucupa-core/internal/access"
	accessregistry "ucupa/ucupa-core/internal/access/registry"
	accessresolve "ucupa/ucupa-core/internal/access/resolve"
	accessvalue "ucupa/ucupa-core/internal/access/value"
	commitpkg "ucupa/ucupa-core/internal/commit"
	commitbinding "ucupa/ucupa-core/internal/commit/binding"
	coretoken "ucupa/ucupa-core/internal/token"
)

// PublicOperations executes the public mutation/recovery contract against the
// same Element commit owner. Its authorization views and space handles belong to
// the one public admission/registry; typed local writes do not require that state.
type PublicOperations struct {
	owner     *commitpkg.Owner
	admission *accesspkg.Admission
	access    *accessregistry.Registry
}

func NewPublic(owner *commitpkg.Owner, admission *accesspkg.Admission, resources *accessregistry.Registry) *PublicOperations {
	return &PublicOperations{owner: owner, admission: admission, access: resources}
}

// Mutate carries the original request's context end to end: the online
// authorization wait honors its cancellation, and a request that is already
// canceled or expired when the wait settles never reaches the commit
// boundary. A cancellation after the commit keeps the committed fact and the
// original idempotent recovery rules.
func (core *PublicOperations) Mutate(ctx context.Context, caller values.CallerContext, operation string, request map[string]any) (map[string]any, error) {
	var boundary values.ElementID
	var handle accessregistry.SpaceUse
	var original map[string]any
	var err error
	recovering := operation == "recover-outcome"
	if recovering {
		if _, err := accessvalue.Object(request, "op", "reference", "operationId", "request"); err != nil {
			return nil, err
		}
		boundary, err = accessvalue.Reference(core.owner.AuthorityRef(), request["reference"])
		if err != nil {
			return nil, err
		}
		original, _ = request["request"].(map[string]any)
		if original == nil || original["operationId"] != request["operationId"] {
			return nil, values.PublicInvalid
		}
	} else {
		core.owner.Lock()
		handle, err = accessresolve.SpaceLocked(core.owner, core.access, caller, request["handle"])
		core.owner.Unlock()
		if err != nil {
			return nil, err
		}
		boundary = handle.Boundary
		original = make(map[string]any, len(request))
		for key, value := range request {
			if key != "handle" {
				original[key] = value
			}
		}
	}
	binding, operationID, expected, err := commitbinding.ParsePublicMutation(original)
	if err != nil {
		return nil, err
	}
	binding.Actor, binding.Boundary = caller.Subject, boundary
	if expected != (values.PublicToken{}) && expected.Owner != core.owner.AuthorityRef() {
		return nil, values.PublicInvalid
	}
	requirements := accesspkg.ClosureRequirements(core.owner.AuthorityRef(), boundary, accessvalue.Intent(commitbinding.KindName(binding.Kind))...)
	view, permit, err := core.admission.Authorize(ctx, caller, requirements)
	defer core.admission.ReleasePermit(ctx, permit)
	if err != nil {
		return nil, err
	}
	core.owner.Lock()
	defer core.owner.Unlock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		return nil, values.PublicUnavailable
	}
	if err := core.admission.Validate(ctx, caller, view, requirements, false, permit); err != nil {
		return nil, err
	}
	if result, err, found := core.owner.LookupPublicMutationLocked(operationID, binding, recovering); found {
		return core.publicMutationResultLocked(result, err)
	}
	if recovering {
		return nil, values.PublicOutcomeUnknown
	}
	if _, err := accessresolve.SpaceLocked(core.owner, core.access, caller, request["handle"]); err != nil {
		return nil, err
	}
	if !core.owner.PublicBindingMembersLocked(binding) {
		return nil, values.PublicInvalid
	}
	binding.Expected = core.owner.PublicExpectedRevisionLocked(binding.Element, expected)
	result, err := core.owner.ApplyElementLocked(operationID, binding)
	if errors.Is(err, corecontract.ErrStore) {
		return nil, values.PublicIndeterminate
	}
	return core.publicMutationResultLocked(result, err)
}

func (core *PublicOperations) publicMutationResultLocked(result corecontract.MutationResult, err error) (map[string]any, error) {
	if err != nil {
		return nil, err
	}
	status := "COMMITTED"
	if result.Status == corecontract.MutationUnchanged {
		status = "UNCHANGED"
	}
	response := map[string]any{"status": status}
	if result.Record != nil {
		response["record"] = accessvalue.ElementRecordJSON(core.owner.SigningLocked(), *result.Record)
	}
	if coretoken.ValidRevision(result.TombstoneRevision) {
		response["tombstoneRevision"] = accessvalue.TokenJSON(core.owner.SigningLocked().ElementRevision(result.TombstoneRevision))
	}
	return response, nil
}
