package main

import (
	"context"
	"net"
	"strings"
	"testing"

	appconfig "github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/panelcert"
	"github.com/stretchr/testify/require"
)

// With grpc.edge_proxies the name nodes dial resolves to the edge proxy, not
// to this machine, and the gRPC port is plain: doctor must not call either a
// failure, nor probe the port with TLS.
func TestDoctorDescribesTheEdgeLayout(t *testing.T) {
	db := openCommandDB(t)
	cfg := &appconfig.Config{Domain: "new-x.anixops.com", Listen: "127.0.0.1:8080"}
	cfg.TLS.Mode = appconfig.TLSModeExternal
	cfg.GRPC.PublicName = "grpc.new-x.anixops.com"
	cfg.GRPC.EdgeProxies = []string{"10.0.0.2"}
	cfg.ModuleRuntime.CAKEK = panelTestKEK
	env := doctorEnvironment{
		localAddresses: func() []net.IP { return []net.IP{net.ParseIP("10.0.0.1")} },
		resolver: fakeResolver{
			"new-x.anixops.com":      {"198.51.100.1"}, // the edge proxy
			"grpc.new-x.anixops.com": {"198.51.100.1"}, // the edge proxy
		},
		checkPort: func(address string) (string, string) {
			if strings.HasSuffix(address, ":80") {
				return panelcert.PortError, "permission denied"
			}
			return panelcert.PortInUse, "in use"
		},
		probeTLS: func(address, serverName string) ([]string, error) {
			t.Fatalf("the plain gRPC port must not be probed with TLS (%s, %s)", address, serverName)
			return nil, nil
		},
	}
	byID := doctorByID(doctorChecks(context.Background(), cfg, db, env))

	require.Equal(t, doctorInfo, byID["dns:grpc"].Level, "the edge proxy's address is expected for the gRPC name")
	require.Contains(t, byID["dns:grpc"].Message, "not this machine")
	require.Equal(t, doctorOK, byID["port:0.0.0.0:50051"].Level)
	require.Contains(t, byID["port:0.0.0.0:50051"].Message, "plain")
}
