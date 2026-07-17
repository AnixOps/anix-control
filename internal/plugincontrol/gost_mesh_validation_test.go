package plugincontrol

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type gostMeshSemanticContract struct {
	Format         string                         `json:"format"`
	MaxConfigBytes int                            `json:"max_config_bytes"`
	ValidCases     []gostMeshSemanticContractCase `json:"valid_cases"`
	InvalidCases   []gostMeshSemanticContractCase `json:"invalid_cases"`
}

type gostMeshSemanticContractCase struct {
	Name            string          `json:"name"`
	Generator       string          `json:"generator"`
	Role            string          `json:"role"`
	Transport       string          `json:"transport"`
	Pointer         string          `json:"pointer"`
	Value           json.RawMessage `json:"value"`
	ExpectedError   string          `json:"expected_error"`
	Count           int             `json:"count"`
	ExitRouteZeroes bool            `json:"exit_route_zeroes"`
}

func gostMeshContractHealth(enabled bool, sourceAddress string) map[string]any {
	target := ""
	if enabled {
		target = "1.1.1.1:443"
	}
	return map[string]any{
		"enabled": enabled, "target": target, "source_address": sourceAddress,
		"interval_seconds": 15, "timeout_seconds": 3, "failure_threshold": 3,
		"restart_delay_seconds": 3, "restart_limit": 10,
	}
}

func gostMeshContractEntry(index int, transport string) map[string]any {
	wssPath := ""
	if transport == "wss" {
		wssPath = "/anixops-mesh"
	}
	return map[string]any{
		"id": fmt.Sprintf("mesh-entry-%03d", index), "role": "entry", "transport": transport,
		"remote": map[string]any{"host": "exit.example.com", "port": 443},
		"tun": map[string]any{
			"name": fmt.Sprintf("anxg%03d", index), "address": fmt.Sprintf("172.16.%d.2/30", index),
			"peer_address": fmt.Sprintf("172.16.%d.1", index), "port": 18000 + index, "mtu": 1280,
		},
		"routing": map[string]any{
			"source_cidrs": []any{fmt.Sprintf("10.0.%d.0/24", index)}, "route_cidrs": []any{},
			"table": index + 1, "priority": 10000 + index,
		},
		"tls": map[string]any{
			"server_name": "exit.example.com", "ca_file": "/run/anixops/secrets/mesh-ca.pem",
			"cert_file": "/run/anixops/secrets/mesh-client.pem", "key_file": "/run/anixops/secrets/mesh-client-key.pem",
		},
		"wss_path": wssPath,
		"health":   gostMeshContractHealth(true, fmt.Sprintf("10.0.%d.1", index)),
	}
}

func gostMeshContractExit(transport string, explicitZeroes bool) map[string]any {
	routing := map[string]any{"source_cidrs": []any{}, "route_cidrs": []any{"10.66.0.0/24"}}
	if explicitZeroes {
		routing["table"], routing["priority"] = 0, 0
	}
	wssPath := ""
	if transport == "wss" {
		wssPath = "/anixops-mesh"
	}
	return map[string]any{
		"id": "mesh-exit-" + transport, "role": "exit", "transport": transport,
		"listen": map[string]any{"address": "0.0.0.0", "port": 443},
		"tun": map[string]any{
			"name": "anx" + transport + "x", "address": "172.31.66.1/30",
			"peer_address": "172.31.66.2", "port": 18421, "mtu": 1280,
		},
		"routing": routing,
		"tls": map[string]any{
			"server_name": "", "ca_file": "/run/anixops/secrets/mesh-ca.pem",
			"cert_file": "/run/anixops/secrets/mesh-server.pem", "key_file": "/run/anixops/secrets/mesh-server-key.pem",
		},
		"wss_path": wssPath,
		"health":   gostMeshContractHealth(false, ""),
	}
}

func gostMeshContractConfig(tunnels []any) map[string]any {
	return map[string]any{
		"api_version": gostMeshAPIVersion, "apply": true, "rollback_on_exit": true, "tunnels": tunnels,
	}
}

