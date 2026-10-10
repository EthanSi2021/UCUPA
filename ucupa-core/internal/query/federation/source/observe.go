package source

import (
	"context"
	"encoding/base64"
	"strconv"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	federationcontract "ucupa/ucupa-core/contract/federation"
	accesspkg "ucupa/ucupa-core/internal/access"
	accessregistry "ucupa/ucupa-core/internal/access/registry"
	accessresolve "ucupa/ucupa-core/internal/access/resolve"
	federationdenial "ucupa/ucupa-core/internal/query/federation/denial"

	accessvalue "ucupa/ucupa-core/internal/access/value"
)

// ObserveFederationReadPart keeps the original caller/view and authenticated
// coordinator binding. Unlike reading a CurrentVerified snapshot, observing
// must survive ordinary writes after its cut: those writes are its purpose.
// It does not acquire a new view, renew a lease, or capture a replacement cut.
func (core *Operations) ObserveFederationReadPart(ctx context.Context, caller values.CallerContext, source values.FactAuthorityRef, token values.PublicToken, request federationcontract.FederationObserveRequest) (federationcontract.FederationObservationPage, error) {
	finish, err := core.enterFederationCall(ctx)
	if err != nil {
		return federationcontract.FederationObservationPage{}, err
	}
	defer finish()
	if !source.Valid() || !token.Valid() || token.Kind != "read-receipt" || token.Owner != core.owner.AuthorityRef() {
		return federationcontract.FederationObservationPage{}, values.PublicInvalid
	}
	if request.Operation != "begin" && request.Operation != "poll" || request.Operation == "begin" && (request.Cursor != nil || request.Limit != 0) || request.Operation == "poll" && (request.Cursor == nil || request.Limit == 0 || request.Limit > 256) {
		return federationcontract.FederationObservationPage{}, values.PublicInvalid
	}
	// The retained observation view is captured under the lock, rechecked online
	// outside it, and the same tokens are re-resolved before the in-lock run
	// consumes the permit minted for exactly this operation.
	core.owner.Lock()
	basis, err := accessresolve.ReadLocked(core.owner, core.registry, caller, accessvalue.TokenJSON(token))
	if err == nil && (!basis.Source.Present || basis.Source.Source != source) {
		err = values.PublicDenied
	}
	if err == nil && (!basis.Source.Request.PrepareObserve || !basis.HasPreparation) {
		err = values.PublicInvalidContext
	}
	var prepRef accessregistry.Ref
	var prepView values.AuthorizationView
	var prepRequirements []values.Requirement
	var denialView values.AuthorizationDenialView
	var denialRequirement values.Requirement
	if err == nil && basis.Source.Denial != nil {
		denialView, denialRequirement = federationdenial.Authorization(*basis.Source.Denial)
	} else if err == nil {
		prep, present := core.registry.PreparationLocked(basis.Preparation)
		if !present || !prep.HasQueue || !prep.HasSource || prep.Source != basis.Ref {
			err = values.PublicGap
		} else {
			prepRef, prepView, prepRequirements = prep.Ref, prep.View, prep.Requirements
		}
	}
	core.owner.Unlock()
	if err != nil {
		return federationcontract.FederationObservationPage{}, err
	}
	var permit accesspkg.Permit
	var denialPermit accesspkg.DenialPermit
	if basis.Source.Denial != nil {
		denialPermit, err = core.admission.RecheckDenial(ctx, caller, denialView, denialRequirement)
		if err != nil {
			return federationcontract.FederationObservationPage{}, err
		}
	} else {
		permit, err = core.admission.Recheck(ctx, caller, prepView, prepRequirements, true)
		if err != nil && err != values.PublicUnavailable {
			// A denial or changed basis permanently gaps the same preparation,
			// exactly as the original in-lock failure did; a temporarily
			// unavailable Realm keeps it retryable.
			core.owner.Lock()
			if _, present := core.registry.PreparationLocked(prepRef); present {
				core.registry.GapLocked(prepRef)
			}
			core.owner.Unlock()
		}
		if err != nil {
			return federationcontract.FederationObservationPage{}, err
		}
	}
	core.owner.Lock()
	defer core.owner.Unlock()
	basis, err = accessresolve.ReadLocked(core.owner, core.registry, caller, accessvalue.TokenJSON(token))
	if err != nil {
		return federationcontract.FederationObservationPage{}, err
	}
	if !basis.Source.Present || basis.Source.Source != source {
		return federationcontract.FederationObservationPage{}, values.PublicDenied
	}
	if !basis.Source.Request.PrepareObserve || !basis.HasPreparation {
		return federationcontract.FederationObservationPage{}, values.PublicInvalidContext
	}
	if basis.Source.Denial != nil {
		return core.observeFederationHiddenLocked(ctx, caller, source, basis, request, denialPermit)
	}
	prep, present := core.registry.PreparationLocked(basis.Preparation)
	if !present || !prep.HasQueue || !prep.HasSource || prep.Source != basis.Ref {
		return federationcontract.FederationObservationPage{}, values.PublicGap
	}
	operation := "begin-observe"
	input := map[string]any{"op": operation, "preparation": accessvalue.TokenJSON(prep.Token)}
	if request.Operation == "poll" {
		cursor, e := accessresolve.CursorLocked(core.owner, core.registry, caller, accessvalue.TokenJSON(*request.Cursor))
		if e != nil {
			return federationcontract.FederationObservationPage{}, e
		}
		if cursor.Preparation != prep.Ref {
			return federationcontract.FederationObservationPage{}, values.PublicInvalidContext
		}
		operation = "poll"
		input = map[string]any{"op": operation, "cursor": accessvalue.TokenJSON(*request.Cursor), "limit": float64(request.Limit)}
	}
	if ctx.Err() != nil {
		return federationcontract.FederationObservationPage{}, values.PublicUnavailable
	}
	// This is the same authorization, relation, exact-read binding, queue and
	// cursor state machine as owner-local Observe, not an alternative history.
	result, err := core.observations.RunLocked(ctx, caller, operation, input, permit)
	if err != nil {
		return federationcontract.FederationObservationPage{}, err
	}
	field := "cursor"
	if request.Operation == "poll" {
		field = "nextCursor"
	}
	cursor, err := accessvalue.Token(result[field], "observation-cursor")
	if err != nil {
		return federationcontract.FederationObservationPage{}, values.PublicUnavailable
	}
	out := federationcontract.FederationObservationPage{Cursor: cursor}
	if request.Operation == "poll" {
		for _, raw := range result["groups"].([]any) {
			group := raw.(map[string]any)
			id, e1 := accessvalue.Token(group["changeId"], "change-id")
			cut, e2 := accessvalue.Token(group["basis"], "change-basis")
			if e1 != nil || e2 != nil {
				return federationcontract.FederationObservationPage{}, values.PublicUnavailable
			}
			item := federationcontract.FederationChangeGroup{ID: id, Basis: cut}
			for _, category := range group["category"].([]any) {
				item.Categories = append(item.Categories, category.(string))
			}
			if terminal, ok := group["terminal"]; ok {
				statement, e := terminalEvidence(terminal)
				if e != nil {
					return federationcontract.FederationObservationPage{}, values.PublicUnavailable
				}
				item.Terminal = &statement
			}
			out.Groups = append(out.Groups, item)
		}
	}
	if ctx.Err() != nil {
		return federationcontract.FederationObservationPage{}, values.PublicUnavailable
	}
	return out, nil
}

