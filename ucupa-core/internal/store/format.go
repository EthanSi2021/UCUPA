package store

// Core recognizes one exact private format, including writer and transfer journals.
const (
	ElementBucket    = "ucupa.core.elements"
	OperationBucket  = "ucupa.core.operations"
	CommitBucket     = "ucupa.core.commits"
	MountEventBucket = "ucupa.core.mount-events"
	FormatMarker     = "ucupa.core.local@9c4f2a6e8b1d7350f6a3c9e2b7d4f1a8"

	FormatMarkerTag      byte = 0
	TokenScopeTag        byte = 2
	CurrentCutTag        byte = 3
	RevisionSerialTag    byte = 4
	AuthorityRefTag      byte = 5
	AttachmentJournalTag byte = 6
	PublicRevisionTag    byte = 7
	TransferJournalTag   byte = 9
	RealmContactTag      byte = 10
)
