package main

import (
	"fmt"
	"go/format"
	"path"
	"strings"
)

type sourceSize struct{ files, lines int }

// A package is the direct directory, not the whole owner or its descendants.
// Production, tests and registered generated bindings are counted separately.
type packageSize struct{ production, tests, generated sourceSize }

func countPackageSource(packages map[string]packageSize, file string, data []byte) {
	if formatted, err := format.Source(data); err == nil {
		data = formatted
	}
	dir := path.Dir(file)
	size := packages[dir]
	part := &size.production
	if generatedSource(file) {
		part = &size.generated
	} else if strings.HasSuffix(file, "_test.go") {
		part = &size.tests
	}
	part.files++
	part.lines += physicalLines(data)
	packages[dir] = size
}

type packageAllowance struct {
	maximum sourceSize
	reason  string
}

// These reviewed first-slice counts are ceilings, not blanket exemptions.
// The token slice added 47 production / 15 test lines relative to P6. The
// Store byte slice adds five imports to existing root tests. The streaming
// transaction slice adds five lines so the format-source test scans the
// newly owning internal/store directory; no root file is added. Acceptance adds
// five token-contract imports, 35 spatial-contract import/grouping lines and
// nine Facet shared-contract import/grouping lines. The Core Element public
// value move adds five actual contract imports; outbound federation adds one
// actual connection import. These six lines do not add acceptance behavior.
// Core Facet/read contract migration adds six net import/grouping lines (five
// contract imports, two now-unused root imports removed, three group separators).
// Mount contract migration adds five more net lines: four contract imports,
// two obsolete root imports removed, and three import-group separators.
// Query contract migration adds seven net lines: four contract imports,
// two obsolete root imports removed, and five import-group separators.
// Outbound MountConnections adds four acceptance imports and removes two
// obsolete root imports, a net two lines without new acceptance behavior.
// Public relationship values add two contract imports to the same acceptance
// scenarios; the declared API and all existing assertions remain unchanged.
// No acceptance scenario, assertion or fixture is added by these moves.
// The finite public-retry lookup adds seven disclosure/cut cases to the existing
// durability test. The aggregate and Store have now moved into commit; the root
// white-box scenarios still need their own responsibility migration, while the
// pure Mount codec test already follows commit. Root allowances ratchet down to
// 31 production / 4747 lines and 47 test / 9138 lines after local/connected
// query, Element entry, Facet/shared admission and complete Mount migration.
// The five original Element Store failure groups now follow commit's real
// operations test; 35 public Element groups follow tests/element. Only fixtures
// still consumed by unmigrated root tests remain, with no dropped assertions.
// Read-resource persistence follows tests/query through real public entry points;
// only that exact test reads a quiescent file copy, never the private live Store.
// Acceptance adds 15 explicit same-owner FacetControl assembly statements and
// one composition import; all original scenarios and assertions remain unchanged.
// Mount control adds 15 explicit same-owner assembly statements, removes three
// obsolete root imports and adds two net crash-fixture port-binding lines.
// Those 14 net acceptance lines preserve the existing real crash/RPC scenarios.
// The connected contract adds one net acceptance import for the same error and
// opaque values; the source identity checks, fixtures and assertions are unchanged.
// Pure record projection moves out of root; its seven existing white-box
// consumers add only the actual internal value import, without new scenarios.
// The connected reply validator test follows its actual private owner, removing
// 49 root lines without dropping any valid/invalid-reply assertion.
// Federation's 17 service values and two ports move out of root. Exact type
// imports, private traversal access and longer-name gofmt wrapping add 41 net
// root test lines and eight acceptance lines without changing test semantics.
// Finite Federation projection and the unique transcript now leave root;
// the original traversal test follows that leaf. The remaining public parent
// evidence test uses the actual finite projection to compare its raw digest.
// Twelve original public Connected scenarios follow real Core/contract entry
// points in tests/connected; two filter and one world white-box tests follow
// their implementations. Remaining root private fixtures are not compatibility.
// The nested-source public scenario follows tests/connected; Capture witness
// failure follows query's external test with a test-only resource-count bridge.
// Remaining Connected concurrency/resource scenarios follow their real owners;
// the same-cut attachment test follows public entry points. The now-unused root
// connected fixtures are deleted, not retained as a second assembly route.
// No released headroom
// can be reused for new root behavior; the next file or line requires review.
// Retire Core's two entries when its root migration is complete. The
// acceptance entry documents the existing cross-owner integration corpus; it
// does not authorize unlimited additions or moving private owner tests there.
// The four fixed Facet service methods now belong to actual facet.Operations,
// sharing the original Admission and Registry. Both old root files are gone;
// the remaining public declaration assertions remove those four methods and
// the two permission constants moved to contract. No root headroom is reused.
// Shared observation and the complete owner-local federation source now leave
// root. Source receives the same Admission/Registry/query/commit components;
// the collector's local connection directly targets it. Root retains only the
// remaining public coordination and assembly. Two original source tests follow
// source and its resource owner; five acceptance lines explicitly bind the
// same source port on create/reopen. No released root headroom is reusable.
var productionAllowances = map[string]packageAllowance{
	"ucupa-realm":                    {sourceSize{0, 0}, "navigation-only root; no implementation or compatibility alias"},
	"cmd/verify":                     {sourceSize{29, 4576}, "4 exact carrier network registrations (transition carrier ports, fence codec, transitionconn orchestration, admission carrier approval), 2 three-party transfer test registrations (fixture + suite), 1 source.go import entry, 1 admission file allowlist entry, eight-RPC embed and carrierApprovals field boundaries, 7 facet-replacement registrations (contract, commit x2, facet operation x2, fact, codec), 2 Core instance consumer entries and 1 inbound replacement contract edge, 1 re-acceptance composition consumer entry and 1 re-acceptance commit-test dependency registration, 1 re-acceptance network acceptance consumer entry, 1 recovery composition consumer entry and 1 recovery network acceptance consumer entry, 1 retirement composition consumer entry, 1 retirement commit-test source registration and 1 retirement Core facet test registration, 1 federation source observe.go terminal-evidence contract edge, 1 registry terminal-lifecycle test transfer-fact edge; no wildcard; two exact H3 test-fixture instance-consumer entries, no package-wide permission; S2P2 registers the two reviewed Core Realm contact control files, their two local scenario tests, their exact dependency edges and the independent-Realm composition consumer alongside the existing contact Store files; S2P3a adds one generated Realm protocol binding registration line and extends the connection-bound and protocol-conversion adapter boundaries plus the location approvals to the Realm bindings, transport and SDK directories, without a new file or wildcard; S2P3b registers the reviewed access permit source file, the access dependency edge of the source observe boundary, the six transfer files' access dependency edges and the two network acceptance consumer test entries; one exact mount control acceptance consumer entry reopens the exclusive original Core Store against the real Realm product; no wildcard; Realm normalization adds one per-role dependency/location policy and two zero-root entries, migrates the actual consumers and prohibits old root imports; exact current size without spare headroom"},
	"ucupa-core/internal/commit":     {sourceSize{66, 9387}, "418 lines record, replay and serialize the facet-replacement control milestones beside the existing transfer record path under the original owner lock; no new state or transaction; 10 more lines make Element deletion refuse an open re-acceptance obligation and state its earlier transfer refusal; 33 more lines expose one departed element's durable successor statement under the existing owner lock, returning proven absence as a value; 33 more lines carry the facet's final disposition through the release decision and its journal replay, block Element deletion while a detached relationship still needs its release, expose the public release command kind and rotate the journal marker because a P7.3d journal may hold a committed deletion of a detached lineage that the release protocol recomputes as refused; 19 more lines build each own committed departure's terminal successor evidence and its pending own-decision set beside its inactive identity inside the same owner visibility; S2P2 adds the owner-local Realm contact history and canonical Store projection under the original commit lock and transaction, with a 1 MiB canonical contact history cap and retained original-result lookup, plus required contact metadata at load, the initial-transaction contact binding and loaded-snapshot contact validation"},
	"ucupa-core":                     {sourceSize{0, 0}, "all production responsibilities now have actual owners; no root production code or compatibility facade may return"},
	"ucupa-facet/internal/namespace": {sourceSize{30, 5507}, "S2P2 adds the owner-local Realm contact decision/byte codec, the settled current-contact authorizer and the Inspect/Update/Recover contact control beside the existing Facet namespace Store; both histories share its original commit gate, without a second fact owner or transport dependency, capped at 1 MiB canonical contact bytes with original-result lookup; the S2P2 acceptance round privatizes the bare contact commit behind the authenticated maintenance path, exposes the current-contact read and threads the captured contact approval through every final local read and commit boundary so a superseded generation fails closed as unavailable; 28 net lines also guard retained receipt/conflict and existing import-state returns at their first owner lock, removing the now-redundant acceptance check in that same lock; no new file, owner, protocol or replay rule; the S2P3b fix round threads the original request context through the same contact authorizer, writer and public read/mutation/observe/acceptance boundaries and folds one duplicated write-gate deadline check, five net lines, still without a new file, owner, protocol or replay rule; S2P3c adds the request-local authenticated permit beside the same owner and splits the existing replacement source/import workflows into two cohesive files: 215 net production lines, no second registry, fact state, commit gate, wire codec or transport dependency; public content, relationship, observation, writer and replacement consumers perform network waits outside the original fact lock, recheck the original view after Core reads, and consume only current contact/request evidence at the final boundary"},
	"ucupa-facet":                    {sourceSize{0, 0}, "module root is navigation only; no production implementation or compatibility facade"},
}

