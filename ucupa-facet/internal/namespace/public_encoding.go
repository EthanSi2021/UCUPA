package namespace

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"

	"ucupa/shared/values"
	facetcontract "ucupa/ucupa-facet/contract"
	requestvalue "ucupa/ucupa-facet/internal/namespace/request"
)

func (f *Owner) publicRevision(revision revisionData) values.PublicToken {
	mac := hmac.New(sha256.New, f.state.PublicKey[:])
	var serial [8]byte
	binary.BigEndian.PutUint64(serial[:], revision.Serial)
	for _, part := range [][]byte{[]byte("ucupa.facet.public-revision"), []byte(f.authority), []byte("record-revision"), revision.Scope[:], []byte(revision.Element), revision.Type, serial[:]} {
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(part)))
		mac.Write(length[:])
		mac.Write(part)
	}
	token, _ := values.PublicTokenFromBytes(f.authority, "record-revision", [32]byte(mac.Sum(nil)))
	return token
}

func (f *Owner) publicKeyLocked(access requestvalue.Access) (values.FacetAcceptanceKey, error) {
	if err := f.publicAvailableLocked(); err != nil {
		return values.FacetAcceptanceKey{}, err
	}
	var latest accepted
	for _, a := range f.state.Acceptances {
		if a.Key.Element == access.Element && a.Key.Core == access.Core && a.At > latest.At {
			latest = a
		}
	}
	if latest.At == 0 {
		return values.FacetAcceptanceKey{}, values.PublicUnavailable
	}
	return latest.Key.value(), nil
}
func (f *Owner) publicRequirements(element values.ElementID, typ values.FacetTypeRef, permission values.PermissionID, observe bool) []values.Requirement {
	scope := values.FacetElementScope(f.authority, element)
	observed := facetcontract.PermissionObserveElement
	if typ.Valid() {
		scope = values.FacetKeyScope(f.authority, element, typ)
		observed = facetcontract.PermissionObserveKey
	}
	result := []values.Requirement{{Permission: permission, Scope: scope}}
	if observe {
		result = append(result, values.Requirement{Permission: observed, Scope: scope})
	}
	return result
}
