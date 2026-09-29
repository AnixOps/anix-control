package grpc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestServerRejectsPartialTLSConfig(t *testing.T) {
	tests := []struct {
		name string
		cert string
		key  string
	}{
		{name: "cert only", cert: "control.crt"},
		{name: "key only", key: "control.key"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := DefaultServerConfig()
			config.Host = "127.0.0.1"
			config.Port = 0
			config.TLSCertFile = test.cert
			config.TLSKeyFile = test.key

			err := NewServer(config).Start()
			require.ErrorContains(t, err, "cert and key must be configured together")
		})
	}
}
