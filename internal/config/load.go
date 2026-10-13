package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// FileSource is the DeprecatedKey.Source of a key set in the configuration
// file.
const FileSource = "the configuration file"

// DeprecatedKey is one deprecated key the configuration sets.
type DeprecatedKey struct {
	// Path is the key, for example plugins.dispatch_enabled.
	Path string
	// Source is FileSource or the ANIX_CONTROL_* variable that set it; more
	// than one source are joined with " and ".
	Source string
	// Status is KeyRead (still applied) or KeyIgnored.
	Status KeyStatus
	// RemovedIn, Reason and Replacement are the key's entry in the
	// deprecated list.
	RemovedIn   string
	Reason      string
	Replacement string
}

// Warning is the log line of the key: one per key, whatever sets it.
func (d DeprecatedKey) Warning() string {
	use := ""
	if d.Replacement != "" {
		use = " Use " + d.Replacement + "."
	}
	if d.Status == KeyRead {
		return fmt.Sprintf("Configuration: %s (set in %s) is deprecated and still read until %s: %s.%s", d.Path, d.Source, d.RemovedIn, d.Reason, use)
	}
	return fmt.Sprintf("Configuration: %s (set in %s) is no longer read and is ignored: %s.%s", d.Path, d.Source, d.Reason, use)
}

// DeprecatedConfig records the deprecated keys a configuration sets.
type DeprecatedConfig struct {
	keys []DeprecatedKey
}

// Keys returns the deprecated keys the configuration sets, by path.
func (d DeprecatedConfig) Keys() []DeprecatedKey {
	return append([]DeprecatedKey(nil), d.keys...)
}

// LoadReport is what loading a configuration found besides its values: the
// deprecated keys it sets and the ANIX_CONTROL_* variables that match no
// key.
type LoadReport struct {
	Deprecated []DeprecatedKey
	// UnknownEnv are the variables that match no key, each with the closest
	// known variable.
	UnknownEnv []UnknownVariable
}

// UnknownVariable is an ANIX_CONTROL_* variable that matches no key.
type UnknownVariable struct {
	Name    string
	Closest string
}

// Warnings are the report's log lines.
func (r LoadReport) Warnings() []string {
	var lines []string
	for _, key := range r.Deprecated {
		lines = append(lines, key.Warning())
	}
	for _, variable := range r.UnknownEnv {
		lines = append(lines, fmt.Sprintf("Configuration: ignoring the unknown variable %s (did you mean %s? anix-control -print-env lists them)", variable.Name, variable.Closest))
	}
	return lines
}

// loader overlays a file and the environment on the built-in defaults.
type loader struct {
	cfg        *Config
	deprecated map[string]*DeprecatedKey
	unknownEnv []UnknownVariable
	// setBy names the key that set a field in the current source, so that
	// a renamed key and its replacement cannot disagree
	// (app.subscribe_path and subscribe_path).
	setBy map[string]setValue
}

// setValue is a field a source set.
type setValue struct {
	key   string
	value any
}

// claim records that key set the field at fieldPath to value; another key
// of the same source that set it to something else is an error.
func (l *loader) claim(key Key, value any) error {
	if l.setBy == nil {
		l.setBy = map[string]setValue{}
	}
	field := key.fieldPath()
	if previous, ok := l.setBy[field]; ok && previous.key != key.Path && !reflect.DeepEqual(previous.value, value) {
		return fmt.Errorf("%s and %s set the same value differently: keep %s", previous.key, key.Path, operatorPath(previous.key, key.Path))
	}
	l.setBy[field] = setValue{key: key.Path, value: value}
	return nil
}

// operatorPath names the operator key of two keys of one field.
func operatorPath(a, b string) string {
	if key, ok := LookupKey(a); ok && !key.Deprecated() {
		return a
	}
	return b
}

