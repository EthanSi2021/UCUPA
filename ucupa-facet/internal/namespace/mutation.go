package namespace

import (
	"bytes"
	"errors"
	"reflect"

	"ucupa/shared/values"
	facetcontract "ucupa/ucupa-facet/contract"
)

// decide is the single deterministic owner transition used live and on replay.
// Recorded external observations are input, never callbacks during replay.
// A re-acceptance records the previous Core's durable successor statement
// next to the new Core's proposal evidence; a recovery after the same Core's
// local detach records none; every other decision records none.
func decide(s *facetState, scope [32]byte, c command, p proof, d departureData, plan transferPlan) (outcome, error) {
	out := outcome{Status: facetcontract.Rejected, Cut: s.Cut}
	reject := func(code string) (outcome, error) { out.Error = code; return out, nil }
	id := c.Key.id()
	acceptance, exists := s.Acceptances[id]
	conn, connected := s.Contacts[id]
	if exists && (!connected || !conn.Address.Valid() || conn.Serial == 0 || conn.Serial > s.Cut || acceptance.Key != c.Key || acceptance.At == 0 || acceptance.At > s.Cut) {
		return outcome{}, facetcontract.ErrUnavailable
	}
	if c.Kind == "retire" {
		return decideRetire(s, c, p, d)
	}
	if c.Kind == "accept" {
		// A retired namespace is terminally disposed: no acceptance of the
		// element can ever appear at this Facet again.
		if _, retired := s.Retired[string(c.Key.Element)]; retired {
			return reject("retired")
		}
		if exists {
			out.Status = facetcontract.Unchanged
			out.AcceptedAt = acceptance.At
			out.Address = conn.Address
			out.ContactSerial = conn.Serial
			return out, nil
		}
		if _, fenced := s.Fenced[string(c.Key.Element)]; fenced {
			return reject("fenced")
		}
		if p.valid(c.Key, true) {
			if p.State != facetcontract.CoreNone || p.Pending != c.Key {
				return reject("proposal")
			}
			for _, a := range s.Acceptances {
				if a.Key.Element == c.Key.Element && a.Key.Core != c.Key.Core {
					return reject("lineage")
				}
			}
			if err := nextCut(s); err != nil {
				return outcome{}, err
			}
			s.Acceptances[id] = accepted{c.Key, s.Cut}
			s.Contacts[id] = contact{c.Address, s.Cut}
			out.Status = facetcontract.Committed
			out.Cut = s.Cut
			out.AcceptedAt = s.Cut
			out.Address = c.Address
			out.ContactSerial = s.Cut
			return out, nil
		}
		// Re-acceptance after a completed Core transfer and recovery after the
		// same Core's local detach both renew one lineage: the content stays
		// in this Facet and only the acceptance key moves. The Core must
		// expose the revoked predecessor key and the recorded predecessor
		// relationship must still be complete.
		if !p.validReaccept(c.Key) && !p.validRecovery(c.Key) {
			return outcome{}, facetcontract.ErrUnavailable
		}
		predecessor := p.Active
		previous, previousExists := s.Acceptances[predecessor.id()]
		previousConn, previousConnected := s.Contacts[predecessor.id()]
		if !previousExists || previous.Key != predecessor || !previousConnected || !previousConn.Address.Valid() ||
			previousConn.Serial == 0 || previousConn.Serial > s.Cut || previous.At == 0 || previous.At > s.Cut {
			return reject("acceptance")
		}
		if p.validReaccept(c.Key) {
			// The previous Core's durable successor statement must bind the
			// element to exactly this new Core.
			if !d.validSuccessor(predecessor, c.Key) {
				return reject("owner")
			}
			for _, a := range s.Acceptances {
				if a.Key.Element == c.Key.Element && a.Key.Core != c.Key.Core && a.Key != predecessor {
					return reject("lineage")
				}
			}
		} else {
			// A local detach reads no departure: the same Core's DETACHED
			// disclosure is the revocation evidence, so any other acceptance
			// of this Element — a foreign Core included — rejects.
			for _, a := range s.Acceptances {
				if a.Key.Element == c.Key.Element && a.Key != predecessor {
					return reject("lineage")
				}
			}
		}
		if err := nextCut(s); err != nil {
			return outcome{}, err
		}
		// The new acceptance is the sole writable lineage: the predecessor
		// relationship is superseded and its stored records are never copied.
		delete(s.Acceptances, predecessor.id())
		delete(s.Contacts, predecessor.id())
		s.Acceptances[id] = accepted{c.Key, s.Cut}
		s.Contacts[id] = contact{c.Address, s.Cut}
		out.Status = facetcontract.Committed
		out.Cut = s.Cut
		out.AcceptedAt = s.Cut
		out.Address = c.Address
		out.ContactSerial = s.Cut
		return out, nil
	}
	if c.Kind == "contact" {
		if !exists {
			return reject("acceptance")
		}
		out.AcceptedAt = acceptance.At
		if c.ContactScope != scope || c.ContactKey != c.Key || c.ContactSerial != conn.Serial {
			return reject("contact")
		}
		out.Status = facetcontract.Unchanged
		out.Address = conn.Address
		out.ContactSerial = conn.Serial
		if c.Address == conn.Address {
			return out, nil
		}
		if err := nextCut(s); err != nil {
			return outcome{}, err
		}
		s.Contacts[id] = contact{c.Address, s.Cut}
		out.Status = facetcontract.Committed
		out.Cut = s.Cut
		out.Address = c.Address
		out.ContactSerial = s.Cut
		return out, nil
	}
	if c.Kind != "put" && c.Kind != "delete" {
		return decideTransfer(s, scope, c, p, d, plan)
	}
	if !exists || !p.valid(c.Key, false) {
		return outcome{}, facetcontract.ErrUnavailable
	}
	out.AcceptedAt = acceptance.At
	out.Core = p
	if code := coreRejection(p, c.Key); code != "" {
		return reject(code)
	}
	if _, fenced := s.Fenced[string(c.Key.Element)]; fenced {
		return reject("fenced")
	}
	key := recordID(c.Key.Element, c.Type)
	current, found := s.Records[key]
	if c.Kind == "put" {
		matches := (c.Absent && !found) || (!c.Absent && found && reflect.DeepEqual(current.Revision, c.Expected))
		if !matches {
			return reject("revision")
		}
		if !found {
			count := 0
			for _, r := range s.Records {
				if r.Element == c.Key.Element {
					count++
				}
			}
			if count >= maxTypes {
				return reject("capacity")
			}
		}
		if found && bytes.Equal(current.Payload, c.Payload) {
			out.Status = facetcontract.Unchanged
			out.Revision = current.Revision
			return out, nil
		}
		if err := nextCut(s); err != nil {
			return outcome{}, err
		}
		rev := revisionData{scope, c.Key.Element, append([]byte(nil), c.Type...), s.Cut}
		s.Records[key] = record{c.Key.Element, append([]byte(nil), c.Type...), append([]byte(nil), c.Payload...), rev}
		delete(s.Tombstones, key)
		out.Status = facetcontract.Committed
		out.Cut = s.Cut
		out.Revision = rev
		return out, nil
	}
	if c.Kind == "delete" {
		if !found {
			return reject("key")
		}
		if !reflect.DeepEqual(current.Revision, c.Expected) {
			return reject("revision")
		}
		if err := nextCut(s); err != nil {
			return outcome{}, err
		}
		delete(s.Records, key)
		rev := revisionData{scope, c.Key.Element, append([]byte(nil), c.Type...), s.Cut}
		s.Tombstones[key] = rev
		out.Status = facetcontract.Committed
		out.Cut = s.Cut
		out.Revision = rev
		return out, nil
	}
	return outcome{}, facetcontract.ErrInvalid
}

