package sealedsecrets

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	configtables "github.com/AnixOps/anix-control/v4/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type extractionRow struct {
	Method  string `json:"method"`
	Path    string `json:"path"`
	Package string `json:"package_id"`
	RouteID string `json:"route_id"`
	Mode    string `json:"mode"`
}

func loadExtraction(t *testing.T) map[string]extractionRow {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "config", "package-extraction.json"))
	require.NoError(t, err)
	var document struct {
		Routes []extractionRow `json:"routes"`
	}
	require.NoError(t, json.Unmarshal(raw, &document))
	rows := map[string]extractionRow{}
	for _, row := range document.Routes {
		rows[row.RouteID] = row
	}
	return rows
}

// The kernel binary embeds config/node-secret-fields.json; every listed
// route is a route of the extraction map, and a path parameter target is
// one of its path's parameters.
func TestTheEmbeddedFieldListIsValid(t *testing.T) {
	onDisk, err := os.ReadFile(filepath.Join("..", "..", "config", "node-secret-fields.json"))
	require.NoError(t, err)
	require.Equal(t, string(onDisk), string(configtables.NodeSecretFields))
	table, err := ParseTable(configtables.NodeSecretFields)
	require.NoError(t, err)
	require.NotEmpty(t, table.RouteIDs())
	extraction := loadExtraction(t)
	for _, id := range table.RouteIDs() {
		row, ok := extraction[id]
		require.True(t, ok, "route %s is not in config/package-extraction.json", id)
		route, _ := table.Route(id)
		if route.Target.PathParam != "" {
			assert.Contains(t, strings.Split(row.Path, "/"), ":"+route.Target.PathParam, id)
		}
	}
	sealer := DefaultSealer()
	require.ElementsMatch(t, table.RouteIDs(), sealer.Table().RouteIDs())
	assert.True(t, sealer.Listed("proxy.admin.nodes.post"))
	assert.False(t, sealer.Listed("knowledge.article.list"))
}

// The routes and fields of node-ops-service.md section 6 that carry a node
// secret are listed.
func TestTheFieldListCoversTheDesignedRoutes(t *testing.T) {
	table, err := ParseTable(configtables.NodeSecretFields)
	require.NoError(t, err)
	request := func(id string) []string {
		route, ok := table.Route(id)
		require.True(t, ok, id)
		var pointers []string
		for _, field := range route.Request {
			pointers = append(pointers, field.Pointer)
		}
		return pointers
	}
	answer := func(id string) map[string]string {
		route, ok := table.Route(id)
		require.True(t, ok, id)
		names := map[string]string{}
		for _, field := range route.Answer {
			names[field.Pointer] = field.Name
		}
		return names
	}
	protocolColumns := []string{"/settings", "/tls_settings", "/transport_settings", "/reality_settings", "/custom_config"}
	assert.Equal(t, protocolColumns, request("protocol.admin.nodes.id.protocols.post"))
	assert.Equal(t, protocolColumns, request("protocol.admin.nodes.id.protocols.protocol_id.put"))
	for _, id := range []string{"proxy.admin.nodes.post", "proxy.admin.nodes.id.put", "proxy.admin.nodes.id.raw_config.put", "proxy.admin.nodes.validate_config.post"} {
		assert.Equal(t, []string{"/raw_config"}, request(id), id)
	}
	assert.Equal(t, []string{"/api_token"}, request("forward.admin.forward.nodes.post"))
	assert.Equal(t, []string{"/api_token"}, request("forward.admin.forward.nodes.id.put"))
	assert.Equal(t, map[string]string{"/data/api_key": "api_key", "/data/secret": "secret"}, answer("proxy.admin.nodes.post"))
	assert.Equal(t, map[string]string{"/data/api_token": "api_token"}, answer("forward.admin.forward.nodes.post"))
	assert.Equal(t, map[string]string{"/data/key": "key"}, answer("proxy.admin.auth_keys.post"))
	assert.Equal(t, map[string]string{"/data/key": "key"}, answer("proxy.internal.auth_keys.post"))
	assert.Equal(t, map[string]string{"/data/token": "token"}, answer("forward.admin.forward.agents.post"))
	validate, _ := table.Route("proxy.admin.nodes.validate_config.post")
	assert.Equal(t, TargetNone, validate.Target.Kind, "a validation stores nothing")
}

