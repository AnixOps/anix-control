package config

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
)

// EnvPrefix starts every configuration environment variable. A variable name
// is the upper-cased yaml path joined with underscores, for example
// grpc.public_name -> ANIX_CONTROL_GRPC_PUBLIC_NAME.
const EnvPrefix = "ANIX_CONTROL_"

// EnvFileSuffix selects the file variant of a variable: the value is read from
// the named file, which is how Docker and Kubernetes secrets are mounted.
const EnvFileSuffix = "_FILE"

// ConfigPathEnv names the config file when -config is not given.
const ConfigPathEnv = EnvPrefix + "CONFIG"

// ACMEDirectoryEnv points Control's ACME client at another directory than
// the public CA's: a staging directory, or a test CA in CI. It is not a
// configuration key and is read only from the environment.
const ACMEDirectoryEnv = EnvPrefix + "ACME_DIRECTORY_URL"

// ACMECAFileEnv names a PEM file of extra roots the ACME client trusts for
// the directory's https endpoint (a test CA in CI). It is not a
// configuration key.
const ACMECAFileEnv = EnvPrefix + "ACME_CA_FILE"

// reservedEnvNames are ANIX_CONTROL_* variables that are not configuration
// keys: the config path, and the variables the kernel passes to package hosts.
var reservedEnvNames = map[string]bool{
	ConfigPathEnv:                          true,
	ACMEDirectoryEnv:                       true,
	ACMECAFileEnv:                          true,
	"ANIX_CONTROL_HOST_SOCKET":             true,
	"ANIX_CONTROL_HOST_DIR_FD":             true,
	"ANIX_CONTROL_HOST_MAX_RESPONSE_BYTES": true,
	"ANIX_CONTROL_PACKAGE_BRIDGE_FD":       true,
}

// EnvVar describes one operator key and its variable.
type EnvVar struct {
	Name    string // ANIX_CONTROL_DOMAIN
	Path    string // domain
	Type    string // string, bool, int, list
	Default string // value in the supplied config; empty for secrets
	Secret  bool
	// Description says what the key does.
	Description string
}

// EnvVars lists the operator keys with their value in cfg, in their
// documented order. The deprecated keys' variables are still read or warned
// about (deprecated_keys.yaml) but are not listed here.
func EnvVars(cfg *Config) []EnvVar {
	if cfg == nil {
		cfg = &Config{}
	}
	var vars []EnvVar
	for _, key := range OperatorKeys() {
		field, ok := configField(cfg, key.fieldPath())
		if !ok {
			continue
		}
		typeName, ok := envTypeName(field.Type())
		if !ok {
			continue
		}
		entry := EnvVar{Name: key.EnvName(), Path: key.Path, Type: typeName, Secret: key.Secret, Description: key.Description}
		if !entry.Secret {
			entry.Default = formatEnvValue(field)
		}
		vars = append(vars, entry)
	}
	return vars
}

func lookupEnvValue(values map[string]string, name string) (string, bool, error) {
	value, direct := values[name]
	path, fromFile := values[name+EnvFileSuffix]
	switch {
	case direct && fromFile:
		return "", false, fmt.Errorf("both %s and %s%s are set; use one", name, name, EnvFileSuffix)
	case fromFile:
		data, err := os.ReadFile(path) // #nosec G304 -- the operator names the secret file.
		if err != nil {
			return "", false, fmt.Errorf("read %s%s: %w", name, EnvFileSuffix, err)
		}
		return strings.TrimRight(string(data), "\r\n"), true, nil
	default:
		return value, direct, nil
	}
}

func envTypeName(fieldType reflect.Type) (string, bool) {
	switch fieldType.Kind() {
	case reflect.String:
		return "string", true
	case reflect.Bool:
		return "bool", true
	case reflect.Int, reflect.Int64:
		return "int", true
	case reflect.Pointer:
		if fieldType.Elem().Kind() == reflect.Bool {
			return "bool", true
		}
	case reflect.Slice:
		if fieldType.Elem().Kind() == reflect.String {
			return "list", true
		}
	}
	return "", false
}

