package commit

import (
	"bytes"
	"encoding/json"
	"math"
	"ucupa/shared/values"
)

const maxWriterDecisions = 1024

type writerRecord struct {
	Request   values.WriterHandoffRequest
	Caller    values.PrincipalRef
	FinalCut  [32]byte
	Installed bool
}
type writerJournal struct {
	Owner   values.FactAuthorityRef
	Handoff bool
	Epoch   uint64
	Records []writerRecord
}

// Operation and caller names are opaque byte identities. JSON strings would
// replace invalid UTF-8, so the private representation uses byte fields.
type writerRecordBytes struct {
	Operation        []byte
	ExpectedEpoch    uint64
	Target           [32]byte
	Realm, Principal []byte
	FinalCut         [32]byte
	Installed        bool
}

func (r writerRecord) MarshalJSON() ([]byte, error) {
	return json.Marshal(writerRecordBytes{[]byte(r.Request.Operation), r.Request.ExpectedEpoch, r.Request.Target, []byte(r.Caller.Realm), []byte(r.Caller.Principal), r.FinalCut, r.Installed})
}
func (r *writerRecord) UnmarshalJSON(raw []byte) error {
	var b writerRecordBytes
	if err := json.Unmarshal(raw, &b); err != nil {
		return err
	}
	*r = writerRecord{Request: values.WriterHandoffRequest{Operation: string(b.Operation), ExpectedEpoch: b.ExpectedEpoch, Target: b.Target}, Caller: values.PrincipalRef{Realm: values.RealmID(b.Realm), Principal: values.PrincipalID(b.Principal)}, FinalCut: b.FinalCut, Installed: b.Installed}
	return nil
}

func initialWriter(owner values.FactAuthorityRef, cfg *values.WriterStartup) writerJournal {
	j := writerJournal{Owner: owner, Epoch: 1}
	if cfg != nil {
		j.Handoff = cfg.Handoff
	}
	return j
}
func (j writerJournal) pending() bool {
	return len(j.Records) > 0 && !j.Records[len(j.Records)-1].Installed
}
func (j writerJournal) valid() bool {
	if !j.Owner.Valid() || len(j.Records) > maxWriterDecisions {
		return false
	}
	epoch := uint64(1)
	ids := map[string]bool{}
	targets := map[[32]byte]bool{}
	for n, r := range j.Records {
		if !r.Request.Valid() || !r.Caller.Valid() || r.FinalCut == [32]byte{} || !j.Handoff ||
			r.Request.ExpectedEpoch != epoch || epoch == math.MaxUint64 || ids[r.Request.Operation] || targets[r.Request.Target] {
			return false
		}
		ids[r.Request.Operation] = true
		targets[r.Request.Target] = true
		if r.Installed {
			epoch++
		} else if n != len(j.Records)-1 {
			return false
		}
	}
	return j.Epoch == epoch
}
func decodeWriter(raw []byte, owner values.FactAuthorityRef) (writerJournal, error) {
	var j writerJournal
	if len(raw) == 0 || len(raw) > 1<<20 || json.Unmarshal(raw, &j) != nil || !j.valid() || j.Owner != owner {
		return j, values.PublicUnavailable
	}
	canonical, err := json.Marshal(j)
	if err != nil || !bytes.Equal(raw, canonical) {
		return j, values.PublicUnavailable
	}
	return j, nil
}
func (j writerJournal) status(operation string) (values.WriterStatus, error) {
	s := values.WriterStatus{Owner: j.Owner, Epoch: j.Epoch, Pending: j.pending()}
	if operation == "" {
		return s, nil
	}
	for _, r := range j.Records {
		if r.Request.Operation == operation {
			s.Decision = &values.WriterDecision{Operation: operation, ClosedEpoch: r.Request.ExpectedEpoch, FinalCut: r.FinalCut, Installed: r.Installed}
			return s, nil
		}
	}
	return values.WriterStatus{}, values.PublicNotFound
}

// qualified is read-only and runs both before opening writable and under the
// original Store's exclusive lock. It never treats an address or Ref as proof.
// The enable flag is the only startup comparison: authorization identity is
// constrained by the original Store's durable RealmAttachment.
func (j writerJournal) qualified(cfg *values.WriterStartup) error {
	if !cfg.Valid() {
		return values.PublicInvalid
	}
	handoff := cfg != nil && cfg.Handoff
	if handoff != j.Handoff {
		return values.PublicDenied
	}
	if cfg == nil || cfg.Install == nil {
		if j.pending() {
			return values.PublicUnavailable
		}
		return nil
	}
	if len(j.Records) == 0 {
		return values.PublicConflict
	}
	last := j.Records[len(j.Records)-1]
	if cfg.Install.Operation != last.Request.Operation || cfg.Install.Commitment() != last.Request.Target {
		return values.PublicDenied
	}
	return nil
}