var testAllowances = map[string]packageAllowance{
	"ucupa-realm":                {sourceSize{0, 0}, "tests follow policy, identity, transport and public instance scenarios"},
	"cmd/verify":                 {sourceSize{59, 11334}, "exact dependency registrations plus pure codec reverse-owner, I/O, lock and unregistered-file rejection tests; 5 lines skip Windows per-drive pseudo environment variables in the gate test; 1 more line mirrors encoding/base64 into the observe boundary stdlib set for terminal-evidence decoding; 1 more line registers the registry terminal-lifecycle test's transfer-fact visibility edge; one explicit positive check for the H3 fixture-only instance import; S2P2 adds the Core instance contact-authorizer surface lines plus the reviewed contact/maintenance location, dependency, consumer, surface and size-boundary registrations; two S2P3b completion lines register only the finite receipt cleanup method and the resource-only private contact adapter method, with no wildcard or public Core surface expansion; S2P3c adds one line to derive the inactive/unregistered-generated overflow fixture from the current exact production ceiling, preserving both rejection assertions; Realm normalization adds one boundary rejection suite and updates the old root positive to the public instance; no negative is weakened"},
	"ucupa-core":                 {sourceSize{0, 0}, "all original root regressions now follow actual owners or public scenarios; obsolete helpers removed, no root tests may return"},
	"ucupa-facet":                {sourceSize{0, 0}, "tests follow the actual namespace/access/commit responsibilities; no root test package"},
	"ucupa-core/internal/commit": {sourceSize{118, 18373}, "355 lines of replacement control milestone, serial-guard, fault and journal replay tests on the real commit owner; 145 lines prove the re-acceptance obligation survives cancellation, deletion and repeated transfer and that activation alone renews the acceptance key and continues the transfer chain; 27 more lines extend the lifecycle chain to the same-Facet recovery: after a local detach the obligation still refuses cancellation and deletion and a matching acceptance renews the key without copying the Payload; 14 more lines reopen one durable Store still marked with the exact superseded P7.3c marker and assert the open refuses it, because that older journal may hold the rejected re-propose of a locally detached Element which P7.3d recomputes as committed; 134 lines prove release retires a detached record only under its own matching final disposition, keeps the undeleted lineage blocked, frees the Element after the release and binds the disposition evidence to its Release binding in replay; 15 more lines reopen one durable journal still marked with the exact superseded P7.3d marker and assert the open refuses it, because that journal may hold the committed deletion of a detached relationship which the release protocol recomputes as refused; 78 more lines keep the RETIRED lineage transferable: the retired record crosses the same slice validation, manifest decode, owner return and restart as terminal and never becomes a fresh proposal, while an incoherent retired member and a forged retired predecessor are refused; S2P2 adds two focused contact tests for retained CAS decisions, Store reopen and forged projection rejection plus exact encoded capacity refusal and pre/postcommit fault reconciliation, binds the existing fixtures to the trusted contact assembly, proves the settled contact authorizer inside projection authorization, refuses Store opens without the durable contact record and adds the missing-contact corruption mode; the S2P3b fix round passes the original request context through the same fixture call sites, four net lines with no new scenario, owner or replay rule"},
	"acceptance":                 {sourceSize{99, 18659}, "368 lines cover one nonempty namespace move across three TLS hosts; 117 additional lines cover zero-entry replacement recovery after restart, fenced-source refusal and legal empty target reads; one more file of 399 lines proves the same-Facet re-acceptance public chain across two independent TLS Core hosts: successful rebind, wrong-identity and stale-proposal refusal, restart replay and unchanged Payload; one more file of 331 lines proves the same-Facet recovery public chain over TLS Core hosts: successful rebind after a local detach, wrong-identity and stale-proposal refusal, restart replay and unchanged Payload; one more file of 436 lines proves the content-retirement public chain across independent TLS Core and Facet hosts: the acceptance must be rebuilt publicly before disposal, a premature retire and an unreachable release are refused, the committed retire and release replay after restart and the retired namespace durably blocks its reads, writes and revival; the real two-product transition chain grew by 71 P7.4 lines across old read and observation invalidation, new target observation and write, restart retention, and the settled successor's terminal evidence held on the old observation with idempotent reread, exhausted GAP, disconnect unavailability and restart loss; one more file of 324 lines proves the same-Facet recovery and content-retirement chains over one ucupa-core product and one ucupa-facet product: public relationship building, byte-identical recovery payload, committed retire and release with exact restart replays, the stale-session release retry, the terminal seal on reads, disposal and revival, and the released element's public deletion; one more file of 354 lines proves the cross-Core re-acceptance chain over two ucupa-core products and one ucupa-facet product: the slice transfer settles under a live target approval, the attachment arrives DETACHED, the re-acceptance resolution and successor proof cross the product network, wrong-hop and stale-proposal requests are refused, and full restarts replay the settled transfer and committed acceptance with exact receipts while the payload stays byte-identical; one more file of 414 lines proves the namespace replacement chain over one ucupa-core product and two ucupa-facet products: both relationships are built over the public planes, two entries move byte for byte through the Core product's recovery control, the fenced source refuses its former reads, a reserved never-written namespace survives a full three-product restart and settles afterwards to read absence with zero types, and every settled recovery replays its exact original receipt; 6 more fixture lines seed the Core realm's inert facet-service principal and credential so the two Facet products present distinct fixed service identities to the one Core product; 17 P8 smoke lines prove Core public Entity write/read in the same independent two-product relationship and content flow; 42 P8 lines copy and hash the four stopped Stores without clone startup, then cold-revoke a Facet caller in its original Realm Store and prove product reauthentication fails; P8 also rejects missing original Core and Facet Stores at actual host startup, recovers exact Core/Facet receipts after stopping both products, exercises sequential Core and Facet writer handoff in one cell, and keeps 1000-Entity latency plus 1 MiB/256-type boundary measurements in one opt-in file; the product test launcher now gives each restart a fresh startup deadline instead of reusing the expired build deadline; P8 signal helpers use separate Windows process groups and POSIX Interrupt to verify actual orderly host exit without changing the product interface; the fixed cell measures three orderly-stop and three forced-stop original-Store reopens to the first successful Core and Facet sessions; two opt-in Linux/other fault harness files (plus fixed-cell call sites) guard a bounded isolated ext4 ENOSPC run and an independently traced in-flight Facet Store pwrite64 SIGKILL, then recover or retry the original OperationID from the original Store without partial visibility; neither fault mode runs in the ordinary verifier; eight measurement lines record the original Core and Facet OperationID recovery round-trip after the final original-Store reopen, without changing their semantics; one opt-in real-host Core product and SDK probe file plus four certificate-helper lines prove a private-LAN two-machine TLS target, wrong-key, wrong-owner and wrong-credential rejection, public write/read, and original OperationID recovery after orderly reopening the original Stores; one more opt-in file of 700 lines puts the Core product and the Facet product on two separate real private-LAN machines: the cross-host attachment is proposed, accepted, activated, written and read through the public planes, each owner denies an inert unauthorized caller, wrong TLS target, wrong expected authority and wrong credential are refused from both machines, reverse contacts repair idempotently on both sides, a stopped peer fails explicitly as unavailable without masking, and the original-Store restart keeps the content, the original receipts and the acceptance replay byte-identical; disposable credentials are transferred only over authenticated SSH and removed afterward; five opt-in H3 real-host federation files (+874 lines) split host A, host B, public calls, assertions and fixture: two independent physical hosts, A-to-B-to-C public mount and Read/List/Query pagination, original caller authorization at each owner, peer identity rejection, explicit source outage and bounded owner-API fixture seeding only while product stopped, then public original-caller Read/List/Query, peer identity rejection, explicit outage and original-Store restart without silent continuation; four further fixture lines keep finite A-side public-byte headroom so an injected peer outage is not masked by a local capacity failure; one opt-in H4 byte-only bridge file and exact H3 fixture extensions (+194 lines) prove port-scoped connection loss while the remote product stays live, bounded incomplete coverage, restoration, abrupt remote product death and original-Store recovery; finite test-only quotas cover retained leases across both faults without changing product policy; S2P2 migrates the product constructors across the existing scenarios to the trusted Realm contact assembly without adding scenario files; the S2P2 acceptance round rewires the same network fixtures to the owner-current Realm contact authentication, still without adding scenario files; seven test-only lines keep the existing HTTP/2 retry injector connection open until the client finishes, preventing Windows reset from discarding its response; assertions and SDK retry behavior are unchanged; one more file of 304 lines proves the S2P3a Realm product's foreign identity chain across one ucupa-core issuer product and one ucupa-realm receiver product: a proof bound to the admitted caller's owner establishes exactly the issuer's subject with only the seeded authorization, the online liveness check runs over the receiver's own pinned issuer connection, a misbound owner is rejected, an offline issuer is unavailable, a restarted issuer rejects the old proof and a fresh proof works again; S2P3b adds two more files: one of 789 lines proves the Core product chain over the network Realm profile through one ucupa-realm product and one ucupa-core product with real gRPC/TLS — full authorization and session derivation through the Core's mTLS service identity, Core restart recovery through the persisted contact, stale run-local handles refusing across a Realm restart, offline fail-closure, online revocation and restoration, client deadline without commit and same-id retry, wrong SPKI/Realm pins refusing startup and an unadmitted service identity serving nothing; the same file's identity section proves the P6 source-side identity chain online; the second file proves the original contact-maintenance matrix over the same products — a live current address authorizing a move to an offline candidate, no disclosure without an explicit same-Realm candidate, candidate repair of disclosure and retained decisions, the product proving the new address online at open with the original recovery semantics, a dead contact repaired through a candidate, revoked permissions denying online without contact change, and a byte-level hold barrier proving a write waiting on its online authorization never holds the fact lock while a lock-needing Close succeeds; the S2P3b acceptance-fix round extends that same maintenance file with the canceled-authorization-wait and expired-deadline-wait regressions — the barrier holds one write's authorization bytes back, the original request cancels or its deadline expires during the Realm wait, the release proves no commit through original-outcome recovery and a NOT_FOUND fact read, a fresh request through the same chain still commits, and the committed original-result recovery semantics stay intact ; the independent completion round adds 38 test lines proving 128 completed original-result recoveries and 40 read/Close cycles within one 600-second TTL against an unchanged 64-handle budget, without early release of live read evidence; one obsolete TTL-only fixture comment line is removed; 78 net lines in the existing mount control acceptance file prove real remote view survival through Resolve and Apply rejection plus destructive release after request cancellation using the exclusive original Core Store and the pinned Realm product; S2P3c adds one shared network-Realm product fixture and reuses the existing relationship, recovery-retirement and replacement product chains: 270 net test lines prove independent pinned Realm services for each Facet, original Store contact repair through an explicit same-Realm candidate, more completed authorization calls than the unchanged handle budget, Realm outage and restart fail-closure, content preservation and source-target independent namespace authorization; no copied lifecycle scenario or new product obligation; S2P3d adds four finite combination test files and reuses the existing Core-Facet relationship and mounted federation product flows: shared versus independent original Realm Stores, owner-specific service certificates, independent revocation and exact receipt recovery, startup target rejection, committed response loss through a byte-only relay, and original-requester federation with both authorization sources online; the P6 foreign request explicitly separates signed relay Actor from admitted receiving owner without changing Store semantics or adding management RPCs; no new lifecycle scenario framework or future headroom; Realm normalization adds 32 net import/grouping lines for instance and contract paths, with no new scenario or assertion"},
}

