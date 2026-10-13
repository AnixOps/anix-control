package config

import (
	_ "embed"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// KeyStatus says what Control does with a configuration key.
type KeyStatus string

const (
	// KeyOperator: an operator key, documented and applied.
	KeyOperator KeyStatus = "operator"
	// KeyRead: a deprecated key that is still applied, with a warning, until
	// the milestone that removes its feature (RemovedIn).
	KeyRead KeyStatus = "read"
	// KeyIgnored: a deprecated key that is warned about once and not
	// applied; the built-in value stands.
	KeyIgnored KeyStatus = "ignored"
)

// Key is one configuration key: an operator key, or a deprecated one from
// deprecated_keys.yaml.
type Key struct {
	// Path is the dotted YAML path, for example tls.mode.
	Path string `yaml:"key"`
	// Status is KeyOperator, KeyRead or KeyIgnored.
	Status KeyStatus `yaml:"status"`
	// RemovedIn is the milestone that removes a deprecated key's reader.
	RemovedIn string `yaml:"removed_in"`
	// Reason says why a key is deprecated; Description what an operator
	// key does.
	Reason      string `yaml:"reason"`
	Description string `yaml:"-"`
	// Replacement is the operator key that takes over, if any.
	Replacement string `yaml:"replacement"`
	// Subtree: the key and everything below it (a section a template had).
	Subtree bool `yaml:"subtree"`
	// Secret: the value is never printed.
	Secret bool `yaml:"secret"`

	// field is the Config field's YAML path when it is not Path
	// (subscribe_path is App.SubscribePath).
	field string
}

// EnvName is the key's ANIX_CONTROL_* variable.
func (k Key) EnvName() string {
	return EnvPrefix + strings.ToUpper(strings.ReplaceAll(k.Path, ".", "_"))
}

// fieldPath is the YAML path of the Config field the key sets.
func (k Key) fieldPath() string {
	if k.field != "" {
		return k.field
	}
	return k.Path
}

// Deprecated tells whether the key is not an operator key.
func (k Key) Deprecated() bool { return k.Status != KeyOperator }

// operatorKeys are the configuration an operator writes (the design's
// section 5): an ordinary install sets domain alone.
var operatorKeys = []Key{
	{Path: "domain", Description: "The panel's name: the web, API, subscription and install address (https://<domain>), the name of the certificate Control obtains (tls.mode acme) and the name nodes dial for gRPC. Required in tls.mode acme."},
	{Path: "tls.mode", Description: "acme (default with domain): Control obtains its certificate and serves https on 443 and http on 80. external: a reverse proxy or CDN terminates TLS and sends plain http to listen. files: Control serves https on 443 with tls.cert_file."},
	{Path: "tls.cert_file", Description: "PEM certificate: the gRPC listener's in tls.mode external (reloaded when it changes), the web certificate in tls.mode files."},
	{Path: "tls.key_file", Description: "The private key of tls.cert_file."},
	{Path: "listen", Description: "tls.mode external only: the plain http address the reverse proxy sends the panel's traffic to."},
	{Path: "grpc.port", Description: "The port of the node gRPC listener; nodes dial <grpc.public_name or domain>:<grpc.port>."},
	{Path: "grpc.public_name", Description: "Only when domain sits behind a CDN: another name that reaches this machine directly, for the nodes' gRPC (mutual TLS cannot cross a CDN)."},
	{Path: "subscribe_path", Description: "The subscription path (/<subscribe_path>/:token). Change it only to keep links already handed out working.", field: "app.subscribe_path"},
	{Path: "data_dir", Description: "The data directory: the root key (root.key), the backups (backups/) and, later, the database. Absolute."},
}

//go:embed deprecated_keys.yaml
var deprecatedKeysYAML []byte

// keyRegistry is every key Control knows, by path.
type keyRegistry struct {
	keys    []Key
	byPath  map[string]Key
	byEnv   map[string]Key
	section map[string]bool
}

var registry = mustBuildRegistry()

func mustBuildRegistry() *keyRegistry {
	built, err := buildRegistry(deprecatedKeysYAML)
	if err != nil {
		panic(fmt.Sprintf("config: deprecated_keys.yaml: %v", err))
	}
	return built
}

func buildRegistry(data []byte) (*keyRegistry, error) {
	var document struct {
		Keys []Key `yaml:"keys"`
	}
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&document); err != nil {
		return nil, err
	}
	r := &keyRegistry{byPath: map[string]Key{}, byEnv: map[string]Key{}, section: map[string]bool{}}
	add := func(key Key) error {
		if key.Path == "" {
			return fmt.Errorf("an entry has no key")
		}
		if _, taken := r.byPath[key.Path]; taken {
			return fmt.Errorf("%s is listed twice", key.Path)
		}
		switch key.Status {
		case KeyOperator:
		case KeyRead, KeyIgnored:
			if strings.TrimSpace(key.Reason) == "" || strings.TrimSpace(key.RemovedIn) == "" {
				return fmt.Errorf("%s needs a reason and removed_in", key.Path)
			}
		default:
			return fmt.Errorf("%s: unknown status %q", key.Path, key.Status)
		}
		if key.Status == KeyRead {
			if _, ok := configFieldTypes[key.fieldPath()]; !ok {
				return fmt.Errorf("%s is read but no configuration field holds it", key.Path)
			}
		}
		if key.Status == KeyOperator {
			if _, ok := configFieldTypes[key.fieldPath()]; !ok {
				return fmt.Errorf("operator key %s has no configuration field", key.Path)
			}
		}
		r.byPath[key.Path] = key
		if !key.Subtree && envCapable(key) {
			if other, taken := r.byEnv[key.EnvName()]; taken {
				return fmt.Errorf("%s and %s have the same variable %s", key.Path, other.Path, key.EnvName())
			}
			r.byEnv[key.EnvName()] = key
		}
		parts := strings.Split(key.Path, ".")
		for i := 1; i < len(parts); i++ {
			r.section[strings.Join(parts[:i], ".")] = true
		}
		r.keys = append(r.keys, key)
		return nil
	}
	for _, key := range operatorKeys {
		key.Status = KeyOperator
		if err := add(key); err != nil {
			return nil, err
		}
	}
	for _, key := range document.Keys {
		if key.Status == KeyOperator {
			return nil, fmt.Errorf("%s: operator keys are defined in keys.go", key.Path)
		}
		if err := add(key); err != nil {
			return nil, err
		}
	}
	for path := range r.byPath {
		if r.section[path] {
			return nil, fmt.Errorf("%s is both a key and a section", path)
		}
	}
	return r, nil
}

