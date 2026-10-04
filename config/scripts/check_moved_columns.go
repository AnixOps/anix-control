//go:build ignore

// check_moved_columns is the static gate of the node credential split
// (docs/architecture/node-ops-service.md, sections 4.3 and 9): it refuses a
// kernel read of a moved credential column outside internal/nodesecrets.
//
// A finalized table's legacy columns hold tombstones, so a kernel reader
// that reads them directly fails (closed) once the table is finalized; one
// that reads them before finalize skips the split's phase rule. Every read
// goes through internal/nodesecrets, which applies the phase.
//
// What it refuses, in the non-test Go code of ./internal/... and ./cmd/...
// (type-checked):
//
//   - reading a moved field of a model row: model.Node's APIKey, APIKeyHash
//     and Secret; model.AuthorizedKey's Key and KeyHash;
//     model.ForwardNode's APIToken; model.ForwardCleanAgent's Token;
//     model.WireGuardPeer's PrivateKey and PresharedKey. Writing one (the
//     left side of an assignment, a composite literal) is the writers' part
//     and is allowed: they call nodesecrets.Sync in the same transaction;
//   - naming a moved column in the SQL of a gorm call (Where, Select, Pluck,
//     Order, Raw and the like): api_key, api_key_hash, api_token, key_hash,
//     private_key and preshared_key anywhere; secret, key and token where
//     the call's chain reads v2_node, v2_authorized_key or
//     v2_forward_clean_agent.
//
// Allowed: internal/nodesecrets itself, internal/model (the definitions),
// cmd/migrate and cmd/sqlite2postgres (they copy whole rows between
// databases, then call the writer or prune), tests, and the reasoned
// entries of allowed below.
//
// The JSON settings columns (v2_node.raw_config, v2_node_protocol's five
// settings columns) are not checked: their non-secret parts are read
// everywhere, and their secret positions are resolved by
// nodesecrets.ResolveProtocols and ResolveNodeRawConfig, which a finalized
// table answers from v4_kernel_protocol_secret only.
//
// Usage: go run ./config/scripts/check_moved_columns.go [-root dir] [-allowlist=false]
package main

