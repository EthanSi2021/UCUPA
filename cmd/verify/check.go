package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type issue struct {
	path, message string
	review        bool
}

func checkRepository(root string, stdlib map[string]bool) []issue {
	var issues []issue
	packages := map[string]packageSize{}
	for _, name := range []string{"go.mod", "go.sum"} {
		if info, err := os.Stat(filepath.Join(root, name)); err != nil || !info.Mode().IsRegular() {
			issues = append(issues, issue{path: name, message: "required root module file is missing or not a regular file"})
		}
	}
	module := ""
	if data, err := os.ReadFile(filepath.Join(root, "go.mod")); err == nil {
		var moduleIssues []issue
		module, moduleIssues = checkModule(data)
		issues = append(issues, moduleIssues...)
	}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if walkErr != nil {
			issues = append(issues, issue{path: rel, message: "cannot scan: " + walkErr.Error()})
			return nil
		}
		if entry.Name() == ".git" {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			issues = append(issues, issue{path: rel, message: "symbolic link is not an approved source location; target is not scanned"})
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		switch entry.Name() {
		case "go.mod", "go.sum":
			if rel != entry.Name() {
				issues = append(issues, issue{path: rel, message: "nested module file is forbidden; keep one root go.mod/go.sum"})
			}
		case "go.work", "go.work.sum":
			issues = append(issues, issue{path: rel, message: "Go workspace files are forbidden in this single-module repository"})
		}
		if strings.HasSuffix(entry.Name(), ".go") {
			data, err := os.ReadFile(path)
			if err != nil {
				issues = append(issues, issue{path: rel, message: "cannot read: " + err.Error()})
			} else {
				issues = append(issues, checkSource(rel, data, module, stdlib)...)
				countPackageSource(packages, rel, data)
			}
		}
		return nil
	})
	if err != nil {
		issues = append(issues, issue{path: ".", message: "repository scan failed: " + err.Error()})
	}
	issues = append(issues, checkPackageSizes(packages)...)
	sort.Slice(issues, func(i, j int) bool {
		if issues[i].path != issues[j].path {
			return issues[i].path < issues[j].path
		}
		return issues[i].message < issues[j].message
	})
	return issues
}

func checkModule(data []byte) (string, []issue) {
	var issues []issue
	module := ""
	for _, line := range strings.Split(string(data), "\n") {
		line, _, _ = strings.Cut(line, "//")
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "replace":
			issues = append(issues, issue{path: "go.mod", message: "replace directives are forbidden"})
		case "module":
			if len(fields) == 2 {
				module = fields[1]
				if unquoted, err := strconv.Unquote(module); err == nil {
					module = unquoted
				}
			}
		}
	}
	if module != "ucupa" {
		issues = append(issues, issue{path: "go.mod", message: "expected the single root module ucupa"})
	}
	return module, issues
}

func checkSource(path string, data []byte, module string, stdlib map[string]bool) []issue {
	var issues []issue
	add := func(message string) { issues = append(issues, issue{path: path, message: message}) }
	formatted, err := format.Source(data)
	if err != nil {
		add("Go source cannot be formatted: " + err.Error())
	} else if !bytes.Equal(data, formatted) {
		add("not gofmt formatted (check only; source was not changed)")
	}
	counted := data
	if err == nil {
		counted = formatted
	}
	lines := physicalLines(counted)
	review, limit := 500, 800
	test := strings.HasSuffix(path, "_test.go")
	if test {
		review, limit = 800, 1000
	}
	if generatedSource(path) {
		review, limit = 100000, 100000
	}
	if lines > limit {
		add(fmt.Sprintf("%d physical lines exceed hard limit %d; split by responsibility", lines, limit))
	} else if lines > review {
		issues = append(issues, issue{path, fmt.Sprintf("%d physical lines exceed responsibility review threshold %d", lines, review), true})
	}
	f, err := parser.ParseFile(token.NewFileSet(), path, data, parser.ParseComments)
	if err != nil {
		add("cannot inspect Go declarations/imports: " + err.Error())
		return issues
	}
	for _, message := range checkWithAdmissionUses(path, f, module, test) {
		add(message)
	}
	for _, message := range checkCoreMountRPCDeclarations(path, f, test) {
		add(message)
	}
	for _, message := range checkCoreAdmissionDeclarations(path, f, test) {
		add(message)
	}
	for _, message := range checkCoreEntryRPCDeclarations(path, f, test) {
		add(message)
	}
	if generatedSource(path) && (!ast.IsGenerated(f) || !bytes.Contains(data, []byte("protoc-gen-go"))) {
		add("registered binding lacks its generator header")
	}
	if ast.IsGenerated(f) && !generatedSource(path) {
		add("generated source is not approved: declare source, pinned tool and reproducible generation before adding it; limits still apply")
	}
	for _, group := range f.Comments {
		for _, comment := range group.List {
			if strings.HasPrefix(comment.Text, "//go:generate") {
				add("go:generate is not approved: no generation workflow is currently registered")
			}
		}
	}
	if message := checkSourceLocation(path); message != "" {
		add(message)
	}
	for _, imp := range f.Imports {
		name, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			add("invalid import: " + imp.Path.Value)
			continue
		}
		if message := checkDependency(path, name, module, test, stdlib); message != "" {
			add(fmt.Sprintf("import %q: %s", name, message))
		}
	}
	return issues
}

func physicalLines(data []byte) int {
	lines := bytes.Count(data, []byte{'\n'})
	if len(data) > 0 && data[len(data)-1] != '\n' {
		lines++
	}
	return lines
}
