package token

import "ucupa/shared/values"

// CoreReadSnapshot names one retained owner-local view. Its representation
// carries no registry, deadline or access grant.
type CoreReadSnapshot struct {
	owner values.FactAuthorityRef
	run   [32]byte
	id    uint64
	actor values.PrincipalRef
	scope values.ElementID
	cut   CoreReadCut
}

// ValidCoreReadSnapshot preserves structural validity only. The read owner
// still checks the caller, current run, retained entry and lifetime.
func ValidCoreReadSnapshot(snapshot CoreReadSnapshot) bool {
	return snapshot.owner.Valid() && snapshot.run != ([32]byte{}) && snapshot.id != 0 &&
		snapshot.actor.Valid() && snapshot.scope.Valid() && Valid(snapshot.cut)
}

// CoreReadSnapshotFromParts binds values already issued by the read owner.
// It does not register a handle or authorize a read.
func CoreReadSnapshotFromParts(owner values.FactAuthorityRef, run [32]byte, id uint64, actor values.PrincipalRef, scope values.ElementID, cut CoreReadCut) CoreReadSnapshot {
	return CoreReadSnapshot{owner: owner, run: run, id: id, actor: actor, scope: scope, cut: cut}
}
func SnapshotOwner(snapshot CoreReadSnapshot) values.FactAuthorityRef { return snapshot.owner }
func SnapshotRun(snapshot CoreReadSnapshot) [32]byte                  { return snapshot.run }
func SnapshotID(snapshot CoreReadSnapshot) uint64                     { return snapshot.id }
func SnapshotActor(snapshot CoreReadSnapshot) values.PrincipalRef     { return snapshot.actor }
func SnapshotScope(snapshot CoreReadSnapshot) values.ElementID        { return snapshot.scope }
func SnapshotCut(snapshot CoreReadSnapshot) CoreReadCut               { return snapshot.cut }

// ConnectedCut identifies a bounded runtime-only context. It supplies neither
// an address nor permission and cannot revive an evicted context.
type ConnectedCut struct {
	owner   values.FactAuthorityRef
	run     [32]byte
	id      uint64
	callers [32]byte
}

func (cut ConnectedCut) Equal(other ConnectedCut) bool { return cut == other }
func ValidConnectedCut(cut ConnectedCut) bool {
	return cut.owner.Valid() && cut.run != ([32]byte{}) && cut.id != 0 && cut.callers != ([32]byte{})
}
func ConnectedCutFromParts(owner values.FactAuthorityRef, run [32]byte, id uint64, callers [32]byte) ConnectedCut {
	return ConnectedCut{owner: owner, run: run, id: id, callers: callers}
}
func ConnectedOwner(cut ConnectedCut) values.FactAuthorityRef { return cut.owner }
func ConnectedRun(cut ConnectedCut) [32]byte                  { return cut.run }
func ConnectedID(cut ConnectedCut) uint64                     { return cut.id }
func ConnectedCallers(cut ConnectedCut) [32]byte              { return cut.callers }
