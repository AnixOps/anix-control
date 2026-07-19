//go:build ignore

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
)

var anyMethods = []string{
	"GET",
	"POST",
	"PUT",
	"PATCH",
	"HEAD",
	"OPTIONS",
	"DELETE",
	"CONNECT",
	"TRACE",
}

var websocketPaths = map[string]struct{}{
	"/api/v2/admin/ws/monitor": {},
	"/api/v2/agent/ws":         {},
	"/api/v2/node/ws":          {},
}

const controlConfigImportPath = "github.com/AnixOps/anix-control/v4/internal/config"

type route struct {
	Method          string `json:"method"`
	Path            string `json:"path"`
	Handler         string `json:"handler"`
	MiddlewareGroup string `json:"middleware_group"`
	Transport       string `json:"transport"`
	Binding         string `json:"binding"`
	PackageID       string `json:"package_id"`
	RouteID         string `json:"route_id"`
}

type handlerRegistration struct {
	handler   string
	binding   string
	packageID string
	routeID   string
}

type group struct {
	base       string
	middleware []string
}

type scope struct {
	parent             *scope
	groups             map[string]*group
	stringValues       map[string]string
	safeSubscribePaths map[string]bool
	names              map[string]struct{}
}

func newScope(parent *scope) *scope {
	return &scope{
		parent:             parent,
		groups:             make(map[string]*group),
		stringValues:       make(map[string]string),
		safeSubscribePaths: make(map[string]bool),
		names:              make(map[string]struct{}),
	}
}

func (s *scope) declareName(name string) {
	if name != "_" {
		s.names[name] = struct{}{}
	}
}

func (s *scope) isNameBound(name string) bool {
	for current := s; current != nil; current = current.parent {
		if _, found := current.names[name]; found {
			return true
		}
	}
	return false
}

func (s *scope) define(name string, value *group) {
	s.groups[name] = value
}

func (s *scope) assign(name string, value *group) {
	for current := s; current != nil; current = current.parent {
		if _, found := current.groups[name]; found {
			current.groups[name] = value
			return
		}
	}
	s.groups[name] = value
}

func (s *scope) lookup(name string) (*group, bool) {
	for current := s; current != nil; current = current.parent {
		if value, found := current.groups[name]; found {
			return value, true
		}
	}
	return nil, false
}

func (s *scope) defineString(name, value string) {
	s.stringValues[name] = value
}

func (s *scope) assignString(name, value string) {
	for current := s; current != nil; current = current.parent {
		if _, found := current.stringValues[name]; found {
			current.stringValues[name] = value
			return
		}
	}
	s.stringValues[name] = value
}

func (s *scope) unsetString(name string) {
	for current := s; current != nil; current = current.parent {
		if _, found := current.stringValues[name]; found {
			delete(current.stringValues, name)
			return
		}
	}
}

func (s *scope) lookupString(name string) (string, bool) {
	for current := s; current != nil; current = current.parent {
		if value, found := current.stringValues[name]; found {
			return value, true
		}
	}
	return "", false
}

func (s *scope) defineSafeSubscribePath(name string, safe bool) {
	s.safeSubscribePaths[name] = safe
}

func (s *scope) assignSafeSubscribePath(name string, safe bool) {
	for current := s; current != nil; current = current.parent {
		if _, found := current.safeSubscribePaths[name]; found {
			current.safeSubscribePaths[name] = safe
			return
		}
	}
	s.safeSubscribePaths[name] = safe
}

func (s *scope) isSafeSubscribePath(name string) bool {
	for current := s; current != nil; current = current.parent {
		if safe, found := current.safeSubscribePaths[name]; found {
			return safe
		}
	}
	return false
}

type collector struct {
	fset                 *token.FileSet
	routes               []route
	configPackageAliases map[string]struct{}
}

