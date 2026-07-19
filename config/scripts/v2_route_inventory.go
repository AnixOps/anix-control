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

type route struct {
	Method          string `json:"method"`
	Path            string `json:"path"`
	Handler         string `json:"handler"`
	MiddlewareGroup string `json:"middleware_group"`
	Transport       string `json:"transport"`
}

type group struct {
	base       string
	middleware []string
}

type scope struct {
	parent       *scope
	groups       map[string]*group
	stringValues map[string]string
}

func newScope(parent *scope) *scope {
	return &scope{parent: parent, groups: make(map[string]*group), stringValues: make(map[string]string)}
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

type collector struct {
	fset   *token.FileSet
	routes []route
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

	collector := collector{fset: fset}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Body == nil {
			continue
		}

		env := newScope(nil)
		if !seedEngineParameters(function, env) {
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

func seedEngineParameters(function *ast.FuncDecl, env *scope) bool {
	if function.Type.Params == nil {
		return false
	}

	found := false
	for _, field := range function.Type.Params.List {
		if !isGinEngine(field.Type) {
			continue
		}
		for _, name := range field.Names {
			env.define(name.Name, &group{})
			found = true
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
	switch node := statement.(type) {
	case *ast.BlockStmt:
		return c.processBlock(node, env)
	case *ast.DeclStmt:
		return c.processDeclaration(node.Decl, env)
	case *ast.AssignStmt:
		return c.processAssignment(node, env)
	case *ast.ExprStmt:
		return c.processExpression(node.X, env)
	case *ast.IfStmt:
		branchEnv := newScope(env)
		if node.Init != nil {
			if err := c.processStatement(node.Init, branchEnv); err != nil {
				return err
			}
		}
		if err := c.processBlock(node.Body, branchEnv); err != nil {
			return err
		}
		if node.Else != nil {
			return c.processStatement(node.Else, branchEnv)
		}
	case *ast.ForStmt:
		loopEnv := newScope(env)
		if node.Init != nil {
			if err := c.processStatement(node.Init, loopEnv); err != nil {
				return err
			}
		}
		return c.processBlock(node.Body, loopEnv)
	case *ast.RangeStmt:
		return c.processBlock(node.Body, env)
	case *ast.SwitchStmt:
		if node.Init != nil {
			if err := c.processStatement(node.Init, env); err != nil {
				return err
			}
		}
		for _, clause := range node.Body.List {
			caseClause, ok := clause.(*ast.CaseClause)
			if !ok {
				continue
			}
			caseEnv := newScope(env)
			for _, bodyStatement := range caseClause.Body {
				if err := c.processStatement(bodyStatement, caseEnv); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (c *collector) processDeclaration(declaration ast.Decl, env *scope) error {
	general, ok := declaration.(*ast.GenDecl)
	if !ok || general.Tok != token.VAR {
		return nil
	}
	for _, specification := range general.Specs {
		value, ok := specification.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for index, name := range value.Names {
			if index >= len(value.Values) {
				continue
			}
			expression := value.Values[index]
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
	for index, left := range assignment.Lhs {
		if index >= len(assignment.Rhs) {
			continue
		}
		name, ok := left.(*ast.Ident)
		if !ok {
			continue
		}
		expression := assignment.Rhs[index]
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

func (c *collector) processExpression(expression ast.Expr, env *scope) error {
	call, ok := expression.(*ast.CallExpr)
	if !ok {
		return nil
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	target, err := c.resolveGroup(selector.X, env)
	if err != nil || target == nil {
		return err
	}

	switch selector.Sel.Name {
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
	default:
		return nil
	}
}

func (c *collector) resolveGroup(expression ast.Expr, env *scope) (*group, error) {
	switch node := expression.(type) {
	case *ast.Ident:
		value, _ := env.lookup(node.Name)
		return value, nil
	case *ast.ParenExpr:
		return c.resolveGroup(node.X, env)
	case *ast.CallExpr:
		selector, ok := node.Fun.(*ast.SelectorExpr)
		if !ok {
			return nil, nil
		}
		parent, err := c.resolveGroup(selector.X, env)
		if err != nil || parent == nil {
			return parent, err
		}
		switch selector.Sel.Name {
		case "Group":
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
			if parent.base != "" {
				for _, middleware := range node.Args {
					parent.middleware = append(parent.middleware, expressionIdentifier(middleware))
				}
			}
			return parent, nil
		default:
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

func (c *collector) registerArguments(target *group, method string, args []ast.Expr, position token.Pos, env *scope) error {
	if len(args) < 2 {
		return c.errorAt(position, "Gin %s registration must include a path and handler", method)
	}
	relativePath, ok := resolveString(args[0], env)
	if !ok {
		if isV2Group(target) {
			return c.errorAt(args[0].Pos(), "Gin %s path under /api/v2 must be a string literal", method)
		}
		return nil
	}
	fullPath := joinPath(target.base, relativePath)
	if !isV2Path(fullPath) {
		return nil
	}

	routeMiddleware := append([]string(nil), target.middleware...)
	for _, middleware := range args[1 : len(args)-1] {
		routeMiddleware = append(routeMiddleware, expressionIdentifier(middleware))
	}
	middlewareGroup, err := classifyMiddleware(routeMiddleware)
	if err != nil {
		return c.errorAt(position, "%w", err)
	}
	handler := expressionIdentifier(args[len(args)-1])
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
		})
	}
	return nil
}

func isHTTPMethod(method string) bool {
	for _, candidate := range anyMethods {
		if method == candidate {
			return true
		}
	}
	return false
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
