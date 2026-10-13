package config

import (
	"bytes"
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/rootkey"
	"gopkg.in/yaml.v3"
)

// RewriteOptions configure Rewrite.
type RewriteOptions struct {
	// Source names the old file in the new file's header.
	Source string
	// Now stamps the header.
	Now time.Time
	// ResolvePath resolves a relative path of the old file the way Control
	// resolves it next to that file (the SQLite database); nil keeps paths
	// as they are.
	ResolvePath func(string) string
	// RootKeyPath is root.key in the data directory, RootKey its content
	// (nil when there is none or it cannot be read, RootKeyProblem says
	// why): the secrets of the old file may be dropped only when root.key
	// holds them.
	RootKeyPath    string
	RootKey        *rootkey.File
	RootKeyProblem string
	// Force drops module_runtime.ca_kek and identity.kek even when root.key
	// does not hold them.
	Force bool
}

// RewriteNote is one line of the rewrite's report: an old key and what
// became of it.
type RewriteNote struct {
	Key    string
	To     string
	Value  string
	Reason string
}

// RewriteResult is the new file and the report.
type RewriteResult struct {
	Output []byte
	// Written are the new file's keys with where they came from.
	Written []RewriteNote
	// Kept are old keys the new file keeps as they were.
	Kept []RewriteNote
	// Dropped are the old keys the new file leaves out, with the reason.
	Dropped []RewriteNote
	// Notes say what the new file means for the running panel.
	Notes []string
	// Refused are the reasons the rewrite stops (unless Force): secrets
	// root.key does not hold yet.
	Refused []string
}

// Report is the rewrite's report: the new keys with where they came from,
// what is kept, every dropped key with the reason, and the notes. It never
// shows a secret: values are shown only for the operator keys.
func (r *RewriteResult) Report() string {
	var builder strings.Builder
	if len(r.Written) > 0 {
		builder.WriteString("\nNew keys:\n")
		for _, note := range r.Written {
			line := "  " + note.To + ": " + note.Value
			if note.Key != "" && note.Key != note.To {
				line += " (from " + note.Key + ")"
			}
			if note.Reason != "" {
				line += "; " + note.Reason
			}
			builder.WriteString(line + "\n")
		}
	}
	if len(r.Kept) > 0 {
		builder.WriteString("\nKept:\n")
		for _, note := range r.Kept {
			builder.WriteString("  " + note.Key + ": " + note.Reason + "\n")
		}
	}
	if len(r.Dropped) > 0 {
		builder.WriteString("\nDropped:\n")
		for _, note := range r.Dropped {
			builder.WriteString("  " + note.Key + ": " + note.Reason + "\n")
		}
	}
	if len(r.Notes)+len(r.Refused) > 0 {
		builder.WriteString("\nNotes:\n")
		for _, note := range r.Notes {
			builder.WriteString("  " + note + "\n")
		}
		for _, reason := range r.Refused {
			builder.WriteString("  refused without --force: " + reason + "\n")
		}
	}
	return builder.String()
}

// rewriteLeaf is one key of the old file.
type rewriteLeaf struct {
	path  string
	node  *yaml.Node
	known bool
}