func main() {
	routerPath := flag.String("router", "internal/router/router.go", "Gin router source file")
	format := flag.String("format", "json", "output format")
	flag.Parse()

	if *format != "json" {
		fail(fmt.Errorf("unsupported format %q", *format))
	}

	routes, err := inventory(*routerPath)
	if err != nil {
		fail(err)
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(routes); err != nil {
		fail(fmt.Errorf("encode route inventory: %w", err))
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "v2 route inventory:", err)
	os.Exit(1)
}

func inventory(routerPath string) ([]route, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, routerPath, nil, parser.ParseComments|parser.AllErrors)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", routerPath, err)
	}

	collector := collector{fset: fset, configPackageAliases: configPackageAliases(file)}
	for _, declaration := range file.Decls {
		if err := collector.validateTopLevelDeclaration(declaration); err != nil {
			return nil, err
		}
	}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Body == nil {
			continue
		}

		env := newScope(nil)
		if !seedEngineParameters(function, env) {
			if err := collector.rejectUnresolvedGinSelectors(function.Body, env); err != nil {
				return nil, err
			}
			continue
		}
		if err := collector.processBlock(function.Body, env); err != nil {
			return nil, err
		}
	}

	sort.Slice(collector.routes, func(i, j int) bool {
		left, right := collector.routes[i], collector.routes[j]
		if left.Path != right.Path {
			return left.Path < right.Path
		}
		if left.Method != right.Method {
			return left.Method < right.Method
		}
		return left.Handler < right.Handler
	})
	return collector.routes, nil
}

func (c *collector) validateTopLevelDeclaration(declaration ast.Decl) error {
	general, ok := declaration.(*ast.GenDecl)
	if !ok || general.Tok != token.VAR {
		return nil
	}
	for _, specification := range general.Specs {
		value, ok := specification.(*ast.ValueSpec)
		if !ok || len(value.Values) == 0 {
			continue
		}
		return c.errorAt(value.Pos(), "top-level variable initializers are unsupported while analyzing Gin routes")
	}
	return nil
}

func (c *collector) rejectUnresolvedGinSelectors(node ast.Node, env *scope) error {
	var validationError error
	ast.Inspect(node, func(current ast.Node) bool {
		if validationError != nil {
			return false
		}
		call, ok := current.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || !isGinRouteSelector(selector.Sel.Name) {
			return true
		}
		target, err := c.resolveGroupWithMiddlewareMutation(selector.X, env, false)
		if err != nil {
			validationError = err
			return false
		}
		if target == nil {
			validationError = c.errorAt(call.Pos(), "unresolved Gin selector receiver for %s", selector.Sel.Name)
			return false
		}
		return true
	})
	return validationError
}

func configPackageAliases(file *ast.File) map[string]struct{} {
	aliases := make(map[string]struct{})
	for _, importSpec := range file.Imports {
		importPath, err := strconv.Unquote(importSpec.Path.Value)
		if err != nil || importPath != controlConfigImportPath {
			continue
		}

		alias := "config"
		if importSpec.Name != nil {
			alias = importSpec.Name.Name
		}
		if alias != "." && alias != "_" {
			aliases[alias] = struct{}{}
		}
	}
	return aliases
}

func seedEngineParameters(function *ast.FuncDecl, env *scope) bool {
	if function.Type.Params == nil {
		return false
	}

	found := false
	if function.Recv != nil {
		for _, field := range function.Recv.List {
			for _, name := range field.Names {
				env.declareName(name.Name)
			}
		}
	}
	for _, field := range function.Type.Params.List {
		for _, name := range field.Names {
			env.declareName(name.Name)
		}
		if !isGinEngine(field.Type) {
			continue
		}
		for _, name := range field.Names {
			env.define(name.Name, &group{})
			found = true
		}
	}
	if function.Type.Results != nil {
		for _, field := range function.Type.Results.List {
			for _, name := range field.Names {
				env.declareName(name.Name)
			}
		}
	}
	return found
}

func isGinEngine(expression ast.Expr) bool {
	star, ok := expression.(*ast.StarExpr)
	if !ok {
		return false
	}
	selector, ok := star.X.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Engine" {
		return false
	}
	packageName, ok := selector.X.(*ast.Ident)
	return ok && packageName.Name == "gin"
}

func (c *collector) processBlock(block *ast.BlockStmt, parent *scope) error {
	env := newScope(parent)
	for _, statement := range block.List {
		if err := c.processStatement(statement, env); err != nil {
			return err
		}
	}
	return nil
}

