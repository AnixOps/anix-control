package forwardddns

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// Alibaba Cloud DNS (Alidns, API version 2015-01-09), RPC style, signed
// with signature V3 (ACS3-HMAC-SHA256).
const (
	aliDNSHost    = "alidns.aliyuncs.com"
	aliDNSVersion = "2015-01-09"
	acs3Algorithm = "ACS3-HMAC-SHA256"
)

type aliDNS struct {
	host   string
	keyID  string
	secret string
	opts   Options
}

func newAliDNS(endpoint, keyID, secret string, opts Options) Provider {
	if endpoint == "" {
		endpoint = aliDNSHost
	}
	return perRecord{api: &aliDNS{host: endpoint, keyID: keyID, secret: secret, opts: opts}}
}

func (a *aliDNS) kind() string { return KindAliDNS }

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func hmacSHA256(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}

// acs3Sign signs a request with signature V3: headers are the lower-case
// headers signed (host, content-type when sent, every x-acs-*), query the
// canonical query string. It answers the Authorization header and the
// canonical request.
func acs3Sign(method, path, query string, headers map[string]string, payloadHash, keyID, secret string) (string, string) {
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
	canonical := method + "\n" + path + "\n" + query + "\n" + canonicalHeaders.String() + "\n" + signed + "\n" + payloadHash
	toSign := acs3Algorithm + "\n" + sha256Hex([]byte(canonical))
	signature := hex.EncodeToString(hmacSHA256([]byte(secret), []byte(toSign)))
	return acs3Algorithm + " Credential=" + keyID + ",SignedHeaders=" + signed + ",Signature=" + signature, canonical
}

type aliDNSError struct {
	Code    string `json:"Code"`
	Message string `json:"Message"`
}

func (a *aliDNS) call(ctx context.Context, action string, params url.Values, out any) error {
	query := canonicalQuery(params)
	payloadHash := sha256Hex(nil)
	headers := map[string]string{
		"host":                  a.host,
		"x-acs-action":          action,
		"x-acs-version":         aliDNSVersion,
		"x-acs-date":            a.opts.now().Format("2006-01-02T15:04:05Z"),
		"x-acs-signature-nonce": a.opts.nonce(),
		"x-acs-content-sha256":  payloadHash,
	}
	authorization, _ := acs3Sign(http.MethodPost, "/", query, headers, payloadHash, a.keyID, a.secret)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://"+a.host+"/?"+query, http.NoBody)
	if err != nil {
		return err
	}
	for name, value := range headers {
		if name != "host" {
			request.Header.Set(name, value)
		}
	}
	request.Header.Set("Authorization", authorization)
	response, err := a.opts.client().Do(request)
	if err != nil {
		return fmt.Errorf("%s: %w", KindAliDNS, err)
	}
	raw, err := readBody(response)
	if err != nil {
		return fmt.Errorf("%s: %w", KindAliDNS, err)
	}
	if response.StatusCode/100 != 2 {
		var refusal aliDNSError
		_ = json.Unmarshal(raw, &refusal)
		return &APIError{Kind: KindAliDNS, Status: response.StatusCode, Code: refusal.Code, Message: clip(refusal.Message)}
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("%s: decode answer: %w", KindAliDNS, err)
		}
	}
	return nil
}

func (a *aliDNS) list(ctx context.Context, set RecordSet) ([]record, error) {
	var answer struct {
		DomainRecords struct {
			Record []struct {
				RecordID string `json:"RecordId"`
				RR       string `json:"RR"`
				Type     string `json:"Type"`
				Value    string `json:"Value"`
				TTL      uint32 `json:"TTL"`
			} `json:"Record"`
		} `json:"DomainRecords"`
	}
	params := url.Values{"SubDomain": {set.Name}, "DomainName": {set.Zone}, "Type": {set.Type}, "PageSize": {"500"}}
	if err := a.call(ctx, "DescribeSubDomainRecords", params, &answer); err != nil {
		return nil, err
	}
	rr := relative(set.Name, set.Zone)
	var out []record
	for _, r := range answer.DomainRecords.Record {
		if strings.EqualFold(r.RR, rr) && r.Type == set.Type {
			out = append(out, record{ID: r.RecordID, Value: r.Value, TTL: r.TTL})
		}
	}
	return out, nil
}

func (a *aliDNS) create(ctx context.Context, set RecordSet, value string) error {
	return a.call(ctx, "AddDomainRecord", url.Values{
		"DomainName": {set.Zone}, "RR": {relative(set.Name, set.Zone)}, "Type": {set.Type}, "Value": {value},
		"TTL": {strconv.FormatUint(uint64(set.TTL), 10)},
	}, nil)
}

func (a *aliDNS) update(ctx context.Context, set RecordSet, rec record) error {
	return a.call(ctx, "UpdateDomainRecord", url.Values{
		"RecordId": {rec.ID}, "RR": {relative(set.Name, set.Zone)}, "Type": {set.Type}, "Value": {rec.Value},
		"TTL": {strconv.FormatUint(uint64(set.TTL), 10)},
	}, nil)
}

func (a *aliDNS) remove(ctx context.Context, _ RecordSet, rec record) error {
	return a.call(ctx, "DeleteDomainRecord", url.Values{"RecordId": {rec.ID}}, nil)
}