// Rewrite turns an earlier configuration file into the operator keys
// (docs/design/single-binary/README.md, section 5, "升级时改写"):
//
//   - domain comes from domain, else the host of agent_install.public_url,
//     else tls.domain;
//   - grpc.port and subscribe_path (or app.subscribe_path) are kept only
//     when they are not the defaults;
//   - grpc.tls_cert_file and grpc.tls_key_file become tls.cert_file and
//     tls.key_file with tls.mode external when the old file did not use
//     ACME (it had no domain and no tls.mode acme); with ACME they are
//     dropped, since the panel's certificate serves gRPC too;
//   - the host of agent_install.grpc_target becomes grpc.public_name when
//     it is not domain (nodes keep dialling the same name);
//   - the database section is kept, with a relative SQLite path made
//     absolute, while PostgreSQL support exists (M2, P4): dropping it would
//     move the panel to another database; a closed or restricted
//     registration and a cluster name other than default are kept while
//     they are read (keptWhileRead);
//   - only values that differ from the defaults are written; every other
//     key is dropped with its reason.
//
// The old file's secrets go to root.key, not to the new file: the rewrite
// refuses (unless Force) to drop a module_runtime.ca_kek or identity.kek
// that root.key does not hold yet, since what they sealed could not be
// opened again. Start the new Control once with the old file and it adopts
// them.
func Rewrite(old []byte, opts RewriteOptions) (*RewriteResult, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(old, &document); err != nil {
		return nil, fmt.Errorf("the old configuration is not YAML: %w", err)
	}
	var leaves []rewriteLeaf
	if len(document.Content) > 0 {
		root := document.Content[0]
		switch {
		case root.Kind == yaml.MappingNode:
			collectRewriteLeaves(root, "", &leaves)
		case root.Kind == yaml.ScalarNode && root.Tag == "!!null":
		default:
			return nil, fmt.Errorf("the old configuration is not a mapping of keys")
		}
	}
	values := map[string]*yaml.Node{}
	for _, leaf := range leaves {
		values[leaf.path] = leaf.node
	}
	scalar := func(path string) string {
		node := values[path]
		if node == nil || node.Kind != yaml.ScalarNode || node.Tag == "!!null" {
			return ""
		}
		return strings.TrimSpace(node.Value)
	}

	result := &RewriteResult{}
	consumed := map[string]string{} // old key -> the new key it became
	defaults := Defaults()
	newValues := map[string]any{}
	written := func(key, from, reason string, value any) {
		newValues[key] = value
		result.Written = append(result.Written, RewriteNote{Key: from, To: key, Value: fmt.Sprint(value), Reason: reason})
		if from != "" {
			for _, source := range strings.Split(from, ", ") {
				consumed[source] = key
			}
		}
	}

	// domain.
	domain, domainFrom := "", ""
	for _, candidate := range []struct{ key, value string }{
		{"domain", scalar("domain")},
		{"agent_install.public_url", urlHost(scalar("agent_install.public_url"))},
		{"tls.domain", scalar("tls.domain")},
	} {
		name := normalizeHostName(candidate.value)
		if name == "" {
			continue
		}
		if !validDNSName(name) {
			result.Dropped = append(result.Dropped, RewriteNote{Key: candidate.key, Reason: fmt.Sprintf("%q is not a DNS name Control can obtain a certificate for: set domain by hand", name)})
			consumed[candidate.key] = "-"
			continue
		}
		domain, domainFrom = name, candidate.key
		break
	}
	if domain != "" {
		written("domain", domainFrom, "the panel's name", domain)
		for _, other := range []string{"domain", "agent_install.public_url", "tls.domain"} {
			if other != domainFrom && values[other] != nil {
				if _, done := consumed[other]; !done {
					consumed[other] = "domain"
					result.Dropped = append(result.Dropped, RewriteNote{Key: other, Reason: "domain comes from " + domainFrom})
				}
			}
		}
		if publicURL := scalar("agent_install.public_url"); publicURL != "" && domainFrom == "agent_install.public_url" {
			if parsed, err := url.Parse(publicURL); err == nil && (parsed.Port() != "" || strings.Trim(parsed.Path, "/") != "") {
				result.Dropped = append(result.Dropped, RewriteNote{Key: "agent_install.public_url", Reason: fmt.Sprintf("only its host became domain: nodes and browsers now use https://%s, without the port or path of %s", domain, publicURL)})
			}
		}
	}

	// tls.mode and the certificate files. A 4.x file has neither tls.mode
	// nor domain: its tls.cert_file served https on the web port only with
	// tls.enable, and its gRPC listener had certificate files of its own.
	oldMode := strings.ToLower(scalar("tls.mode"))
	fourX := oldMode == "" && normalizeHostName(scalar("domain")) == ""
	oldUsedACME := oldMode == TLSModeACME || (oldMode == "" && !fourX)
	mode := oldMode
	certFile, keyFile := scalar("tls.cert_file"), scalar("tls.key_file")
	certFrom, keyFrom := "tls.cert_file", "tls.key_file"
	enabled := strings.EqualFold(scalar("tls.enable"), "true")
	if values["tls.enable"] != nil {
		consumed["tls.enable"] = "-"
		reason := "TLS follows tls.mode"
		if enabled && fourX && certFile != "" && domain != "" {
			reason = "Control served https with tls.cert_file: tls.mode files serves it on 443 for domain"
		}
		result.Dropped = append(result.Dropped, RewriteNote{Key: "tls.enable", Reason: reason})
	}
	if fourX {
		switch {
		case enabled && certFile != "" && keyFile != "" && domain != "":
			mode = TLSModeFiles
			result.Notes = append(result.Notes, "tls.mode files: Control serves https with tls.cert_file on 443 (no longer on the web port) for "+domain)
		case certFile != "" || keyFile != "":
			for _, key := range []string{"tls.cert_file", "tls.key_file"} {
				if values[key] != nil {
					consumed[key] = "-"
					result.Dropped = append(result.Dropped, RewriteNote{Key: key, Reason: "not used: tls.enable was off (or there is no domain to serve)"})
				}
			}
			certFile, keyFile = "", ""
		}
	}
	grpcCert, grpcKey := scalar("grpc.tls_cert_file"), scalar("grpc.tls_key_file")
	switch {
	case grpcCert != "" && grpcKey != "" && !oldUsedACME && certFile == "":
		certFile, keyFile, certFrom, keyFrom = grpcCert, grpcKey, "grpc.tls_cert_file", "grpc.tls_key_file"
		if mode == "" {
			mode = TLSModeExternal
			result.Notes = append(result.Notes, "tls.mode external: the old file did not use ACME, so a reverse proxy keeps terminating TLS for the web side; its gRPC certificate becomes tls.cert_file")
		}
	case grpcCert != "" || grpcKey != "":
		reason := "the panel's ACME certificate serves gRPC too"
		if !oldUsedACME {
			reason = "tls.cert_file serves gRPC too"
		}
		for _, key := range []string{"grpc.tls_cert_file", "grpc.tls_key_file"} {
			if values[key] != nil {
				consumed[key] = "-"
				result.Dropped = append(result.Dropped, RewriteNote{Key: key, Reason: reason})
			}
		}
	}
	defaultMode := TLSModeExternal
	if domain != "" {
		defaultMode = TLSModeACME
	}
	if mode != "" && mode != defaultMode {
		written("tls.mode", "tls.mode", "", mode)
	} else if values["tls.mode"] != nil {
		consumed["tls.mode"] = "-"
		result.Dropped = append(result.Dropped, RewriteNote{Key: "tls.mode", Reason: "the default (" + defaultMode + ")"})
	}
	effectiveMode := mode
	if effectiveMode == "" {
		effectiveMode = defaultMode
	}
	if certFile != "" && keyFile != "" && (effectiveMode == TLSModeExternal || effectiveMode == TLSModeFiles) {
		written("tls.cert_file", certFrom, "", certFile)
		written("tls.key_file", keyFrom, "", keyFile)
	} else {
		for _, key := range []string{"tls.cert_file", "tls.key_file"} {
			if values[key] != nil {
				consumed[key] = "-"
				result.Dropped = append(result.Dropped, RewriteNote{Key: key, Reason: "tls.mode " + effectiveMode + " does not read certificate files"})
			}
		}
	}
	if effectiveMode == TLSModeACME {
		result.Notes = append(result.Notes, "tls.mode acme (the default with domain): Control serves https on 443 and http on 80 itself; stop the reverse proxy in front of it, or set tls.mode: external")
	}

	// listen.
	if listen := scalar("listen"); listen != "" {
		if effectiveMode == TLSModeExternal && listen != DefaultListenAddress {
			written("listen", "listen", "", listen)
		} else {
			consumed["listen"] = "-"
			result.Dropped = append(result.Dropped, RewriteNote{Key: "listen", Reason: "the default, or not read in tls.mode " + effectiveMode})
		}
	}
	if effectiveMode == TLSModeExternal {
		result.Notes = append(result.Notes, "tls.mode external: point the reverse proxy at "+firstNonEmpty(scalar("listen"), DefaultListenAddress)+" for the whole panel (web, /api, subscriptions, /install): it is one listener now")
	}

	// grpc.port and grpc.public_name.
	if raw := scalar("grpc.port"); raw != "" {
		port, err := strconv.Atoi(raw)
		switch {
		case err != nil || port < 1 || port > 65535:
			consumed["grpc.port"] = "-"
			result.Dropped = append(result.Dropped, RewriteNote{Key: "grpc.port", Reason: fmt.Sprintf("%q is not a port", raw)})
		case port != DefaultGRPCPort:
			written("grpc.port", "grpc.port", "", port)
		default:
			consumed["grpc.port"] = "-"
			result.Dropped = append(result.Dropped, RewriteNote{Key: "grpc.port", Reason: "the default (50051)"})
		}
	}
	publicName, publicFrom := normalizeHostName(scalar("grpc.public_name")), "grpc.public_name"
	if target := scalar("agent_install.grpc_target"); target != "" {
		host, port, err := net.SplitHostPort(target)
		host = normalizeHostName(strings.Trim(host, "[]"))
		reason := ""
		switch {
		case err != nil:
			reason = fmt.Sprintf("%q is not host:port", target)
		case publicName == "" && host != domain && validDNSName(host):
			publicName, publicFrom = host, "agent_install.grpc_target"
		case host != domain && host != publicName:
			reason = fmt.Sprintf("nodes now dial %s instead of %s", firstNonEmpty(publicName, domain), host)
		default:
			reason = "nodes dial grpc.public_name (or domain) and grpc.port"
		}
		wantPort := strconv.Itoa(DefaultGRPCPort)
		if value, ok := newValues["grpc.port"]; ok {
			wantPort = fmt.Sprint(value)
		}
		if err == nil && port != wantPort {
			portReason := fmt.Sprintf("its port %s is not grpc.port %s: nodes now dial port %s", port, wantPort, wantPort)
			if reason == "" {
				reason = portReason
			} else {
				reason += "; " + portReason
			}
		}
		if reason != "" {
			consumed["agent_install.grpc_target"] = "-"
			result.Dropped = append(result.Dropped, RewriteNote{Key: "agent_install.grpc_target", Reason: reason})
		}
	}
	if publicName != "" && publicName != domain {
		written("grpc.public_name", publicFrom, "", publicName)
	} else if values["grpc.public_name"] != nil {
		consumed["grpc.public_name"] = "-"
		result.Dropped = append(result.Dropped, RewriteNote{Key: "grpc.public_name", Reason: "the default (domain)"})
	}

	// subscribe_path.
	for _, key := range []string{"subscribe_path", "app.subscribe_path"} {
		raw := scalar(key)
		if raw == "" {
			continue
		}
		normalized, err := NormalizeSubscribePath(raw)
		switch {
		case err != nil:
			return nil, fmt.Errorf("%s %q: %w", key, raw, err)
		case normalized == "s":
			consumed[key] = "-"
			result.Dropped = append(result.Dropped, RewriteNote{Key: key, Reason: "the default (s)"})
		case newValues["subscribe_path"] == nil:
			written("subscribe_path", key, "keeps the subscription links already handed out", normalized)
		case newValues["subscribe_path"] != normalized:
			consumed[key] = "-"
			result.Dropped = append(result.Dropped, RewriteNote{Key: key, Reason: "subscribe_path of the old file wins"})
		default:
			consumed[key] = "subscribe_path"
		}
	}

	// data_dir.
	if raw := scalar("data_dir"); raw != "" {
		if filepath.Clean(raw) != defaults.DataDir {
			written("data_dir", "data_dir", "", raw)
		} else {
			consumed["data_dir"] = "-"
			result.Dropped = append(result.Dropped, RewriteNote{Key: "data_dir", Reason: "the default"})
		}
	}

	// The database section, while PostgreSQL support exists.
	var database []rewriteLeaf
	for _, leaf := range leaves {
		if strings.HasPrefix(leaf.path, "database.") && leaf.known {
			database = append(database, leaf)
		}
	}
	if len(database) > 0 {
		driver := strings.ToLower(scalar("database.driver"))
		sqlite := driver == "" || driver == "sqlite" || driver == "sqlite3"
		if sqlite {
			path := firstNonEmpty(scalar("database.database"), "config/data/v2board.db")
			if opts.ResolvePath != nil && !filepath.IsAbs(path) {
				path = opts.ResolvePath(path)
			}
			newValues["database.driver"] = "sqlite"
			newValues["database.database"] = path
		}
		for _, leaf := range database {
			consumed[leaf.path] = leaf.path
			if sqlite && (leaf.path == "database.driver" || leaf.path == "database.database") {
				continue
			}
			node := *leaf.node
			node.HeadComment, node.LineComment, node.FootComment = "", "", ""
			newValues[leaf.path] = &node
		}
		result.Kept = append(result.Kept, RewriteNote{Key: "database", Reason: "still read until PostgreSQL support ends (M2, P4): the panel keeps its database" +
			map[bool]string{true: " (" + fmt.Sprint(newValues["database.database"]) + ")", false: ""}[sqlite]})
	}

	// Keys still read whose dropping would weaken the panel or change the
	// nodes' identities are kept while they differ from the built-in value:
	// a closed or restricted registration, and the cluster in every Agent
	// identity.
	for _, leaf := range leaves {
		if !leaf.known || !keptWhileRead(leaf.path) {
			continue
		}
		key, _ := LookupKey(leaf.path)
		if key.Status != KeyRead {
			continue
		}
		field, ok := configField(defaults, key.fieldPath())
		if !ok {
			continue
		}
		value := reflect.New(field.Type())
		if leaf.node.Kind == yaml.ScalarNode && leaf.node.Tag == "!!null" {
			continue
		}
		if err := leaf.node.Decode(value.Interface()); err != nil || reflect.DeepEqual(value.Elem().Interface(), field.Interface()) ||
			(field.Kind() == reflect.Slice && value.Elem().Len() == 0 && field.Len() == 0) {
			continue
		}
		node := *leaf.node
		node.HeadComment, node.LineComment, node.FootComment = "", "", ""
		newValues[leaf.path] = &node
		consumed[leaf.path] = leaf.path
		result.Kept = append(result.Kept, RewriteNote{Key: leaf.path, Reason: "still read until " + key.RemovedIn + ", and not the built-in value: " + key.Reason})
	}

	// The secrets: root.key must hold them before the file may drop them.
	if raw := scalar("module_runtime.ca_kek"); raw != "" {
		consumed["module_runtime.ca_kek"] = "-"
		key, err := rootkey.ParseKEK(raw)
		switch {
		case err != nil:
			result.Dropped = append(result.Dropped, RewriteNote{Key: "module_runtime.ca_kek", Reason: "not a 32-byte key: " + err.Error()})
		case opts.RootKey != nil && opts.RootKey.Key.Equal(key):
			result.Dropped = append(result.Dropped, RewriteNote{Key: "module_runtime.ca_kek", Reason: "the root key, kept in " + opts.RootKeyPath})
		default:
			result.Refused = append(result.Refused, fmt.Sprintf("module_runtime.ca_kek is not the key in %s%s. Start this Control once with the old file: it adopts the key into root.key. "+
				"Dropping it (--force) leaves the CA, the ACME keys, DNS credentials and join tokens it sealed unreadable", opts.RootKeyPath, rootKeyProblem(opts)))
			result.Dropped = append(result.Dropped, RewriteNote{Key: "module_runtime.ca_kek", Reason: "the root key: not in root.key yet"})
		}
	}
	if raw := scalar("identity.kek"); raw != "" {
		consumed["identity.kek"] = "-"
		kek, err := rootkey.ParseKEK(raw)
		switch {
		case err != nil:
			result.Dropped = append(result.Dropped, RewriteNote{Key: "identity.kek", Reason: "not a 32-byte key: " + err.Error()})
		case opts.RootKey != nil && opts.RootKey.AdoptedIdentityKEK == kek.Base64():
			result.Dropped = append(result.Dropped, RewriteNote{Key: "identity.kek", Reason: "adopted, kept in " + opts.RootKeyPath})
		default:
			result.Refused = append(result.Refused, fmt.Sprintf("identity.kek is not adopted in %s%s. Start this Control once with the old file: it records the KEK in root.key. "+
				"Dropping it (--force) leaves the identity signing keys it sealed unreadable", opts.RootKeyPath, rootKeyProblem(opts)))
			result.Dropped = append(result.Dropped, RewriteNote{Key: "identity.kek", Reason: "not adopted in root.key yet"})
		}
	}
	if raw := scalar("jwt.secret"); raw != "" {
		consumed["jwt.secret"] = "-"
		reason := "derived from the root key from now on: every user signs in again once"
		switch {
		case PlaceholderJWTSecret(raw):
			reason = "a template value: derived from the root key from now on"
		case opts.RootKey != nil && opts.RootKey.AdoptedJWTSecret == raw:
			reason = "adopted, kept in " + opts.RootKeyPath
		}
		result.Dropped = append(result.Dropped, RewriteNote{Key: "jwt.secret", Reason: reason})
	}

	// Every other old key is dropped.
	for _, leaf := range leaves {
		if _, done := consumed[leaf.path]; done {
			continue
		}
		reason := "unknown key: Control never read it"
		if key, ok := LookupKey(leaf.path); ok {
			switch {
			case !key.Deprecated():
				reason = "the default"
			case key.Status == KeyRead:
				reason = "still read until " + key.RemovedIn + ", not written: " + key.Reason
			default:
				reason = key.Reason
			}
		}
		result.Dropped = append(result.Dropped, RewriteNote{Key: leaf.path, Reason: reason})
	}
	sort.SliceStable(result.Dropped, func(i, j int) bool { return result.Dropped[i].Key < result.Dropped[j].Key })

	output, err := marshalRewrite(newValues, opts)
	if err != nil {
		return nil, err
	}
	result.Output = output
	if _, _, err := loadBytes(output); err != nil {
		return nil, fmt.Errorf("the rewritten configuration does not load: %w", err)
	}
	return result, nil
}

