package main

import (
	"go/ast"
	"go/token"
	"path"
	"strconv"
)

type withAdmissionUse struct {
	selector *ast.SelectorExpr
	call     *ast.CallExpr
}

// checkWithAdmissionUses is intentionally a small rule for the one public
// envelope shape. It is not a general data-flow engine: the successful
// Acquire guard, direct assignment and owner/admission expressions are all
// fixed so a new call site receives review instead of silently widening the
// admitted-context capability.
func checkWithAdmissionUses(sourcePath string, file *ast.File, module string, test bool) []string {
	if test {
		return nil
	}
	uses := collectWithAdmissionUses(file)
	if len(uses) == 0 {
		return nil
	}
	issues := make([]string, 0, len(uses))
	for _, use := range uses {
		if !validPublicCallWithAdmission(sourcePath, file, module, use) {
			issues = append(issues, "WithAdmission may only be used in publicCall after a checked successful core.admission.Acquire")
		}
	}
	return issues
}

func collectWithAdmissionUses(file *ast.File) []withAdmissionUse {
	var uses []withAdmissionUse
	selectors := make([]*ast.SelectorExpr, 0)
	callBySelector := map[*ast.SelectorExpr]*ast.CallExpr{}
	callIdentifiers := map[token.Pos]bool{}
	selectorIdentifiers := map[token.Pos]bool{}
	declarationIdentifiers := map[token.Pos]bool{}
	importIdentifiers := map[token.Pos]bool{}

	ast.Inspect(file, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.FuncDecl:
			if node.Name != nil && node.Name.Name == "WithAdmission" {
				declarationIdentifiers[node.Name.Pos()] = true
			}
		case *ast.ImportSpec:
			if node.Name != nil && node.Name.Name == "WithAdmission" {
				importIdentifiers[node.Name.Pos()] = true
			}
		case *ast.CallExpr:
			switch function := node.Fun.(type) {
			case *ast.SelectorExpr:
				if function.Sel.Name == "WithAdmission" {
					callBySelector[function] = node
				}
			case *ast.Ident:
				if function.Name == "WithAdmission" {
					uses = append(uses, withAdmissionUse{call: node})
					callIdentifiers[function.Pos()] = true
				}
			}
		case *ast.SelectorExpr:
			if node.Sel.Name == "WithAdmission" {
				selectors = append(selectors, node)
				selectorIdentifiers[node.Sel.Pos()] = true
			}
		case *ast.Ident:
			if node.Name == "WithAdmission" && !callIdentifiers[node.Pos()] && !selectorIdentifiers[node.Pos()] &&
				!declarationIdentifiers[node.Pos()] && !importIdentifiers[node.Pos()] {
				uses = append(uses, withAdmissionUse{})
			}
		}
		return true
	})
	for _, selector := range selectors {
		uses = append(uses, withAdmissionUse{selector: selector, call: callBySelector[selector]})
	}
	return uses
}

func validPublicCallWithAdmission(sourcePath string, file *ast.File, module string, use withAdmissionUse) bool {
	if sourcePath != "ucupa-core/internal/access/entry/request.go" || use.selector == nil || use.call == nil {
		return false
	}
	alias := admissionImportAlias(file, module)
	if alias == "" {
		return false
	}
	selector, ok := use.selector.X.(*ast.Ident)
	if !ok || selector.Name != alias || len(use.call.Args) != 2 || !identNamed(use.call.Args[0], "ctx") || !selectorNamed(use.call.Args[1], "core", "admission") {
		return false
	}
	publicCall := findFunction(file, "publicCall", "Operations")
	if publicCall == nil || publicCall.Body == nil {
		return false
	}
	for index, statement := range publicCall.Body.List {
		assignment, ok := statement.(*ast.AssignStmt)
		if !ok || assignment.Tok != token.ASSIGN || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 || !identNamed(assignment.Lhs[0], "ctx") {
			continue
		}
		call, ok := assignment.Rhs[0].(*ast.CallExpr)
		if !ok || call != use.call {
			continue
		}
		for guardIndex, prior := range publicCall.Body.List[:index] {
			if successfulAcquireGuard(prior) && validAdmissionMarkerGap(publicCall.Body.List, guardIndex, index) {
				return true
			}
		}
		return false
	}
	return false
}

