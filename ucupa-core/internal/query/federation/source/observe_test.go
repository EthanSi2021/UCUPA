package source

import (
	federationcontract "ucupa/ucupa-core/contract/federation"

	"context"
	"errors"
	"reflect"
	"testing"

	"ucupa/shared/values"
)

func TestFederationObserveSourceSurvivesCurrentWrites(t *testing.T) {
	f := newSourceFixture(t, false)
	c, caller := f.ops, f.caller
	part := f.open(t, "query", true, 0)
	// No initial hits still requires this source's stream. A write between cut
	// capture and Begin must invalidate that same current read basis while the
	// prepared stream remains valid and observes the write.
	f.create(t, 93901, 0)
	if err := c.CheckFederationReadPart(context.Background(), caller, c.AuthorityRef(), part.Basis, false); !errors.Is(err, values.PublicInvalidContext) {
		t.Fatal("current read remained valid after observed write", err)
	}
	begin, err := c.ObserveFederationReadPart(context.Background(), caller, c.AuthorityRef(), part.Basis, federationcontract.FederationObserveRequest{Operation: "begin"})
	if err != nil || !begin.Cursor.Valid() {
		t.Fatal(begin, err)
	}
	request := federationcontract.FederationObserveRequest{Operation: "poll", Cursor: &begin.Cursor, Limit: 1}
	page, err := c.ObserveFederationReadPart(context.Background(), caller, c.AuthorityRef(), part.Basis, request)
	if err != nil || len(page.Groups) != 1 || page.Groups[0].ID.Owner != c.AuthorityRef() || page.Groups[0].Basis.Owner != c.AuthorityRef() {
		t.Fatal(page, err)
	}
	repeat, err := c.ObserveFederationReadPart(context.Background(), caller, c.AuthorityRef(), part.Basis, request)
	if err != nil || !reflect.DeepEqual(page, repeat) {
		t.Fatal("unstable retry", repeat, err)
	}
	page.Groups[0].Categories[0] = "corrupted-client-copy"
	repeat, err = c.ObserveFederationReadPart(context.Background(), caller, c.AuthorityRef(), part.Basis, request)
	if err != nil || repeat.Groups[0].Categories[0] != "spatial" {
		t.Fatal("source aliases returned categories", repeat, err)
	}
	request.Cursor = &repeat.Cursor
	end, err := c.ObserveFederationReadPart(context.Background(), caller, c.AuthorityRef(), part.Basis, request)
	if err != nil || len(end.Groups) != 0 {
		t.Fatal(end, err)
	}
	if err := c.ReleaseFederationReadPart(context.Background(), caller, c.AuthorityRef(), part.Basis); err != nil {
		t.Fatal(err)
	}
	// Exact registry byte/context refunds are tested at that resource owner.
	// This real source call must retire both its receipt and the exposed cursor.
	if _, err := c.ObserveFederationReadPart(context.Background(), caller, c.AuthorityRef(), part.Basis, request); !errors.Is(err, values.PublicInvalidContext) {
		t.Fatal("observation retained after source release", err)
	}
}

func TestFederationObserveExactReadRejectsUnboundAndUnrelatedChanges(t *testing.T) {
	// Preserves the former root PreparedExactRead case without reimplementing
	// the shared observation state machine or reaching into Registry internals.
	f := newSourceFixture(t, false)
	observed := f.createUnder(t, 40, f.root, 0)
	part := f.open(t, "read", true, 0)
	ctx := context.Background()
	owner := f.ops.AuthorityRef()
	if _, err := f.ops.ObserveFederationReadPart(ctx, f.caller, owner, part.Basis, federationcontract.FederationObserveRequest{Operation: "begin"}); !errors.Is(err, values.PublicInvalidContext) {
		t.Fatal("unbound exact observation exposed", err)
	}
	if _, err := f.ops.ProjectFederationReadPart(ctx, f.caller, owner, part.Basis, federationcontract.FederationProjectionRequest{Kind: "read", Element: observed}, nil, 1); err != nil {
		t.Fatal(err)
	}
	f.createUnder(t, 41, f.root, 0)
	begin, err := f.ops.ObserveFederationReadPart(ctx, f.caller, owner, part.Basis, federationcontract.FederationObserveRequest{Operation: "begin"})
	if err != nil || !begin.Cursor.Valid() {
		t.Fatal(begin, err)
	}
	page, err := f.ops.ObserveFederationReadPart(ctx, f.caller, owner, part.Basis, federationcontract.FederationObserveRequest{Operation: "poll", Cursor: &begin.Cursor, Limit: 1})
	if err != nil || len(page.Groups) != 0 {
		t.Fatal("exact read disclosed unrelated change", page, err)
	}
	if err := f.ops.ReleaseFederationReadPart(ctx, f.caller, owner, part.Basis); err != nil {
		t.Fatal(err)
	}
}

