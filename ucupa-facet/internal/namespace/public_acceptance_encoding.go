package namespace

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"

	"ucupa/shared/values"
	"ucupa/ucupa-facet/contract"
	requestvalue "ucupa/ucupa-facet/internal/namespace/request"
	"ucupa/ucupa-facet/internal/token"
)

func publicAcceptanceRequest(kind string, ref contract.PublicAcceptanceRef, address values.NextHop, id contract.OperationID, expected values.PublicToken) string {
	r := map[string]any{"op": kind, "core": string(ref.Core), "element": string(ref.Element),
		"proposal": requestvalue.TokenJSON(ref.Proposal), "nextHop": string(address), "operationId": values.EncodePublicBytes([]byte(id))}
	if kind == "contact" {
		r["expected"] = requestvalue.TokenJSON(expected)
	}
	raw, _ := values.CanonicalJSONValue(r)
	return string(raw)
}

func parseAcceptanceRequest(c command) (contract.PublicAcceptanceRef, values.PublicToken, bool) {
	var none contract.PublicAcceptanceRef
	v, err := values.ParseCanonicalJSON([]byte(c.PublicRequest), values.PublicJSONLimits())
	if err != nil {
		return none, values.PublicToken{}, false
	}
	fields := []string{"op", "core", "element", "proposal", "nextHop", "operationId"}
	if c.Kind == "contact" {
		fields = append(fields, "expected")
	} else if c.Kind == "retire" {
		fields = []string{"op", "core", "element", "proposal", "operationId"}
	} else if c.Kind != "accept" {
		return none, values.PublicToken{}, false
	}
	o, err := requestvalue.Object(v, fields...)
	if err != nil || len(o) != len(fields) || o["op"] != c.Kind || (c.Kind != "retire" && o["nextHop"] != string(c.Address)) {
		return none, values.PublicToken{}, false
	}
	id, ok := o["operationId"].(string)
	if !ok {
		return none, values.PublicToken{}, false
	}
	raw, err := values.DecodePublicBytes(id)
	if err != nil || string(raw) != string(c.ID) {
		return none, values.PublicToken{}, false
	}
	core, a := o["core"].(string)
	element, b := o["element"].(string)
	proposal, err := requestvalue.Token(o["proposal"], "attachment-revision")
	ref := contract.PublicAcceptanceRef{Core: values.FactAuthorityRef(core), Element: values.ElementID(element), Proposal: proposal}
	if !a || !b || err != nil || !validAcceptanceRef(ref) || ref.Core != c.Key.Core || ref.Element != c.Key.Element {
		return none, values.PublicToken{}, false
	}
	var expected values.PublicToken
	if c.Kind == "contact" {
		expected, err = requestvalue.Token(o["expected"], "contact-revision")
		if err != nil || expected.Owner != c.Owner {
			return none, values.PublicToken{}, false
		}
	}
	return ref, expected, true
}

func (f *Owner) publicContactRevision(r contract.ContactRevision) values.PublicToken {
	if !token.ValidContactRevision(r) {
		return values.PublicToken{}
	}
	contactScope, contactKey, contactSerial := token.ContactRevisionParts(r)
	mac := hmac.New(sha256.New, f.state.PublicKey[:])
	var serialBytes [8]byte
	binary.BigEndian.PutUint64(serialBytes[:], contactSerial)
	for _, part := range [][]byte{[]byte("ucupa.facet.public-contact"), []byte(f.authority), contactScope[:],
		[]byte(contactKey.Core), []byte(contactKey.Facet), []byte(contactKey.Element), contactKey.Proposal.Bytes(), serialBytes[:]} {
		var size [8]byte
		binary.BigEndian.PutUint64(size[:], uint64(len(part)))
		mac.Write(size[:])
		mac.Write(part)
	}
	token, _ := values.PublicTokenFromBytes(f.authority, "contact-revision", [32]byte(mac.Sum(nil)))
	return token
}

// The exact public selector is reconstructed from the existing accepted
// operation journal. It is not a second mutable relationship or address map.
func (f *Owner) publicAcceptanceKeyLocked(ref contract.PublicAcceptanceRef) (values.FacetAcceptanceKey, bool, error) {
	var key values.FacetAcceptanceKey
	for _, e := range f.events {
		if e.Input.Kind != "accept" || e.Input.PublicRequest == "" || e.Outcome.AcceptedAt == 0 {
			continue
		}
		r, _, ok := parseAcceptanceRequest(e.Input)
		if !ok {
			return key, false, values.PublicUnavailable
		}
		if r == ref {
			if key.Valid() && packKey(key) != e.Input.Key {
				return values.FacetAcceptanceKey{}, false, values.PublicUnavailable
			}
			key = e.Input.Key.value()
		}
	}
	return key, key.Valid(), nil
}

func (f *Owner) hasPublicAcceptanceBinding(key keyData) bool {
	for _, e := range f.events {
		if e.Input.Kind == "accept" && e.Input.Key == key && e.Input.PublicRequest != "" && e.Outcome.AcceptedAt > 0 {
			return true
		}
	}
	return false
}

// validatePublicAcceptanceHistory binds a public selector to the key observed
// through the fixed service. Contacts can only use an already-issued mapping;
// replay never calls Core, guesses a proposal, or trusts the current address.
func validatePublicAcceptanceHistory(e event, claims map[keyData]contract.PublicAcceptanceRef, keys map[contract.PublicAcceptanceRef]keyData) bool {
	if e.Input.PublicRequest == "" || (e.Input.Kind != "accept" && e.Input.Kind != "contact") {
		return true
	}
	ref, _, ok := parseAcceptanceRequest(e.Input)
	if !ok {
		return false
	}
	previous, bound := claims[e.Input.Key]
	if bound && previous != ref {
		return false
	}
	if previousKey, bound := keys[ref]; bound && previousKey != e.Input.Key {
		return false
	}
	if e.Input.Kind == "contact" {
		return bound
	}
	if !bound && (!e.Core.valid(e.Input.Key, true) || e.Core.State != contract.CoreNone || e.Core.Pending != e.Input.Key) &&
		!e.Core.validReaccept(e.Input.Key) && !e.Core.validRecovery(e.Input.Key) {
		return false
	}
	if e.Outcome.AcceptedAt > 0 {
		claims[e.Input.Key], keys[ref] = ref, e.Input.Key
	}
	return true
}
