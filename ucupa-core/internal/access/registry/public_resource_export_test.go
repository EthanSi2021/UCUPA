package registry

import "time"

// SetPublicResourceScenarioForTest preserves only the three fixed faults from
// the former root resource tests. It cannot set arbitrary limits, replace an
// owner, return mutable state, or be called from a production build.
func SetPublicResourceScenarioForTest(r *Registry, scenario string) bool {
	r.locker.Lock()
	defer r.locker.Unlock()
	switch scenario {
	case "queue-one":
		r.limits.Groups = 1
	case "working-memory":
		r.limits.Bytes = 1 << 19
	case "expired":
		r.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	default:
		return false
	}
	return true
}

// PublicResourceCountsForTest returns detached accounting values under the
// existing owner lock; it exposes neither the entries nor their contents.
func PublicResourceCountsForTest(r *Registry) (int, uint64) {
	r.locker.Lock()
	defer r.locker.Unlock()
	return len(r.entries), r.bytes
}
