package values

import (
	"crypto/rand"
	"crypto/sha256"
	"errors"
)

// WriterStartup is trusted deployment configuration, not a request credential.
// Handoff only enables or disables the owner's writer-handoff capability; the
// durable RealmAttachment of the original Store constrains its authorization
// identity, so no free-form assembly label remains here.
type WriterStartup struct {
	Handoff bool
	Install *WriterInstallation
}

// WriterInstallation is private to the designated successor's startup. Secret
// must be independently generated with crypto/rand and never logged or sent to
// the source. Only Commitment is published in the handoff request.
type WriterInstallation struct {
	Operation string
	Secret    [32]byte
}

// NewWriterInstallation prepares one target-held installation capability.
// The caller securely provisions Secret only to the designated successor.
func NewWriterInstallation(operation string) (WriterInstallation, error) {
	i := WriterInstallation{Operation: operation}
	if len(operation) == 0 || len(operation) > 128 {
		return i, errors.New("invalid writer operation")
	}
	if _, err := rand.Read(i.Secret[:]); err != nil {
		return WriterInstallation{}, err
	}
	return i, nil
}

func (i WriterInstallation) Commitment() [32]byte { return sha256.Sum256(i.Secret[:]) }

type WriterHandoffRequest struct {
	Operation     string
	ExpectedEpoch uint64
	Target        [32]byte
}

type WriterDecision struct {
	Operation   string
	ClosedEpoch uint64
	FinalCut    [32]byte
	Installed   bool
}

type WriterStatus struct {
	Owner    FactAuthorityRef
	Epoch    uint64
	Pending  bool
	Decision *WriterDecision
}

// Valid checks input shape only; neither a commitment nor a claimed epoch is
// authorization or proof that any installation has occurred.
func (r WriterHandoffRequest) Valid() bool {
	return len(r.Operation) > 0 && len(r.Operation) <= 128 && r.ExpectedEpoch > 0 && r.Target != [32]byte{} && r.Target != sha256.Sum256(make([]byte, 32))
}

func (s *WriterStartup) Valid() bool {
	if s == nil {
		return true
	}
	return s.Install == nil || (len(s.Install.Operation) > 0 && len(s.Install.Operation) <= 128 && s.Install.Secret != [32]byte{})
}
