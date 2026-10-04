package forwardddns

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// DNSPod on the Tencent Cloud API 3.0 (version 2021-03-23), signed with
// TC3-HMAC-SHA256.
const (
	dnspodHost     = "dnspod.tencentcloudapi.com"
	dnspodService  = "dnspod"
	dnspodVersion  = "2021-03-23"
	tc3Algorithm   = "TC3-HMAC-SHA256"
	tc3ContentType = "application/json; charset=utf-8"
	// dnspodDefaultLine is the default resolution line ("默认").
	dnspodDefaultLine = "默认"
	// dnspodNoRecords is DescribeRecordList's answer for a name without
	// records.
	dnspodNoRecords = "ResourceNotFound.NoDataOfRecord"
)

type dnspod struct {
	host      string
	secretID  string
	secretKey string
	opts      Options
}

func newDNSPod(endpoint, secretID, secretKey string, opts Options) Provider {
	if endpoint == "" {
		endpoint = dnspodHost
	}
	return perRecord{api: &dnspod{host: endpoint, secretID: secretID, secretKey: secretKey, opts: opts}}
}

func (d *dnspod) kind() string { return KindDNSPod }

// tc3Sign signs a Tencent Cloud API 3.0 POST with a JSON payload; the
// signed headers are content-type, host and x-tc-action. It answers the
// Authorization header and the canonical request.
func tc3Sign(secretID, secretKey, service, host, action string, payload []byte, timestamp int64) (string, string) {
	date := time.Unix(timestamp, 0).UTC().Format("2006-01-02")
	signed := "content-type;host;x-tc-action"
	canonical := "POST\n/\n\n" +
		"content-type:" + tc3ContentType + "\n" +
		"host:" + host + "\n" +
		"x-tc-action:" + strings.ToLower(action) + "\n\n" +
		signed + "\n" + sha256Hex(payload)
	scope := date + "/" + service + "/tc3_request"
	toSign := tc3Algorithm + "\n" + strconv.FormatInt(timestamp, 10) + "\n" + scope + "\n" + sha256Hex([]byte(canonical))
	key := hmacSHA256([]byte("TC3"+secretKey), []byte(date))
	key = hmacSHA256(key, []byte(service))
	key = hmacSHA256(key, []byte("tc3_request"))
	signature := hex.EncodeToString(hmacSHA256(key, []byte(toSign)))
	return tc3Algorithm + " Credential=" + secretID + "/" + scope + ", SignedHeaders=" + signed + ", Signature=" + signature, canonical
}

type tc3Error struct {
	Code    string `json:"Code"`
	Message string `json:"Message"`
}

func (d *dnspod) call(ctx context.Context, action string, params any, out any) error {
	payload, err := json.Marshal(params)
	if err != nil {
		return err
	}
	timestamp := d.opts.now().Unix()
	authorization, _ := tc3Sign(d.secretID, d.secretKey, dnspodService, d.host, action, payload, timestamp)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://"+d.host+"/", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", tc3ContentType)
	request.Header.Set("X-TC-Action", action)
	request.Header.Set("X-TC-Timestamp", strconv.FormatInt(timestamp, 10))
	request.Header.Set("X-TC-Version", dnspodVersion)
	request.Header.Set("Authorization", authorization)
	response, err := d.opts.client().Do(request)
	if err != nil {
		return fmt.Errorf("%s: %w", KindDNSPod, err)
	}
	raw, err := readBody(response)
	if err != nil {
		return fmt.Errorf("%s: %w", KindDNSPod, err)
	}
	var envelope struct {
		Response json.RawMessage `json:"Response"`
	}
	var refusal struct {
		Error *tc3Error `json:"Error"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || len(envelope.Response) == 0 {
		return &APIError{Kind: KindDNSPod, Status: response.StatusCode, Message: "answer is not a Tencent Cloud API response"}
	}
	_ = json.Unmarshal(envelope.Response, &refusal)
	if refusal.Error != nil {
		return &APIError{Kind: KindDNSPod, Status: response.StatusCode, Code: refusal.Error.Code, Message: clip(refusal.Error.Message)}
	}
	if response.StatusCode/100 != 2 {
		return &APIError{Kind: KindDNSPod, Status: response.StatusCode}
	}
	if out != nil {
		if err := json.Unmarshal(envelope.Response, out); err != nil {
			return fmt.Errorf("%s: decode answer: %w", KindDNSPod, err)
		}
	}
	return nil
}

func (d *dnspod) list(ctx context.Context, set RecordSet) ([]record, error) {
	var answer struct {
		RecordList []struct {
			RecordID uint64 `json:"RecordId"`
			Name     string `json:"Name"`
			Type     string `json:"Type"`
			Value    string `json:"Value"`
			TTL      uint32 `json:"TTL"`
		} `json:"RecordList"`
	}
	sub := relative(set.Name, set.Zone)
	err := d.call(ctx, "DescribeRecordList", map[string]any{
		"Domain": set.Zone, "Subdomain": sub, "RecordType": set.Type, "Limit": 3000,
	}, &answer)
	if apiErr, ok := err.(*APIError); ok && apiErr.Code == dnspodNoRecords {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []record
	for _, r := range answer.RecordList {
		if strings.EqualFold(r.Name, sub) && r.Type == set.Type {
			out = append(out, record{ID: strconv.FormatUint(r.RecordID, 10), Value: r.Value, TTL: r.TTL})
		}
	}
	return out, nil
}

func (d *dnspod) create(ctx context.Context, set RecordSet, value string) error {
	return d.call(ctx, "CreateRecord", map[string]any{
		"Domain": set.Zone, "SubDomain": relative(set.Name, set.Zone), "RecordType": set.Type,
		"RecordLine": dnspodDefaultLine, "Value": value, "TTL": set.TTL,
	}, nil)
}

func recordID(rec record) (uint64, error) {
	id, err := strconv.ParseUint(rec.ID, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: record id %q is not a number", KindDNSPod, rec.ID)
	}
	return id, nil
}

func (d *dnspod) update(ctx context.Context, set RecordSet, rec record) error {
	id, err := recordID(rec)
	if err != nil {
		return err
	}
	return d.call(ctx, "ModifyRecord", map[string]any{
		"Domain": set.Zone, "RecordId": id, "SubDomain": relative(set.Name, set.Zone), "RecordType": set.Type,
		"RecordLine": dnspodDefaultLine, "Value": rec.Value, "TTL": set.TTL,
	}, nil)
}

func (d *dnspod) remove(ctx context.Context, set RecordSet, rec record) error {
	id, err := recordID(rec)
	if err != nil {
		return err
	}
	return d.call(ctx, "DeleteRecord", map[string]any{"Domain": set.Zone, "RecordId": id}, nil)
}
