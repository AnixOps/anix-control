package v4api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// dnsForward answers the DNS methods of ForwardControl.
type dnsForward struct {
	forwardv1.ForwardControlClient
	created *forwardv1.CreateDnsProviderRequest
	updated *forwardv1.UpdateDnsProviderRequest
	deleted *forwardv1.DeleteDnsBindingRequest
	binding *forwardv1.CreateDnsBindingRequest
	err     error
}

var storedProvider = &forwardv1.DnsProvider{Id: 3, Name: "cf", Kind: forwardv1.DnsProviderKind_DNS_PROVIDER_KIND_CLOUDFLARE, CredentialNames: []string{"api_token"}}

func (f *dnsForward) CreateDnsProvider(_ context.Context, in *forwardv1.CreateDnsProviderRequest, _ ...grpc.CallOption) (*forwardv1.CreateDnsProviderResponse, error) {
	f.created = in
	if f.err != nil {
		return nil, f.err
	}
	return &forwardv1.CreateDnsProviderResponse{Provider: storedProvider}, nil
}

func (f *dnsForward) UpdateDnsProvider(_ context.Context, in *forwardv1.UpdateDnsProviderRequest, _ ...grpc.CallOption) (*forwardv1.UpdateDnsProviderResponse, error) {
	f.updated = in
	return &forwardv1.UpdateDnsProviderResponse{Provider: storedProvider}, nil
}

func (f *dnsForward) ListDnsProviders(context.Context, *forwardv1.ListDnsProvidersRequest, ...grpc.CallOption) (*forwardv1.ListDnsProvidersResponse, error) {
	return &forwardv1.ListDnsProvidersResponse{Providers: []*forwardv1.DnsProvider{storedProvider}}, nil
}

func (f *dnsForward) CreateDnsBinding(_ context.Context, in *forwardv1.CreateDnsBindingRequest, _ ...grpc.CallOption) (*forwardv1.CreateDnsBindingResponse, error) {
	f.binding = in
	if f.err != nil {
		return nil, f.err
	}
	binding := in.GetBinding()
	binding.Id = 9
	return &forwardv1.CreateDnsBindingResponse{Binding: binding}, nil
}

func (f *dnsForward) ListDnsBindings(context.Context, *forwardv1.ListDnsBindingsRequest, ...grpc.CallOption) (*forwardv1.ListDnsBindingsResponse, error) {
	return &forwardv1.ListDnsBindingsResponse{Bindings: []*forwardv1.DnsBinding{{Id: 9, RouteId: "01R"}}}, nil
}

func (f *dnsForward) DeleteDnsBinding(_ context.Context, in *forwardv1.DeleteDnsBindingRequest, _ ...grpc.CallOption) (*forwardv1.DeleteDnsBindingResponse, error) {
	f.deleted = in
	return &forwardv1.DeleteDnsBindingResponse{}, f.err
}

func (f *dnsForward) GetRouteDns(_ context.Context, in *forwardv1.GetRouteDnsRequest, _ ...grpc.CallOption) (*forwardv1.GetRouteDnsResponse, error) {
	return &forwardv1.GetRouteDnsResponse{Status: &forwardv1.RouteDnsStatus{RouteId: in.GetRouteId(), State: "ok", CnameTarget: "r1.ha.example.net"}}, nil
}

func serveDNS(t *testing.T, fake *dnsForward, method, path, body string) (int, map[string]any, string) {
	t.Helper()
	svc := &Service{Forward: fake, NewRequestID: func() string { return "generated" }}
	request := Request{Method: method, Path: PublicPrefix + path, Body: []byte(body), Query: map[string][]string{}}
	if i := strings.Index(path, "?"); i >= 0 {
		request.Path = PublicPrefix + path[:i]
		for _, pair := range strings.Split(path[i+1:], "&") {
			key, value, _ := strings.Cut(pair, "=")
			request.Query[key] = append(request.Query[key], value)
		}
	}
	answer := svc.Serve(context.Background(), request)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(answer.Body, &decoded), string(answer.Body))
	return answer.StatusCode, decoded, string(answer.Body)
}