import (
	"bufio"
	"flag"
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// allowed are reads outside internal/nodesecrets, by "file function", with
// the reason each is safe. A function literal counts as its enclosing
// function.
var allowed = map[string]string{
	"internal/handler/node.go CreateNode": "answers the key and secret the writer just generated, from memory, " +
		"once (as the legacy route always did)",
	"internal/agentpki/enrollment.go authenticateNodeCredential": "selects api_token only to hand the row to " +
		"nodesecrets.ForwardNodeTokenMatches, which applies the phase",
	"internal/service/agent_pki_hooks.go forwardNodeAgentRevocation": "selects api_token only to hand the row to " +
		"nodesecrets.ForwardNodeToken; compares the incoming row's value with it, a tombstone counting as unchanged",
	"internal/service/node_credential_ops.go ProxyNodeHasCredentials": "selects the columns only to hand the row to " +
		"nodesecrets.NodeAPIKey and NodeSharedSecret",
	"internal/service/node_credential_ops.go ForwardNodeHasToken": "selects api_token only to hand the row to " +
		"nodesecrets.ForwardNodeToken",
	"internal/service/node_credential_ops.go IssueForwardNodeTokenTx": "selects api_token only to hand the row to " +
		"nodesecrets.ForwardNodeToken",
	"internal/service/node_secrets.go MaskAuthorizedKeys": "masks the column for an answer: any value, a tombstone " +
		"too, is shown as the placeholder",
	"internal/service/wireguard_peer.go applyWireGuardPeer": "the peer comes from GetOrCreateWireGuardPeer, which " +
		"resolves its keys through nodesecrets.ResolveWireGuardPeers or generated them",
	"internal/service/wireguard_peer.go BuildWireGuardRuntimeUserExtras": "the peer comes from " +
		"GetOrCreateWireGuardPeer, which resolves its keys through nodesecrets.ResolveWireGuardPeers or generated them",
}

// exemptPrefixes are package directories (relative to the module root)
// whose reads are allowed.
var exemptPrefixes = []string{"internal/nodesecrets", "internal/model", "cmd/migrate", "cmd/sqlite2postgres"}

// movedFields are the moved fields by model type, with their columns.
var movedFields = map[string]map[string]string{
	"Node":              {"APIKey": "api_key", "APIKeyHash": "api_key_hash", "Secret": "secret"},
	"AuthorizedKey":     {"Key": "key", "KeyHash": "key_hash"},
	"ForwardNode":       {"APIToken": "api_token"},
	"ForwardCleanAgent": {"Token": "token"},
	"WireGuardPeer":     {"PrivateKey": "private_key", "PresharedKey": "preshared_key"},
}

var modelTables = map[string]string{
	"Node": "v2_node", "AuthorizedKey": "v2_authorized_key", "ForwardNode": "v2_forward_node",
	"ForwardCleanAgent": "v2_forward_clean_agent", "WireGuardPeer": "v2_wireguard_peer",
}

// distinctiveColumns are refused in any gorm SQL; genericColumns only where
// the chain reads their table.
var (
	distinctiveColumns = []string{"api_key", "api_key_hash", "api_token", "key_hash", "private_key", "preshared_key"}
	genericColumns     = map[string]string{"secret": "v2_node", "key": "v2_authorized_key", "token": "v2_forward_clean_agent"}
)

// sqlMethods are the gorm methods whose string arguments are SQL that
// reads.
var sqlMethods = map[string]bool{
	"Where": true, "Or": true, "Not": true, "Select": true, "Pluck": true, "Order": true, "Group": true,
	"Having": true, "Joins": true, "Raw": true, "Distinct": true,
}

// chainModelMethods take a destination or model whose type names the table.
var chainModelMethods = map[string]bool{
	"Model": true, "First": true, "Find": true, "Take": true, "Last": true, "Scan": true, "FirstOrCreate": true,
	"FirstOrInit": true, "Delete": true, "FindInBatches": true,
}

// used records the allowed entries a read matched; an entry that matches
// none is stale and fails the gate.
var used = map[string]bool{}

type finding struct {
	position string
	message  string
}

func main() {
	root := flag.String("root", ".", "module root")
	useAllowed := flag.Bool("allowlist", true, "apply the allowed entries, and fail on a stale one (false for a test tree)")
	flag.Parse()
	if !*useAllowed {
		allowed = map[string]string{}
	}
	absolute, err := filepath.Abs(*root)
	if err != nil {
		fail(err)
	}
	module, err := modulePath(filepath.Join(absolute, "go.mod"))
	if err != nil {
		fail(err)
	}
	config := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo,
		Dir:  absolute,
		Env:  append(os.Environ(), "GOWORK=off"),
	}
	var patterns []string
	for _, dir := range []string{"internal", "cmd"} {
		if info, err := os.Stat(filepath.Join(absolute, dir)); err == nil && info.IsDir() {
			patterns = append(patterns, "./"+dir+"/...")
		}
	}
	loaded, err := packages.Load(config, patterns...)
	if err != nil {
		fail(err)
	}
	var findings []finding
	broken := false
	checked := 0
	for _, pkg := range loaded {
		for _, problem := range pkg.Errors {
			fmt.Fprintln(os.Stderr, problem)
			broken = true
		}
		relative := strings.TrimPrefix(strings.TrimPrefix(pkg.PkgPath, module), "/")
		if exempt(relative) {
			continue
		}
		checked++
		for _, file := range pkg.Syntax {
			name := pkg.Fset.Position(file.Pos()).Filename
			if strings.HasSuffix(name, "_test.go") {
				continue
			}
			rel, err := filepath.Rel(absolute, name)
			if err != nil {
				rel = name
			}
			findings = append(findings, checkFile(pkg, file, filepath.ToSlash(rel), module+"/internal/model")...)
		}
	}
	if broken {
		fmt.Fprintln(os.Stderr, "moved column gate: the packages do not type-check")
		os.Exit(2)
	}
	for entry := range allowed {
		if !used[entry] {
			findings = append(findings, finding{position: "config/scripts/check_moved_columns.go",
				message: fmt.Sprintf("the allowed entry %q matches no read; remove it", entry)})
		}
	}
	sort.Slice(findings, func(i, j int) bool { return findings[i].position < findings[j].position })
	for _, item := range findings {
		fmt.Fprintf(os.Stderr, "%s: %s\n", item.position, item.message)
	}
	if len(findings) > 0 {
		fmt.Fprintf(os.Stderr, "moved column gate: %d kernel read(s) of a moved credential column outside internal/nodesecrets; "+
			"read through internal/nodesecrets, or add a reasoned entry to config/scripts/check_moved_columns.go\n", len(findings))
		os.Exit(1)
	}
	fmt.Printf("moved column gate passed (%d packages)\n", checked)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "moved column gate:", err)
	os.Exit(2)
}

