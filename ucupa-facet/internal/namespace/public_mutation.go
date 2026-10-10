package namespace

import (
	"bytes"
	"context"

	"ucupa/shared/values"
	facetcontract "ucupa/ucupa-facet/contract"
	requestvalue "ucupa/ucupa-facet/internal/namespace/request"
	"ucupa/ucupa-facet/internal/token"
)

func (f *Owner) publicMutation(ctx context.Context, caller values.CallerContext, operation string, request map[string]any) (map[string]any, error) {
	original := request
	var recoveryAccess requestvalue.Access
	if operation == "recover-outcome" {
		if _, err := requestvalue.Object(request, "op", "access", "operationId", "request"); err != nil {
			return nil, err
		}
		var err error
		recoveryAccess, err = requestvalue.ParseAccess(f.authority, request["access"])
		if err != nil {
			return nil, err
		}
		var ok bool
		original, ok = request["request"].(map[string]any)
		if !ok || original["operationId"] != request["operationId"] {
			return nil, values.PublicInvalid
		}
	}
	op, _ := original["op"].(string)
	allowed := []string{"op", "access", "typeRef", "expected", "operationId"}
	if op == "put" {
		allowed = append(allowed, "payload")
	} else if op != "delete" {
		return nil, values.PublicInvalid
	}
	if _, err := requestvalue.Object(original, allowed...); err != nil {
		return nil, err
	}
	access, err := requestvalue.ParseAccess(f.authority, original["access"])
	if err != nil {
		return nil, err
	}
	if operation == "recover-outcome" && recoveryAccess != access {
		return nil, values.PublicInvalid
	}
	typ, err := requestvalue.Type(original["typeRef"])
	if err != nil {
		return nil, err
	}
	idText, ok := original["operationId"].(string)
	if !ok {
		return nil, values.PublicInvalid
	}
	id, err := values.DecodePublicBytes(idText)
	if err != nil || !validOperation(facetcontract.OperationID(id)) {
		return nil, values.PublicInvalid
	}
	var payload []byte
	if op == "put" {
		text, ok := original["payload"].(string)
		if !ok {
			return nil, values.PublicInvalid
		}
		if len(text) > ((maxPayloadBytes+2)/3)*4 {
			return nil, values.PublicResourceExhausted
		}
		payload, err = values.DecodePublicBytes(text)
		if err != nil {
			return nil, values.PublicInvalid
		}
		if len(payload) > maxPayloadBytes {
			return nil, values.PublicResourceExhausted
		}
	}
	absent := false
	var expected values.PublicToken
	if op == "put" {
		condition, err := requestvalue.Object(original["expected"], "absent", "revision")
		if err != nil || len(condition) != 1 {
			return nil, values.PublicInvalid
		}
		if value, ok := condition["absent"]; ok {
			if value != true {
				return nil, values.PublicInvalid
			}
			absent = true
		} else {
			expected, err = requestvalue.Token(condition["revision"], "record-revision")
			if err != nil {
				return nil, err
			}
		}
	} else {
		expected, err = requestvalue.Token(original["expected"], "record-revision")
		if err != nil {
			return nil, err
		}
	}
	if !absent && expected.Owner != f.authority {
		return nil, values.PublicInvalid
	}
	permission := facetcontract.PermissionPut
	if op == "delete" {
		permission = facetcontract.PermissionDelete
	}
	req := f.publicRequirements(access.Element, typ, permission, false)
	approval, ok := f.contactSnapshot()
	if !ok {
		return nil, values.PublicUnavailable
	}
	permit, err := f.authorizePublic(ctx, caller, req)
	if err != nil {
		return nil, err
	}
	defer permit.release(ctx)
	canonical, _ := values.CanonicalJSONValue(original)
	f.mu.Lock()
	if err := f.publicAvailableLocked(); err != nil {
		f.mu.Unlock()
		return nil, err
	}
	if err := permit.guardLocked(ctx, false); err != nil {
		f.mu.Unlock()
		return nil, err
	}
	if index, exists := f.operations[byteString(id)]; exists {
		e := f.events[index]
		if e.Input.Actor != (actorData{Realm: byteString(caller.Subject.Realm), Principal: byteString(caller.Subject.Principal)}) {
			f.mu.Unlock()
			if operation == "recover-outcome" {
				return nil, values.PublicOutcomeUnknown
			}
			return nil, values.PublicConflict
		}
		if e.Input.PublicRequest != string(canonical) {
			f.mu.Unlock()
			return nil, values.PublicConflict
		}
		response, err := f.publicMutationResultLocked(e.Outcome.result(f.scope, e.Input.Key), rejectionErrors[e.Outcome.Error])
		f.mu.Unlock()
		return response, err
	}
	if operation == "recover-outcome" {
		f.mu.Unlock()
		return nil, values.PublicOutcomeUnknown
	}
	key, err := f.publicKeyLocked(access)
	var snapshot captured
	if err == nil {
		snapshot, err = f.captureLocked(packKey(key))
	}
	if err != nil {
		f.mu.Unlock()
		return nil, err
	}
	c := f.base(caller.Subject, key, facetcontract.OperationID(id), op)
	c.PublicRequest = string(canonical)
	c.Type = typ.Bytes()
	c.Payload = payload
	c.Absent = absent
	if !absent {
		current, found := f.state.Records[recordID(key.Element, c.Type)]
		if found && f.publicRevision(current.Revision) == expected {
			c.Expected = current.Revision
		} else {
			foreign := f.scope
			foreign[0] ^= 0xff
			if foreign == ([32]byte{}) {
				foreign[1] = 1
			}
			c.Expected = revisionData{Scope: foreign, Element: key.Element, Type: c.Type, Serial: 1}
		}
	}
	f.mu.Unlock()
	source, ok := f.cores.(coreControlSource)
	if !ok {
		return nil, values.PublicUnavailable
	}
	control, err := source.ReadControl(ctx, snapshot.contact.Address, key, false)
	if err != nil {
		return nil, facetControlError(err)
	}
	if !packProof(control.Evidence).valid(packKey(key), false) || coreRejection(packProof(control.Evidence), packKey(key)) != "" || control.Proposal != access.Proposal {
		return nil, values.PublicUnavailable
	}
	result, err := f.write(ctx, c, permit, approval)
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.publicMutationResultLocked(result, err)
}