func checkPackageSizes(packages map[string]packageSize) []issue {
	var issues []issue
	for dir, size := range packages {
		issues = append(issues, checkProductionSize(dir, size.production)...)
		issues = append(issues, checkTestSize(dir, size.tests)...)
	}
	return issues
}

func checkProductionSize(dir string, size sourceSize) []issue {
	if allowance, ok := productionAllowances[dir]; ok {
		if size.files > allowance.maximum.files || size.lines > allowance.maximum.lines {
			return []issue{{path: dir, message: fmt.Sprintf(
				"production package %d files/%d lines exceeds reviewed ceiling %d/%d: %s",
				size.files, size.lines, allowance.maximum.files, allowance.maximum.lines, allowance.reason)}}
		}
		return nil
	}
	if size.files > 50 || size.lines > 10000 {
		return []issue{{path: dir, message: fmt.Sprintf(
			"production package %d files/%d lines exceeds hard limit 50 files or 10000 lines; review responsibility before expansion",
			size.files, size.lines)}}
	}
	if size.files > 25 || size.lines > 5000 {
		return []issue{{path: dir, review: true, message: fmt.Sprintf(
			"production package %d files/%d lines exceeds responsibility review threshold 25 files or 5000 lines",
			size.files, size.lines)}}
	}
	return nil
}

func checkTestSize(dir string, size sourceSize) []issue {
	if allowance, ok := testAllowances[dir]; ok {
		if size.files > allowance.maximum.files || size.lines > allowance.maximum.lines {
			return []issue{{path: dir, message: fmt.Sprintf(
				"test package %d files/%d lines exceeds reviewed ceiling %d/%d: %s",
				size.files, size.lines, allowance.maximum.files, allowance.maximum.lines, allowance.reason)}}
		}
		return nil
	}
	if size.files > 50 || size.lines > 10000 {
		return []issue{{path: dir, review: true, message: fmt.Sprintf(
			"test package %d files/%d lines requires responsibility and fixture review (50 files or 10000 lines)",
			size.files, size.lines)}}
	}
	return nil
}
