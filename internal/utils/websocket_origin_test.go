package utils

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsWebSocketOriginAllowed(t *testing.T) {
	tests := []struct {
		name           string
		host           string
		origin         string
		allowedOrigins []string
		want           bool
	}{
		{
			name: "empty origin allowed for non browser clients",
			host: "panel.example.com",
			want: true,
		},
		{
			name:   "same host allowed",
			host:   "panel.example.com",
			origin: "https://panel.example.com",
			want:   true,
		},
		{
			name:   "same host with default port allowed",
			host:   "panel.example.com",
			origin: "https://panel.example.com:443",
			want:   true,
		},
		{
			name:   "configured origin allowed",
			host:   "api.example.com",
			origin: "https://admin.example.com",
			allowedOrigins: []string{
				"https://admin.example.com",
			},
			want: true,
		},
		{
			name:           "wildcard allowed explicitly",
			host:           "api.example.com",
			origin:         "https://any.example.net",
			allowedOrigins: []string{"*"},
			want:           true,
		},
		{
			name:   "cross origin rejected without allowlist",
			host:   "api.example.com",
			origin: "https://evil.example.net",
			want:   false,
		},
		{
			name:   "invalid origin rejected",
			host:   "api.example.com",
			origin: "not a url",
			want:   false,
		},
		{
			name:   "unsupported origin scheme rejected",
			host:   "api.example.com",
			origin: "file://api.example.com",
			want:   false,
		},
		{
			name:   "ws origin normalizes to http",
			host:   "api.example.com",
			origin: "ws://api.example.com",
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "http://"+tt.host+"/ws", nil)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}

			got := IsWebSocketOriginAllowed(req, tt.allowedOrigins)
			assert.Equal(t, tt.want, got)
		})
	}
}
