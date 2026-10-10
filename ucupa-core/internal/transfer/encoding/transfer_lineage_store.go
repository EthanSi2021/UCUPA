package encoding

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	storerecord "ucupa/ucupa-core/internal/store/record"
	"ucupa/ucupa-core/internal/token"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// Narrow per-member custody values, not copies of earlier full manifests.
// Arbitrary valid operation bytes retain the original binary identity.
type StoredLineageStep struct {
	Controller      values.FactAuthorityRef    `json:"controller"`
	Source          values.FactAuthorityRef    `json:"source"`
	Target          values.FactAuthorityRef    `json:"target"`
	Operation       []byte                     `json:"operation"`
	Kind            contract.ElementKind       `json:"kind"`
	SourceRevision  storerecord.StoredRevision `json:"sourceRevision"`
	FinalScope      [32]byte                   `json:"finalScope"`
	FinalSerial     uint64                     `json:"finalSerial"`
	InstalledScope  [32]byte                   `json:"installedScope"`
	InstalledSerial uint64                     `json:"installedSerial"`
	Decision        [32]byte                   `json:"decision"`
	Installation    [32]byte                   `json:"installation"`
	SourceAddress   values.NextHop             `json:"sourceAddress"`
	TargetAddress   values.NextHop             `json:"targetAddress"`
}

func storeLineageStep(s transferfact.LineageStep) StoredLineageStep {
	return StoredLineageStep{Controller: s.Controller, Source: s.Source, Target: s.Target, Operation: []byte(s.Operation), Kind: s.Kind, SourceRevision: storerecord.StoreRevision(s.SourceRevision),
		FinalScope: token.Scope(s.FinalCut), FinalSerial: token.Serial(s.FinalCut), InstalledScope: token.Scope(s.InstalledCut), InstalledSerial: token.Serial(s.InstalledCut),
		Decision: s.Decision, Installation: s.Installation, SourceAddress: s.SourceAddress, TargetAddress: s.TargetAddress}
}

func (s StoredLineageStep) step() (transferfact.LineageStep, error) {
	revision, err := s.SourceRevision.Revision()
	return transferfact.LineageStep{Controller: s.Controller, Source: s.Source, Target: s.Target, Operation: contract.OperationID(string(s.Operation)), Kind: s.Kind, SourceRevision: revision,
		FinalCut: token.FromParts(s.FinalScope, s.FinalSerial), InstalledCut: token.FromParts(s.InstalledScope, s.InstalledSerial),
		Decision: s.Decision, Installation: s.Installation, SourceAddress: s.SourceAddress, TargetAddress: s.TargetAddress}, err
}