func modulePath(goMod string) (string, error) {
	file, err := os.Open(goMod) // #nosec G304 -- the gate reads the go.mod of the tree it checks.
	if err != nil {
		return "", err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module ")), nil
		}
	}
	return "", fmt.Errorf("%s names no module", goMod)
}

func exempt(relative string) bool {
	for _, prefix := range exemptPrefixes {
		if relative == prefix || strings.HasPrefix(relative, prefix+"/") {
			return true
		}
	}
	return false
}

func checkFile(pkg *packages.Package, file *ast.File, rel, modelPath string) []finding {
	info := pkg.TypesInfo
	// Assignment targets are writes.
	written := map[ast.Expr]bool{}
	ast.Inspect(file, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.AssignStmt:
			for _, lhs := range typed.Lhs {
				written[lhs] = true
			}
		case *ast.IncDecStmt:
			written[typed.X] = true
		}
		return true
	})
	var findings []finding
	report := func(function string, pos token.Pos, message string) {
		if _, ok := allowed[rel+" "+function]; ok {
			used[rel+" "+function] = true
			return
		}
		findings = append(findings, finding{position: pkg.Fset.Position(pos).String(), message: message})
	}
	for _, decl := range file.Decls {
		function := ""
		if fn, ok := decl.(*ast.FuncDecl); ok {
			function = fn.Name.Name
		}
		parents := map[ast.Node]ast.Node{}
		var stack []ast.Node
		ast.Inspect(decl, func(node ast.Node) bool {
			if node == nil {
				stack = stack[:len(stack)-1]
				return true
			}
			if len(stack) > 0 {
				parents[node] = stack[len(stack)-1]
			}
			stack = append(stack, node)
			switch typed := node.(type) {
			case *ast.SelectorExpr:
				if written[typed] {
					return true
				}
				if typeName, column, ok := movedSelection(info, typed, modelPath); ok {
					report(function, typed.Sel.Pos(), fmt.Sprintf("reads %s.%s (model.%s.%s) outside internal/nodesecrets",
						modelTables[typeName], column, typeName, typed.Sel.Name))
				}
			case *ast.CallExpr:
				method, ok := gormMethod(info, typed)
				if !ok || !sqlMethods[method] {
					return true
				}
				for _, arg := range typed.Args {
					text, ok := constantString(info, arg)
					if !ok {
						continue
					}
					for _, column := range sqlColumns(text, chainTables(info, typed, parents, modelPath)) {
						report(function, arg.Pos(), fmt.Sprintf("names the moved column %s in the SQL of %s outside internal/nodesecrets", column, method))
					}
				}
			}
			return true
		})
	}
	return findings
}

// movedSelection reports whether sel reads a moved field of a model type.
func movedSelection(info *types.Info, sel *ast.SelectorExpr, modelPath string) (string, string, bool) {
	selection, ok := info.Selections[sel]
	if !ok || selection.Kind() != types.FieldVal {
		return "", "", false
	}
	typeName, ok := modelType(selection.Recv(), modelPath)
	if !ok {
		return "", "", false
	}
	column, ok := movedFields[typeName][sel.Sel.Name]
	return typeName, column, ok
}

