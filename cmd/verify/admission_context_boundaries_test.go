package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validAdmissionEnvelope = `package entry

import accesspkg "ucupa/ucupa-core/internal/access"

func (core *Operations) publicCall(ctx any) any {
	if err := core.admission.Acquire(); err != nil {
		return nil
	}
	defer core.admission.Release()
	defer core.access.DrainRetired()
	ctx = accesspkg.WithAdmission(ctx, core.admission)
	return ctx
}
`

func TestWithAdmissionProductionShapeIsActuallyCheckedBySourceScan(t *testing.T) {
	if issues := checkSource("ucupa-core/internal/access/entry/request.go", []byte(validAdmissionEnvelope), "ucupa", testStdlib()); containsAdmissionIssue(issues) {
		t.Fatalf("valid public envelope rejected: %v", issues)
	}
	for _, tc := range []struct {
		name, path, source string
	}{
		{"old-root-location", "ucupa-core/public_context.go", validAdmissionEnvelope},
		{"cross-file", "ucupa-core/internal/access/entry/space.go", validAdmissionEnvelope},
		{"wrong-receiver", "ucupa-core/internal/access/entry/request.go", strings.Replace(validAdmissionEnvelope, "accesspkg.WithAdmission(ctx, core.admission)", "other.WithAdmission(ctx, core.admission)", 1)},
		{"different-admission", "ucupa-core/internal/access/entry/request.go", strings.Replace(validAdmissionEnvelope, "accesspkg.WithAdmission(ctx, core.admission)", "accesspkg.WithAdmission(ctx, other.admission)", 1)},
		{"ignored-acquire-error", "ucupa-core/internal/access/entry/request.go", strings.Replace(validAdmissionEnvelope, "if err := core.admission.Acquire(); err != nil {\n\t\treturn nil\n\t}", "core.admission.Acquire()", 1)},
		{"failure-does-not-exit", "ucupa-core/internal/access/entry/request.go", strings.Replace(validAdmissionEnvelope, "if err := core.admission.Acquire(); err != nil {\n\t\treturn nil\n\t}", "if err := core.admission.Acquire(); err != nil {\n\t\t_ = err\n\t}", 1)},
		{"immediate-release", "ucupa-core/internal/access/entry/request.go", strings.Replace(validAdmissionEnvelope, "defer core.admission.Release()", "core.admission.Release()", 1)},
		{"reassigned-admission", "ucupa-core/internal/access/entry/request.go", strings.Replace(validAdmissionEnvelope, "defer core.admission.Release()", "core.admission = other.admission\n\tdefer core.admission.Release()", 1)},
		{"reassigned-context", "ucupa-core/internal/access/entry/request.go", strings.Replace(validAdmissionEnvelope, "defer core.admission.Release()", "defer core.admission.Release()\n\tctx = other", 1)},
		{"unexpected-defer", "ucupa-core/internal/access/entry/request.go", strings.Replace(validAdmissionEnvelope, "defer core.access.DrainRetired()", "defer other.Release()", 1)},
		{"nested-closure", "ucupa-core/internal/access/entry/request.go", strings.Replace(validAdmissionEnvelope, "ctx = accesspkg.WithAdmission(ctx, core.admission)\n\treturn ctx", "func() {\n\t\tctx = accesspkg.WithAdmission(ctx, core.admission)\n\t}()\n\treturn ctx", 1)},
		{"function-value", "ucupa-core/internal/access/entry/request.go", strings.Replace(validAdmissionEnvelope, "ctx = accesspkg.WithAdmission(ctx, core.admission)", "_ = accesspkg.WithAdmission", 1)},
		{"helper-function", "ucupa-core/internal/access/entry/request.go", strings.Replace(validAdmissionEnvelope, "func (core *Operations) publicCall(ctx any) any {", "func (core *Operations) helper(ctx any) any {", 1)},
		{"non-entry-receiver", "ucupa-core/internal/access/entry/request.go", strings.Replace(validAdmissionEnvelope, "func (core *Operations) publicCall(ctx any) any {", "func (core *Other) publicCall(ctx any) any {", 1)},
		{"value-entry-receiver", "ucupa-core/internal/access/entry/request.go", strings.Replace(validAdmissionEnvelope, "func (core *Operations) publicCall(ctx any) any {", "func (core Operations) publicCall(ctx any) any {", 1)},
		{"wrong-receiver-name", "ucupa-core/internal/access/entry/request.go", strings.Replace(validAdmissionEnvelope, "func (core *Operations) publicCall(ctx any) any {", "func (owner *Operations) publicCall(ctx any) any {", 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !containsAdmissionIssue(checkSource(tc.path, []byte(tc.source), "ucupa", testStdlib())) {
				t.Fatalf("invalid WithAdmission shape accepted: %s", tc.name)
			}
		})
	}
}

func TestWithAdmissionRuleAcceptsCurrentPublicEntrySource(t *testing.T) {
	root, err := currentRepositoryRoot(t)
	if err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(root, "ucupa-core", "internal", "access", "entry", "request.go")
	source, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if issues := checkSource("ucupa-core/internal/access/entry/request.go", source, "ucupa", testStdlib()); containsAdmissionIssue(issues) {
		t.Fatalf("current public entry admission flow rejected: %v", issues)
	}
}

func TestWithAdmissionRuleDoesNotPoliceTestFixtures(t *testing.T) {
	fixture := `package core_test

import accesspkg "ucupa/ucupa-core/internal/access"

func testFixture(ctx any, admission *accesspkg.Admission) any {
	return accesspkg.WithAdmission(ctx, admission)
}
`
	if issues := checkSource("ucupa-core/public_context_test.go", []byte(fixture), "ucupa", testStdlib()); containsAdmissionIssue(issues) {
		t.Fatalf("test-only WithAdmission fixture was rejected: %v", issues)
	}
}

func containsAdmissionIssue(issues []issue) bool {
	for _, item := range issues {
		if strings.Contains(item.message, "WithAdmission") {
			return true
		}
	}
	return false
}
