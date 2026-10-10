package query

// ResourceCountsForTest is a detached accounting projection compiled only
// into query's tests. It exposes no registry, view, handle or mutable state.
type ResourceCountsForTest struct {
	Views   int
	Handles int
	Active  uint32
	Bytes   uint64
}

func ReadResourceCountsForTest(core *Operations) ResourceCountsForTest {
	if core == nil || core.reads == nil {
		return ResourceCountsForTest{}
	}
	core.reads.mu.Lock()
	defer core.reads.mu.Unlock()
	return ResourceCountsForTest{
		Views: len(core.reads.views), Handles: len(core.reads.entries),
		Active: core.reads.active, Bytes: core.reads.bytes,
	}
}
