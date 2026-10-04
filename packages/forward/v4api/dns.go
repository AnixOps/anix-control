package v4api

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Entry high availability through DNS (forward-sdk.md section 7.4, L2):
// DNS provider accounts, the routes' bindings and the route DNS status.
// Provider credentials are write-only: they go to the kernel, which seals
// them, and no answer carries them. The kernel lets only a super
// administrator write a provider.

// ProviderKind describes what a DNS provider kind takes, for a form.
type ProviderKind struct {
	Kind        string   `json:"kind"`
	Name        string   `json:"name"`
	Config      []string `json:"config"`
	Required    []string `json:"required_config"`
	Credentials []string `json:"credentials"`
}

// ProviderKinds are the DNS provider kinds of H21.
var ProviderKinds = []ProviderKind{
	{Kind: "DNS_PROVIDER_KIND_CLOUDFLARE", Name: "Cloudflare", Config: []string{"endpoint"}, Required: []string{}, Credentials: []string{"api_token"}},
	{Kind: "DNS_PROVIDER_KIND_ALIDNS", Name: "Alibaba Cloud DNS", Config: []string{"endpoint"}, Required: []string{}, Credentials: []string{"access_key_id", "access_key_secret"}},
	{Kind: "DNS_PROVIDER_KIND_DNSPOD", Name: "DNSPod (Tencent Cloud)", Config: []string{"endpoint"}, Required: []string{}, Credentials: []string{"secret_id", "secret_key"}},
	{Kind: "DNS_PROVIDER_KIND_HUAWEICLOUD", Name: "Huawei Cloud DNS", Config: []string{"endpoint"}, Required: []string{}, Credentials: []string{"access_key", "secret_key"}},
	{Kind: "DNS_PROVIDER_KIND_WEBHOOK", Name: "Webhook", Config: []string{"url"}, Required: []string{"url"}, Credentials: []string{"secret"}},
}

// dnsFromStatus is fromStatus with a malformed DNS write answered as
// invalid_request, with its violations (invalid_route is for routes).
func dnsFromStatus(err error) Response {
	if st, ok := status.FromError(err); ok && st.Code() == codes.InvalidArgument {
		return failure(http.StatusBadRequest, "invalid_request", st.Message(), detailViolations(st))
	}
	return fromStatus(err)
}

func pathID(params map[string]string) (uint64, *Response) {
	id, err := strconv.ParseUint(params["id"], 10, 64)
	if err != nil || id == 0 {
		answer := failure(http.StatusNotFound, "not_found", "not found", nil)
		return 0, &answer
	}
	return id, nil
}

func (s *Service) dnsKinds(context.Context, Request, map[string]string) Response {
	return data(http.StatusOK, map[string]any{"kinds": ProviderKinds})
}

func (s *Service) listDNSProviders(ctx context.Context, _ Request, _ map[string]string) Response {
	answer, err := s.Forward.ListDnsProviders(ctx, &forwardv1.ListDnsProvidersRequest{})
	if err != nil {
		return dnsFromStatus(err)
	}
	return data(http.StatusOK, map[string]any{"providers": pjList(answer.GetProviders())})
}

func (s *Service) getDNSProvider(ctx context.Context, _ Request, params map[string]string) Response {
	id, refusal := pathID(params)
	if refusal != nil {
		return *refusal
	}
	answer, err := s.Forward.GetDnsProvider(ctx, &forwardv1.GetDnsProviderRequest{Id: id})
	if err != nil {
		return dnsFromStatus(err)
	}
	return data(http.StatusOK, map[string]any{"provider": pj(answer.GetProvider())})
}

// createDNSProvider: the body is {"provider": DnsProvider, "credentials":
// {name: value}}.
func (s *Service) createDNSProvider(ctx context.Context, request Request, _ map[string]string) Response {
	body := &forwardv1.CreateDnsProviderRequest{}
	if answer := decode(request.Body, body, false); answer != nil {
		return *answer
	}
	body.RequestId = s.requestID(request, "")
	answer, err := s.Forward.CreateDnsProvider(ctx, body)
	if err != nil {
		return dnsFromStatus(err)
	}
	return data(http.StatusCreated, map[string]any{"provider": pj(answer.GetProvider())})
}

// updateDNSProvider: as create; a credential left out or sent as
// "********" keeps the stored value.
func (s *Service) updateDNSProvider(ctx context.Context, request Request, params map[string]string) Response {
	id, refusal := pathID(params)
	if refusal != nil {
		return *refusal
	}
	body := &forwardv1.UpdateDnsProviderRequest{}
	if answer := decode(request.Body, body, false); answer != nil {
		return *answer
	}
	if body.GetProvider() == nil {
		return failure(http.StatusBadRequest, "invalid_request", "provider is required", nil)
	}
	if body.GetProvider().GetId() != 0 && body.GetProvider().GetId() != id {
		return failure(http.StatusBadRequest, "invalid_request", "the body's id is not the path's", nil)
	}
	body.Provider.Id, body.RequestId = id, s.requestID(request, "")
	answer, err := s.Forward.UpdateDnsProvider(ctx, body)
	if err != nil {
		return dnsFromStatus(err)
	}
	return data(http.StatusOK, map[string]any{"provider": pj(answer.GetProvider())})
}

