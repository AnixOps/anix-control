package nategresscontract

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

func TestSchemaCompilesWithControlValidatorAndMatchesRuntimeBoundaries(t *testing.T) {
	schemaBytes, err := os.ReadFile("../config.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	const resourceURL = "https://anixops.invalid/plugins/nat-egress/1.0.0/config-schema.json"
	if err := compiler.AddResource(resourceURL, bytes.NewReader(schemaBytes)); err != nil {
		t.Fatal(err)
	}
	compiled, err := compiler.Compile(resourceURL)
	if err != nil {
		t.Fatal(err)
	}

	defaults := readConfig(t, "../config.defaults.json")
	assertSchemaAccepts(t, compiled, defaults)

	validIPv6 := cloneConfig(t, defaults)
	validIPv6["ipv4_masquerade"] = false
	validIPv6["ipv6_masquerade"] = true
	validIPv6["health_check_target"] = "[2001:db8::1]:443"
	assertSchemaAccepts(t, compiled, validIPv6)

	validDualStack := cloneConfig(t, defaults)
	validDualStack["ipv6_masquerade"] = true
	validDualStack["health_check_target"] = "egress.example.com:443"
	assertSchemaAccepts(t, compiled, validDualStack)

	invalid := []struct {
		name  string
		field string
		value any
	}{
		{name: "unsafe table", field: "table_name", value: "bad-name"},
		{name: "long interface", field: "egress_interface", value: "interface-name-16"},
		{name: "zero mark", field: "default_mark", value: 0},
		{name: "reserved policy table", field: "policy_table", value: 253},
		{name: "priority overflow", field: "rule_priority", value: 32766},
		{name: "short health interval", field: "health_check_interval_seconds", value: 4},
		{name: "health timeout overflow", field: "health_check_timeout_seconds", value: 31},
		{name: "missing health port", field: "health_check_target", value: "example.com"},
		{name: "health port overflow", field: "health_check_target", value: "example.com:65536"},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			config := cloneConfig(t, defaults)
			config[test.field] = test.value
			assertSchemaRejects(t, compiled, config)
		})
	}

	t.Run("both address families disabled", func(t *testing.T) {
		config := cloneConfig(t, defaults)
		config["ipv4_masquerade"] = false
		config["ipv6_masquerade"] = false
		assertSchemaRejects(t, compiled, config)
	})

	t.Run("rollback on exit disabled", func(t *testing.T) {
		config := cloneConfig(t, defaults)
		config["rollback_on_exit"] = false
		assertSchemaRejects(t, compiled, config)
	})

	t.Run("IPv6 literal requires IPv6 only", func(t *testing.T) {
		config := cloneConfig(t, defaults)
		config["health_check_target"] = "[2001:db8::1]:443"
		assertSchemaRejects(t, compiled, config)
	})

	t.Run("IPv4 literal requires IPv4 only", func(t *testing.T) {
		config := cloneConfig(t, defaults)
		config["ipv4_masquerade"] = false
		config["ipv6_masquerade"] = true
		config["health_check_target"] = "192.0.2.1:443"
		assertSchemaRejects(t, compiled, config)
	})

	t.Run("dual stack literal is rejected", func(t *testing.T) {
		config := cloneConfig(t, defaults)
		config["ipv6_masquerade"] = true
		config["health_check_target"] = "192.0.2.1:443"
		assertSchemaRejects(t, compiled, config)
	})

	t.Run("unknown field", func(t *testing.T) {
		config := cloneConfig(t, defaults)
		config["unknown"] = true
		assertSchemaRejects(t, compiled, config)
	})
}

func readConfig(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value map[string]any
	if err := decoder.Decode(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

func cloneConfig(t *testing.T, value map[string]any) map[string]any {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var cloned map[string]any
	if err := decoder.Decode(&cloned); err != nil {
		t.Fatal(err)
	}
	return cloned
}

func assertSchemaAccepts(t *testing.T, schema *jsonschema.Schema, value any) {
	t.Helper()
	if err := schema.Validate(value); err != nil {
		t.Fatalf("expected schema acceptance: %v", err)
	}
}

func assertSchemaRejects(t *testing.T, schema *jsonschema.Schema, value any) {
	t.Helper()
	if err := schema.Validate(value); err == nil {
		t.Fatal("expected schema rejection")
	}
}
