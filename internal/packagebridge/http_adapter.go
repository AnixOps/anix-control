package packagebridge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
)

type httpRequestMetadata struct {
	Path                             string              `json:"path,omitempty"`
	Query                            map[string][]string `json:"query,omitempty"`
	Headers                          map[string][]string `json:"headers,omitempty"`
	PathParams                       map[string]string   `json:"path_params,omitempty"`
	ClientIP                         string              `json:"client_ip,omitempty"`
	UserAgent                        string              `json:"user_agent,omitempty"`
	NodeID                           uint                `json:"node_id,omitempty"`
	TrustedAgentWebSocketAuth        bool                `json:"trusted_agent_websocket_auth,omitempty"`
	TrustedAgentWebSocketForwardNode bool                `json:"trusted_agent_websocket_forward_node,omitempty"`
}

type bridgePrincipal struct {
	ActorID uint   `json:"actor_id"`
	Admin   bool   `json:"admin"`
	Package string `json:"package_id"`
}

// NewHTTPAdapter turns a deliberately allowlisted legacy Gin handler into a
// bridge operation. It uses only the request snapshot retained by Session;
// package-supplied payload bytes are never treated as an HTTP request.
func NewHTTPAdapter(handler gin.HandlerFunc) OperationHandler {
	return func(ctx context.Context, call Call) (Response, error) {
		if handler == nil {
			return Response{}, ErrCapabilityRejected
		}
		metadata, err := decodeHTTPMetadata(call.Request.MetadataJSON)
		if err != nil {
			return Response{}, ErrCapabilityRejected
		}
		principal, err := decodeBridgePrincipal(call.Request.PrincipalJSON, call.Host.PackageID)
		if err != nil {
			return Response{}, ErrCapabilityRejected
		}
		request, err := bridgeHTTPRequest(ctx, call.Request, metadata)
		if err != nil {
			return Response{}, ErrCapabilityRejected
		}
		recorder := httptest.NewRecorder()
		ginContext, _ := gin.CreateTestContext(recorder)
		ginContext.Request = request
		ginContext.Set("request_id", call.Request.RequestID)
		ginContext.Set("user_id", principal.ActorID)
		ginContext.Set("is_admin", principal.Admin)
		if metadata.NodeID != 0 {
			ginContext.Set("node_id", metadata.NodeID)
		}
		for key, value := range metadata.PathParams {
			ginContext.Params = append(ginContext.Params, gin.Param{Key: key, Value: value})
		}
		handler(ginContext)
		response := Response{StatusCode: uint32(recorder.Code), Body: append([]byte(nil), recorder.Body.Bytes()...), Headers: responseHeaders(recorder.Header())}
		if response.StatusCode == 0 {
			response.StatusCode = http.StatusOK
		}
		if err := validateResponse(response); err != nil {
			return Response{}, err
		}
		return response, nil
	}
}