func rootKeyProblem(opts RewriteOptions) string {
	if opts.RootKeyProblem != "" {
		return " (" + opts.RootKeyProblem + ")"
	}
	return ""
}

// collectRewriteLeaves lists the keys of an old file: known keys (with a
// map-valued key or a subtree as one leaf) and unknown ones.
func collectRewriteLeaves(node *yaml.Node, prefix string, leaves *[]rewriteLeaf) {
	for i := 0; i+1 < len(node.Content); i += 2 {
		name, value := node.Content[i].Value, node.Content[i+1]
		if value.Kind == yaml.AliasNode && value.Alias != nil {
			value = value.Alias
		}
		path := name
		if prefix != "" {
			path = prefix + "." + name
		}
		if _, known := LookupKey(path); known {
			*leaves = append(*leaves, rewriteLeaf{path: path, node: value, known: true})
			continue
		}
		if value.Kind == yaml.MappingNode && registry.section[path] {
			collectRewriteLeaves(value, path, leaves)
			continue
		}
		if value.Kind == yaml.ScalarNode && value.Tag == "!!null" && registry.section[path] {
			continue
		}
		*leaves = append(*leaves, rewriteLeaf{path: path, node: value})
	}
}

// urlHost returns the host of an https URL, "" when there is none.
func urlHost(raw string) string {
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// keptWhileRead tells which keys still read the rewrite keeps when they
// differ from the built-in value (besides the database section).
func keptWhileRead(path string) bool {
	return strings.HasPrefix(path, "auth.registration.") || path == "module_runtime.cluster"
}

// rewriteOrder is the order of the new file's keys.
var rewriteOrder = []string{"domain", "tls.mode", "tls.cert_file", "tls.key_file", "listen", "grpc.port", "grpc.public_name", "subscribe_path", "data_dir"}

// marshalRewrite writes the new file: a header, the operator keys in their
// order, then the database section.
func marshalRewrite(values map[string]any, opts RewriteOptions) ([]byte, error) {
	root := &yaml.Node{Kind: yaml.MappingNode}
	section := func(parent *yaml.Node, name string) *yaml.Node {
		for i := 0; i+1 < len(parent.Content); i += 2 {
			if parent.Content[i].Value == name {
				return parent.Content[i+1]
			}
		}
		child := &yaml.Node{Kind: yaml.MappingNode}
		parent.Content = append(parent.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: name}, child)
		return child
	}
	add := func(path string, value any) error {
		parts := strings.Split(path, ".")
		parent := root
		for _, part := range parts[:len(parts)-1] {
			parent = section(parent, part)
		}
		var node *yaml.Node
		if existing, ok := value.(*yaml.Node); ok {
			node = existing
		} else {
			node = &yaml.Node{}
			if err := node.Encode(value); err != nil {
				return err
			}
		}
		parent.Content = append(parent.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: parts[len(parts)-1]}, node)
		return nil
	}
	for _, path := range rewriteOrder {
		if value, ok := values[path]; ok {
			if err := add(path, value); err != nil {
				return nil, err
			}
		}
	}
	var kept []string
	for path := range values {
		if strings.HasPrefix(path, "database.") || keptWhileRead(path) {
			kept = append(kept, path)
		}
	}
	sort.Strings(kept)
	for _, path := range kept {
		if err := add(path, values[path]); err != nil {
			return nil, err
		}
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		switch root.Content[i].Value {
		case "database":
			root.Content[i].HeadComment = "Still read until PostgreSQL support ends (M2, P4): the panel keeps its\ndatabase (a PostgreSQL password stays here until then)."
		case "auth":
			root.Content[i].HeadComment = "Still read until the registration policy becomes an administrator\nsetting (M3): the panel keeps its registration rules."
		case "module_runtime":
			root.Content[i].HeadComment = "Still read until M2 (P4): the cluster in every Agent and module identity."
		}
	}

	var buffer bytes.Buffer
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	_, _ = fmt.Fprintf(&buffer, "# AnixOps Control configuration, written by `anix-control config rewrite`\n# from %s on %s.\n", firstNonEmpty(opts.Source, "an earlier file"), now.UTC().Format(time.RFC3339))
	buffer.WriteString("# Keys and defaults: docs/reference/configuration.md. The root key and the\n# secrets it adopted are in data_dir/root.key, not in this file.\n")
	if len(root.Content) == 0 {
		buffer.WriteString("# Every key has its default.\n")
		return buffer.Bytes(), nil
	}
	encoder := yaml.NewEncoder(&buffer)
	encoder.SetIndent(2)
	if err := encoder.Encode(root); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// loadBytes loads a configuration from memory with no environment, as
// Control would load the file.
func loadBytes(data []byte) (*Config, LoadReport, error) {
	l := &loader{cfg: Defaults(), deprecated: map[string]*DeprecatedKey{}}
	l.cfg.Database = DatabaseConfig{}
	if err := l.applyYAML(data, "the rewritten configuration"); err != nil {
		return nil, LoadReport{}, err
	}
	report := l.report()
	l.cfg.Deprecated = DeprecatedConfig{keys: report.Deprecated}
	return l.cfg, report, l.cfg.validateLoaded()
}