// LoadFile loads a configuration as Control does, without logging:
//
//  1. the built-in defaults (defaults.yaml);
//  2. the file at path, when path is not empty, key by key. The YAML is
//     read strictly: an operator key is applied; a deprecated key is
//     recorded and, when it is still read, applied; any other key is an
//     error that names the closest known key. The database section keeps
//     its 4.x meaning while PostgreSQL is supported: with a file, only the
//     file's database keys count (an empty driver is SQLite);
//  3. the ANIX_CONTROL_* variables of environ, the same way (an unknown
//     variable is reported, not refused: the environment is shared with
//     other tools).
//
// The result is validated; the report lists the deprecated keys and unknown
// variables.
func LoadFile(path string, environ []string) (*Config, LoadReport, error) {
	l := &loader{cfg: Defaults(), deprecated: map[string]*DeprecatedKey{}}
	if path != "" {
		data, err := os.ReadFile(path) // #nosec G304 -- config path is supplied by the operator or test harness, not by an HTTP user.
		if err != nil {
			return nil, LoadReport{}, err
		}
		l.cfg.Database = DatabaseConfig{}
		if err := l.applyYAML(data, path); err != nil {
			return nil, LoadReport{}, err
		}
	}
	if err := l.applyEnv(environ); err != nil {
		return nil, LoadReport{}, err
	}
	report := l.report()
	l.cfg.Deprecated = DeprecatedConfig{keys: report.Deprecated}
	if err := l.cfg.validateLoaded(); err != nil {
		return nil, report, err
	}
	return l.cfg, report, nil
}

// report collects the findings, by key path.
func (l *loader) report() LoadReport {
	report := LoadReport{UnknownEnv: l.unknownEnv}
	for _, key := range l.deprecated {
		report.Deprecated = append(report.Deprecated, *key)
	}
	sort.Slice(report.Deprecated, func(i, j int) bool { return report.Deprecated[i].Path < report.Deprecated[j].Path })
	sort.Slice(report.UnknownEnv, func(i, j int) bool { return report.UnknownEnv[i].Name < report.UnknownEnv[j].Name })
	return report
}

// deprecate records that source sets a deprecated key.
func (l *loader) deprecate(key Key, source string) {
	if existing, ok := l.deprecated[key.Path]; ok {
		if !strings.Contains(existing.Source, source) {
			existing.Source += " and " + source
		}
		return
	}
	l.deprecated[key.Path] = &DeprecatedKey{Path: key.Path, Source: source, Status: key.Status,
		RemovedIn: key.RemovedIn, Reason: key.Reason, Replacement: key.Replacement}
}

// applyYAML overlays a configuration file.
func (l *loader) applyYAML(data []byte, file string) error {
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return fmt.Errorf("%s: %w", file, err)
	}
	if len(document.Content) == 0 {
		return nil // an empty file sets nothing
	}
	root := document.Content[0]
	if root.Kind == yaml.ScalarNode && root.Tag == "!!null" {
		return nil
	}
	if root.Kind != yaml.MappingNode {
		return fmt.Errorf("%s:%d: the configuration must be a mapping of keys, such as domain: panel.example.com", file, root.Line)
	}
	return l.walk(root, "", file)
}

// walk applies the keys of one mapping.
func (l *loader) walk(node *yaml.Node, prefix, file string) error {
	seen := map[string]bool{}
	for i := 0; i+1 < len(node.Content); i += 2 {
		name, value := node.Content[i], node.Content[i+1]
		if value.Kind == yaml.AliasNode && value.Alias != nil {
			value = value.Alias
		}
		path := name.Value
		if prefix != "" {
			path = prefix + "." + name.Value
		}
		if name.Value == "<<" {
			return fmt.Errorf("%s:%d: YAML merge keys (<<) are not supported in the configuration", file, name.Line)
		}
		if seen[name.Value] {
			return fmt.Errorf("%s:%d: %s is set twice", file, name.Line, path)
		}
		seen[name.Value] = true
		key, known := LookupKey(path)
		switch {
		case known && key.Subtree:
			l.deprecate(key, FileSource)
		case known && key.Status == KeyIgnored:
			l.deprecate(key, FileSource)
		case known:
			if key.Status == KeyRead {
				l.deprecate(key, FileSource)
			}
			if err := l.applyNode(key, value); err != nil {
				return fmt.Errorf("%s:%d: invalid %s: %w", file, value.Line, path, err)
			}
		case registry.section[path]:
			if value.Kind == yaml.ScalarNode && value.Tag == "!!null" {
				continue
			}
			if value.Kind != yaml.MappingNode {
				return fmt.Errorf("%s:%d: %s is a section of keys, not a value", file, value.Line, path)
			}
			if err := l.walk(value, path, file); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%s:%d: unknown configuration key %s; did you mean %s? (docs/reference/configuration.md lists the keys)", file, name.Line, path, closestKey(path))
		}
	}
	return nil
}