func setEnvValue(target reflect.Value, raw string) error {
	switch target.Kind() {
	case reflect.String:
		target.SetString(raw)
	case reflect.Bool:
		parsed, err := strconv.ParseBool(strings.TrimSpace(raw))
		if err != nil {
			return err
		}
		target.SetBool(parsed)
	case reflect.Int, reflect.Int64:
		parsed, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil {
			return err
		}
		if target.OverflowInt(parsed) {
			return fmt.Errorf("%d is out of range", parsed)
		}
		target.SetInt(parsed)
	case reflect.Pointer:
		parsed, err := strconv.ParseBool(strings.TrimSpace(raw))
		if err != nil {
			return err
		}
		target.Set(reflect.ValueOf(&parsed))
	case reflect.Slice:
		items := []string{}
		for _, item := range strings.Split(raw, ",") {
			if item = strings.TrimSpace(item); item != "" {
				items = append(items, item)
			}
		}
		target.Set(reflect.ValueOf(items))
	default:
		return fmt.Errorf("unsupported type %s", target.Type())
	}
	return nil
}

func formatEnvValue(value reflect.Value) string {
	switch value.Kind() {
	case reflect.Pointer:
		if value.IsNil() {
			return ""
		}
		return formatEnvValue(value.Elem())
	case reflect.Slice:
		items := make([]string, value.Len())
		for index := range items {
			items[index] = value.Index(index).String()
		}
		return strings.Join(items, ",")
	default:
		return fmt.Sprint(value.Interface())
	}
}

// PathFromEnv returns the config file named by ANIX_CONTROL_CONFIG, if any.
func PathFromEnv() string {
	value, _ := os.LookupEnv(ConfigPathEnv)
	return strings.TrimSpace(value)
}

// emptyDefaultMeanings explains the built-in defaults that are empty on
// purpose because the code resolves them.
var emptyDefaultMeanings = map[string]string{
	"domain":           "none; required in `tls.mode` `acme`",
	"tls.mode":         "empty: `acme` with `domain`, else `external`",
	"listen":           "empty: `" + DefaultListenAddress + "`",
	"grpc.public_name": "empty: `domain`",
}

// EnvMarkdownTable renders the variable reference table of
// docs/reference/environment-variables.md from cfg (the built-in defaults).
func EnvMarkdownTable(cfg *Config) string {
	var builder strings.Builder
	builder.WriteString("| Variable | Key | Type | Built-in default |\n")
	builder.WriteString("|----------|-----|------|------------------|\n")
	for _, variable := range EnvVars(cfg) {
		value := "`" + variable.Default + "`"
		switch {
		case variable.Secret:
			value = "secret, no default"
		case variable.Default == "" && emptyDefaultMeanings[variable.Path] != "":
			value = emptyDefaultMeanings[variable.Path]
		case variable.Default == "":
			value = ""
		}
		_, _ = fmt.Fprintf(&builder, "| `%s` | `%s` | %s | %s |\n", variable.Name, variable.Path, variable.Type, value)
	}
	return builder.String()
}

// DeprecatedKeysMarkdownTable renders the deprecated key table of
// docs/reference/configuration.md from deprecated_keys.yaml.
func DeprecatedKeysMarkdownTable() string {
	var builder strings.Builder
	builder.WriteString("| Key | Variable | Status | Removed in | Why |\n")
	builder.WriteString("|-----|----------|--------|------------|-----|\n")
	for _, key := range Keys() {
		if !key.Deprecated() {
			continue
		}
		variable := "`" + key.EnvName() + "`"
		path := "`" + key.Path + "`"
		switch {
		case key.Subtree:
			path, variable = "`"+key.Path+".*`", "none"
		case !envCapable(key):
			variable = "none (YAML only)"
		}
		status := "ignored"
		if key.Status == KeyRead {
			status = "still read"
		}
		why := key.Reason
		if key.Replacement != "" {
			why += "; use `" + key.Replacement + "`"
		}
		_, _ = fmt.Fprintf(&builder, "| %s | %s | %s | %s | %s |\n", path, variable, status, key.RemovedIn, why)
	}
	return builder.String()
}