func TestParseTableRefusesMalformedLists(t *testing.T) {
	valid := `{"route_id":"r","target":{"kind":"forward","path_param":"id"},"request":[{"pointer":"/api_token","kind":"value"}]}`
	cases := map[string]string{
		"format":            `{"format":"other","routes":[]}`,
		"unknown field":     `{"format":"` + FieldsFormat + `","routes":[],"extra":1}`,
		"duplicate route":   `{"format":"` + FieldsFormat + `","routes":[` + valid + `,` + valid + `]}`,
		"target both":       `{"format":"` + FieldsFormat + `","routes":[{"route_id":"r","target":{"kind":"forward","path_param":"id","new":true},"request":[{"pointer":"/a","kind":"value"}]}]}`,
		"target neither":    `{"format":"` + FieldsFormat + `","routes":[{"route_id":"r","target":{"kind":"forward"},"request":[{"pointer":"/a","kind":"value"}]}]}`,
		"none with param":   `{"format":"` + FieldsFormat + `","routes":[{"route_id":"r","target":{"kind":"none","path_param":"id"},"request":[{"pointer":"/a","kind":"value"}]}]}`,
		"unknown target":    `{"format":"` + FieldsFormat + `","routes":[{"route_id":"r","target":{"kind":"user","new":true},"request":[{"pointer":"/a","kind":"value"}]}]}`,
		"no fields":         `{"format":"` + FieldsFormat + `","routes":[{"route_id":"r","target":{"kind":"forward","new":true}}]}`,
		"pointer":           `{"format":"` + FieldsFormat + `","routes":[{"route_id":"r","target":{"kind":"forward","new":true},"request":[{"pointer":"a","kind":"value"}]}]}`,
		"whole document":    `{"format":"` + FieldsFormat + `","routes":[{"route_id":"r","target":{"kind":"forward","new":true},"request":[{"pointer":"","kind":"value"}]}]}`,
		"spelled twice":     `{"format":"` + FieldsFormat + `","routes":[{"route_id":"r","target":{"kind":"forward","new":true},"request":[{"pointer":"/api_token","kind":"value"},{"pointer":"/ApiToken","kind":"value"}]}]}`,
		"field kind":        `{"format":"` + FieldsFormat + `","routes":[{"route_id":"r","target":{"kind":"forward","new":true},"request":[{"pointer":"/a","kind":"blob"}]}]}`,
		"answer name":       `{"format":"` + FieldsFormat + `","routes":[{"route_id":"r","target":{"kind":"forward","new":true},"answer":[{"pointer":"/data/a","name":""}]}]}`,
		"answer name twice": `{"format":"` + FieldsFormat + `","routes":[{"route_id":"r","target":{"kind":"forward","new":true},"answer":[{"pointer":"/data/a","name":"a"},{"pointer":"/data/b","name":"a"}]}]}`,
		"no route id":       `{"format":"` + FieldsFormat + `","routes":[{"route_id":"","target":{"kind":"forward","new":true},"answer":[{"pointer":"/data/a","name":"a"}]}]}`,
		"two documents":     `{"format":"` + FieldsFormat + `","routes":[]} {}`,
		"bad escape":        `{"format":"` + FieldsFormat + `","routes":[{"route_id":"r","target":{"kind":"forward","new":true},"answer":[{"pointer":"/data/~2","name":"a"}]}]}`,
	}
	for name, document := range cases {
		_, err := ParseTable([]byte(document))
		assert.Error(t, err, name)
	}
	table, err := ParseTable([]byte(`{"format":"` + FieldsFormat + `","routes":[` + valid + `]}`))
	require.NoError(t, err)
	_, ok := table.Route("r")
	assert.True(t, ok)
}

// Member names match a listed field as the legacy handlers bind them.
func TestFieldNamesMatchInEverySpelling(t *testing.T) {
	for _, spelling := range []string{"api_token", "API_TOKEN", "ApiToken", "apitoken", "api-token", "Api_Token", "api_to\u212aen"} {
		assert.Equal(t, foldKey("api_token"), foldKey(spelling), spelling)
	}
	assert.Equal(t, foldKey("settings"), foldKey("\u017fettings"), "the long s folds to s, as encoding/json binds it")
	assert.NotEqual(t, foldKey("api_token"), foldKey("api_tokens"))
}

func TestJSONPointers(t *testing.T) {
	tokens, err := splitPointer("/a~1b/c~0d/0")
	require.NoError(t, err)
	assert.Equal(t, []string{"a/b", "c~d", "0"}, tokens)
	assert.Equal(t, "/a~1b/c~0d/0", joinPointer(tokens))
	_, err = splitPointer("a")
	assert.Error(t, err)
}
