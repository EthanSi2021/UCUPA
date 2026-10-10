// Package exchange owns the opaque, trusted-local connected-query envelope.
// It is an in-process value boundary, not a network protocol or credential.
package exchange

import (
	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	facetfact "ucupa/ucupa-core/internal/facet/fact"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

type Action uint8

const (
	Probe Action = iota + 1
	Capture
	Borrow
	Check
	End
	Release
)

type Kind uint8

const (
	Resolve Kind = iota + 1
	List
	Query
)

type Intent struct {
	Kind  Kind
	Path  corecontract.Path
	Query corecontract.PathRegionQuery
}

type Grant struct {
	Kind       Kind
	Scope      values.ElementID
	Prefix     []values.ElementID
	Adjacency  []values.ElementID
	Mount      corecontract.MountKey
	ProofOnly  bool
	VerifyOnly bool
	EntryProof bool
}

type Facts struct {
	Present        bool
	Authority      values.FactAuthorityRef
	Scope          [32]byte
	CutSerial      uint64
	Elements       elementfact.View
	Attachments    facetfact.View
	Mount          mountfact.View
	Transfers      transferfact.Visibility
	Cut            corecontract.CoreReadCut
	Run            [32]byte
	AdmissionRange corecontract.BoundedSphere
}

type LeaseID struct {
	Run [32]byte
	ID  uint64
}

type RequestData struct {
	Action       Action
	Actor        values.PrincipalRef
	Intent       Intent
	Grant        Grant
	Key          corecontract.MountKey
	ProofOnly    bool
	Mode         corecontract.QueryReadMode
	Cut          corecontract.CoreReadCut
	Run          [32]byte
	Lease        LeaseID
	Snapshot     *corecontract.CoreReadSnapshot
	Contact      mountfact.Contact
	CheckCurrent bool
	Cleanup      *Cleanup
	Replan       bool
	VerifyOnly   bool
	Bounds       Grant
}

type ReplyData struct {
	Authority values.FactAuthorityRef
	Facts     Facts
	Grant     Grant
	Mode      corecontract.QueryReadMode
	Lease     LeaseID
	Snapshot  *corecontract.CoreReadSnapshot
	Contact   mountfact.Contact
}

// Request and Reply deliberately reveal no payload to assembly. Only Core's
// internal connected owner uses the constructor and reader functions.
type Request struct{ data RequestData }
type Reply struct{ data ReplyData }

func NewRequest(data RequestData) Request     { return Request{data: data} }
func ReadRequest(request Request) RequestData { return request.data }
func NewReply(data ReplyData) Reply           { return Reply{data: data} }
func ReadReply(reply Reply) ReplyData         { return reply.data }