func admissionImportAlias(file *ast.File, module string) string {
	want := module + "/ucupa-core/internal/access"
	for _, spec := range file.Imports {
		pathValue, err := strconv.Unquote(spec.Path.Value)
		if err != nil || pathValue != want {
			continue
		}
		if spec.Name == nil {
			return path.Base(pathValue)
		}
		if spec.Name.Name == "_" || spec.Name.Name == "." {
			return ""
		}
		return spec.Name.Name
	}
	return ""
}

func findFunction(file *ast.File, name, receiverType string) *ast.FuncDecl {
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && function.Name != nil && function.Name.Name == name && isPointerMethod(function, receiverType) {
			return function
		}
	}
	return nil
}

func isPointerMethod(function *ast.FuncDecl, receiverType string) bool {
	if function.Recv == nil || len(function.Recv.List) != 1 {
		return false
	}
	field := function.Recv.List[0]
	if len(field.Names) != 1 || field.Names[0].Name != "core" {
		return false
	}
	pointer, ok := field.Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	ident, ok := pointer.X.(*ast.Ident)
	return ok && ident.Name == receiverType
}

func validAdmissionMarkerGap(statements []ast.Stmt, guardIndex, markerIndex int) bool {
	if guardIndex < 0 || markerIndex != guardIndex+3 || markerIndex >= len(statements) {
		return false
	}
	return admissionDefer(statements[guardIndex+1], "admission", "Release") &&
		admissionDefer(statements[guardIndex+2], "access", "DrainRetired")
}

func admissionDefer(statement ast.Stmt, field, method string) bool {
	deferStmt, ok := statement.(*ast.DeferStmt)
	if !ok || deferStmt.Call == nil || len(deferStmt.Call.Args) != 0 {
		return false
	}
	return methodSelectorNamed(deferStmt.Call.Fun, "core", field, method)
}

func successfulAcquireGuard(statement ast.Stmt) bool {
	guard, ok := statement.(*ast.IfStmt)
	if !ok || guard.Else != nil || guard.Init == nil || guard.Body == nil || len(guard.Body.List) != 1 {
		return false
	}
	assignment, ok := guard.Init.(*ast.AssignStmt)
	if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 || !identNamed(assignment.Lhs[0], "err") {
		return false
	}
	acquire, ok := assignment.Rhs[0].(*ast.CallExpr)
	if !ok || !methodSelectorNamed(acquire.Fun, "core", "admission", "Acquire") {
		return false
	}
	condition, ok := guard.Cond.(*ast.BinaryExpr)
	if !ok || condition.Op != token.NEQ || !identNamed(condition.X, "err") || !identNamed(condition.Y, "nil") {
		return false
	}
	_, ok = guard.Body.List[0].(*ast.ReturnStmt)
	return ok
}

func identNamed(expression ast.Expr, name string) bool {
	ident, ok := expression.(*ast.Ident)
	return ok && ident.Name == name
}

func selectorNamed(expression ast.Expr, receiver, method string) bool {
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != method {
		return false
	}
	ident, ok := selector.X.(*ast.Ident)
	return ok && ident.Name == receiver
}

func methodSelectorNamed(expression ast.Expr, receiver, field, method string) bool {
	outer, ok := expression.(*ast.SelectorExpr)
	if !ok || outer.Sel.Name != method {
		return false
	}
	inner, ok := outer.X.(*ast.SelectorExpr)
	if !ok || inner.Sel.Name != field {
		return false
	}
	ident, ok := inner.X.(*ast.Ident)
	return ok && ident.Name == receiver
}
