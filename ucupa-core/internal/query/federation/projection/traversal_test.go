package projection

import (
	"context"
	"reflect"
	"testing"
	"ucupa/shared/values"
	elementfact "ucupa/ucupa-core/internal/element/fact"
)

func TestFederationTraversalRequiresEveryParentAndCompletePages(t *testing.T) {
	root, child := projectionID(96000), projectionID(96001)
	projection := func(count int) elementfact.State {
		builder := elementfact.NewProjection()
		builder.AddAnchor(root, elementfact.Anchor{ID: root})
		builder.AddAnchor(child, elementfact.Anchor{ID: child, Parent: root})
		for i := 0; i < count; i++ {
			id := projectionID(uint64(96100 + i))
			builder.AddEntity(id, elementfact.Entity{ID: id, Parent: root})
		}
		return builder.Build()
	}
	s := &Projection{elements: projection(257).View}
	closure := map[values.ElementID]struct{}{root: {}, child: {}}
	if _, err := s.traversal(context.Background(), root, nil); err != values.PublicUnavailable {
		t.Fatal("missing scope accepted", err)
	}
	w, err := s.traversal(context.Background(), root, closure)
	if err != nil {
		t.Fatal(err)
	}
	first := w.Sum()
	s.elements = projection(256).View // The same projection without the last-page child.
	w, err = s.traversal(context.Background(), root, closure)
	if err != nil || reflect.DeepEqual(first, w.Sum()) {
		t.Fatal("last-page child not bound", err)
	}
	delete(closure, child)
	if _, err := s.traversal(context.Background(), root, closure); err != values.PublicUnavailable {
		t.Fatal("unvisited child anchor accepted", err)
	}
	closure[child] = struct{}{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.traversal(ctx, root, closure); err != values.PublicUnavailable {
		t.Fatal("cancelled traversal completed", err)
	}
}
