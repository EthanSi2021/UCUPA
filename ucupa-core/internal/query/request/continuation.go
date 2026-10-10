package request

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/internal/token"
)

// QueryContinuation is a private, caller/query/run-bound position. It never
// chooses a different cut or grants access. Only Retained carries a snapshot.
type QueryContinuation struct {
	owner    values.FactAuthorityRef
	scope    [32]byte
	run      [32]byte
	query    RegionQuery
	actor    values.PrincipalRef
	cut      token.CoreReadCut
	last     values.ElementID
	mode     QueryReadMode
	root     values.ElementID
	depth    uint64
	snapshot *token.CoreReadSnapshot
}

type ConnectedQueryContinuation struct {
	cut   token.ConnectedCut
	query PathRegionQuery
	last  values.ElementID
}

type ConnectedDirectContinuation struct {
	cut  token.ConnectedCut
	path Path
	last values.ElementID
}

func ValidQueryContinuation(c QueryContinuation) bool {
	q, ok := CanonicalRegionQuery(c.query)
	return ok && q == c.query && c.owner.Valid() && c.scope != ([32]byte{}) && c.run != ([32]byte{}) &&
		c.actor.Valid() && c.last.Valid() && c.root.Valid() && token.Valid(c.cut) && token.Scope(c.cut) == c.scope &&
		((c.mode == QueryCurrentVerified && c.snapshot == nil) ||
			(c.mode == QueryRetained && c.snapshot != nil && token.ValidCoreReadSnapshot(*(c.snapshot)) && token.SnapshotCut(*(c.snapshot)) == c.cut &&
				token.SnapshotOwner(*(c.snapshot)) == c.owner && token.SnapshotScope(*(c.snapshot)) == c.root && token.SnapshotActor(*(c.snapshot)) == c.actor && token.SnapshotRun(*(c.snapshot)) == c.run))
}

// These internal reconstruction functions preserve even partial scan positions.
// They do not validate or issue a retained lease, contact a source, or authorize.
// Partial scans pass nil snapshot; the query owner finishes the returned value
// with BindQueryContinuation. Reconstruction itself preserves pointer values.
func QueryContinuationFromParts(owner values.FactAuthorityRef, scope, run [32]byte, query RegionQuery, actor values.PrincipalRef, cut token.CoreReadCut, last values.ElementID, mode QueryReadMode, root values.ElementID, depth uint64, snapshot *token.CoreReadSnapshot) *QueryContinuation {
	return &QueryContinuation{owner, scope, run, query, actor, cut, last, mode, root, depth, snapshot}
}

// BindQueryContinuation completes only the bindings formerly assigned by the
// local query owner. The page snapshot and continuation keep separate values.
func BindQueryContinuation(position QueryContinuation, owner values.FactAuthorityRef, query RegionQuery, actor values.PrincipalRef, run [32]byte, mode QueryReadMode, root values.ElementID, depth uint64, snapshot *token.CoreReadSnapshot) QueryContinuation {
	position.owner, position.query, position.actor = owner, query, actor
	position.run, position.mode, position.root, position.depth = run, mode, root, depth
	if snapshot != nil {
		copy := *snapshot
		position.snapshot = &copy
	}
	return position
}
func ConnectedQueryContinuationFromParts(cut token.ConnectedCut, query PathRegionQuery, last values.ElementID) *ConnectedQueryContinuation {
	// The issuing owner supplies its already-cloned canonical request.
	return &ConnectedQueryContinuation{cut, query, last}
}
func ConnectedDirectContinuationFromParts(cut token.ConnectedCut, path Path, last values.ElementID) *ConnectedDirectContinuation {
	// The issuing owner supplies its already-cloned canonical path.
	return &ConnectedDirectContinuation{cut, path, last}
}
func QueryOwner(c QueryContinuation) values.FactAuthorityRef              { return c.owner }
func QueryScope(c QueryContinuation) [32]byte                             { return c.scope }
func QueryRun(c QueryContinuation) [32]byte                               { return c.run }
func QueryQuery(c QueryContinuation) RegionQuery                          { return c.query }
func QueryActor(c QueryContinuation) values.PrincipalRef                  { return c.actor }
func QueryCut(c QueryContinuation) token.CoreReadCut                      { return c.cut }
func QueryLast(c QueryContinuation) values.ElementID                      { return c.last }
func QueryMode(c QueryContinuation) QueryReadMode                         { return c.mode }
func QueryRoot(c QueryContinuation) values.ElementID                      { return c.root }
func QueryDepth(c QueryContinuation) uint64                               { return c.depth }
func QuerySnapshot(c QueryContinuation) *token.CoreReadSnapshot           { return c.snapshot }
func ConnectedQueryCut(c ConnectedQueryContinuation) token.ConnectedCut   { return c.cut }
func ConnectedQuery(c ConnectedQueryContinuation) PathRegionQuery         { return c.query }
func ConnectedQueryLast(c ConnectedQueryContinuation) values.ElementID    { return c.last }
func ConnectedDirectCut(c ConnectedDirectContinuation) token.ConnectedCut { return c.cut }
func ConnectedDirectPath(c ConnectedDirectContinuation) Path              { return c.path }
func ConnectedDirectLast(c ConnectedDirectContinuation) values.ElementID  { return c.last }
