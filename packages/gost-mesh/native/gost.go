package native

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// clientTimeout is the kernel gost client's default timeout.
const clientTimeout = 10 * time.Second

// client is the part of the kernel's gost API client (internal/gost) that
// the connection test uses: same requests, same errors.
type client struct {
	baseURL    string
	authPass   string
	httpClient *http.Client
}

func newClient(baseURL, token string) *client {
	return &client{baseURL: baseURL, authPass: token, httpClient: &http.Client{Timeout: clientTimeout}}
}

func (c *client) doRequest(ctx context.Context, method, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.authPass != "" {
		req.SetBasicAuth("", c.authPass)
	}
	// The administrator names the gost API to test, as in the kernel's
	// handler.
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		if closeErr := resp.Body.Close(); closeErr != nil {
			return nil, fmt.Errorf("close response after read failure: %w", closeErr)
		}
		return nil, fmt.Errorf("read response: %w", err)
	}
	if err := resp.Body.Close(); err != nil {
		return nil, fmt.Errorf("close response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error: %s - %s", resp.Status, string(respBody))
	}
	return respBody, nil
}

// healthCheck is the kernel client's HealthCheck: the service list answers.
func (c *client) healthCheck(ctx context.Context) error {
	_, err := c.doRequest(ctx, http.MethodGet, "/api/config/services")
	return err
}

// getServices is the kernel client's GetServices.
func (c *client) getServices(ctx context.Context) (*ServiceListResponse, error) {
	respBody, err := c.doRequest(ctx, http.MethodGet, "/api/config/services")
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data ServiceListResponse `json:"data"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	return &resp.Data, nil
}

// The gost API types below are the kernel client's: a decoding error names
// the type and field, so their names and fields are the same.

// ServiceListResponse is the gost service list.
type ServiceListResponse struct {
	Count int              `json:"count"`
	List  []*ServiceConfig `json:"list"`
}

// ServiceConfig is a gost service.
type ServiceConfig struct {
	Name      string            `json:"name"`
	Addr      string            `json:"addr"`
	Interface string            `json:"interface,omitempty"`
	Handler   *HandlerConfig    `json:"handler,omitempty"`
	Listener  *ListenerConfig   `json:"listener,omitempty"`
	Forwarder *ForwarderConfig  `json:"forwarder,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// HandlerConfig is a gost service's handler.
type HandlerConfig struct {
	Type     string            `json:"type"`
	Auth     *AuthConfig       `json:"auth,omitempty"`
	Chain    string            `json:"chain,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// ListenerConfig is a gost service's listener.
type ListenerConfig struct {
	Type     string            `json:"type"`
	Auth     *AuthConfig       `json:"auth,omitempty"`
	Chain    string            `json:"chain,omitempty"`
	TLS      *TLSConfig        `json:"tls,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// ForwarderConfig is a gost service's forwarder.
type ForwarderConfig struct {
	Nodes    []ForwarderNode `json:"nodes,omitempty"`
	Selector *SelectorConfig `json:"selector,omitempty"`
}

// ForwarderNode is a forwarding target.
type ForwarderNode struct {
	Name     string `json:"name"`
	Addr     string `json:"addr"`
	Protocol string `json:"protocol,omitempty"`
}

// SelectorConfig is a forwarder's node selector.
type SelectorConfig struct {
	Strategy    string `json:"strategy"`
	MaxFails    int    `json:"maxFails,omitempty"`
	FailTimeout string `json:"failTimeout,omitempty"`
}

// AuthConfig is a handler's or listener's authentication.
type AuthConfig struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

// TLSConfig is a listener's TLS.
type TLSConfig struct {
	CertFile   string `json:"certFile,omitempty"`
	KeyFile    string `json:"keyFile,omitempty"`
	CAFile     string `json:"caFile,omitempty"`
	ServerName string `json:"serverName,omitempty"`
	Secure     bool   `json:"secure,omitempty"`
}