// terminalEvidence decodes the one terminal successor statement of a group.
// A malformed statement is never disclosed as evidence; the call fails.
func terminalEvidence(raw any) (contract.TransitionTerminal, error) {
	statement, ok := raw.(map[string]any)
	if !ok {
		return contract.TransitionTerminal{}, values.PublicUnavailable
	}
	encoded, ok := statement["operationId"].(string)
	if !ok {
		return contract.TransitionTerminal{}, values.PublicUnavailable
	}
	operation, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return contract.TransitionTerminal{}, values.PublicUnavailable
	}
	source, ok := statement["source"].(string)
	if !ok {
		return contract.TransitionTerminal{}, values.PublicUnavailable
	}
	target, ok := statement["target"].(string)
	if !ok {
		return contract.TransitionTerminal{}, values.PublicUnavailable
	}
	serial, ok := statement["finalCut"].(string)
	if !ok {
		return contract.TransitionTerminal{}, values.PublicUnavailable
	}
	finalCut, err := strconv.ParseUint(serial, 10, 64)
	if err != nil {
		return contract.TransitionTerminal{}, values.PublicUnavailable
	}
	settled, ok := statement["settled"].(bool)
	if !ok {
		return contract.TransitionTerminal{}, values.PublicUnavailable
	}
	nextHop, ok := statement["nextHop"].(string)
	if !ok {
		return contract.TransitionTerminal{}, values.PublicUnavailable
	}
	return contract.TransitionTerminal{
		Operation: contract.OperationID(operation),
		Source:    values.FactAuthorityRef(source),
		Target:    values.FactAuthorityRef(target),
		FinalCut:  finalCut,
		Settled:   settled,
		NextHop:   values.NextHop(nextHop),
	}, nil
}
