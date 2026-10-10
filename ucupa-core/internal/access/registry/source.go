package registry

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	federationcontract "ucupa/ucupa-core/contract/federation"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	federationdenial "ucupa/ucupa-core/internal/query/federation/denial"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// SourceConfig is the finite, already-authenticated source binding installed
// in a source receipt. It is construction input, not mutable registry state.
type SourceConfig struct {
	Source   values.FactAuthorityRef
	Request  federationcontract.FederationReadRequest
	Mode     contract.QueryReadMode
	Incoming mountfact.Incoming
	Denial   *federationdenial.Evidence
}

// SourceUse is a detached finite view of one source binding. The outgoing and
// exact-read authorization maps remain private to Registry.
type SourceUse struct {
	Present bool
	Source  values.FactAuthorityRef
	Request federationcontract.FederationReadRequest
	Mode    contract.QueryReadMode
	Denial  *federationdenial.Evidence
}

type sourceState struct {
	source    values.FactAuthorityRef
	request   federationcontract.FederationReadRequest
	mode      contract.QueryReadMode
	incoming  mountfact.Incoming
	outgoing  map[values.ElementID]mountfact.Record
	readViews map[values.ElementID]values.AuthorizationView
	denial    *federationdenial.Evidence
}

// CheckSourceConfig checks the captured incoming relation before a receipt is
// issued. An empty outgoing set has no other live dependencies at this point.
func CheckSourceConfig(config SourceConfig, current mountfact.View) error {
	if current.Incoming() != config.Incoming {
		return values.PublicInvalidContext
	}
	if in := config.Incoming; in.Key.Valid() && (in.Detached || in.Gate != mountfact.GateOpen || !mountfact.ValidMountAdmissionKey(in.Admission.Key)) {
		return values.PublicUnavailable
	}
	return nil
}

// IssueSourceLocked converts a prior 4096-byte reservation into the source
// receipt without changing the original pre-issuance error order.
func (r *Registry) IssueSourceLocked(reservation *Reservation, caller values.CallerContext, boundary values.ElementID, cut contract.CoreReadCut, view values.AuthorizationView, requirements []values.Requirement, config SourceConfig) (ReadUse, error) {
	if reservation == nil || !reservation.active || reservation.registry != r || reservation.bytes != 4096 {
		return ReadUse{}, values.PublicInvalidContext
	}
	item, err := r.issueReadLocked(caller, boundary, Ref{}, 0)
	if err != nil {
		return ReadUse{}, err
	}
	if !r.transferLocked(reservation, item) {
		r.removeLocked(item)
		return ReadUse{}, values.PublicInvalidContext
	}
	item.read.cut = cut
	item.read.view = view
	item.read.requirements = cloneRequirements(requirements)
	item.read.source = newSourceState(config)
	return readUse(item), nil
}

// IssueHiddenSourceLocked retains only denial and incoming evidence. It never
// owns a query snapshot or the normal source reservation.
func (r *Registry) IssueHiddenSourceLocked(caller values.CallerContext, boundary values.ElementID, cut contract.CoreReadCut, config SourceConfig) (ReadUse, error) {
	item, err := r.issueReadLocked(caller, boundary, Ref{}, 0)
	if err != nil {
		return ReadUse{}, err
	}
	item.read.cut = cut
	item.read.source = newSourceState(config)
	return readUse(item), nil
}

func newSourceState(config SourceConfig) *sourceState {
	state := &sourceState{
		source: config.Source, request: config.Request, mode: config.Mode,
		incoming: config.Incoming, outgoing: make(map[values.ElementID]mountfact.Record),
	}
	if config.Denial != nil {
		copy := *config.Denial
		state.denial = &copy
	}
	return state
}

func sourceUse(state *sourceState) SourceUse {
	result := SourceUse{
		Present: true, Source: state.source, Request: state.request,
		Mode: state.mode,
	}
	if state.denial != nil {
		copy := *state.denial
		result.Denial = &copy
	}
	return result
}

// CheckSourceRelationsLocked verifies the exact incoming and every followed
// outgoing relation retained by this source receipt.
func (r *Registry) CheckSourceRelationsLocked(ref Ref, current mountfact.View, transfers transferfact.Visibility) error {
	if !r.presentLocked(ref) || ref.entry.read == nil || ref.entry.read.source == nil {
		return values.PublicInvalidContext
	}
	state := ref.entry.read.source
	if current.Incoming() != state.incoming {
		return values.PublicInvalidContext
	}
	if in := state.incoming; in.Key.Valid() && (in.Detached || in.Gate != mountfact.GateOpen || !mountfact.ValidMountAdmissionKey(in.Admission.Key)) {
		return values.PublicUnavailable
	}
	for id, old := range state.outgoing {
		relation, ok := current.Outgoing(id)
		if !ok || old != relation {
			return values.PublicInvalidContext
		}
		if !transfers.OutgoingReadable(id, relation) {
			return values.PublicUnavailable
		}
	}
	return nil
}

// SourceReadViewLocked returns only the immutable view first bound to an exact
// Element read; it does not expose the backing map.
func (r *Registry) SourceReadViewLocked(ref Ref, element values.ElementID) (values.AuthorizationView, bool) {
	if !r.presentLocked(ref) || ref.entry.read == nil || ref.entry.read.source == nil {
		return values.AuthorizationView{}, false
	}
	view, ok := ref.entry.read.source.readViews[element]
	return view, ok
}

// BindSourceReadViewLocked installs the first exact-read view at its original
// post-disclosure validation point and charges it once. If another binding is
// already present, that immutable first view wins.
func (r *Registry) BindSourceReadViewLocked(ref Ref, element values.ElementID, proposed values.AuthorizationView) (values.AuthorizationView, error) {
	if !r.presentLocked(ref) || ref.entry.read == nil || ref.entry.read.source == nil {
		return values.AuthorizationView{}, values.PublicInvalidContext
	}
	state := ref.entry.read.source
	if bound, ok := state.readViews[element]; ok {
		return bound, nil
	}
	if !r.canChargeLocked(1024) {
		return values.AuthorizationView{}, values.PublicResourceExhausted
	}
	if state.readViews == nil {
		state.readViews = make(map[values.ElementID]values.AuthorizationView)
	}
	state.readViews[element] = proposed
	ref.entry.bytes += 1024
	r.bytes += 1024
	return proposed, nil
}

// BindSourceDependenciesLocked checks and records dependencies one by one.
// It deliberately preserves the original partial-binding behavior: a later
// mismatch or capacity failure does not roll back already-bound edges.
func (r *Registry) BindSourceDependenciesLocked(ref Ref, current mountfact.View, dependencies map[values.ElementID]mountfact.Record) error {
	if !r.presentLocked(ref) || ref.entry.read == nil || ref.entry.read.source == nil {
		return values.PublicInvalidContext
	}
	state := ref.entry.read.source
	for id, captured := range dependencies {
		if current.Record(id) != captured {
			return values.PublicInvalidContext
		}
		if _, exists := state.outgoing[id]; exists {
			continue
		}
		if !r.canChargeLocked(1024) {
			return values.PublicResourceExhausted
		}
		r.bytes += 1024
		ref.entry.bytes += 1024
		state.outgoing[id] = captured
	}
	return nil
}