func generateGostMeshContractCase(t *testing.T, testCase gostMeshSemanticContractCase, maxConfigBytes int) []byte {
	t.Helper()
	var value map[string]any
	switch testCase.Generator {
	case "single":
		if testCase.Role == "entry" {
			value = gostMeshContractConfig([]any{gostMeshContractEntry(66, testCase.Transport)})
		} else {
			value = gostMeshContractConfig([]any{gostMeshContractExit(testCase.Transport, testCase.ExitRouteZeroes)})
		}
	case "entry-set":
		tunnels := make([]any, 0, testCase.Count)
		for index := 0; index < testCase.Count; index++ {
			transport := "wss"
			if index%2 == 0 {
				transport = "quic"
			}
			tunnels = append(tunnels, gostMeshContractEntry(index, transport))
		}
		value = gostMeshContractConfig(tunnels)
	case "duplicate-object":
		value = gostMeshContractConfig([]any{gostMeshContractEntry(66, "quic"), gostMeshContractEntry(66, "quic")})
	case "oversized-document":
		tunnels := make([]any, 0, 128)
		padding := strings.Repeat("a", 768)
		for index := 0; index < 128; index++ {
			tunnel := gostMeshContractEntry(index, "quic")
			tunnel["tls"] = map[string]any{
				"server_name": "exit.example.com",
				"ca_file":     fmt.Sprintf("/run/anixops/secrets/ca-%03d-%s.pem", index, padding),
				"cert_file":   fmt.Sprintf("/run/anixops/secrets/cert-%03d-%s.pem", index, padding),
				"key_file":    fmt.Sprintf("/run/anixops/secrets/key-%03d-%s.pem", index, padding),
			}
			tunnels = append(tunnels, tunnel)
		}
		value = gostMeshContractConfig(tunnels)
	default:
		t.Fatalf("unsupported semantic contract generator %q", testCase.Generator)
	}
	if testCase.Pointer != "" {
		var replacement any
		decoder := json.NewDecoder(bytes.NewReader(testCase.Value))
		decoder.UseNumber()
		require.NoError(t, decoder.Decode(&replacement))
		applyGostMeshContractPointer(t, value, testCase.Pointer, replacement)
	}
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	if testCase.Generator == "oversized-document" {
		require.Greater(t, len(encoded), maxConfigBytes)
	}
	return encoded
}

func applyGostMeshContractPointer(t *testing.T, value any, pointer string, replacement any) {
	t.Helper()
	parts := strings.Split(strings.TrimPrefix(pointer, "/"), "/")
	current := value
	for _, rawPart := range parts[:len(parts)-1] {
		part := strings.ReplaceAll(strings.ReplaceAll(rawPart, "~1", "/"), "~0", "~")
		switch typed := current.(type) {
		case map[string]any:
			current = typed[part]
		case []any:
			index, err := strconv.Atoi(part)
			require.NoError(t, err)
			current = typed[index]
		default:
			t.Fatalf("pointer %q traverses unsupported value %T", pointer, current)
		}
	}
	last := strings.ReplaceAll(strings.ReplaceAll(parts[len(parts)-1], "~1", "/"), "~0", "~")
	switch typed := current.(type) {
	case map[string]any:
		typed[last] = replacement
	case []any:
		index, err := strconv.Atoi(last)
		require.NoError(t, err)
		typed[index] = replacement
	default:
		t.Fatalf("pointer %q targets unsupported value %T", pointer, current)
	}
}

func TestGostMeshConfigurationValidatorUsesSharedSemanticContract(t *testing.T) {
	contractPath := filepath.Join("..", "..", "packages", "gost-mesh", "tests", "semantic_contract_cases.json")
	contents, err := os.ReadFile(contractPath)
	require.NoError(t, err)
	var contract gostMeshSemanticContract
	require.NoError(t, json.Unmarshal(contents, &contract))
	require.Equal(t, "anixops.gost-mesh.semantic-cases/v1", contract.Format)
	require.Equal(t, gostMeshMaxConfigSize, contract.MaxConfigBytes)
	require.NotEmpty(t, contract.ValidCases)
	require.NotEmpty(t, contract.InvalidCases)

	executor := NewGostMeshExecutor(nil)
	for _, testCase := range contract.ValidCases {
		t.Run("valid/"+testCase.Name, func(t *testing.T) {
			raw := generateGostMeshContractCase(t, testCase, contract.MaxConfigBytes)
			require.NoError(t, executor.ValidateConfiguration(context.Background(), raw))
		})
	}
	for _, testCase := range contract.InvalidCases {
		t.Run("invalid/"+testCase.Name, func(t *testing.T) {
			raw := generateGostMeshContractCase(t, testCase, contract.MaxConfigBytes)
			err := executor.ValidateConfiguration(context.Background(), raw)
			require.ErrorIs(t, err, ErrInvalidPluginInput)
			require.ErrorContains(t, err, testCase.ExpectedError)
		})
	}
}