func (s *Service) deleteDNSProvider(ctx context.Context, request Request, params map[string]string) Response {
	id, refusal := pathID(params)
	if refusal != nil {
		return *refusal
	}
	if _, err := s.Forward.DeleteDnsProvider(ctx, &forwardv1.DeleteDnsProviderRequest{RequestId: s.requestID(request, ""), Id: id}); err != nil {
		return dnsFromStatus(err)
	}
	return data(http.StatusOK, map[string]any{"deleted": strconv.FormatUint(id, 10)})
}

func (s *Service) listDNSBindings(ctx context.Context, request Request, _ map[string]string) Response {
	list := &forwardv1.ListDnsBindingsRequest{RouteId: query(request, "route_id")}
	if raw := query(request, "provider_id"); raw != "" {
		id, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return failure(http.StatusBadRequest, "invalid_request", "provider_id must be a number", nil)
		}
		list.ProviderId = id
	}
	answer, err := s.Forward.ListDnsBindings(ctx, list)
	if err != nil {
		return dnsFromStatus(err)
	}
	return data(http.StatusOK, map[string]any{"bindings": pjList(answer.GetBindings())})
}

func (s *Service) getDNSBinding(ctx context.Context, _ Request, params map[string]string) Response {
	id, refusal := pathID(params)
	if refusal != nil {
		return *refusal
	}
	answer, err := s.Forward.ListDnsBindings(ctx, &forwardv1.ListDnsBindingsRequest{})
	if err != nil {
		return dnsFromStatus(err)
	}
	for _, binding := range answer.GetBindings() {
		if binding.GetId() == id {
			return data(http.StatusOK, map[string]any{"binding": pj(binding)})
		}
	}
	return failure(http.StatusNotFound, "not_found", "forward dns binding not found", nil)
}

// createDNSBinding: the body is a DnsBinding.
func (s *Service) createDNSBinding(ctx context.Context, request Request, _ map[string]string) Response {
	binding := &forwardv1.DnsBinding{}
	if answer := decode(request.Body, binding, false); answer != nil {
		return *answer
	}
	answer, err := s.Forward.CreateDnsBinding(ctx, &forwardv1.CreateDnsBindingRequest{RequestId: s.requestID(request, ""), Binding: binding})
	if err != nil {
		return dnsFromStatus(err)
	}
	return data(http.StatusCreated, map[string]any{"binding": pj(answer.GetBinding())})
}

// updateDNSBinding: the body is the DnsBinding; record_types, ttl and
// paused change.
func (s *Service) updateDNSBinding(ctx context.Context, request Request, params map[string]string) Response {
	id, refusal := pathID(params)
	if refusal != nil {
		return *refusal
	}
	binding := &forwardv1.DnsBinding{}
	if answer := decode(request.Body, binding, false); answer != nil {
		return *answer
	}
	if binding.GetId() != 0 && binding.GetId() != id {
		return failure(http.StatusBadRequest, "invalid_request", "the body's id is not the path's", nil)
	}
	binding.Id = id
	answer, err := s.Forward.UpdateDnsBinding(ctx, &forwardv1.UpdateDnsBindingRequest{RequestId: s.requestID(request, ""), Binding: binding})
	if err != nil {
		return dnsFromStatus(err)
	}
	return data(http.StatusOK, map[string]any{"binding": pj(answer.GetBinding())})
}

// deleteDNSBinding: query purge=true deletes the published records first.
func (s *Service) deleteDNSBinding(ctx context.Context, request Request, params map[string]string) Response {
	id, refusal := pathID(params)
	if refusal != nil {
		return *refusal
	}
	purge := false
	if raw := query(request, "purge"); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return failure(http.StatusBadRequest, "invalid_request", "purge must be true or false", nil)
		}
		purge = value
	}
	_, err := s.Forward.DeleteDnsBinding(ctx, &forwardv1.DeleteDnsBindingRequest{RequestId: s.requestID(request, ""), Id: id, Purge: purge})
	if st, ok := status.FromError(err); ok && err != nil && st.Code() == codes.Unavailable && strings.Contains(st.Message(), "published records") {
		return failure(http.StatusBadGateway, "dns_purge_failed", st.Message(), nil)
	}
	if err != nil {
		return dnsFromStatus(err)
	}
	return data(http.StatusOK, map[string]any{"deleted": strconv.FormatUint(id, 10)})
}

func (s *Service) routeDNS(ctx context.Context, _ Request, params map[string]string) Response {
	answer, err := s.Forward.GetRouteDns(ctx, &forwardv1.GetRouteDnsRequest{RouteId: params["id"]})
	if err != nil {
		return dnsFromStatus(err)
	}
	return data(http.StatusOK, map[string]any{"status": pj(answer.GetStatus())})
}