func (c *collector) processStatement(statement ast.Stmt, env *scope) error {
	if containsAddressTaking(statement) {
		return c.errorAt(statement.Pos(), "address-taking is unsupported while analyzing Gin routes")
	}
	if _, isBlock := statement.(*ast.BlockStmt); !isBlock && containsFunctionLiteral(statement) && !c.isDirectGinFunctionLiteralCall(statement, env) {
		return c.errorAt(statement.Pos(), "function literals are unsupported outside direct Gin route arguments")
	}
	if c.containsSafeSubscribePath(statement, env) && !c.isDirectSafeSubscribeRegistration(statement, env) {
		return c.errorAt(statement.Pos(), "normalized subscription path escapes its direct registration")
	}
	switch node := statement.(type) {
	case *ast.BlockStmt:
		return c.processBlock(node, env)
	case *ast.DeclStmt:
		return c.processDeclaration(node.Decl, env)
	case *ast.AssignStmt:
		return c.processAssignment(node, env)
	case *ast.ExprStmt:
		return c.processExpression(node.X, env)
	case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
		return c.rejectTrackedGinGroupInControlFlow(node, env)
	}
	return c.rejectTrackedGinGroupInStatement(statement, env)
}

func containsAddressTaking(node ast.Node) bool {
	found := false
	ast.Inspect(node, func(current ast.Node) bool {
		if found {
			return false
		}
		unary, ok := current.(*ast.UnaryExpr)
		if ok && unary.Op == token.AND {
			found = true
			return false
		}
		return true
	})
	return found
}

func containsFunctionLiteral(node ast.Node) bool {
	found := false
	ast.Inspect(node, func(current ast.Node) bool {
		if found {
			return false
		}
		if _, ok := current.(*ast.FuncLit); ok {
			found = true
			return false
		}
		return true
	})
	return found
}

