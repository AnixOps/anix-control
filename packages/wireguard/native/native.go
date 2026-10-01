// Package native implements the wireguard package's one route in the package
// itself: the administrator's WireGuard server keypair for the protocol form.
// It reads and stores nothing: the keys are generated for the caller, who
// saves the private key into a protocol through the kernel's protocol routes
// (protocol-runtime; v2_node_protocol is a protected kernel table). Responses
// have the legacy handler's shape (internal/tests/wireguardcompat); the keys
// are random on both sides.
//
// The answer carries a private key either way: when the route is bridged,
// the legacy handler's answer passes through this package's host too.
package native

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"time"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/sdk/v2compat"
)

// Service holds what the native route needs.
type Service struct {
	// Now defaults to time.Now.
	Now func() time.Time
}

// Handlers returns the native handlers by route id.
func (s *Service) Handlers() map[string]pluginhostsdk.NativeHandler {
	return map[string]pluginhostsdk.NativeHandler{
		"wireguard.admin.wireguard.keypair.post": s.Keypair,
	}
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Keypair is POST /api/v2/admin/wireguard/keypair: a new X25519 keypair,
// both keys in standard base64, as the kernel's
// service.GenerateWireGuardKeypair generates it. The request body is
// ignored.
func (s *Service) Keypair(context.Context, pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		body, marshalErr := json.Marshal(map[string]any{"message": "生成 WireGuard 密钥失败"})
		if marshalErr != nil {
			return pluginhostsdk.NativeResponse{}, marshalErr
		}
		return pluginhostsdk.NativeResponse{
			StatusCode: http.StatusInternalServerError, Body: body,
			Headers: []pluginhostsdk.Header{{Name: "Content-Type", Value: "application/json; charset=utf-8"}},
		}, nil
	}
	return pluginhostsdk.PanelJSON(v2compat.PanelSuccess(map[string]any{
		"private_key": base64.StdEncoding.EncodeToString(key.Bytes()),
		"public_key":  base64.StdEncoding.EncodeToString(key.PublicKey().Bytes()),
	}, s.now()))
}
