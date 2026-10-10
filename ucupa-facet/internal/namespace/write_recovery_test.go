package namespace

import (
	"context"
	"os"
	"os/exec"
	"testing"

	"ucupa/ucupa-facet/contract"
)

func TestDurableWritesSurviveProcessExit(t *testing.T) {
	x := newFixture(t, true)
	x.put(t, "seed", "old", contract.ExpectAbsent())
	if err := Close(x.f); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestFacetProcessWriter$")
	cmd.Env = append(os.Environ(), "UCUPA_FACET_PROCESS_PATH="+x.path)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("child: %s %v", output, err)
	}
	var err error
	x.f, err = OpenDurable(x.auth, x.source, x.path, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	r, err := x.f.Get(context.Background(), testActor, x.key, x.typ)
	if err != nil || fixtureText(r.Payload) != "child" {
		t.Fatalf("process recovery: %+v %v", r, err)
	}
}
func TestFacetProcessWriter(t *testing.T) {
	path := os.Getenv("UCUPA_FACET_PROCESS_PATH")
	if path == "" {
		t.Skip("child only")
	}
	source := &testSource{address: "local:core"}
	f, err := OpenDurable(&testAuthorization{}, source, path, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	k := testKey(f.AuthorityRef(), testCore, testElement, 2)
	source.current = testProof(k, contract.CoreActive, 3)
	typ := fixtureType("example")
	old, err := f.Get(context.Background(), testActor, k, typ)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.Put(context.Background(), testActor, k, typ, fixturePayload("child"), contract.ExpectRevision(old.Revision), "child")
	if err != nil {
		t.Fatal(err)
	}
	// Deliberately bypass Close to exercise committed persistence after process loss.
	os.Exit(0)
}