func (f *Owner) lookupLocked(c command) (facetcontract.MutationResult, error, bool) {
	if err := f.available(); err != nil {
		return facetcontract.MutationResult{}, err, true
	}
	if index, ok := f.operations[c.ID]; ok {
		e := f.events[index]
		if !reflect.DeepEqual(e.Input, c) {
			return facetcontract.MutationResult{}, facetcontract.ErrOperationConflict, true
		}
		return e.Outcome.result(f.scope, c.Key), rejectionErrors[e.Outcome.Error], true
	}
	return facetcontract.MutationResult{}, nil, false
}
func (f *Owner) commitLocked(c command, p proof, departure departureData, plan transferPlan) (facetcontract.MutationResult, error) {
	if r, err, found := f.lookupLocked(c); found {
		return r, err
	}
	next := f.state.clone()
	out, err := decide(&next, f.scope, c, p, departure, plan)
	if err != nil {
		return facetcontract.MutationResult{}, err
	}
	e := event{c, p, departure, plan, out}
	events := append(append([]event(nil), f.events...), e)
	// One replay rule proves all facts, contacts, receipts and cut issuance.
	if err = validateHistory(f.authority, f.scope, next, events); err != nil {
		return facetcontract.MutationResult{}, err
	}
	if f.store != nil {
		if err = f.store.save(f.authority, f.scope, next, events); err != nil {
			if errors.Is(err, facetcontract.ErrIndeterminate) {
				doc, loadErr := loadStore(f.store.db)
				loadErr = errors.Join(loadErr, f.store.checkWriter(f.authority))
				if loadErr == nil && doc.Authority == f.authority && doc.Scope == f.scope {
					f.install(doc)
				} else {
					_ = f.store.db.Close()
					f.store.db = nil
				}
			}
			return facetcontract.MutationResult{}, err
		}
	}
	f.operations[c.ID] = len(f.events)
	f.events = events
	f.projectPublicChangesLocked(c, out)
	f.state = next
	f.refreshContactLocked()
	if f.store != nil && f.store.loseResponse != nil {
		fail := f.store.loseResponse
		f.store.loseResponse = nil
		if fail() != nil {
			return facetcontract.MutationResult{}, facetcontract.ErrIndeterminate
		}
	}
	return out.result(f.scope, c.Key), rejectionErrors[out.Error]
}
func (f *Owner) base(actor values.PrincipalRef, key values.FacetAcceptanceKey, op facetcontract.OperationID, kind string) command {
	return command{ID: byteString(op), Actor: actorData{byteString(actor.Realm), byteString(actor.Principal)}, Owner: f.authority, Kind: kind, Key: packKey(key)}
}