func (c *collector) isDirectGinFunctionLiteralCall(statement ast.Stmt, env *scope) bool {
	expressionStatement, ok := statement.(*ast.ExprStmt)
	if !ok {
		return false
	}
	call, ok := unwrapParens(expressionStatement.X).(*ast.CallExpr)
	if !ok {
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	target, err := c.resolveGroupWithMiddlewareMutation(selector.X, env, false)
	if err != nil || target == nil {
		return false
	}
	for _, argument := range call.Args {
		if containsFunctionLiteral(argument) {
			return true
		}
	}
	return false
}

func (c *collector) processDeclaration(declaration ast.Decl, env *scope) error {
	general, ok := declaration.(*ast.GenDecl)
	if !ok {
		return nil
	}
	if general.Tok == token.CONST {
		for _, specification := range general.Specs {
			value, ok := specification.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, name := range value.Names {
				env.declareName(name.Name)
			}
		}
		return nil
	}
	if general.Tok == token.TYPE {
		for _, specification := range general.Specs {
			typeSpec, ok := specification.(*ast.TypeSpec)
			if ok {
				env.declareName(typeSpec.Name.Name)
			}
		}
		return nil
	}
	if general.Tok != token.VAR {
		return nil
	}
	for _, specification := range general.Specs {
		value, ok := specification.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for index, name := range value.Names {
			if index >= len(value.Values) {
				env.declareName(name.Name)
				env.defineSafeSubscribePath(name.Name, false)
				continue
			}
			expression := value.Values[index]
			if err := c.validateValueExpression(expression, env); err != nil {
				return err
			}
			safeSubscribePath := c.isTrustedSubscribePathCall(expression, env)
			env.declareName(name.Name)
			env.defineSafeSubscribePath(name.Name, safeSubscribePath)
			if stringValue, known := resolveString(expression, env); known {
				env.defineString(name.Name, stringValue)
			}
			resolved, err := c.resolveGroup(expression, env)
			if err != nil {
				return err
			}
			if resolved != nil {
				env.define(name.Name, resolved)
			}
		}
	}
	return nil
}

func (c *collector) processAssignment(assignment *ast.AssignStmt, env *scope) error {
	if assignment.Tok != token.DEFINE && assignment.Tok != token.ASSIGN {
		return c.errorAt(assignment.Pos(), "unsupported assignment operator while analyzing Gin routes")
	}
	if len(assignment.Lhs) > 1 && (len(assignment.Rhs) != 1 || !c.isNormalizeSubscribePathCall(assignment.Rhs[0], env)) {
		return c.errorAt(assignment.Pos(), "unsupported multi-value assignment while analyzing Gin routes")
	}
	for index, left := range assignment.Lhs {
		if index >= len(assignment.Rhs) {
			continue
		}
		name, ok := left.(*ast.Ident)
		if !ok {
			return c.errorAt(left.Pos(), "unsupported assignment target while analyzing Gin routes")
		}
		if ok && assignment.Tok != token.DEFINE {
			if _, found := env.lookup(name.Name); found {
				return c.errorAt(left.Pos(), "Gin group reassignment is unsupported")
			}
		}
		expression := assignment.Rhs[index]
		if err := c.validateValueExpression(expression, env); err != nil {
			return err
		}
		if !ok {
			continue
		}
		safeSubscribePath := c.isTrustedSubscribePathCall(expression, env)
		env.declareName(name.Name)
		if assignment.Tok == token.DEFINE {
			env.defineSafeSubscribePath(name.Name, safeSubscribePath)
		} else {
			env.assignSafeSubscribePath(name.Name, safeSubscribePath)
		}
		if stringValue, known := resolveString(expression, env); known {
			if assignment.Tok == token.DEFINE {
				env.defineString(name.Name, stringValue)
			} else {
				env.assignString(name.Name, stringValue)
			}
		} else if assignment.Tok != token.DEFINE {
			env.unsetString(name.Name)
		}
		resolved, err := c.resolveGroup(expression, env)
		if err != nil {
			return err
		}
		if resolved == nil {
			continue
		}
		if assignment.Tok == token.DEFINE {
			env.define(name.Name, resolved)
		} else {
			env.assign(name.Name, resolved)
		}
	}
	return nil
}

func (c *collector) validateValueExpression(expression ast.Expr, env *scope) error {
	call, ok := unwrapParens(expression).(*ast.CallExpr)
	if ok {
		selector, selectorOK := call.Fun.(*ast.SelectorExpr)
		if selectorOK {
			target, err := c.resolveGroupWithMiddlewareMutation(selector.X, env, false)
			if err != nil {
				return err
			}
			if target != nil {
				switch selector.Sel.Name {
				case "Group", "Use":
					return c.validateGinCallArguments(call, env)
				case "GET", "POST", "PUT", "PATCH", "HEAD", "OPTIONS", "DELETE", "CONNECT", "TRACE", "Any", "Handle", "Match":
					if isV2Group(target) {
						return c.errorAt(call.Pos(), "unsupported Gin registration expression %s on /api/v2 group", selector.Sel.Name)
					}
				default:
					if isV2Group(target) {
						return c.errorAt(call.Pos(), "unsupported Gin selector %s on /api/v2 group", selector.Sel.Name)
					}
				}
			} else if isGinRouteSelector(selector.Sel.Name) {
				return c.errorAt(call.Pos(), "unresolved Gin selector receiver for %s", selector.Sel.Name)
			}
		}
	}
	if c.containsTrackedGinGroup(expression, env) {
		return c.errorAt(expression.Pos(), "unsupported Gin group escape in value expression")
	}
	return nil
}

func (c *collector) processExpression(expression ast.Expr, env *scope) error {
	call, ok := unwrapParens(expression).(*ast.CallExpr)
	if !ok {
		return c.rejectTrackedGinGroupEscape(expression, env)
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return c.rejectTrackedGinGroupEscape(expression, env)
	}
	target, err := c.resolveGroup(selector.X, env)
	if err != nil {
		return err
	}
	if target == nil {
		if isGinRouteSelector(selector.Sel.Name) {
			return c.errorAt(call.Pos(), "unresolved Gin selector receiver for %s", selector.Sel.Name)
		}
		return c.rejectTrackedGinGroupEscape(expression, env)
	}
	if err := c.validateGinCallArguments(call, env); err != nil {
		return err
	}

	switch selector.Sel.Name {
	case "Group":
		return nil
	case "Use":
		if target.base != "" {
			for _, middleware := range call.Args {
				target.middleware = append(target.middleware, expressionIdentifier(middleware))
			}
		}
		return nil
	case "GET", "POST", "PUT", "PATCH", "HEAD", "OPTIONS", "DELETE", "CONNECT", "TRACE", "Any":
		return c.register(target, selector.Sel.Name, call, env)
	case "Handle":
		return c.registerHandle(target, call, env)
	case "Match":
		return c.registerMatch(target, call, env)
	default:
		if isV2Group(target) {
			return c.errorAt(call.Pos(), "unsupported Gin selector %s on /api/v2 group", selector.Sel.Name)
		}
		return c.errorAt(call.Pos(), "unsupported Gin selector %s on tracked Gin group", selector.Sel.Name)
	}
}

func (c *collector) rejectTrackedGinGroupEscape(expression ast.Expr, env *scope) error {
	if c.containsTrackedGinGroup(expression, env) {
		return c.errorAt(expression.Pos(), "unsupported Gin group escape in expression")
	}
	return nil
}

func (c *collector) rejectTrackedGinGroupInControlFlow(node ast.Node, env *scope) error {
	if c.containsTrackedGinGroup(node, env) {
		return c.errorAt(node.Pos(), "unsupported Gin group escape in control flow")
	}
	return nil
}

func (c *collector) rejectTrackedGinGroupInStatement(node ast.Node, env *scope) error {
	if c.containsTrackedGinGroup(node, env) {
		return c.errorAt(node.Pos(), "unsupported Gin group escape in statement")
	}
	return nil
}

func (c *collector) containsSafeSubscribePath(node ast.Node, env *scope) bool {
	found := false
	ast.Inspect(node, func(current ast.Node) bool {
		if found {
			return false
		}
		identifier, ok := current.(*ast.Ident)
		if !ok {
			return true
		}
		if env.isSafeSubscribePath(identifier.Name) {
			found = true
			return false
		}
		return true
	})
	return found
}

func (c *collector) isDirectSafeSubscribeRegistration(statement ast.Stmt, env *scope) bool {
	expressionStatement, ok := statement.(*ast.ExprStmt)
	if !ok {
		return false
	}
	call, ok := unwrapParens(expressionStatement.X).(*ast.CallExpr)
	if !ok {
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "GET" || len(call.Args) < 2 {
		return false
	}
	target, err := c.resolveGroupWithMiddlewareMutation(selector.X, env, false)
	if err != nil || target == nil || target.base != "" || !isSafeSubscribeRegistrationPath(call.Args[0], env) {
		return false
	}
	for _, argument := range call.Args[1:] {
		if c.containsSafeSubscribePath(argument, env) {
			return false
		}
	}
	return true
}

func (c *collector) containsTrackedGinGroup(node ast.Node, env *scope) bool {
	found := false
	ast.Inspect(node, func(current ast.Node) bool {
		if found {
			return false
		}
		identifier, ok := current.(*ast.Ident)
		if !ok {
			return true
		}
		if _, tracked := env.lookup(identifier.Name); tracked {
			found = true
			return false
		}
		return true
	})
	return found
}

func (c *collector) validateGinCallArguments(call *ast.CallExpr, env *scope) error {
	for _, argument := range call.Args {
		if c.containsTrackedGinGroup(argument, env) {
			return c.errorAt(argument.Pos(), "unsupported Gin group escape in call arguments")
		}
	}
	return nil
}

func (c *collector) resolveGroup(expression ast.Expr, env *scope) (*group, error) {
	return c.resolveGroupWithMiddlewareMutation(expression, env, true)
}

func (c *collector) resolveGroupWithMiddlewareMutation(expression ast.Expr, env *scope, mutateMiddleware bool) (*group, error) {
	switch node := expression.(type) {
	case *ast.Ident:
		value, _ := env.lookup(node.Name)
		return value, nil
	case *ast.ParenExpr:
		return c.resolveGroupWithMiddlewareMutation(node.X, env, mutateMiddleware)
	case *ast.CallExpr:
		selector, ok := node.Fun.(*ast.SelectorExpr)
		if !ok {
			return nil, nil
		}
		parent, err := c.resolveGroupWithMiddlewareMutation(selector.X, env, mutateMiddleware)
		if err != nil || parent == nil {
			return parent, err
		}
		switch selector.Sel.Name {
		case "Group":
			if err := c.validateGinCallArguments(node, env); err != nil {
				return nil, err
			}
			if len(node.Args) == 0 {
				return nil, c.errorAt(node.Pos(), "Gin Group call is missing its relative path")
			}
			relativePath, ok := resolveString(node.Args[0], env)
			if !ok {
				return nil, c.errorAt(node.Args[0].Pos(), "Gin Group path must be a string literal or locally resolvable literal")
			}
			child := &group{
				base:       joinPath(parent.base, relativePath),
				middleware: append([]string(nil), parent.middleware...),
			}
			if child.base != "" {
				for _, middleware := range node.Args[1:] {
					child.middleware = append(child.middleware, expressionIdentifier(middleware))
				}
			}
			return child, nil
		case "Use":
			if err := c.validateGinCallArguments(node, env); err != nil {
				return nil, err
			}
			if parent.base != "" {
				if !mutateMiddleware {
					parent = &group{
						base:       parent.base,
						middleware: append([]string(nil), parent.middleware...),
					}
				}
				for _, middleware := range node.Args {
					parent.middleware = append(parent.middleware, expressionIdentifier(middleware))
				}
			}
			return parent, nil
		default:
			if isV2Group(parent) {
				if isGinRegistrationSelector(selector.Sel.Name) {
					return nil, c.errorAt(node.Pos(), "unsupported Gin registration expression %s on /api/v2 group", selector.Sel.Name)
				}
				return nil, c.errorAt(node.Pos(), "unsupported Gin selector %s on /api/v2 group", selector.Sel.Name)
			}
			return nil, nil
		}
	default:
		return nil, nil
	}
}

func (c *collector) register(target *group, method string, call *ast.CallExpr, env *scope) error {
	return c.registerArguments(target, method, call.Args, call.Pos(), env)
}

func (c *collector) registerHandle(target *group, call *ast.CallExpr, env *scope) error {
	if len(call.Args) < 3 {
		return c.errorAt(call.Pos(), "Gin Handle registration must include a method, path, and handler")
	}
	method, ok := resolveString(call.Args[0], env)
	if !ok {
		return c.errorAt(call.Args[0].Pos(), "Gin Handle method must be a string literal or locally resolvable literal")
	}
	method = strings.ToUpper(method)
	if !isHTTPMethod(method) {
		return c.errorAt(call.Args[0].Pos(), "Gin Handle method %q is not supported", method)
	}
	return c.registerArguments(target, method, call.Args[1:], call.Pos(), env)
}

func (c *collector) registerMatch(target *group, call *ast.CallExpr, env *scope) error {
	if len(call.Args) < 3 {
		return c.errorAt(call.Pos(), "Gin Match registration must include methods, a path, and a handler")
	}
	methods, err := c.resolveMethodList(call.Args[0], env)
	if err != nil {
		return err
	}
	for _, method := range methods {
		if err := c.registerArguments(target, method, call.Args[1:], call.Pos(), env); err != nil {
			return err
		}
	}
	return nil
}

func (c *collector) resolveMethodList(expression ast.Expr, env *scope) ([]string, error) {
	literal, ok := expression.(*ast.CompositeLit)
	if !ok {
		return nil, c.errorAt(expression.Pos(), "Gin Match methods must be a literal string list")
	}
	if len(literal.Elts) == 0 {
		return nil, c.errorAt(expression.Pos(), "Gin Match methods must not be empty")
	}
	methods := make([]string, 0, len(literal.Elts))
	for _, element := range literal.Elts {
		method, ok := resolveString(element, env)
		if !ok {
			return nil, c.errorAt(element.Pos(), "Gin Match method must be a string literal or locally resolvable literal")
		}
		method = strings.ToUpper(method)
		if !isHTTPMethod(method) {
			return nil, c.errorAt(element.Pos(), "Gin Match method %q is not supported", method)
		}
		methods = append(methods, method)
	}
	return methods, nil
}

func (c *collector) registerArguments(target *group, method string, args []ast.Expr, position token.Pos, env *scope) error {
	if len(args) < 2 {
		return c.errorAt(position, "Gin %s registration must include a path and handler", method)
	}
	relativePath, ok := resolveString(args[0], env)
	if !ok {
		if target.base == "" && method == "GET" && isSafeSubscribeRegistrationPath(args[0], env) {
			return nil
		}
		if isV2Group(target) {
			return c.errorAt(args[0].Pos(), "Gin %s path under /api/v2 must be a string literal", method)
		}
		if target.base == "" {
			return c.errorAt(args[0].Pos(), "root Gin Engine path must be statically resolvable")
		}
		return c.errorAt(args[0].Pos(), "Gin %s path must be statically resolvable", method)
	}
	fullPath := joinPath(target.base, relativePath)
	if !mayMatchV2Namespace(fullPath) {
		return nil
	}
	if !isV2Path(fullPath) {
		return c.errorAt(position, "Gin route pattern may overlap /api/v2 and must use a literal /api/v2 prefix")
	}

	routeMiddleware := append([]string(nil), target.middleware...)
	for _, middleware := range args[1 : len(args)-1] {
		routeMiddleware = append(routeMiddleware, expressionIdentifier(middleware))
	}
	middlewareGroup, err := classifyMiddleware(routeMiddleware)
	if err != nil {
		return c.errorAt(position, "%w", err)
	}
	registration, err := packageGatewayHandlerRegistration(args[len(args)-1], env)
	if err != nil {
		return c.errorAt(args[len(args)-1].Pos(), "%w", err)
	}
	handler := registration.handler
	if handler == "" {
		return c.errorAt(args[len(args)-1].Pos(), "Gin %s registration has no handler identifier", method)
	}

	methods := []string{method}
	if method == "Any" {
		methods = anyMethods
	}
	for _, concreteMethod := range methods {
		transport := "http"
		if concreteMethod == "GET" {
			if _, websocket := websocketPaths[fullPath]; websocket {
				transport = "websocket"
			}
		}
		c.routes = append(c.routes, route{
			Method:          concreteMethod,
			Path:            fullPath,
			Handler:         handler,
			MiddlewareGroup: middlewareGroup,
			Transport:       transport,
			Binding:         registration.binding,
			PackageID:       registration.packageID,
			RouteID:         registration.routeID,
		})
	}
	return nil
}

func packageGatewayHandlerRegistration(expression ast.Expr, env *scope) (handlerRegistration, error) {
	call, ok := unwrapParens(expression).(*ast.CallExpr)
	if !ok {
		return handlerRegistration{handler: expressionIdentifier(expression), binding: "direct"}, nil
	}

	wrapper := expressionIdentifier(call.Fun)
	expectedArguments := 0
	switch wrapper {
	case "registeredPackageRoute":
		expectedArguments = 4
	case "registeredPackageWebSocketRoute":
		expectedArguments = 5
	default:
		return handlerRegistration{handler: expressionIdentifier(expression), binding: "direct"}, nil
	}
	if len(call.Args) != expectedArguments {
		return handlerRegistration{}, fmt.Errorf("%s must receive %d arguments", wrapper, expectedArguments)
	}
	gateway := expressionIdentifier(call.Args[0])
	if gateway == "" {
		return handlerRegistration{}, fmt.Errorf("%s gateway argument has no handler identifier", wrapper)
	}
	packageID, ok := resolveString(call.Args[1], env)
	if !ok || packageID == "" {
		return handlerRegistration{}, fmt.Errorf("%s package id must be a statically resolvable non-empty string", wrapper)
	}
	routeID, ok := resolveString(call.Args[2], env)
	if !ok || routeID == "" {
		return handlerRegistration{}, fmt.Errorf("%s route id must be a statically resolvable non-empty string", wrapper)
	}
	binding := "package-http"
	if wrapper == "registeredPackageWebSocketRoute" {
		binding = "package-websocket"
	}
	return handlerRegistration{handler: gateway, binding: binding, packageID: packageID, routeID: routeID}, nil
}

func isHTTPMethod(method string) bool {
	for _, candidate := range anyMethods {
		if method == candidate {
			return true
		}
	}
	return false
}

func isGinRegistrationSelector(name string) bool {
	switch name {
	case "GET", "POST", "PUT", "PATCH", "HEAD", "OPTIONS", "DELETE", "CONNECT", "TRACE", "Any", "Handle", "Match":
		return true
	default:
		return false
	}
}

func isGinRouteSelector(name string) bool {
	switch name {
	case "Group", "Use", "GET", "POST", "PUT", "PATCH", "HEAD", "OPTIONS", "DELETE", "CONNECT", "TRACE", "Any", "Handle", "Match", "Static", "StaticFS", "StaticFile", "StaticFileFS", "NoRoute", "NoMethod":
		return true
	default:
		return false
	}
}

func (c *collector) isNormalizeSubscribePathCall(expression ast.Expr, env *scope) bool {
	return c.isTrustedConfigCall(expression, env, "NormalizeSubscribePath")
}

func (c *collector) isTrustedSubscribePathCall(expression ast.Expr, env *scope) bool {
	return c.isNormalizeSubscribePathCall(expression, env) || c.isTrustedConfigCall(expression, env, "PrepareSubscribePath")
}

func (c *collector) isTrustedConfigCall(expression ast.Expr, env *scope, functionName string) bool {
	call, ok := unwrapParens(expression).(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != functionName {
		return false
	}
	packageName, ok := unwrapParens(selector.X).(*ast.Ident)
	if !ok || env.isNameBound(packageName.Name) {
		return false
	}
	_, imported := c.configPackageAliases[packageName.Name]
	return imported
}

func isSafeSubscribeRegistrationPath(expression ast.Expr, env *scope) bool {
	var parts []ast.Expr
	flattenStringConcatenation(unwrapParens(expression), &parts)
	if len(parts) != 3 {
		return false
	}
	if prefix, ok := resolveString(parts[0], env); !ok || prefix != "/" {
		return false
	}
	identifier, ok := unwrapParens(parts[1]).(*ast.Ident)
	if !ok || !env.isSafeSubscribePath(identifier.Name) {
		return false
	}
	suffix, ok := resolveString(parts[2], env)
	return ok && suffix == "/:token"
}

func flattenStringConcatenation(expression ast.Expr, parts *[]ast.Expr) {
	binary, ok := unwrapParens(expression).(*ast.BinaryExpr)
	if !ok || binary.Op != token.ADD {
		*parts = append(*parts, expression)
		return
	}
	flattenStringConcatenation(binary.X, parts)
	flattenStringConcatenation(binary.Y, parts)
}

func unwrapParens(expression ast.Expr) ast.Expr {
	for {
		parenthesized, ok := expression.(*ast.ParenExpr)
		if !ok {
			return expression
		}
		expression = parenthesized.X
	}
}

func (c *collector) errorAt(position token.Pos, format string, args ...any) error {
	location := c.fset.Position(position)
	return fmt.Errorf("%s: %s", location, fmt.Sprintf(format, args...))
}

func resolveString(expression ast.Expr, env *scope) (string, bool) {
	switch node := expression.(type) {
	case *ast.BasicLit:
		return stringLiteral(node)
	case *ast.Ident:
		return env.lookupString(node.Name)
	case *ast.ParenExpr:
		return resolveString(node.X, env)
	case *ast.BinaryExpr:
		if node.Op != token.ADD {
			return "", false
		}
		left, leftOK := resolveString(node.X, env)
		right, rightOK := resolveString(node.Y, env)
		if !leftOK || !rightOK {
			return "", false
		}
		return left + right, true
	default:
		return "", false
	}
}

func stringLiteral(literal *ast.BasicLit) (string, bool) {
	if literal.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(literal.Value)
	if err != nil {
		return "", false
	}
	return value, true
}

func expressionIdentifier(expression ast.Expr) string {
	switch node := expression.(type) {
	case *ast.Ident:
		return node.Name
	case *ast.SelectorExpr:
		left := expressionIdentifier(node.X)
		if left == "" {
			return node.Sel.Name
		}
		return left + "." + node.Sel.Name
	case *ast.CallExpr:
		return expressionIdentifier(node.Fun)
	case *ast.ParenExpr:
		return expressionIdentifier(node.X)
	case *ast.FuncLit:
		return "func"
	default:
		return ""
	}
}

func joinPath(base, relative string) string {
	joined := path.Join(base, relative)
	if joined == "." || joined == "" {
		return "/"
	}
	if !strings.HasPrefix(joined, "/") {
		return "/" + joined
	}
	return joined
}

func isV2Group(target *group) bool {
	return isV2Path(target.base)
}

func isV2Path(routePath string) bool {
	return routePath == "/api/v2" || strings.HasPrefix(routePath, "/api/v2/")
}

func mayMatchV2Namespace(routePath string) bool {
	trimmed := strings.Trim(routePath, "/")
	if trimmed == "" {
		return false
	}
	segments := strings.Split(trimmed, "/")
	for index, expected := range []string{"api", "v2"} {
		if index >= len(segments) {
			return false
		}
		segment := segments[index]
		if strings.Contains(segment, "*") {
			return true
		}
		if strings.Contains(segment, ":") {
			continue
		}
		if segment != expected {
			return false
		}
	}
	return true
}

func classifyMiddleware(middleware []string) (string, error) {
	contains := func(identifier string) bool {
		for _, current := range middleware {
			if current == identifier {
				return true
			}
		}
		return false
	}

	switch {
	case contains("middleware.AppTokenAuth"):
		return "internal", nil
	case contains("middleware.NodeAPIKeyAuth"), contains("middleware.NodeAuth"):
		return "node-api", nil
	case contains("middleware.AdminAuth"):
		return "admin", nil
	case contains("middleware.JWTAuth"):
		return "user", nil
	case contains("publicLimiter.Middleware"):
		return "public", nil
	case contains("userLimiter.Middleware"):
		return "agent", nil
	case len(middleware) == 0:
		return "public", nil
	default:
		return "", fmt.Errorf("unrecognized /api/v2 middleware stack %q", middleware)
	}
}
