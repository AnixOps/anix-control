package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
)

func TestForwardCommandDNS(t *testing.T) {
	db := newForwardCommandDB(t)
	previous := config.Get()
	config.Set(&config.Config{ModuleRuntime: config.ModuleRuntimeConfig{CAKEK: "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="}})
	t.Cleanup(func() { config.Set(previous) })
	ctx := context.Background()
	dir := t.TempDir()
	run := func(arguments ...string) (string, error) {
		var output bytes.Buffer
		err := runAdminCommand(ctx, nil, db, append([]string{"forward", "dns"}, arguments...), &output)
		return output.String(), err
	}
	write := func(name, body string) string {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
		return path
	}
	const secret = "dnspod-secret-key-42"

	for _, arguments := range [][]string{{}, {"providers"}, {"providers", "create"}, {"providers", "delete"}, {"bindings", "bogus"}, {"status"}, {"zones", "list"}} {
		_, err := run(arguments...)
		require.ErrorContains(t, err, "invalid forward command", arguments)
	}

	providerFile := write("provider.json", `{"provider":{"name":"pod","kind":"DNS_PROVIDER_KIND_DNSPOD"},"credentials":{"secret_id":"AKID1","secret_key":"`+secret+`"}}`)
	output, err := run("providers", "create", "-f", providerFile)
	require.NoError(t, err)
	assert.NotContains(t, output, secret)
	provider := &forwardv1.DnsProvider{}
	require.NoError(t, protojson.Unmarshal([]byte(output), provider))
	_, err = run("providers", "create", "-f", write("bad.json", `{"provider":{"name":"x","kind":"DNS_PROVIDER_KIND_DNSPOD"},"credentials":{}}`))
	require.ErrorContains(t, err, "credentials.secret_id")

	output, err = run("providers", "list")
	require.NoError(t, err)
	assert.Contains(t, output, "dnspod")
	assert.Contains(t, output, "secret_id,secret_key")
	assert.NotContains(t, output, secret)

	routeFile := write("route.json", `{"name":"hk-jp","listen":{"port":31000,"protocol":"L4_PROTOCOL_TCP","entry_hostname":"hk.example.com"},
	 "hops":[{"role":"HOP_ROLE_ENTRY","engine":"ENGINE_NFTABLES","node_refs":["forward-11"]},
	         {"role":"HOP_ROLE_EXIT","engine":"ENGINE_NFTABLES","node_refs":["forward-12"],"ingress":{"security":"LINK_SECURITY_RAW"}}],
	 "targets":[{"host":"198.51.100.10","port":443}]}`)
	var routeOut bytes.Buffer
	require.NoError(t, runAdminCommand(ctx, nil, db, []string{"forward", "routes", "create", "-f", routeFile}, &routeOut))
	route := &forwardv1.Route{}
	require.NoError(t, protojson.Unmarshal(routeOut.Bytes(), route))

	bindingFile := write("binding.json", `{"route_id":"`+route.GetId()+`","provider_id":"1","zone":"example.com"}`)
	output, err = run("bindings", "create", "-f", bindingFile)
	require.NoError(t, err)
	assert.Contains(t, output, `"record_name": "hk.example.com"`)
	output, err = run("bindings", "list", "--route", route.GetId())
	require.NoError(t, err)
	assert.Contains(t, output, "hk.example.com")
	output, err = run("status", route.GetId())
	require.NoError(t, err)
	assert.Contains(t, output, "state pending")

	_, err = run("providers", "delete", "1")
	require.ErrorContains(t, err, "--yes")
	_, err = run("providers", "delete", "1", "--yes")
	require.ErrorContains(t, err, "provider_in_use")
	_, err = run("bindings", "delete", "1", "--yes", "--purge")
	require.NoError(t, err)
	_, err = run("providers", "delete", "1", "--yes")
	require.NoError(t, err)

	var entries []model.OperationLog
	require.NoError(t, db.Where("username = ?", "system/cli").Find(&entries).Error)
	for _, entry := range entries {
		assert.NotContains(t, entry.Content, secret)
	}
	require.NotEmpty(t, entries)
}
