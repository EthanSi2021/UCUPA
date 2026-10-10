package connected

import (
	"errors"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	facetfact "ucupa/ucupa-core/internal/facet/fact"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	"ucupa/ucupa-core/internal/query/exchange"
	"ucupa/ucupa-core/internal/token"
)

func validPeerReplyFixture() (connectedSourcePlan, exchange.RequestData, exchange.ReplyData) {
	authority := values.FactAuthorityRef("00000000-0000-4000-8000-000000009791")
	cut := token.FromParts([32]byte{1}, 7)
	run := [32]byte{2}
	facts := exchange.Facts{
		Present:     true,
		Authority:   authority,
		Scope:       token.Scope(cut),
		CutSerial:   token.Serial(cut),
		Elements:    elementfact.New().View,
		Attachments: facetfact.New().View,
		Mount:       mountfact.New().View,
		Cut:         cut,
		Run:         run,
	}
	return connectedSourcePlan{authority: authority}, exchange.RequestData{Action: exchange.Probe}, exchange.ReplyData{
		Authority: authority,
		Facts:     facts,
		Lease:     exchange.LeaseID{Run: run, ID: 1},
		Mode:      contract.QueryCurrentVerified,
	}
}

func TestConnectedPeerRejectsFactsScopeAndSerialMismatch(t *testing.T) {
	plan, request, validReply := validPeerReplyFixture()
	if err := validateConnectedPeerReply(plan, request, validReply); err != nil {
		t.Fatalf("valid finite facts reply rejected: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(*exchange.Facts)
	}{
		{
			name:   "absent",
			mutate: func(bad *exchange.Facts) { bad.Present = false },
		},
		{
			name: "scope",
			mutate: func(bad *exchange.Facts) {
				bad.Scope[0] ^= 0xff
			},
		},
		{
			name: "cut serial",
			mutate: func(bad *exchange.Facts) {
				bad.CutSerial++
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bad := validReply
			tc.mutate(&bad.Facts)
			if err := validateConnectedPeerReply(plan, request, bad); !errors.Is(err, contract.ErrUnavailable) {
				t.Fatalf("inconsistent %s accepted: %v", tc.name, err)
			}
		})
	}
}