// envCapable tells whether a key has an ANIX_CONTROL_* variable: every key
// but maps and free-form values (YAML only).
func envCapable(key Key) bool {
	fieldType, ok := configFieldTypes[key.fieldPath()]
	if !ok {
		// An ignored key without a field still has its old variable.
		return true
	}
	_, ok = envTypeName(fieldType)
	return ok
}

// Keys returns every configuration key: the operator keys in their
// documented order, then the deprecated keys by path.
func Keys() []Key {
	keys := append([]Key(nil), registry.keys...)
	sort.SliceStable(keys, func(i, j int) bool {
		if keys[i].Deprecated() != keys[j].Deprecated() {
			return !keys[i].Deprecated()
		}
		if keys[i].Deprecated() {
			return keys[i].Path < keys[j].Path
		}
		return false
	})
	return keys
}

// OperatorKeys returns the operator keys in their documented order.
func OperatorKeys() []Key {
	var keys []Key
	for _, key := range Keys() {
		if !key.Deprecated() {
			keys = append(keys, key)
		}
	}
	return keys
}

// LookupKey returns the key at path, or the subtree entry that covers it.
func LookupKey(path string) (Key, bool) {
	if key, ok := registry.byPath[path]; ok {
		return key, true
	}
	parts := strings.Split(path, ".")
	for i := len(parts) - 1; i > 0; i-- {
		if key, ok := registry.byPath[strings.Join(parts[:i], ".")]; ok && key.Subtree {
			return key, true
		}
	}
	return Key{}, false
}

// configFieldTypes maps the YAML path of every leaf of Config (a scalar,
// list or map field) to its type.
var configFieldTypes = func() map[string]reflect.Type {
	types := map[string]reflect.Type{}
	var walk func(t reflect.Type, prefix string)
	walk = func(t reflect.Type, prefix string) {
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			name, _, _ := strings.Cut(field.Tag.Get("yaml"), ",")
			if name == "" || name == "-" || !field.IsExported() {
				continue
			}
			path := name
			if prefix != "" {
				path = prefix + "." + name
			}
			if field.Type.Kind() == reflect.Struct {
				walk(field.Type, path)
				continue
			}
			types[path] = field.Type
		}
	}
	walk(reflect.TypeOf(Config{}), "")
	return types
}()

// configField returns the field of cfg at a YAML path.
func configField(cfg *Config, path string) (reflect.Value, bool) {
	value := reflect.ValueOf(cfg).Elem()
	for _, name := range strings.Split(path, ".") {
		if value.Kind() != reflect.Struct {
			return reflect.Value{}, false
		}
		found := false
		for i := 0; i < value.NumField(); i++ {
			field := value.Type().Field(i)
			tag, _, _ := strings.Cut(field.Tag.Get("yaml"), ",")
			if tag == name && field.IsExported() {
				value, found = value.Field(i), true
				break
			}
		}
		if !found {
			return reflect.Value{}, false
		}
	}
	return value, true
}

// closestKey is the known key or section nearest to path by edit
// distance, preferring an operator key on a tie.
func closestKey(path string) string {
	candidates := make([]string, 0, len(registry.keys)+len(registry.section))
	for _, key := range Keys() {
		candidates = append(candidates, key.Path)
	}
	sections := make([]string, 0, len(registry.section))
	for section := range registry.section {
		sections = append(sections, section)
	}
	sort.Strings(sections)
	candidates = append(candidates, sections...)
	best, bestDistance := "", -1
	for _, candidate := range candidates {
		distance := editDistance(path, candidate)
		if bestDistance < 0 || distance < bestDistance {
			best, bestDistance = candidate, distance
		}
	}
	return best
}

// closestEnvName is the known variable nearest to name.
func closestEnvName(name string) string {
	best, bestDistance := "", -1
	for _, key := range Keys() {
		if key.Subtree || !envCapable(key) {
			continue
		}
		distance := editDistance(name, key.EnvName())
		if bestDistance < 0 || distance < bestDistance {
			best, bestDistance = key.EnvName(), distance
		}
	}
	return best
}

// editDistance is the Levenshtein distance of two strings.
func editDistance(a, b string) int {
	previous := make([]int, len(b)+1)
	current := make([]int, len(b)+1)
	for j := range previous {
		previous[j] = j
	}
	for i := 1; i <= len(a); i++ {
		current[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			current[j] = min(previous[j]+1, current[j-1]+1, previous[j-1]+cost)
		}
		previous, current = current, previous
	}
	return previous[len(b)]
}