func TestGostMeshConfigurationValidatorRejectsCrossTunnelConflicts(t *testing.T) {
	tests := []struct {
		name          string
		mutate        func(first, second map[string]any)
		expectedError string
	}{
		{name: "tun name", mutate: func(first, second map[string]any) {
			second["tun"].(map[string]any)["name"] = first["tun"].(map[string]any)["name"]
		}, expectedError: "same TUN name"},
		{name: "routing table", mutate: func(first, second map[string]any) {
			second["routing"].(map[string]any)["table"] = first["routing"].(map[string]any)["table"]
		}, expectedError: "same routing table"},
		{name: "routing priority", mutate: func(first, second map[string]any) {
			second["routing"].(map[string]any)["priority"] = first["routing"].(map[string]any)["priority"]
		}, expectedError: "same routing priority"},
		{name: "tun network", mutate: func(first, second map[string]any) {
			second["tun"].(map[string]any)["address"] = "172.16.66.1/30"
			second["tun"].(map[string]any)["peer_address"] = "172.16.66.2"
		}, expectedError: "overlapping TUN networks"},
		{name: "source CIDR", mutate: func(first, second map[string]any) {
			second["routing"].(map[string]any)["source_cidrs"] = first["routing"].(map[string]any)["source_cidrs"]
			second["health"].(map[string]any)["source_address"] = "10.0.66.2"
		}, expectedError: "overlapping source CIDRs"},
	}
	executor := NewGostMeshExecutor(nil)
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			first := gostMeshContractEntry(66, "quic")
			second := gostMeshContractEntry(67, "wss")
			testCase.mutate(first, second)
			raw, err := json.Marshal(gostMeshContractConfig([]any{first, second}))
			require.NoError(t, err)
			err = executor.ValidateConfiguration(context.Background(), raw)
			require.ErrorIs(t, err, ErrInvalidPluginInput)
			require.ErrorContains(t, err, testCase.expectedError)
		})
	}
}

func TestGostMeshConfigurationValidatorRejectsListenerAndRouteConflicts(t *testing.T) {
	first := gostMeshContractExit("wss", false)
	second := gostMeshContractExit("wss", true)
	second["id"] = "mesh-exit-second"
	second["tun"] = map[string]any{"name": "anxsecond", "address": "172.31.67.1/30", "peer_address": "172.31.67.2", "port": 18422, "mtu": 1280}
	second["routing"].(map[string]any)["route_cidrs"] = []any{"10.67.0.0/24"}
	second["listen"] = map[string]any{"address": "10.0.0.1", "port": 443}
	raw, err := json.Marshal(gostMeshContractConfig([]any{first, second}))
	require.NoError(t, err)
	err = NewGostMeshExecutor(nil).ValidateConfiguration(context.Background(), raw)
	require.ErrorContains(t, err, "conflicting wss listeners")

	second["listen"] = map[string]any{"address": "10.0.0.1", "port": 444}
	second["routing"].(map[string]any)["route_cidrs"] = []any{"10.66.0.0/25"}
	raw, err = json.Marshal(gostMeshContractConfig([]any{first, second}))
	require.NoError(t, err)
	err = NewGostMeshExecutor(nil).ValidateConfiguration(context.Background(), raw)
	require.ErrorContains(t, err, "overlapping route CIDRs")

	second["routing"].(map[string]any)["route_cidrs"] = []any{"10.67.0.0/24"}
	second["tun"].(map[string]any)["port"] = 18421
	raw, err = json.Marshal(gostMeshContractConfig([]any{first, second}))
	require.NoError(t, err)
	err = NewGostMeshExecutor(nil).ValidateConfiguration(context.Background(), raw)
	require.ErrorContains(t, err, "same TUN port")
}

func TestGostMeshConfigurationValidatorRejectsNonAgentJSONNumbersAndStrings(t *testing.T) {
	valid, err := json.Marshal(gostMeshContractConfig([]any{gostMeshContractEntry(66, "quic")}))
	require.NoError(t, err)
	executor := NewGostMeshExecutor(nil)
	for _, replacement := range []string{`"port":443.0`, `"port":1e2`} {
		raw := strings.Replace(string(valid), `"port":443`, replacement, 1)
		err = executor.ValidateConfiguration(context.Background(), json.RawMessage(raw))
		require.ErrorIs(t, err, ErrInvalidPluginInput)
		require.ErrorContains(t, err, "cannot unmarshal number")
	}

	value := gostMeshContractConfig([]any{gostMeshContractEntry(66, "quic")})
	value["tunnels"].([]any)[0].(map[string]any)["role"] = " Entry "
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	err = executor.ValidateConfiguration(context.Background(), raw)
	require.ErrorContains(t, err, "role must use its canonical form")
}

func TestRegistryConfigurationValidationIsOptionalAndVersionBound(t *testing.T) {
	registry, err := NewRegistry(
		registryTestExecutor{id: "schema-only", version: "1.0.0"},
		NewGostMeshExecutor(nil),
	)
	require.NoError(t, err)
	require.NoError(t, registry.ValidateConfiguration(context.Background(), "schema-only", "1.0.0", json.RawMessage(`{"anything":true}`)))
	require.NoError(t, registry.ValidateConfiguration(context.Background(), "unregistered", "9.9.9", json.RawMessage(`{"anything":true}`)))

	err = registry.ValidateConfiguration(context.Background(), GostMeshPluginID, "2.0.0", json.RawMessage(`{}`))
	require.ErrorIs(t, err, ErrConfigurationValidatorNotFound)
	err = registry.ValidateConfiguration(context.Background(), GostMeshPluginID, GostMeshVersion, json.RawMessage(`{}`))
	require.ErrorIs(t, err, ErrInvalidPluginInput)

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	err = registry.ValidateConfiguration(cancelled, GostMeshPluginID, GostMeshVersion, json.RawMessage(`{}`))
	require.ErrorIs(t, err, context.Canceled)
}
