package forwardddns

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
)

// Huawei Cloud DNS (API v2), signed with the AK/SK algorithm
// SDK-HMAC-SHA256. Huawei keeps a record set per name and type, so a set
// is written whole.
const (
	huaweiHost      = "dns.myhuaweicloud.com"
	huaweiAlgorithm = "SDK-HMAC-SHA256"
)

type huawei struct {
	host      string
	accessKey string
	secretKey string
	opts      Options

	mu    sync.Mutex
	zones map[string]string
}

func newHuawei(endpoint, accessKey, secretKey string, opts Options) Provider {
	if endpoint == "" {
		endpoint = huaweiHost
	}
	return &huawei{host: endpoint, accessKey: accessKey, secretKey: secretKey, opts: opts, zones: map[string]string{}}
}

func (h *huawei) Kind() string { return KindHuaweiCloud }

// huaweiCanonicalURI is the path with every segment escaped and a
// trailing slash, as the algorithm signs it.
func huaweiCanonicalURI(path string) string {
	segments := strings.Split(path, "/")
	for i, segment := range segments {
		segments[i] = rfc3986(segment)
	}
	uri := strings.Join(segments, "/")
	if !strings.HasSuffix(uri, "/") {
		uri += "/"
	}
	return uri
}

// huaweiSign signs a request: headers are the lower-case signed headers,
// uri the canonical URI and query the canonical query string. It answers
// the Authorization header and the canonical request.
func huaweiSign(method, uri, query string, headers map[string]string, payloadHash, accessKey, secretKey string) (string, string) {
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	var canonicalHeaders strings.Builder
	for _, name := range names {
		canonicalHeaders.WriteString(name + ":" + strings.TrimSpace(headers[name]) + "\n")
	}
	signed := strings.Join(names, ";")
	canonical := method + "\n" + uri + "\n" + query + "\n" + canonicalHeaders.String() + "\n" + signed + "\n" + payloadHash
	toSign := huaweiAlgorithm + "\n" + headers["x-sdk-date"] + "\n" + sha256Hex([]byte(canonical))
	signature := hex.EncodeToString(hmacSHA256([]byte(secretKey), []byte(toSign)))
	return huaweiAlgorithm + " Access=" + accessKey + ", SignedHeaders=" + signed + ", Signature=" + signature, canonical
}

func (h *huawei) call(ctx context.Context, method, path string, query url.Values, body any, out any) error {
	var payload []byte
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = encoded
	}
	canonical := canonicalQuery(query)
	headers := map[string]string{
		"content-type": "application/json",
		"host":         h.host,
		"x-sdk-date":   h.opts.now().Format("20060102T150405Z"),
	}
	authorization, _ := huaweiSign(method, huaweiCanonicalURI(path), canonical, headers, sha256Hex(payload), h.accessKey, h.secretKey)
	target := "https://" + h.host + path
	if canonical != "" {
		target += "?" + canonical
	}
	request, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", headers["content-type"])
	request.Header.Set("X-Sdk-Date", headers["x-sdk-date"])
	request.Header.Set("Authorization", authorization)
	response, err := h.opts.client().Do(request)
	if err != nil {
		return fmt.Errorf("%s: %w", KindHuaweiCloud, err)
	}
	raw, err := readBody(response)
	if err != nil {
		return fmt.Errorf("%s: %w", KindHuaweiCloud, err)
	}
	if response.StatusCode/100 != 2 {
		var refusal struct {
			Code         string `json:"code"`
			Message      string `json:"message"`
			ErrorCode    string `json:"error_code"`
			ErrorMessage string `json:"error_msg"`
		}
		_ = json.Unmarshal(raw, &refusal)
		apiErr := &APIError{Kind: KindHuaweiCloud, Status: response.StatusCode, Code: refusal.Code, Message: clip(refusal.Message)}
		if apiErr.Code == "" {
			apiErr.Code, apiErr.Message = refusal.ErrorCode, clip(refusal.ErrorMessage)
		}
		return apiErr
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("%s: decode answer: %w", KindHuaweiCloud, err)
		}
	}
	return nil
}

