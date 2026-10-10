package main

import (
	"bytes"
	"testing"
)

const expectedSummary = `UCUPA: PASS
cluster: cluster-p/realm-p owner-local
element: 00000000-0000-4000-8000-000000003004 root-relative=(3,0,0) cross-branch query+same-cut read; identity+facet preserved; deleted subject hidden
connected: two durable Core stores; upward discovery, snapshot pagination+exact read across writes; latest state recovered
public: canonical entry+create+discovery+read; direct Facet and gapless prepared observation
`

func TestRunPrintsDeterministicScenarioSummary(t *testing.T) {
	var firstOut bytes.Buffer
	if err := run(&firstOut); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if got := firstOut.String(); got != expectedSummary {
		t.Fatalf("first summary:\n%s\nwant:\n%s", got, expectedSummary)
	}

	var secondOut bytes.Buffer
	if err := run(&secondOut); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if secondOut.String() != firstOut.String() {
		t.Fatalf("second run differed: %q", secondOut.String())
	}
}
