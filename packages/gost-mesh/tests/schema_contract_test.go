package gostmeshcontract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

func TestSchemaCompilesWithControlValidatorAndMatchesAgentBoundaries(t *testing.T) {
	schemaBytes, err := os.ReadFile("../config.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	const resourceURL = "https://anixops.invalid/plugins/gost-mesh/1.0.0/config-schema.json"
	if err := compiler.AddResource(resourceURL, bytes.NewReader(schemaBytes)); err != nil {
		t.Fatal(err)
	}
	compiled, err := compiler.Compile(resourceURL)
	if err != nil {
		t.Fatal(err)
	}

	defaults := readConfig(t, "../config.defaults.json")
	assertSchemaAccepts(t, compiled, defaults)
	assertSchemaAccepts(t, compiled, configWithTunnel(t, "entry", "wss"))
	assertSchemaAccepts(t, compiled, configWithTunnel(t, "entry", "quic"))
	assertSchemaAccepts(t, compiled, configWithTunnel(t, "exit", "wss"))
	assertSchemaAccepts(t, compiled, configWithTunnel(t, "exit", "quic"))
	exitWithExplicitZeroes := configWithTunnel(t, "exit", "quic")
	routing(t, exitWithExplicitZeroes)["table"] = 0
	routing(t, exitWithExplicitZeroes)["priority"] = 0
	assertSchemaAccepts(t, compiled, exitWithExplicitZeroes)

	t.Run("apply requires tunnel", func(t *testing.T) {
		config := cloneConfig(t, defaults)
		config["apply"] = true
		assertSchemaRejects(t, compiled, config)
	})
	t.Run("rollback is mandatory", func(t *testing.T) {
		config := configWithTunnel(t, "entry", "wss")
		config["rollback_on_exit"] = false
		assertSchemaRejects(t, compiled, config)
	})
	t.Run("TUIC is not a GOST v1 transport", func(t *testing.T) {
		config := configWithTunnel(t, "entry", "wss")
		tunnel(t, config)["transport"] = "tuic"
		assertSchemaRejects(t, compiled, config)
	})
	t.Run("entry requires remote and rejects listen", func(t *testing.T) {
		missing := configWithTunnel(t, "entry", "wss")
		delete(tunnel(t, missing), "remote")
		assertSchemaRejects(t, compiled, missing)
		unexpected := configWithTunnel(t, "entry", "wss")
		tunnel(t, unexpected)["listen"] = map[string]any{"address": "0.0.0.0", "port": 443}
		assertSchemaRejects(t, compiled, unexpected)
	})
	t.Run("exit requires listen and rejects remote", func(t *testing.T) {
		missing := configWithTunnel(t, "exit", "quic")
		delete(tunnel(t, missing), "listen")
		assertSchemaRejects(t, compiled, missing)
		unexpected := configWithTunnel(t, "exit", "quic")
		tunnel(t, unexpected)["remote"] = map[string]any{"host": "entry.example.com", "port": 443}
		assertSchemaRejects(t, compiled, unexpected)
	})
	t.Run("role-specific routing", func(t *testing.T) {
		entry := configWithTunnel(t, "entry", "wss")
		routing(t, entry)["route_cidrs"] = []any{"10.66.0.0/24"}
		assertSchemaRejects(t, compiled, entry)
		missingEntryTable := configWithTunnel(t, "entry", "wss")
		delete(routing(t, missingEntryTable), "table")
		assertSchemaRejects(t, compiled, missingEntryTable)
		missingEntryPriority := configWithTunnel(t, "entry", "wss")
		delete(routing(t, missingEntryPriority), "priority")
		assertSchemaRejects(t, compiled, missingEntryPriority)
		exit := configWithTunnel(t, "exit", "wss")
		routing(t, exit)["source_cidrs"] = []any{"10.66.0.0/24"}
		assertSchemaRejects(t, compiled, exit)
		exitTable := configWithTunnel(t, "exit", "wss")
		routing(t, exitTable)["table"] = 100
		assertSchemaRejects(t, compiled, exitTable)
		exitPriority := configWithTunnel(t, "exit", "wss")
		routing(t, exitPriority)["priority"] = 10100
		assertSchemaRejects(t, compiled, exitPriority)
	})
	t.Run("TLS paths and roles are strict", func(t *testing.T) {
		entry := configWithTunnel(t, "entry", "wss")
		tls(t, entry)["ca_file"] = "relative.pem"
		assertSchemaRejects(t, compiled, entry)
		ipServerName := configWithTunnel(t, "entry", "wss")
		tls(t, ipServerName)["server_name"] = "198.51.100.2"
		assertSchemaRejects(t, compiled, ipServerName)
		maximumPath := configWithTunnel(t, "entry", "wss")
		tls(t, maximumPath)["ca_file"] = "/" + strings.Repeat("a", 4095)
		assertSchemaAccepts(t, compiled, maximumPath)
		tooLongPath := configWithTunnel(t, "entry", "wss")
		tls(t, tooLongPath)["ca_file"] = "/" + strings.Repeat("a", 4096)
		assertSchemaRejects(t, compiled, tooLongPath)
		exit := configWithTunnel(t, "exit", "wss")
		tls(t, exit)["server_name"] = "entry.example.com"
		assertSchemaRejects(t, compiled, exit)
		missingEntryCert := configWithTunnel(t, "entry", "wss")
		tls(t, missingEntryCert)["cert_file"] = ""
		assertSchemaRejects(t, compiled, missingEntryCert)
		missingExitCA := configWithTunnel(t, "exit", "quic")
		tls(t, missingExitCA)["ca_file"] = ""
		assertSchemaRejects(t, compiled, missingExitCA)
	})
	t.Run("transport-specific WSS path", func(t *testing.T) {
		wss := configWithTunnel(t, "entry", "wss")
		tunnel(t, wss)["wss_path"] = ""
		assertSchemaRejects(t, compiled, wss)
		unsafeWSS := configWithTunnel(t, "entry", "wss")
		tunnel(t, unsafeWSS)["wss_path"] = "/mesh?token=unsafe"
		assertSchemaRejects(t, compiled, unsafeWSS)
		doubleSlash := configWithTunnel(t, "entry", "wss")
		tunnel(t, doubleSlash)["wss_path"] = "//mesh"
		assertSchemaRejects(t, compiled, doubleSlash)
		quic := configWithTunnel(t, "entry", "quic")
		tunnel(t, quic)["wss_path"] = "/must-be-empty"
		assertSchemaRejects(t, compiled, quic)
	})
	t.Run("IPv6 and unsafe route values are rejected", func(t *testing.T) {
		ipv6 := configWithTunnel(t, "entry", "quic")
		tun(t, ipv6)["address"] = "2001:db8::2/64"
		assertSchemaRejects(t, compiled, ipv6)
		reserved := configWithTunnel(t, "entry", "quic")
		routing(t, reserved)["table"] = 253
		assertSchemaRejects(t, compiled, reserved)
		peerPrefix := configWithTunnel(t, "entry", "quic")
		tun(t, peerPrefix)["peer_address"] = "172.31.66.1/24"
		assertSchemaRejects(t, compiled, peerPrefix)
	})
	t.Run("MTU and health limits are enforced", func(t *testing.T) {
		mtu := configWithTunnel(t, "entry", "quic")
		tun(t, mtu)["mtu"] = 9000
		assertSchemaAccepts(t, compiled, mtu)
		tun(t, mtu)["mtu"] = 9001
		assertSchemaRejects(t, compiled, mtu)
		health := configWithTunnel(t, "entry", "quic")
		tunnel(t, health)["health"].(map[string]any)["timeout_seconds"] = 31
		assertSchemaRejects(t, compiled, health)
		disabled := configWithTunnel(t, "entry", "quic")
		disabledHealth := tunnel(t, disabled)["health"].(map[string]any)
		disabledHealth["enabled"] = false
		disabledHealth["target"] = ""
		disabledHealth["source_address"] = ""
		assertSchemaAccepts(t, compiled, disabled)
		enabled := configWithTunnel(t, "entry", "quic")
		tunnel(t, enabled)["health"].(map[string]any)["target"] = ""
		assertSchemaRejects(t, compiled, enabled)
		missingSource := configWithTunnel(t, "entry", "quic")
		tunnel(t, missingSource)["health"].(map[string]any)["source_address"] = ""
		assertSchemaRejects(t, compiled, missingSource)
	})
	t.Run("tunnel and CIDR array limits are enforced", func(t *testing.T) {
		config := configWithTunnel(t, "entry", "quic")
		allTunnels := makeNonConflictingTunnels(t, 129)
		tunnels := allTunnels[:128]
		config["tunnels"] = tunnels
		assertSchemaAccepts(t, compiled, config)

		duplicate := configWithTunnel(t, "entry", "quic")
		duplicate["tunnels"] = []any{tunnels[0], cloneObject(t, tunnels[0].(map[string]any))}
		assertSchemaRejects(t, compiled, duplicate)

		config["tunnels"] = allTunnels
		assertSchemaRejects(t, compiled, config)

		cidrs := configWithTunnel(t, "entry", "quic")
		sources := make([]any, 129)
		for index := range sources {
			sources[index] = fmt.Sprintf("10.1.%d.0/24", index%256)
		}
		routing(t, cidrs)["source_cidrs"] = sources
		assertSchemaRejects(t, compiled, cidrs)
	})
	t.Run("identifier and interface name syntax is enforced", func(t *testing.T) {
		config := configWithTunnel(t, "entry", "quic")
		tunnel(t, config)["id"] = "9.mesh"
		assertSchemaAccepts(t, compiled, config)
		tun(t, config)["name"] = "."
		assertSchemaRejects(t, compiled, config)
	})
	t.Run("unknown field", func(t *testing.T) {
		config := configWithTunnel(t, "entry", "quic")
		tunnel(t, config)["protocol_profile"] = "wss-tuic"
		assertSchemaRejects(t, compiled, config)
	})
}

func configWithTunnel(t *testing.T, role, transport string) map[string]any {
	t.Helper()
	value := map[string]any{
		"api_version":      "anixops.gost-mesh/v1",
		"apply":            true,
		"rollback_on_exit": true,
		"tunnels": []any{map[string]any{
			"id": "mesh-one", "role": role, "transport": transport,
			"tun": map[string]any{
				"name": "anixgost0", "address": "172.31.66.2/24", "peer_address": "172.31.66.1",
				"port": 18421, "mtu": 1280,
			},
			"routing": map[string]any{
				"source_cidrs": []any{"10.66.0.0/24"}, "route_cidrs": []any{},
				"table": 100, "priority": 10100,
			},
			"tls": map[string]any{
				"server_name": "exit.example.com", "ca_file": "/run/anixops/secrets/mesh-ca.pem",
				"cert_file": "/run/anixops/secrets/mesh-client.pem", "key_file": "/run/anixops/secrets/mesh-client-key.pem",
			},
			"wss_path": "/anixops-mesh",
			"health": map[string]any{
				"enabled": true, "target": "1.1.1.1:443", "source_address": "10.66.0.1", "interval_seconds": 15,
				"timeout_seconds": 3, "failure_threshold": 3, "restart_delay_seconds": 3, "restart_limit": 10,
			},
		}},
	}
	item := tunnel(t, value)
	if role == "entry" {
		item["remote"] = map[string]any{"host": "exit.example.com", "port": 443}
	} else {
		item["listen"] = map[string]any{"address": "0.0.0.0", "port": 443}
		item["tun"].(map[string]any)["address"] = "172.31.66.1/24"
		item["tun"].(map[string]any)["peer_address"] = "172.31.66.2"
		item["routing"].(map[string]any)["source_cidrs"] = []any{}
		item["routing"].(map[string]any)["route_cidrs"] = []any{"10.66.0.0/24"}
		delete(item["routing"].(map[string]any), "table")
		delete(item["routing"].(map[string]any), "priority")
		item["tls"] = map[string]any{
			"server_name": "", "ca_file": "/run/anixops/secrets/mesh-ca.pem", "cert_file": "/run/anixops/secrets/mesh-cert.pem",
			"key_file": "/run/anixops/secrets/mesh-key.pem",
		}
	}
	if transport == "quic" {
		item["wss_path"] = ""
	}
	return value
}

func makeNonConflictingTunnels(t *testing.T, count int) []any {
	t.Helper()
	tunnels := make([]any, count)
	for index := range tunnels {
		config := configWithTunnel(t, "entry", "quic")
		item := tunnel(t, config)
		item["id"] = fmt.Sprintf("mesh-%03d", index)
		item["tun"] = map[string]any{
			"name":         fmt.Sprintf("anxg%03d", index),
			"address":      fmt.Sprintf("172.16.%d.2/30", index),
			"peer_address": fmt.Sprintf("172.16.%d.1", index),
			"port":         18000 + index,
			"mtu":          1280,
		}
		item["routing"] = map[string]any{
			"source_cidrs": []any{fmt.Sprintf("10.0.%d.0/24", index)},
			"route_cidrs":  []any{},
			"table":        index + 1,
			"priority":     10000 + index,
		}
		item["health"].(map[string]any)["source_address"] = fmt.Sprintf("10.0.%d.1", index)
		tunnels[index] = item
	}
	return tunnels
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

func cloneObject(t *testing.T, value map[string]any) map[string]any {
	t.Helper()
	return cloneConfig(t, value)
}

func tunnel(t *testing.T, config map[string]any) map[string]any {
	t.Helper()
	return config["tunnels"].([]any)[0].(map[string]any)
}

func tun(t *testing.T, config map[string]any) map[string]any {
	t.Helper()
	return tunnel(t, config)["tun"].(map[string]any)
}

func routing(t *testing.T, config map[string]any) map[string]any {
	t.Helper()
	return tunnel(t, config)["routing"].(map[string]any)
}

func tls(t *testing.T, config map[string]any) map[string]any {
	t.Helper()
	return tunnel(t, config)["tls"].(map[string]any)
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
