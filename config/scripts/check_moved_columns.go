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
//   - reading a moved field of a model row: model.WireGuardPeer's
//     PrivateKey and PresharedKey. Writing one (the
//     left side of an assignment, a composite literal) is the writers' part
//     and is allowed: they call nodesecrets.Sync in the same transaction;
//   - naming a moved column in the SQL of a gorm call (Where, Select, Pluck,
//     Order, Raw and the like): private_key and preshared_key.
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
	"WireGuardPeer": {"PrivateKey": "private_key", "PresharedKey": "preshared_key"},
}

var modelTables = map[string]string{"WireGuardPeer": "v2_wireguard_peer"}

// movedColumns are refused in any gorm SQL.
var movedColumns = []string{"private_key", "preshared_key"}

// sqlMethods are the gorm methods whose string arguments are SQL that
// reads.
var sqlMethods = map[string]bool{
	"Where": true, "Or": true, "Not": true, "Select": true, "Pluck": true, "Order": true, "Group": true,
	"Having": true, "Joins": true, "Raw": true, "Distinct": true,
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
		ast.Inspect(decl, func(node ast.Node) bool {
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
					for _, column := range sqlColumns(text) {
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

// sqlColumns answers the moved columns text names as SQL identifiers.
func sqlColumns(text string) []string {
	var found []string
	for _, column := range movedColumns {
		if columnPattern(column).MatchString(text) {
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