func (f *Owner) publicMutationResultLocked(result facetcontract.MutationResult, err error) (map[string]any, error) {
	if err != nil {
		return nil, err
	}
	status := "COMMITTED"
	if result.Status == facetcontract.Unchanged {
		status = "UNCHANGED"
	}
	response := map[string]any{"status": status}
	if token.ValidRevision(result.Revision) {
		response["revision"] = requestvalue.TokenJSON(f.publicRevision(packRevision(result.Revision)))
	}
	return response, nil
}

// Public requests retain the original canonical bytes for exact recovery. The
// owner-local input remains the only operation claim, never a second namespace.
func validPublicCommand(c command) bool {
	if c.PublicRequest == "" {
		return true
	}
	if c.Kind == "accept" || c.Kind == "contact" || c.Kind == "retire" {
		_, _, ok := parseAcceptanceRequest(c)
		return ok
	}
	parsed, err := values.ParseCanonicalJSON([]byte(c.PublicRequest), values.PublicJSONLimits())
	if err != nil {
		return false
	}
	r, ok := parsed.(map[string]any)
	if !ok {
		return false
	}
	fields := []string{"op", "access", "operationId", "typeRef", "expected"}
	if c.Kind == "put" {
		fields = append(fields, "payload")
	}
	if _, err := requestvalue.Object(r, fields...); err != nil || r["op"] != c.Kind {
		return false
	}
	text, ok := r["operationId"].(string)
	if !ok {
		return false
	}
	id, err := values.DecodePublicBytes(text)
	if err != nil || string(id) != string(c.ID) {
		return false
	}
	typ, err := requestvalue.Type(r["typeRef"])
	if err != nil || !bytes.Equal(typ.Bytes(), c.Type) {
		return false
	}
	access, err := requestvalue.ParseAccess(c.Owner, r["access"])
	if err != nil || access.Core != c.Key.Core || access.Element != c.Key.Element {
		return false
	}
	absent, expected, err := requestvalue.CommandCondition(r, c.Kind)
	if err != nil || absent != c.Absent || (!absent && expected.Owner != c.Owner) {
		return false
	}
	if c.Kind == "put" {
		payload, ok := r["payload"].(string)
		if !ok {
			return false
		}
		raw, err := values.DecodePublicBytes(payload)
		if err != nil || !bytes.Equal(raw, c.Payload) {
			return false
		}
	}
	return c.Kind == "put" || c.Kind == "delete"
}

// Recheck public/private precondition binding without decoding tags, scanning
// historical records or consulting a current external owner.
func publicExpectedMatches(c command, scope [32]byte, key [32]byte) bool {
	if c.PublicRequest == "" || c.Absent {
		return true
	}
	// Acceptance requests carry no put/delete condition to recheck; the
	// contact form additionally proves its expected contact revision.
	if c.Kind == "accept" || c.Kind == "contact" || c.Kind == "retire" {
		_, expected, ok := parseAcceptanceRequest(c)
		if !ok || c.Kind != "contact" {
			return ok
		}
		owner := Owner{ownerState: &ownerState{authority: c.Owner, state: facetState{PublicKey: key}}}
		return c.ContactScope == scope && owner.publicContactRevision(token.NewContactRevision(c.ContactScope, c.ContactKey.value(), c.ContactSerial)) == expected
	}
	v, err := values.ParseCanonicalJSON([]byte(c.PublicRequest), values.PublicJSONLimits())
	if err != nil {
		return false
	}
	_, expected, err := requestvalue.CommandCondition(v.(map[string]any), c.Kind)
	if err != nil {
		return false
	}
	owner := Owner{ownerState: &ownerState{authority: c.Owner, state: facetState{PublicKey: key}}}
	if c.Expected.Scope == scope {
		return owner.publicRevision(c.Expected) == expected
	}
	foreign := scope
	foreign[0] ^= 0xff
	if foreign == ([32]byte{}) {
		foreign[1] = 1
	}
	return c.Expected.Scope == foreign && c.Expected.Element == c.Key.Element && bytes.Equal(c.Expected.Type, c.Type) && c.Expected.Serial == 1
}
