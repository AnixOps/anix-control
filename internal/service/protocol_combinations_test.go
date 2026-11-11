package service

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/require"
)

// kernelCombinations reads contracts/agent/v1/protocol-combinations.json,
// the protocol, TLS mode and transport combinations the Agent's kernel
// runs (anix-agent maps each of them in its own test), as "type/tls/transport".
func kernelCombinations(t *testing.T) (listed map[string]bool, types []model.ProtocolType, transports []string) {
	raw, err := os.ReadFile("../../contracts/agent/v1/protocol-combinations.json")
	require.NoError(t, err)
	var table struct {
		Combinations []struct {
			Type       model.ProtocolType `json:"type"`
			TLS        []int              `json:"tls"`
			Transports []string           `json:"transports"`
		} `json:"combinations"`
	}
	require.NoError(t, json.Unmarshal(raw, &table))
	listed = map[string]bool{}
	for _, row := range table.Combinations {
		types = append(types, row.Type)
		transports = append(transports, row.Transports...)
		for _, tls := range row.TLS {
			for _, transport := range row.Transports {
				listed[fmt.Sprintf("%s/%d/%s", row.Type, tls, transport)] = true
			}
		}
	}
	return listed, types, transports
}

// Saving is held to the table where it is exact: it never refuses a listed
// combination, Reality (tls 2) is accepted only where listed, and the
// quick-add templates are listed. For TLS 0 and 1 saving stays lenient by
// design (validateProtocolTransport: the TLS column and transports such as
// h2 are labels on stored rows), so the API still saves unlisted
// combinations there. What the editor offers is held to the same file,
// combination by combination, by the web test protocolCombinations.test.js.
func TestProtocolCombinationsMatchTheKernel(t *testing.T) {
	listed, types, transports := kernelCombinations(t)
	// Transports the kernel does not run but stored protocols may name.
	transports = append(transports, "h2", "quic")
	for _, protocolType := range types {
		for tls := 0; tls <= 2; tls++ {
			for _, transport := range transports {
				key := fmt.Sprintf("%s/%d/%s", protocolType, tls, transport)
				err := validateProtocolTransport(&model.NodeProtocol{Type: protocolType, TLS: tls, Transport: &transport})
				if listed[key] {
					require.NoError(t, err, key)
				} else if tls == 2 {
					require.Error(t, err, key)
				}
			}
		}
	}
	for _, template := range model.GetProtocolTemplates() {
		require.True(t, listed[fmt.Sprintf("%s/%d/%s", template.Type, template.TLS, template.Transport)], template.Name)
	}
}
