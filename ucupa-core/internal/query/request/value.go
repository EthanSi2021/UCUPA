package request

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/internal/geometry"
)

type QuerySphere struct {
	Center geometry.Vec3
	Radius float64
}
type QuerySelector uint8

const (
	QueryAnchors QuerySelector = iota + 1
	QueryEntities
	QueryBoth
)

type RegionQuery struct {
	StartAnchorID values.ElementID
	UpDepth       int64
	BasisAnchorID values.ElementID
	Sphere        QuerySphere
	Selector      QuerySelector
}

// Path follows actual parent edges, then UUID-labelled child edges, from an
// Anchor known at the receiving Core. It is not a global identity lookup.
type Path struct {
	OriginAnchorID values.ElementID
	Up             uint64
	Down           []values.ElementID
}

type PathRegionQuery struct {
	StartPath Path
	UpDepth   int64
	BasisPath Path
	Sphere    QuerySphere
	Selector  QuerySelector
}

type QueryReadMode uint8

const (
	QueryCurrentVerified QueryReadMode = iota + 1
	QueryRetained
)