func (h *huawei) zoneID(ctx context.Context, zone string) (string, error) {
	h.mu.Lock()
	id, ok := h.zones[zone]
	h.mu.Unlock()
	if ok {
		return id, nil
	}
	var answer struct {
		Zones []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"zones"`
	}
	if err := h.call(ctx, http.MethodGet, "/v2/zones", url.Values{"type": {"public"}, "name": {zone + "."}}, nil, &answer); err != nil {
		return "", err
	}
	for _, z := range answer.Zones {
		if NormalizeName(z.Name) == zone && z.ID != "" {
			h.mu.Lock()
			h.zones[zone] = z.ID
			h.mu.Unlock()
			return z.ID, nil
		}
	}
	return "", fmt.Errorf("%s %s: %w", KindHuaweiCloud, zone, ErrNotFound)
}

type huaweiRecordSet struct {
	ID      string   `json:"id,omitempty"`
	Name    string   `json:"name,omitempty"`
	Type    string   `json:"type,omitempty"`
	TTL     uint32   `json:"ttl,omitempty"`
	Records []string `json:"records,omitempty"`
}

// recordSets answers the record sets of exactly name and type.
func (h *huawei) recordSets(ctx context.Context, zoneID, name, rtype string) ([]huaweiRecordSet, error) {
	var answer struct {
		RecordSets []huaweiRecordSet `json:"recordsets"`
	}
	query := url.Values{"name": {name + "."}, "type": {rtype}}
	if err := h.call(ctx, http.MethodGet, "/v2/zones/"+url.PathEscape(zoneID)+"/recordsets", query, nil, &answer); err != nil {
		return nil, err
	}
	var out []huaweiRecordSet
	for _, set := range answer.RecordSets {
		if NormalizeName(set.Name) == name && set.Type == rtype {
			out = append(out, set)
		}
	}
	return out, nil
}

func huaweiValues(rtype string, values []string) []string {
	if rtype != TypeCNAME {
		return values
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, value+".")
	}
	return out
}

func sameValues(rtype string, stored, want []string) bool {
	normal := normalizedValues(RecordSet{Type: rtype, Values: stored})
	if len(normal) != len(want) || len(normal) != len(stored) {
		return false
	}
	for i := range want {
		if normal[i] != want[i] {
			return false
		}
	}
	return true
}

func (h *huawei) SetRecords(ctx context.Context, set RecordSet) error {
	if err := CheckSet(set, false); err != nil {
		return err
	}
	set.Zone, set.Name = NormalizeName(set.Zone), NormalizeName(set.Name)
	want := normalizedValues(set)
	zoneID, err := h.zoneID(ctx, set.Zone)
	if err != nil {
		return err
	}
	zonePath := "/v2/zones/" + url.PathEscape(zoneID) + "/recordsets"
	existing, err := h.recordSets(ctx, zoneID, set.Name, set.Type)
	if err != nil {
		return err
	}
	if len(existing) == 0 {
		return h.call(ctx, http.MethodPost, zonePath, nil, huaweiRecordSet{
			Name: set.Name + ".", Type: set.Type, TTL: set.TTL, Records: huaweiValues(set.Type, want),
		}, nil)
	}
	first := existing[0]
	if first.TTL != set.TTL || !sameValues(set.Type, first.Records, want) {
		if err := h.call(ctx, http.MethodPut, zonePath+"/"+url.PathEscape(first.ID), nil, huaweiRecordSet{
			TTL: set.TTL, Records: huaweiValues(set.Type, want),
		}, nil); err != nil {
			return err
		}
	}
	for _, extra := range existing[1:] {
		if err := h.call(ctx, http.MethodDelete, zonePath+"/"+url.PathEscape(extra.ID), nil, nil, nil); err != nil {
			return err
		}
	}
	return nil
}

func (h *huawei) DeleteRecords(ctx context.Context, zone, name, rtype string) error {
	set := RecordSet{Zone: NormalizeName(zone), Name: NormalizeName(name), Type: rtype}
	if err := CheckSet(set, true); err != nil {
		return err
	}
	zoneID, err := h.zoneID(ctx, set.Zone)
	if err != nil {
		return err
	}
	existing, err := h.recordSets(ctx, zoneID, set.Name, set.Type)
	if err != nil {
		return err
	}
	for _, rs := range existing {
		if err := h.call(ctx, http.MethodDelete, "/v2/zones/"+url.PathEscape(zoneID)+"/recordsets/"+url.PathEscape(rs.ID), nil, nil, nil); err != nil {
			return err
		}
	}
	return nil
}
