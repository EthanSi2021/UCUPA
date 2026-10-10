package commit

import (
	"crypto/rand"
	"fmt"
	"math"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/access/observe"
	commitbinding "ucupa/ucupa-core/internal/commit/binding"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	"ucupa/ucupa-core/internal/token"
)

type coreState struct {
	authority      values.FactAuthorityRef
	publicKey      [32]byte
	scope          [32]byte
	cutSerial      uint64
	revisionSerial uint64
	elements       elementfact.State
	operations     map[contract.OperationID]retainedOperation
	commits        []coreCommit
	mount          mountState
	attachments    attachmentState
	transfers      transferState
	realmContact   realmContactState
}

// coreCommit orders committed changes by cut. Canonical input and the original
// receipt remain in operations, rather than being duplicated in an event body.
type coreCommit struct {
	kind         commitKind
	operation    contract.OperationID
	controlIndex uint64
}

type commitKind uint8

const elementCommitted commitKind = 1

const mountCommitted commitKind = 2

const attachmentCommitted commitKind = 3

const transferCommitted commitKind = 4

func newCoreState(authority values.FactAuthorityRef, scope [32]byte) *coreState {
	var publicKey [32]byte
	_, _ = rand.Read(publicKey[:])
	return &coreState{
		authority:   authority,
		publicKey:   publicKey,
		scope:       scope,
		cutSerial:   1,
		elements:    elementfact.New(),
		operations:  make(map[contract.OperationID]retainedOperation),
		mount:       newMountState(),
		attachments: newAttachmentState(),
		transfers:   newTransferState(),
	}
}

func (state *coreState) clone() *coreState {
	cloned := &coreState{
		authority:      state.authority,
		publicKey:      state.publicKey,
		scope:          state.scope,
		cutSerial:      state.cutSerial,
		revisionSerial: state.revisionSerial,
		elements:       state.elements.Clone(),
		operations:     make(map[contract.OperationID]retainedOperation, len(state.operations)),
		commits:        append([]coreCommit(nil), state.commits...),
		mount:          state.mount.clone(),
		attachments:    state.attachments.clone(),
		transfers:      state.transfers.clone(),
		realmContact:   state.realmContact.clone(),
	}
	for id, value := range state.operations {
		cloned.operations[id] = value
	}
	return cloned
}

func (state *coreState) currentCut() contract.CoreReadCut {
	return token.FromParts(state.scope, state.cutSerial)
}

func (state *coreState) issueFactChange(id values.ElementID) (contract.Revision, contract.CoreReadCut, error) {
	if state.revisionSerial == math.MaxUint64 || state.cutSerial == math.MaxUint64 {
		return contract.Revision{}, contract.CoreReadCut{}, fmt.Errorf("%w: Core token space exhausted", contract.ErrStore)
	}
	state.revisionSerial++
	state.cutSerial++
	return token.RevisionFromParts(state.scope, id, state.revisionSerial),
		state.currentCut(), nil
}

// A retained decision adds its admission result's cut to the one Element
// outcome representation. The fact rule and its durable receipt do not keep
// parallel copies of the record fields.
type retainedOutcome struct {
	elementfact.Outcome
	cut contract.CoreReadCut
}

type retainedOperation struct {
	admission contract.CoreReadCut
	binding   commitbinding.Binding
	outcome   retainedOutcome
}

func rejectedOutcome(kind elementfact.Rejection, cut contract.CoreReadCut) retainedOutcome {
	return retainedOutcome{Outcome: elementfact.Outcome{Status: contract.MutationRejected, Rejection: kind}, cut: cut}
}

func unchangedAnchorOutcome(value elementfact.Anchor, cut contract.CoreReadCut) retainedOutcome {
	return retainedOutcome{Outcome: elementfact.Outcome{Status: contract.MutationUnchanged, Kind: contract.ElementAnchor, Anchor: value}, cut: cut}
}

func unchangedEntityOutcome(value elementfact.Entity, cut contract.CoreReadCut) retainedOutcome {
	return retainedOutcome{Outcome: elementfact.Outcome{Status: contract.MutationUnchanged, Kind: contract.ElementEntity, Entity: value}, cut: cut}
}

func committedAnchorOutcome(value elementfact.Anchor, cut contract.CoreReadCut) retainedOutcome {
	return retainedOutcome{Outcome: elementfact.Outcome{Status: contract.MutationCommitted, Kind: contract.ElementAnchor, Anchor: value}, cut: cut}
}

func committedEntityOutcome(value elementfact.Entity, cut contract.CoreReadCut) retainedOutcome {
	return retainedOutcome{Outcome: elementfact.Outcome{Status: contract.MutationCommitted, Kind: contract.ElementEntity, Entity: value}, cut: cut}
}

func committedDeleteOutcome(revision contract.Revision, cut contract.CoreReadCut) retainedOutcome {
	return retainedOutcome{Outcome: elementfact.Outcome{Status: contract.MutationCommitted, Tombstone: revision}, cut: cut}
}

// prepareMutation keeps deduplication, the write gate and candidate history in
// the commit aggregate. A retained outcome has no candidate to publish.
func (state *coreState) prepareMutation(operationID contract.OperationID, binding commitbinding.Binding) (*coreState, retainedOutcome, error) {
	if retained, exists := state.operations[operationID]; exists {
		if !commitbinding.EqualBindings(retained.binding, binding) {
			return nil, retainedOutcome{}, contract.ErrOperationConflict
		}
		return nil, retained.outcome, nil
	}
	if state.mountOperationClaimed(operationID) {
		return nil, retainedOutcome{}, contract.ErrOperationConflict
	}
	if err := state.guardOrdinaryMutation(binding); err != nil {
		return nil, retainedOutcome{}, err
	}
	next := state.clone()
	outcome, err := decideMutation(next, binding)
	if err != nil {
		return nil, retainedOutcome{}, err
	}
	next.operations[operationID] = retainedOperation{admission: state.currentCut(), binding: binding, outcome: outcome}
	if outcome.Status == contract.MutationCommitted {
		next.commits = append(next.commits, coreCommit{kind: elementCommitted, operation: operationID})
	}
	return next, outcome, nil
}

// publicationView borrows current immutable facts for synchronous observation.
// It cannot retain the aggregate's journal, receipts or Store.
func publicationView(state *coreState) observe.View {
	if state == nil {
		return observe.View{}
	}
	return observe.View{Present: true, CutSerial: state.cutSerial, Elements: state.elements.View,
		Mount: state.mount.facts.View, Attachments: state.attachments.facts.View, Transfers: sourceVisibility(state)}
}
