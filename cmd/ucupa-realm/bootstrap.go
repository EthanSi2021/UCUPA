package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"

	"ucupa/shared/values"
	realm "ucupa/ucupa-realm/contract"
)

// The trusted bootstrap is one explicit document: a new Realm Store starts
// exactly from the declared state, and nothing is seeded beyond it.
const bootstrapEntryLimit = 256

type bootstrapFile struct {
	RealmID     values.RealmID        `json:"realm_id"`
	Principals  []values.PrincipalID  `json:"principals"`
	Bindings    []bootstrapBinding    `json:"bindings"`
	Credentials []bootstrapCredential `json:"credentials"`
}

type bootstrapBinding struct {
	Subject    values.PrincipalRef `json:"subject"`
	Permission values.PermissionID `json:"permission"`
	Scope      scopeFile           `json:"scope"`
}

type bootstrapCredential struct {
	Subject  values.PrincipalID `json:"subject"`
	Audience values.RealmID     `json:"audience"`
	ID       string             `json:"id"`
	Secret   string             `json:"secret"`
}

// scopeFile mirrors the wire scope shape: each kind requires exactly its own
// fields, and any field set outside the kind is rejected before Store work.
type scopeFile struct {
	Kind      string `json:"kind"`
	Authority string `json:"authority"`
	Element   string `json:"element"`
	Facet     string `json:"facet"`
	Realm     string `json:"realm"`
}

func readBootstrap(path string) (realm.Seed, error) {
	var file bootstrapFile
	b, err := readLimited(path, 65536)
	if err != nil {
		return realm.Seed{}, err
	}
	if err := strictJSON(b, &file); err != nil {
		return realm.Seed{}, err
	}
	if file.RealmID == "" ||
		len(file.Principals) > bootstrapEntryLimit || len(file.Bindings) > bootstrapEntryLimit ||
		len(file.Credentials) > bootstrapEntryLimit {
		return realm.Seed{}, errors.New("invalid bootstrap file")
	}
	seed := realm.Seed{
		ID:              file.RealmID,
		LocalPrincipals: make([]values.PrincipalID, len(file.Principals)),
		Bindings:        make([]realm.PermissionBinding, len(file.Bindings)),
		Credentials:     make([]realm.CredentialBootstrap, len(file.Credentials)),
	}
	copy(seed.LocalPrincipals, file.Principals)
	for index, binding := range file.Bindings {
		scope, err := binding.Scope.toScope()
		if err != nil {
			return realm.Seed{}, err
		}
		seed.Bindings[index] = realm.PermissionBinding{
			Subject:    binding.Subject,
			Permission: binding.Permission,
			Scope:      scope,
		}
	}
	for index, credential := range file.Credentials {
		rawID, idErr := decodeFixedHex(credential.ID, 16)
		rawSecret, secretErr := decodeFixedHex(credential.Secret, 32)
		if idErr != nil || secretErr != nil {
			return realm.Seed{}, errors.New("invalid bootstrap file")
		}
		var id realm.CredentialID
		copy(id[:], rawID)
		var secret realm.CredentialSecret
		copy(secret[:], rawSecret)
		seed.Credentials[index] = realm.CredentialBootstrap{
			Subject: credential.Subject,
			Evidence: realm.CredentialEvidence{
				Audience: credential.Audience,
				ID:       id,
				Secret:   secret,
			},
		}
	}
	return seed, nil
}

func (scope scopeFile) toScope() (values.OperationScope, error) {
	var facet values.FacetTypeRef
	if scope.Facet != "" {
		rawFacet, err := decodeFixedHex(scope.Facet, 32)
		if err != nil {
			return values.OperationScope{}, errors.New("invalid bootstrap file")
		}
		facet, err = values.NewFacetTypeRef(rawFacet)
		if err != nil {
			return values.OperationScope{}, errors.New("invalid bootstrap file")
		}
	}
	authority := values.FactAuthorityRef(scope.Authority)
	element := values.ElementID(scope.Element)
	target := values.RealmID(scope.Realm)
	exact := func(authoritySet, elementSet, facetSet, realmSet bool) bool {
		return authoritySet == (scope.Authority != "") &&
			elementSet == (scope.Element != "") &&
			facetSet == (scope.Facet != "") &&
			realmSet == (scope.Realm != "")
	}
	switch scope.Kind {
	case "core_element":
		if !exact(true, true, false, false) {
			return values.OperationScope{}, errors.New("invalid bootstrap file")
		}
		return values.CoreElementScope(authority, element), nil
	case "facet_element":
		if !exact(true, true, false, false) {
			return values.OperationScope{}, errors.New("invalid bootstrap file")
		}
		return values.FacetElementScope(authority, element), nil
	case "facet_key":
		if !exact(true, true, true, false) {
			return values.OperationScope{}, errors.New("invalid bootstrap file")
		}
		return values.FacetKeyScope(authority, element, facet), nil
	case "realm":
		if !exact(false, false, false, true) {
			return values.OperationScope{}, errors.New("invalid bootstrap file")
		}
		return values.RealmScope(target), nil
	case "authority":
		if !exact(true, false, false, false) {
			return values.OperationScope{}, errors.New("invalid bootstrap file")
		}
		return values.AuthorityScope(authority), nil
	case "anchor_closure":
		if !exact(true, true, false, false) {
			return values.OperationScope{}, errors.New("invalid bootstrap file")
		}
		return values.AnchorClosureScope(authority, element), nil
	default:
		return values.OperationScope{}, errors.New("invalid bootstrap file")
	}
}

// decodeFixedHex accepts one exact lowercase hex string of the expected byte
// length; anything else fails without partial decoding.
func decodeFixedHex(text string, size int) ([]byte, error) {
	if len(text) != size*2 {
		return nil, errors.New("invalid hex value")
	}
	raw := make([]byte, size)
	if _, err := hex.Decode(raw, []byte(text)); err != nil {
		return nil, errors.New("invalid hex value")
	}
	if hex.EncodeToString(raw) != text {
		return nil, errors.New("invalid hex value")
	}
	return raw, nil
}

func strictJSON(b []byte, out any) error {
	if _, err := values.ParsePublicJSON(b, values.PublicJSONLimits()); err != nil {
		return errors.New("invalid configuration JSON")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return errors.New("invalid configuration fields")
	}
	return nil
}

func readLimited(path string, maximum int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("cannot read configured file")
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maximum+1))
	if err != nil || int64(len(b)) > maximum {
		return nil, errors.New("invalid configured file size")
	}
	return b, nil
}
