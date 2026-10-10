package namespace

import (
	"context"
	"sort"
	accessstate "ucupa/ucupa-facet/internal/access"

	"ucupa/shared/values"
	facetcontract "ucupa/ucupa-facet/contract"
	requestvalue "ucupa/ucupa-facet/internal/namespace/request"
)

func (f *Owner) publicRead(ctx context.Context, caller values.CallerContext, operation string, request map[string]any) (map[string]any, error) {
	allowed := []string{"op", "access", "prepareObserve"}
	if operation != "list-types" {
		allowed = append(allowed, "typeRef")
	}
	if _, err := requestvalue.Object(request, allowed...); err != nil {
		return nil, err
	}
	access, err := requestvalue.ParseAccess(f.authority, request["access"])
	if err != nil {
		return nil, err
	}
	prepare := false
	if v, ok := request["prepareObserve"]; ok {
		prepare, ok = v.(bool)
		if !ok {
			return nil, values.PublicInvalid
		}
	}
	var typ values.FacetTypeRef
	if operation != "list-types" {
		typ, err = requestvalue.Type(request["typeRef"])
		if err != nil {
			return nil, err
		}
	}
	permission := facetcontract.PermissionGet
	if operation == "list-types" {
		permission = facetcontract.PermissionListTypes
	}
	req := f.publicRequirements(access.Element, typ, permission, prepare)
	approval, ok := f.contactSnapshot()
	if !ok {
		return nil, values.PublicUnavailable
	}
	permit, err := f.authorizePublic(ctx, caller, req)
	if err != nil {
		return nil, err
	}
	retained := false
	defer func() {
		if !retained {
			permit.release(ctx)
		}
	}()
	view := permit.view
	source, ok := f.cores.(coreControlSource)
	if !ok {
		return nil, values.PublicUnavailable
	}
	f.mu.Lock()
	key, err := f.publicKeyLocked(access)
	var snapshot captured
	if err == nil {
		snapshot, err = f.captureLocked(packKey(key))
	}
	var receipt, prep *accessstate.Context
	if err == nil {
		receipt, err = f.issuePublicLocked(caller, "read-receipt", nil)
	}
	if err == nil {
		receipt.View, receipt.Requirements, receipt.Key, receipt.Typ, receipt.Cut = view, req, key, typ, snapshot.cut
		if prepare {
			prep, err = f.issuePublicLocked(caller, "observation-preparation", nil)
			if err == nil {
				prep.View, prep.Requirements, prep.Key, prep.Typ, prep.Cut = view, req, key, typ, snapshot.cut
				prep.Stream = &accessstate.Stream{Address: snapshot.contact.Address, ContactSerial: snapshot.contact.Serial, Proposal: access.Proposal}
			}
		}
	}
	f.mu.Unlock()
	if err != nil {
		if receipt != nil {
			f.mu.Lock()
			f.public.RemoveLocked(receipt.Token.Value)
			f.mu.Unlock()
		}
		return nil, err
	}
	keep := false
	var control facetcontract.CoreControlRead
	defer func() {
		if !keep {
			f.mu.Lock()
			f.public.RemoveLocked(receipt.Token.Value)
			if prep != nil {
				f.public.RemoveLocked(prep.Token.Value)
			}
			f.mu.Unlock()
			if control.Preparation.Valid() {
				f.releaseControl(ctx, snapshot.contact.Address, key, control.Preparation)
			}
		}
	}()
	control, err = source.ReadControl(ctx, snapshot.contact.Address, key, prepare)
	if err != nil {
		return nil, facetControlError(err)
	}
	if !packProof(control.Evidence).valid(packKey(key), false) || coreRejection(packProof(control.Evidence), packKey(key)) != "" || control.Proposal != access.Proposal {
		return nil, values.PublicUnavailable
	}
	if prepare && !control.Preparation.Valid() {
		return nil, values.PublicUnavailable
	}
	if prepare {
		if err := source.CheckControl(ctx, snapshot.contact.Address, key, control.Preparation); err != nil {
			return nil, facetControlError(err)
		}
	}
	if err := permit.recheck(ctx, false); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.public.ExpireLocked()
	if err := f.publicAvailableLocked(); err != nil {
		return nil, err
	}
	if !f.settledContact(approval) {
		return nil, values.PublicUnavailable
	}
	if f.public.LookupLocked(receipt.Token.Value) != receipt || f.state.Cut != snapshot.cut {
		return nil, values.PublicInvalidContext
	}
	if prep != nil && (f.public.LookupLocked(prep.Token.Value) != prep || prep.Stream.Gap) {
		return nil, values.PublicGap
	}
	if err := permit.guardLocked(ctx, false); err != nil {
		return nil, err
	}
	if ctx.Err() != nil {
		return nil, values.PublicUnavailable
	}
	if prep != nil {
		prep.Stream.CorePreparation = control.Preparation
		prep.Release = permit.release
		retained = true
	}
	result := map[string]any{"receipt": requestvalue.TokenJSON(receipt.Token)}
	if prep != nil {
		result["preparation"] = requestvalue.TokenJSON(prep.Token)
	}
	if operation == "list-types" {
		types := []string{}
		for _, record := range f.state.Records {
			if record.Element == key.Element {
				types = append(types, string(record.Type))
			}
		}
		sort.Strings(types)
		encoded := make([]any, len(types))
		for i, typ := range types {
			encoded[i] = values.EncodePublicBytes([]byte(typ))
		}
		result["types"] = encoded
	} else {
		record, found := f.state.Records[recordID(key.Element, typ.Bytes())]
		result["typeRef"] = values.EncodePublicBytes(typ.Bytes())
		if !found {
			result["state"] = "NOT_FOUND"
		} else {
			result["state"] = "PRESENT"
			result["payload"] = values.EncodePublicBytes(record.Payload)
			result["revision"] = requestvalue.TokenJSON(f.publicRevision(record.Revision))
		}
	}
	keep = true
	return result, nil
}

func facetControlError(err error) error {
	switch err {
	case values.PublicDenied, values.PublicGap, values.PublicInvalidContext:
		return values.PublicGap
	case values.PublicResourceExhausted:
		return err
	}
	return values.PublicUnavailable
}
func (f *Owner) controlCall(ctx context.Context, address values.NextHop, key values.FacetAcceptanceKey, request map[string]any) (map[string]any, error) {
	source, ok := f.cores.(coreControlSource)
	if !ok {
		return nil, values.PublicUnavailable
	}
	encoded, err := values.CanonicalJSONValue(request)
	if err != nil {
		return nil, values.PublicInvalid
	}
	raw, err := source.ControlCall(ctx, address, key, encoded)
	if err != nil {
		return nil, facetControlError(err)
	}
	parsed, err := values.ParseCanonicalJSON(raw, values.PublicJSONLimits())
	if err != nil {
		return nil, values.PublicUnavailable
	}
	result, ok := parsed.(map[string]any)
	if !ok {
		return nil, values.PublicUnavailable
	}
	return result, nil
}
func (f *Owner) releaseControl(ctx context.Context, address values.NextHop, key values.FacetAcceptanceKey, prep values.PublicToken) {
	if prep.Valid() {
		_, _ = f.controlCall(ctx, address, key, map[string]any{"op": "release-observe", "preparation": requestvalue.TokenJSON(prep)})
	}
}