func TestFederationObserveActivePathCannotExpandOrLeak(t *testing.T) {
	// Preserves the former root PreparedPath case: Begin freezes the selection,
	// and a rejected expansion cannot partially admit the other branch.
	f := newSourceFixture(t, false)
	a := f.createAnchor(t, 50, f.root, 0, 10)
	b := f.createAnchor(t, 51, f.root, 0, 10)
	part := f.open(t, "read", true, 0)
	ctx := context.Background()
	owner := f.ops.AuthorityRef()
	path := federationcontract.FederationProjectionRequest{Kind: "path", Path: &federationcontract.FederationPathRequest{Origin: f.root, Down: []values.ElementID{a}}}
	if _, err := f.ops.ProjectFederationReadPart(ctx, f.caller, owner, part.Basis, path, nil, 1); err != nil {
		t.Fatal(err)
	}
	begin, err := f.ops.ObserveFederationReadPart(ctx, f.caller, owner, part.Basis, federationcontract.FederationObserveRequest{Operation: "begin"})
	if err != nil || !begin.Cursor.Valid() {
		t.Fatal(begin, err)
	}
	path.Path.Down = []values.ElementID{b}
	if _, err := f.ops.ProjectFederationReadPart(ctx, f.caller, owner, part.Basis, path, nil, 1); !errors.Is(err, values.PublicInvalidContext) {
		t.Fatal("active path expanded", err)
	}
	f.createUnder(t, 52, b, 0)
	page, err := f.ops.ObserveFederationReadPart(ctx, f.caller, owner, part.Basis, federationcontract.FederationObserveRequest{Operation: "poll", Cursor: &begin.Cursor, Limit: 1})
	if err != nil || len(page.Groups) != 0 {
		t.Fatal("rejected path expansion leaked change", page, err)
	}
	if err := f.ops.ReleaseFederationReadPart(ctx, f.caller, owner, part.Basis); err != nil {
		t.Fatal(err)
	}
}

func TestFederationObserveSourceBindingAndClosedShape(t *testing.T) {
	for _, name := range []string{"actor", "caller", "not prepared", "other cursor", "begin cursor", "begin limit", "poll missing", "poll zero", "poll large", "unknown", "cancelled", "authorization"} {
		t.Run(name, func(t *testing.T) {
			f := newSourceFixture(t, false)
			c, caller := f.ops, f.caller
			auth := f.auth
			part := f.open(t, "query", name != "not prepared", 0)
			request := federationcontract.FederationObserveRequest{Operation: "begin"}
			source := c.AuthorityRef()
			ctx := context.Background()
			want := error(values.PublicInvalid)
			switch name {
			case "actor":
				other := newSourceFixture(t, false)
				source = other.ops.AuthorityRef()
				want = values.PublicDenied
			case "caller":
				caller.Establishment[0] = 1
				want = values.PublicDenied
			case "not prepared":
				want = values.PublicInvalidContext
			case "other cursor":
				other := f.open(t, "query", true, 0)
				begin, err := c.ObserveFederationReadPart(ctx, caller, source, other.Basis, request)
				if err != nil {
					t.Fatal(err)
				}
				request = federationcontract.FederationObserveRequest{Operation: "poll", Cursor: &begin.Cursor, Limit: 1}
				want = values.PublicInvalidContext
			case "begin cursor":
				request.Cursor = &part.Basis
			case "begin limit":
				request.Limit = 1
			case "poll missing":
				request.Operation = "poll"
				request.Limit = 1
			case "poll zero":
				request.Operation = "poll"
				request.Cursor = part.Preparation
			case "poll large":
				request.Operation = "poll"
				request.Cursor = part.Preparation
				request.Limit = 257
			case "unknown":
				request.Operation = "read"
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
				want = values.PublicUnavailable
			case "authorization":
				auth.epoch++
				want = values.PublicGap
			}
			if _, err := c.ObserveFederationReadPart(ctx, caller, source, part.Basis, request); !errors.Is(err, want) {
				t.Fatal("unexpected rejection", err, want)
			}
		})
	}
}