func decodeHTTPMetadata(raw []byte) (httpRequestMetadata, error) {
	var metadata httpRequestMetadata
	if err := decodeStrictJSON(raw, &metadata); err != nil {
		return httpRequestMetadata{}, err
	}
	if !strings.HasPrefix(metadata.Path, "/api/v2/") || strings.ContainsAny(metadata.Path, "\r\n\x00") {
		return httpRequestMetadata{}, errors.New("bridge HTTP path is invalid")
	}
	if metadata.ClientIP != "" && net.ParseIP(metadata.ClientIP) == nil {
		return httpRequestMetadata{}, errors.New("bridge client IP is invalid")
	}
	if !validBridgeUserAgent(metadata.UserAgent) {
		return httpRequestMetadata{}, errors.New("bridge user agent is invalid")
	}
	if metadata.TrustedAgentWebSocketAuth && metadata.NodeID == 0 {
		return httpRequestMetadata{}, errors.New("bridge trusted agent identity is invalid")
	}
	if metadata.TrustedAgentWebSocketForwardNode && !metadata.TrustedAgentWebSocketAuth {
		return httpRequestMetadata{}, errors.New("bridge trusted agent identity is invalid")
	}
	if len(metadata.Query) > 64 || len(metadata.Headers) > 64 || len(metadata.PathParams) > 32 {
		return httpRequestMetadata{}, errors.New("bridge request metadata is too large")
	}
	for key, values := range metadata.Query {
		if key == "" || len(key) > 256 || len(values) > 32 {
			return httpRequestMetadata{}, errors.New("bridge query metadata is invalid")
		}
		for _, value := range values {
			if len(value) > 4096 || strings.ContainsAny(value, "\r\n\x00") {
				return httpRequestMetadata{}, errors.New("bridge query metadata is invalid")
			}
		}
	}
	for key, value := range metadata.PathParams {
		if key == "" || len(key) > 256 || len(value) > 4096 || strings.ContainsAny(value, "\r\n\x00") {
			return httpRequestMetadata{}, errors.New("bridge path parameter metadata is invalid")
		}
	}
	for key, values := range metadata.Headers {
		if !validBridgeRequestHeaderName(key) || blockedBridgeRequestHeader(key) || len(values) == 0 || len(values) > 32 {
			return httpRequestMetadata{}, errors.New("bridge request headers are invalid")
		}
		for _, value := range values {
			if len(value) == 0 || len(value) > 8192 || strings.ContainsAny(value, "\r\n\x00") {
				return httpRequestMetadata{}, errors.New("bridge request headers are invalid")
			}
		}
	}
	return metadata, nil
}

func decodeBridgePrincipal(raw []byte, packageID string) (bridgePrincipal, error) {
	var principal bridgePrincipal
	if err := decodeStrictJSON(raw, &principal); err != nil {
		return bridgePrincipal{}, err
	}
	if principal.Package != packageID {
		return bridgePrincipal{}, errors.New("bridge principal package is invalid")
	}
	return principal, nil
}

func decodeStrictJSON(raw []byte, destination any) error {
	if len(raw) == 0 {
		return errors.New("bridge JSON is required")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("bridge JSON is invalid")
	}
	return nil
}

func bridgeHTTPRequest(ctx context.Context, request Request, metadata httpRequestMetadata) (*http.Request, error) {
	if request.Method == "" || strings.ContainsAny(request.Method, "\r\n\x00") {
		return nil, errors.New("bridge HTTP method is invalid")
	}
	target := &url.URL{Scheme: "http", Host: "package-bridge", Path: metadata.Path}
	query := make(url.Values, len(metadata.Query))
	for key, values := range metadata.Query {
		query[key] = append([]string(nil), values...)
	}
	target.RawQuery = query.Encode()
	httpRequest, err := http.NewRequestWithContext(ctx, request.Method, target.String(), bytes.NewReader(request.Body))
	if err != nil {
		return nil, err
	}
	if metadata.ClientIP != "" {
		httpRequest.RemoteAddr = net.JoinHostPort(metadata.ClientIP, "0")
	}
	for name, values := range metadata.Headers {
		for _, value := range values {
			httpRequest.Header.Add(name, value)
		}
	}
	if metadata.UserAgent != "" {
		httpRequest.Header.Set("User-Agent", metadata.UserAgent)
	}
	return httpRequest, nil
}

func validBridgeRequestHeaderName(value string) bool {
	if value == "" || len(value) > 256 {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '-' {
			continue
		}
		return false
	}
	return true
}

func blockedBridgeRequestHeader(value string) bool {
	switch strings.ToLower(value) {
	case "authorization", "proxy-authorization", "cookie", "set-cookie", "x-api-key", "x-app-token",
		"connection", "keep-alive", "proxy-connection", "te", "trailer", "transfer-encoding", "upgrade", "content-length":
		return true
	default:
		return false
	}
}

func validBridgeUserAgent(value string) bool {
	if len(value) > 1024 || strings.ContainsAny(value, "\r\n\x00") {
		return false
	}
	return true
}

func responseHeaders(headers http.Header) []Header {
	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]Header, 0, len(headers))
	for _, key := range keys {
		for _, value := range headers.Values(key) {
			result = append(result, Header{Name: key, Value: value})
		}
	}
	return result
}