func TestDNSProviderCredentialsAreWriteOnly(t *testing.T) {
	fake := &dnsForward{}
	code, _, raw := serveDNS(t, fake, http.MethodPost, "/dns/providers",
		`{"provider":{"name":"cf","kind":"DNS_PROVIDER_KIND_CLOUDFLARE"},"credentials":{"api_token":"cf-secret"}}`)
	require.Equal(t, http.StatusCreated, code, raw)
	require.Equal(t, "cf-secret", fake.created.GetCredentials()["api_token"])
	require.Equal(t, "generated", fake.created.GetRequestId())
	require.NotContains(t, raw, "cf-secret")
	require.Contains(t, raw, `"credential_names":["api_token"]`)

	code, _, raw = serveDNS(t, fake, http.MethodPut, "/dns/providers/3", `{"provider":{"name":"cf"},"credentials":{"api_token":"********"}}`)
	require.Equal(t, http.StatusOK, code, raw)
	require.EqualValues(t, 3, fake.updated.GetProvider().GetId())
	code, _, _ = serveDNS(t, fake, http.MethodPut, "/dns/providers/3", `{"provider":{"id":"4"}}`)
	require.Equal(t, http.StatusBadRequest, code)
	code, _, _ = serveDNS(t, fake, http.MethodGet, "/dns/providers/x", "")
	require.Equal(t, http.StatusNotFound, code)

	code, body, _ := serveDNS(t, fake, http.MethodGet, "/dns/providers", "")
	require.Equal(t, http.StatusOK, code)
	require.Len(t, body["data"].(map[string]any)["providers"], 1)

	code, body, _ = serveDNS(t, fake, http.MethodGet, "/dns/kinds", "")
	require.Equal(t, http.StatusOK, code)
	require.Len(t, body["data"].(map[string]any)["kinds"], 5)

	// A refusal answers the kernel's violations.
	st, err := status.New(codes.FailedPrecondition, "no key").WithDetails(&forwardv1.CreateDnsProviderResponse{
		Violations: []*forwardv1.Violation{{Field: "credentials", Code: "secret_store_unavailable", Message: "set module_runtime.ca_kek"}},
	})
	require.NoError(t, err)
	fake.err = st.Err()
	code, body, _ = serveDNS(t, fake, http.MethodPost, "/dns/providers", `{"provider":{"name":"cf","kind":"DNS_PROVIDER_KIND_CLOUDFLARE"},"credentials":{"api_token":"x"}}`)
	require.Equal(t, http.StatusConflict, code)
	require.Equal(t, "refused", errorCode(body))
}

func TestDNSBindingsAndRouteStatus(t *testing.T) {
	fake := &dnsForward{}
	code, body, raw := serveDNS(t, fake, http.MethodPost, "/dns/bindings",
		`{"route_id":"01R","provider_id":"3","zone":"example.com","mode":"DNS_BINDING_MODE_DDNS","record_types":["DNS_RECORD_TYPE_A","DNS_RECORD_TYPE_AAAA"],"ttl":60}`)
	require.Equal(t, http.StatusCreated, code, raw)
	require.EqualValues(t, 3, fake.binding.GetBinding().GetProviderId())
	require.Equal(t, "9", body["data"].(map[string]any)["binding"].(map[string]any)["id"])

	st, err := status.New(codes.InvalidArgument, "refused").WithDetails(&forwardv1.CreateDnsBindingResponse{
		Violations: []*forwardv1.Violation{{Field: "binding.record_name", Code: "hostname_mismatch"}},
	})
	require.NoError(t, err)
	fake.err = st.Err()
	code, body, _ = serveDNS(t, fake, http.MethodPost, "/dns/bindings", `{"route_id":"01R"}`)
	require.Equal(t, http.StatusBadRequest, code)
	require.Equal(t, "invalid_request", errorCode(body))
	require.Len(t, body["error"].(map[string]any)["violations"], 1)
	fake.err = nil

	code, _, _ = serveDNS(t, fake, http.MethodGet, "/dns/bindings/9", "")
	require.Equal(t, http.StatusOK, code)
	code, _, _ = serveDNS(t, fake, http.MethodGet, "/dns/bindings/10", "")
	require.Equal(t, http.StatusNotFound, code)

	code, _, _ = serveDNS(t, fake, http.MethodDelete, "/dns/bindings/9?purge=true", "")
	require.Equal(t, http.StatusOK, code)
	require.True(t, fake.deleted.GetPurge())
	fake.err = status.Error(codes.Unavailable, "forward dns: the provider did not delete the published records: HTTP 403")
	code, body, _ = serveDNS(t, fake, http.MethodDelete, "/dns/bindings/9?purge=true", "")
	require.Equal(t, http.StatusBadGateway, code)
	require.Equal(t, "dns_purge_failed", errorCode(body))
	fake.err = nil

	code, body, _ = serveDNS(t, fake, http.MethodGet, "/routes/01R/dns", "")
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, "r1.ha.example.net", body["data"].(map[string]any)["status"].(map[string]any)["cname_target"])
}
