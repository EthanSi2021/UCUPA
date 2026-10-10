package request

import (
	"slices"

	"ucupa/shared/values"
	"ucupa/ucupa-core/internal/geometry"
)

func CanonicalRegionQuery(query RegionQuery) (RegionQuery, bool) {
	if !query.StartAnchorID.Valid() || query.UpDepth < -1 || !query.BasisAnchorID.Valid() ||
		!geometry.Finite(query.Sphere.Center.X) || !geometry.Finite(query.Sphere.Center.Y) || !geometry.Finite(query.Sphere.Center.Z) ||
		!geometry.Finite(query.Sphere.Radius) || query.Sphere.Radius < 0 ||
		(query.Selector != QueryAnchors && query.Selector != QueryEntities && query.Selector != QueryBoth) {
		return RegionQuery{}, false
	}
	query.Sphere.Center.X = geometry.CanonicalZero(query.Sphere.Center.X)
	query.Sphere.Center.Y = geometry.CanonicalZero(query.Sphere.Center.Y)
	query.Sphere.Center.Z = geometry.CanonicalZero(query.Sphere.Center.Z)
	query.Sphere.Radius = geometry.CanonicalZero(query.Sphere.Radius)
	return query, true
}

func ValidPath(path Path) bool {
	if !path.OriginAnchorID.Valid() {
		return false
	}
	for _, id := range path.Down {
		if !id.Valid() {
			return false
		}
	}
	return true
}

func ClonePath(path Path) Path {
	path.Down = append([]values.ElementID(nil), path.Down...)
	return path
}

func EqualPath(a, b Path) bool {
	return a.OriginAnchorID == b.OriginAnchorID && a.Up == b.Up && slices.Equal(a.Down, b.Down)
}

func CloneQuery(query PathRegionQuery) PathRegionQuery {
	query.StartPath, query.BasisPath = ClonePath(query.StartPath), ClonePath(query.BasisPath)
	return query
}

func EqualQuery(a, b PathRegionQuery) bool {
	return EqualPath(a.StartPath, b.StartPath) && EqualPath(a.BasisPath, b.BasisPath) &&
		a.UpDepth == b.UpDepth && a.Sphere == b.Sphere && a.Selector == b.Selector
}
