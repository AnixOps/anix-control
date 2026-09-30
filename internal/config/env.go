package config

import (
	"fmt"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// EnvPrefix starts every configuration environment variable. A variable name
// is the upper-cased yaml path joined with underscores, for example
// database.password -> ANIX_CONTROL_DATABASE_PASSWORD.
const EnvPrefix = "ANIX_CONTROL_"

// EnvFileSuffix selects the file variant of a variable: the value is read from
// the named file, which is how Docker and Kubernetes secrets are mounted.
const EnvFileSuffix = "_FILE"

// ConfigPathEnv names the config file when -config is not given.
const ConfigPathEnv = EnvPrefix + "CONFIG"

// reservedEnvNames are ANIX_CONTROL_* variables that are not configuration
// keys: the config path, and the variables the kernel passes to package hosts.
var reservedEnvNames = map[string]bool{
	ConfigPathEnv:                          true,
	"ANIX_CONTROL_HOST_SOCKET":             true,
	"ANIX_CONTROL_HOST_DIR_FD":             true,
	"ANIX_CONTROL_HOST_MAX_RESPONSE_BYTES": true,
	"ANIX_CONTROL_PACKAGE_BRIDGE_FD":       true,
}

// EnvVar describes one configuration key that can be set from the environment.
type EnvVar struct {
	Name    string // ANIX_CONTROL_DATABASE_PASSWORD
	Path    string // database.password
	Type    string // string, bool, int, list
	Default string // value in the supplied config; empty for secrets
	Secret  bool
}

type envField struct {
	EnvVar
	value reflect.Value
}

// EnvVars lists every environment-settable key with its value in cfg, sorted
// by name. Secret values are never included.
func EnvVars(cfg *Config) []EnvVar {
	if cfg == nil {
		cfg = &Config{}
	}
	fields := envFields(cfg)
	vars := make([]EnvVar, 0, len(fields))
	for _, field := range fields {
		entry := field.EnvVar
		if !entry.Secret {
			entry.Default = formatEnvValue(field.value)
		}
		vars = append(vars, entry)
	}
	return vars
}

// ApplyEnv overrides cfg with ANIX_CONTROL_* entries from environ ("KEY=value"
// pairs, as returned by os.Environ). It returns the ANIX_CONTROL_* names that
// match no configuration key so the caller can warn about typos.
func ApplyEnv(cfg *Config, environ []string) ([]string, error) {
	values := make(map[string]string, len(environ))
	for _, entry := range environ {
		name, value, ok := strings.Cut(entry, "=")
		if ok && strings.HasPrefix(name, EnvPrefix) {
			values[name] = value
		}
	}
	known := make(map[string]bool)
	for _, field := range envFields(cfg) {
		known[field.Name] = true
		known[field.Name+EnvFileSuffix] = true
		raw, set, err := lookupEnvValue(values, field.Name)
		if err != nil {
			return nil, err
		}
		if !set {
			continue
		}
		if err := setEnvValue(field.value, raw); err != nil {
			return nil, fmt.Errorf("invalid %s for %s: %w", field.Name, field.Path, err)
		}
	}
	var unknown []string
	for name := range values {
		if !known[name] && !reservedEnvNames[name] {
			unknown = append(unknown, name)
		}
	}
	sort.Strings(unknown)
	return unknown, nil
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

func envFields(cfg *Config) []envField {
	var fields []envField
	collectEnvFields(reflect.ValueOf(cfg).Elem(), nil, &fields)
	sort.Slice(fields, func(i, j int) bool { return fields[i].Name < fields[j].Name })
	return fields
}

func collectEnvFields(value reflect.Value, path []string, fields *[]envField) {
	valueType := value.Type()
	for index := 0; index < valueType.NumField(); index++ {
		structField := valueType.Field(index)
		key, _, _ := strings.Cut(structField.Tag.Get("yaml"), ",")
		if key == "" || key == "-" || !structField.IsExported() {
			continue
		}
		fieldPath := append(append([]string(nil), path...), key)
		fieldValue := value.Field(index)
		if structField.Type.Kind() == reflect.Struct {
			collectEnvFields(fieldValue, fieldPath, fields)
			continue
		}
		typeName, ok := envTypeName(structField.Type)
		if !ok {
			continue // maps and free-form values are YAML-only
		}
		*fields = append(*fields, envField{
			EnvVar: EnvVar{
				Name:   EnvPrefix + strings.ToUpper(strings.Join(fieldPath, "_")),
				Path:   strings.Join(fieldPath, "."),
				Type:   typeName,
				Secret: secretConfigKey(key),
			},
			value: fieldValue,
		})
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

func secretConfigKey(key string) bool {
	switch key {
	case "password", "secret", "token", "api_token", "redis_password", "dsn", "ca_kek", "kek":
		return true
	default:
		return false
	}
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
		case variable.Default == "":
			value = ""
		}
		_, _ = fmt.Fprintf(&builder, "| `%s` | `%s` | %s | %s |\n", variable.Name, variable.Path, variable.Type, value)
	}
	return builder.String()
}
