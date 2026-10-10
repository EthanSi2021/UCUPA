package fact

import (
	"fmt"
	"strings"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/geometry"
	"ucupa/ucupa-core/internal/token"
)

func validationID(n int) values.ElementID {
	return values.ElementID(fmt.Sprintf("00000000-0000-4000-8000-%012x", n))
}

func validationSphere(radius float64) contract.BoundedSphere {
	sphere, err := geometry.NewBoundedSphere(radius)
	if err != nil {
		panic(err)
	}
	return sphere
}

func validationState() (*State, [32]byte, values.ElementID) {
	var scope [32]byte
	scope[0] = 1
	root := validationID(1)
	return &State{
		View: View{
			anchors: map[values.ElementID]Anchor{root: {
				ID: root, Parent: "", LocalPose: contract.Pose{}, Range: geometry.UnboundedRootRange(),
				Revision: token.RevisionFromParts(scope, root, 1),
			}},
			entities: make(map[values.ElementID]Entity),
		},
		claimed:    map[values.ElementID]struct{}{root: {}},
		tombstones: make(map[values.ElementID]contract.Revision),
	}, scope, root
}

func TestValidateCorruption(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*State, [32]byte, values.ElementID)
		want   string
	}{
		{
			name: "missing claim",
			mutate: func(state *State, _ [32]byte, root values.ElementID) {
				delete(state.claimed, root)
			},
			want: "claim set does not match current records and tombstones",
		},
		{
			name: "duplicate representation",
			mutate: func(state *State, scope [32]byte, root values.ElementID) {
				state.entities[root] = Entity{ID: root, Parent: root, LocalPose: geometry.IdentityPose(), Range: validationSphere(1), Revision: token.RevisionFromParts(scope, root, 2)}
			},
			want: "claim set does not match current records and tombstones",
		},
		{
			name: "bad revision",
			mutate: func(state *State, scope [32]byte, root values.ElementID) {
				wrong := scope
				wrong[0] = 2
				anchor := state.anchors[root]
				anchor.Revision = token.RevisionFromParts(wrong, root, 1)
				state.anchors[root] = anchor
			},
			want: `Anchor "00000000-0000-4000-8000-000000000001" identity or Revision is invalid`,
		},
		{
			name: "unknown parent",
			mutate: func(state *State, scope [32]byte, root values.ElementID) {
				id := validationID(2)
				state.claimed[id] = struct{}{}
				state.anchors[id] = Anchor{ID: id, Parent: validationID(99), LocalPose: geometry.IdentityPose(), Range: geometry.BoundedRange(validationSphere(1)), Revision: token.RevisionFromParts(scope, id, 2)}
			},
			want: `Anchor "00000000-0000-4000-8000-000000000002" is not contained by its parent`,
		},
		{
			name: "cycle",
			mutate: func(state *State, scope [32]byte, root values.ElementID) {
				a, b := validationID(2), validationID(3)
				rangeValue := geometry.BoundedRange(validationSphere(1))
				state.claimed[a], state.claimed[b] = struct{}{}, struct{}{}
				state.anchors[a] = Anchor{ID: a, Parent: b, LocalPose: geometry.IdentityPose(), Range: rangeValue, Revision: token.RevisionFromParts(scope, a, 2)}
				state.anchors[b] = Anchor{ID: b, Parent: a, LocalPose: geometry.IdentityPose(), Range: rangeValue, Revision: token.RevisionFromParts(scope, b, 3)}
			},
			want: "Anchor cycle includes",
		},
		{
			name: "external unadmitted",
			mutate: func(state *State, scope [32]byte, _ values.ElementID) {
				external := validationID(90)
				child := validationID(2)
				state.claimed = map[values.ElementID]struct{}{child: {}}
				state.anchors = map[values.ElementID]Anchor{child: {
					ID: child, Parent: external, LocalPose: geometry.IdentityPose(), Range: geometry.BoundedRange(validationSphere(1)),
					Revision: token.RevisionFromParts(scope, child, 1),
				}}
				state.entities = make(map[values.ElementID]Entity)
				state.tombstones = make(map[values.ElementID]contract.Revision)
			},
			want: "external mount content exists before initial admission",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			state, scope, root := validationState()
			tc.mutate(state, scope, root)
			external := ExternalParent{}
			if tc.name == "external unadmitted" {
				external = ExternalParent{Present: true, ID: validationID(90), Range: validationSphere(10), Admitted: false}
			}
			revisionSerial := uint64(1)
			if tc.name == "cycle" || tc.name == "unknown parent" || tc.name == "duplicate representation" {
				revisionSerial = 3
			}
			err := state.Validate(scope, revisionSerial, external)
			if err == nil {
				t.Fatal("Validate returned nil for corrupted state")
			}
			if tc.want != "" && !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate error = %q, want substring %q", err, tc.want)
			}
			// Historical claims belong to admission; all live corruption must
			// still be rejected on each retained projection read.
			viewErr := state.View.Validate(scope, revisionSerial, external)
			if tc.name == "missing claim" {
				if viewErr != nil {
					t.Fatalf("live projection acquired history: %v", viewErr)
				}
			} else if viewErr == nil || !strings.Contains(viewErr.Error(), tc.want) {
				t.Fatalf("live projection lost validation: %v; want %q", viewErr, tc.want)
			}
		})
	}
}

func TestValidateAcceptsRoot(t *testing.T) {
	state, scope, _ := validationState()
	if err := state.Validate(scope, 1, ExternalParent{}); err != nil {
		t.Fatalf("Validate(valid root) = %v", err)
	}
	if err := state.View.Validate(scope, 1, ExternalParent{}); err != nil {
		t.Fatalf("View.Validate(valid root) = %v", err)
	}
}