// yamlLinePrefix is the "line N: " yaml.v3 puts in front of its errors.
var yamlLinePrefix = regexp.MustCompile(`^(yaml: )?(unmarshal errors:\s*)?(line \d+: )?`)

// applyNode decodes a value into the key's field. A null value leaves the
// built-in default.
func (l *loader) applyNode(key Key, value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode && value.Tag == "!!null" {
		return nil
	}
	field, ok := configField(l.cfg, key.fieldPath())
	if !ok {
		return errors.New("no configuration field holds it")
	}
	target := reflect.New(field.Type())
	if err := value.Decode(target.Interface()); err != nil {
		return errors.New(yamlLinePrefix.ReplaceAllString(strings.TrimSpace(err.Error()), ""))
	}
	if err := l.claim(key, target.Elem().Interface()); err != nil {
		return err
	}
	field.Set(target.Elem())
	return nil
}

// applyEnv overlays the ANIX_CONTROL_* variables.
func (l *loader) applyEnv(environ []string) error {
	values := make(map[string]string, len(environ))
	for _, entry := range environ {
		name, value, ok := strings.Cut(entry, "=")
		if ok && strings.HasPrefix(name, EnvPrefix) {
			values[name] = value
		}
	}
	handled := map[string]bool{}
	l.setBy = nil
	// Deprecated keys first: an operator key that sets the same field wins
	// (and must agree).
	keys := Keys()
	sort.SliceStable(keys, func(i, j int) bool { return keys[i].Deprecated() && !keys[j].Deprecated() })
	for _, key := range keys {
		if key.Subtree || !envCapable(key) {
			continue
		}
		name := key.EnvName()
		handled[name], handled[name+EnvFileSuffix] = true, true
		_, direct := values[name]
		_, fromFile := values[name+EnvFileSuffix]
		if !direct && !fromFile {
			continue
		}
		source := name
		if fromFile {
			source = name + EnvFileSuffix
		}
		if key.Status == KeyIgnored {
			l.deprecate(key, source)
			continue
		}
		raw, _, err := lookupEnvValue(values, name)
		if err != nil {
			return err
		}
		field, ok := configField(l.cfg, key.fieldPath())
		if !ok {
			return fmt.Errorf("%s: no configuration field holds %s", name, key.Path)
		}
		target := reflect.New(field.Type()).Elem()
		if err := setEnvValue(target, raw); err != nil {
			return fmt.Errorf("invalid %s for %s: %w", name, key.Path, err)
		}
		if err := l.claim(key, target.Interface()); err != nil {
			return fmt.Errorf("%s: %w", source, err)
		}
		field.Set(target)
		if key.Status == KeyRead {
			l.deprecate(key, source)
		}
	}
	for name := range values {
		if handled[name] || reservedEnvNames[name] {
			continue
		}
		l.unknownEnv = append(l.unknownEnv, UnknownVariable{Name: name, Closest: closestEnvName(strings.TrimSuffix(name, EnvFileSuffix))})
	}
	return nil
}

// validateLoaded checks a loaded configuration and normalizes what the
// code expects normalized.
func (c *Config) validateLoaded() error {
	normalizeBrandDefaults(c)
	var err error
	c.App.SubscribePath, err = NormalizeSubscribePath(c.App.SubscribePath)
	if err != nil {
		return fmt.Errorf("invalid subscribe_path: %w", err)
	}
	if dataDir := strings.TrimSpace(c.DataDir); dataDir == "" || !filepath.IsAbs(dataDir) {
		return fmt.Errorf("invalid data_dir %q: use an absolute path, for example /var/lib/anix-control", c.DataDir)
	}
	c.DataDir = filepath.Clean(strings.TrimSpace(c.DataDir))
	if err := c.Plugins.ValidatePayloadLimits(); err != nil {
		return err
	}
	if err := c.Database.validateConnectionSettings(); err != nil {
		return err
	}
	if _, err := c.Server.DrainDelay(); err != nil {
		return err
	}
	if err := validateTrustedProxies(c.Server.TrustedProxies); err != nil {
		return err
	}
	if err := c.Alerts.validate(); err != nil {
		return err
	}
	if err := c.AgentInstall.Validate(); err != nil {
		return err
	}
	if err := c.validatePanel(); err != nil {
		return err
	}
	switch c.Log.Format {
	case "", "text", "json":
	default:
		return fmt.Errorf("invalid log.format %q: use text or json", c.Log.Format)
	}
	return nil
}