// modelType answers the name of a model type, through pointers, slices and
// arrays.
func modelType(t types.Type, modelPath string) (string, bool) {
	for {
		switch typed := t.(type) {
		case *types.Pointer:
			t = typed.Elem()
			continue
		case *types.Slice:
			t = typed.Elem()
			continue
		case *types.Array:
			t = typed.Elem()
			continue
		case *types.Named:
			object := typed.Obj()
			if object.Pkg() == nil || object.Pkg().Path() != modelPath {
				return "", false
			}
			_, ok := movedFields[object.Name()]
			return object.Name(), ok
		}
		return "", false
	}
}

// gormMethod answers the name of a method called on a *gorm.DB.
func gormMethod(info *types.Info, call *ast.CallExpr) (string, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	selection, ok := info.Selections[sel]
	if !ok || selection.Kind() != types.MethodVal {
		return "", false
	}
	recv := selection.Recv()
	if pointer, ok := recv.(*types.Pointer); ok {
		recv = pointer.Elem()
	}
	named, ok := recv.(*types.Named)
	if !ok || named.Obj().Pkg() == nil || named.Obj().Pkg().Path() != "gorm.io/gorm" || named.Obj().Name() != "DB" {
		return "", false
	}
	return sel.Sel.Name, true
}

func constantString(info *types.Info, expr ast.Expr) (string, bool) {
	value, ok := info.Types[expr]
	if !ok || value.Value == nil || value.Value.Kind() != constant.String {
		return "", false
	}
	return constant.StringVal(value.Value), true
}

// chainTables answers the tables a gorm call chain reads: the types given
// to Model, First, Find and the like, and Table names, along the whole
// chain the call is part of.
func chainTables(info *types.Info, call *ast.CallExpr, parents map[ast.Node]ast.Node, modelPath string) map[string]bool {
	// Climb to the outermost call of the chain.
	top := ast.Expr(call)
	for {
		sel, ok := parents[top].(*ast.SelectorExpr)
		if !ok || sel.X != top {
			break
		}
		outer, ok := parents[sel].(*ast.CallExpr)
		if !ok || outer.Fun != sel {
			break
		}
		top = outer
	}
	tables := map[string]bool{}
	for expr := top; ; {
		outer, ok := expr.(*ast.CallExpr)
		if !ok {
			break
		}
		sel, ok := outer.Fun.(*ast.SelectorExpr)
		if !ok {
			break
		}
		switch {
		case sel.Sel.Name == "Table" && len(outer.Args) > 0:
			if name, ok := constantString(info, outer.Args[0]); ok {
				tables[strings.Fields(name + " ")[0]] = true
			}
		case chainModelMethods[sel.Sel.Name] && len(outer.Args) > 0:
			if t := info.TypeOf(outer.Args[0]); t != nil {
				if name, ok := modelType(t, modelPath); ok {
					tables[modelTables[name]] = true
				}
			}
		}
		expr = sel.X
	}
	return tables
}

// sqlColumns answers the moved columns text names as SQL identifiers.
func sqlColumns(text string, tables map[string]bool) []string {
	var found []string
	for _, column := range distinctiveColumns {
		if columnPattern(column).MatchString(text) {
			found = append(found, column)
		}
	}
	for column, table := range genericColumns {
		if tables[table] && columnPattern(column).MatchString(text) {
			found = append(found, column)
		}
	}
	sort.Strings(found)
	return found
}

var columnPatterns = map[string]*regexp.Regexp{}

// columnPattern matches a column as an identifier, bare, quoted or
// qualified, but not inside a longer identifier or a string value.
func columnPattern(column string) *regexp.Regexp {
	if pattern, ok := columnPatterns[column]; ok {
		return pattern
	}
	pattern := regexp.MustCompile(`(?i)(^|[^a-z0-9_'])["` + "`" + `]?` + regexp.QuoteMeta(column) + `["` + "`" + `]?($|[^a-z0-9_'])`)
	columnPatterns[column] = pattern
	return pattern
}
