package forwardddns

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"sync"
)

// cloudflareHost is the Cloudflare API v4's host.
const cloudflareHost = "api.cloudflare.com"

// cloudflare is the Cloudflare API v4 with an API token (Bearer). The
// token needs Zone:Read and DNS:Edit on the zone.
type cloudflare struct {
	base  string
	token string
	opts  Options

	mu    sync.Mutex
	zones map[string]string
}

func newCloudflare(endpoint, token string, opts Options) Provider {
	if endpoint == "" {
		endpoint = cloudflareHost
	}
	return perRecord{api: &cloudflare{base: "https://" + endpoint + "/client/v4", token: token, opts: opts, zones: map[string]string{}}}
}

func (c *cloudflare) kind() string { return KindCloudflare }

type cloudflareEnvelope struct {
	Success bool `json:"success"`
	Errors  []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
	Result json.RawMessage `json:"result"`
}

func (c *cloudflare) call(ctx context.Context, method, path string, query url.Values, body any, out any) error {
	target := c.base + path
	if len(query) > 0 {
		target += "?" + canonicalQuery(query)
	}
	var reader *bytes.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	} else {
		reader = bytes.NewReader(nil)
	}
	request, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.opts.client().Do(request)
	if err != nil {
		return fmt.Errorf("%s: %w", KindCloudflare, err)
	}
	raw, err := readBody(response)
	if err != nil {
		return fmt.Errorf("%s: %w", KindCloudflare, err)
	}
	var envelope cloudflareEnvelope
	decodeErr := json.Unmarshal(raw, &envelope)
	if response.StatusCode/100 != 2 || decodeErr != nil || !envelope.Success {
		apiErr := &APIError{Kind: KindCloudflare, Status: response.StatusCode}
		if len(envelope.Errors) > 0 {
			apiErr.Code = strconv.Itoa(envelope.Errors[0].Code)
			apiErr.Message = clip(envelope.Errors[0].Message)
		}
		return apiErr
	}
	if out != nil {
		if err := json.Unmarshal(envelope.Result, out); err != nil {
			return fmt.Errorf("%s: decode answer: %w", KindCloudflare, err)
		}
	}
	return nil
}

func (c *cloudflare) zoneID(ctx context.Context, zone string) (string, error) {
	c.mu.Lock()
	id, ok := c.zones[zone]
	c.mu.Unlock()
	if ok {
		return id, nil
	}
	var zones []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := c.call(ctx, http.MethodGet, "/zones", url.Values{"name": {zone}}, nil, &zones); err != nil {
		return "", err
	}
	for _, z := range zones {
		if NormalizeName(z.Name) == zone && z.ID != "" {
			c.mu.Lock()
			c.zones[zone] = z.ID
			c.mu.Unlock()
			return z.ID, nil
		}
	}
	return "", fmt.Errorf("%s %s: %w", KindCloudflare, zone, ErrNotFound)
}

type cloudflareRecord struct {
	ID      string `json:"id,omitempty"`
	Type    string `json:"type,omitempty"`
	Name    string `json:"name,omitempty"`
	Content string `json:"content,omitempty"`
	TTL     uint32 `json:"ttl,omitempty"`
	Proxied *bool  `json:"proxied,omitempty"`
}

func (c *cloudflare) list(ctx context.Context, set RecordSet) ([]record, error) {
	zoneID, err := c.zoneID(ctx, set.Zone)
	if err != nil {
		return nil, err
	}
	var records []cloudflareRecord
	query := url.Values{"type": {set.Type}, "name": {set.Name}, "per_page": {"100"}}
	if err := c.call(ctx, http.MethodGet, "/zones/"+url.PathEscape(zoneID)+"/dns_records", query, nil, &records); err != nil {
		return nil, err
	}
	var out []record
	for _, r := range records {
		if NormalizeName(r.Name) == set.Name && r.Type == set.Type {
			out = append(out, record{ID: r.ID, Value: r.Content, TTL: r.TTL})
		}
	}
	return out, nil
}

func (c *cloudflare) create(ctx context.Context, set RecordSet, value string) error {
	zoneID, err := c.zoneID(ctx, set.Zone)
	if err != nil {
		return err
	}
	proxied := false
	body := cloudflareRecord{Type: set.Type, Name: set.Name, Content: value, TTL: set.TTL, Proxied: &proxied}
	return c.call(ctx, http.MethodPost, "/zones/"+url.PathEscape(zoneID)+"/dns_records", nil, body, nil)
}

func (c *cloudflare) update(ctx context.Context, set RecordSet, rec record) error {
	zoneID, err := c.zoneID(ctx, set.Zone)
	if err != nil {
		return err
	}
	return c.call(ctx, http.MethodPatch, "/zones/"+url.PathEscape(zoneID)+"/dns_records/"+url.PathEscape(rec.ID), nil, map[string]uint32{"ttl": set.TTL}, nil)
}

func (c *cloudflare) remove(ctx context.Context, set RecordSet, rec record) error {
	zoneID, err := c.zoneID(ctx, set.Zone)
	if err != nil {
		return err
	}
	return c.call(ctx, http.MethodDelete, "/zones/"+url.PathEscape(zoneID)+"/dns_records/"+url.PathEscape(rec.ID), nil, nil, nil)
}
