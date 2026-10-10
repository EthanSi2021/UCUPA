package source

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	federationcontract "ucupa/ucupa-core/contract/federation"
	federationvalue "ucupa/ucupa-core/internal/query/federation/value"

	accessregistry "ucupa/ucupa-core/internal/access/registry"
	federationdenial "ucupa/ucupa-core/internal/query/federation/denial"
	"ucupa/ucupa-core/internal/token"
)

// Request-local opaque evidence, not raw storage serials, credentials or
// AuthorizationView bytes. The private random basis salts each capture so a
// public stamp cannot reveal counters or correlate otherwise hidden reads.
// Authenticity still comes from the source's authenticated connection; these
// stamps neither grant access nor constitute a standalone digital signature.
func federationReadStamps(basis accessregistry.ReadUse) (cut, authorization [32]byte) {
	stamp := func(domain string, parts ...[]byte) [32]byte {
		return federationStamp(basis, domain, parts...)
	}
	var serial [8]byte
	binary.BigEndian.PutUint64(serial[:], token.Serial(basis.Cut))
	scope := token.Scope(basis.Cut)
	cut = stamp("ucupa.core.read-cut", scope[:], serial[:])
	if denial := basis.Source.Denial; denial != nil {
		view, _ := federationdenial.Authorization(*denial)
		cut = stamp("ucupa.core.hidden-closure")
		authorization = stamp("ucupa.core.authorization-denial", []byte(basis.Caller.Subject.Realm), []byte(basis.Caller.Subject.Principal), basis.Caller.Establishment[:], view[:])
		return
	}
	authorization = stamp("ucupa.core.authorization-view", []byte(basis.Caller.Subject.Realm), []byte(basis.Caller.Subject.Principal), basis.Caller.Establishment[:], basis.View[:])
	return
}

func federationStamp(basis accessregistry.ReadUse, domain string, parts ...[]byte) [32]byte {
	h := hmac.New(sha256.New, []byte(basis.Token.Value))
	var length [8]byte
	for _, part := range append([][]byte{[]byte(domain), []byte(basis.Token.Owner), []byte(basis.Boundary), basis.Source.Request.Purpose[:]}, parts...) {
		binary.BigEndian.PutUint64(length[:], uint64(len(part)))
		h.Write(length[:])
		h.Write(part)
	}
	var result [32]byte
	copy(result[:], h.Sum(nil))
	return result
}

func federationCoverageStamp(basis accessregistry.ReadUse, conditions string, page federationcontract.FederationProjectionPage) [32]byte {
	cut, view := federationReadStamps(basis)
	traversal := federationvalue.ProjectionTraversal(page)
	return federationStamp(basis, "ucupa.core.query-coverage", cut[:], view[:], []byte(conditions), traversal[:], []byte{byte(page.Gap)})
}
