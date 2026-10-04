package forwardddns

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// The generic webhook: Control POSTs each change as JSON to the
// operator's HTTPS URL, signed with the shared secret.
//
//	POST <url>
//	Content-Type: application/json
//	X-AnixOps-Event: forward.dns
//	X-AnixOps-Timestamp: <Unix seconds>
//	X-AnixOps-Signature: sha256=<hex HMAC-SHA256(secret, timestamp + "." + body)>
//
//	{"version":1,"action":"set","zone":"example.com","name":"hk.example.com",
//	 "type":"A","values":["192.0.2.10"],"ttl":60,"timestamp":1759579200}
//
// action "delete" carries no values. Any 2xx answer is success. The
// receiver should refuse a timestamp more than 5 minutes old.
const (
	WebhookEvent           = "forward.dns"
	WebhookHeaderEvent     = "X-AnixOps-Event"
	WebhookHeaderTimestamp = "X-AnixOps-Timestamp"
	WebhookHeaderSignature = "X-AnixOps-Signature"
)

// WebhookPayload is the body of a webhook call.
type WebhookPayload struct {
	Version   int      `json:"version"`
	Action    string   `json:"action"`
	Zone      string   `json:"zone"`
	Name      string   `json:"name"`
	Type      string   `json:"type"`
	Values    []string `json:"values,omitempty"`
	TTL       uint32   `json:"ttl,omitempty"`
	Timestamp int64    `json:"timestamp"`
}

// WebhookSignature is the X-AnixOps-Signature value of body sent at
// timestamp.
func WebhookSignature(secret string, timestamp int64, body []byte) string {
	message := append([]byte(strconv.FormatInt(timestamp, 10)+"."), body...)
	return "sha256=" + hex.EncodeToString(hmacSHA256([]byte(secret), message))
}

func webhookURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return "", errors.New("must be an https URL without credentials or fragment")
	}
	return parsed.String(), nil
}

type webhook struct {
	url    string
	secret string
	opts   Options
}

func (w *webhook) Kind() string { return KindWebhook }

func (w *webhook) send(ctx context.Context, payload WebhookPayload) error {
	payload.Version = 1
	payload.Timestamp = w.opts.now().Unix()
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, w.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "anixops-control-forward-dns")
	request.Header.Set(WebhookHeaderEvent, WebhookEvent)
	request.Header.Set(WebhookHeaderTimestamp, strconv.FormatInt(payload.Timestamp, 10))
	request.Header.Set(WebhookHeaderSignature, WebhookSignature(w.secret, payload.Timestamp, body))
	response, err := w.opts.client().Do(request)
	if err != nil {
		return fmt.Errorf("%s: %w", KindWebhook, err)
	}
	raw, _ := readBody(response)
	if response.StatusCode/100 != 2 {
		return &APIError{Kind: KindWebhook, Status: response.StatusCode, Message: clip(string(raw))}
	}
	return nil
}

func (w *webhook) SetRecords(ctx context.Context, set RecordSet) error {
	if err := CheckSet(set, false); err != nil {
		return err
	}
	return w.send(ctx, WebhookPayload{
		Action: "set", Zone: NormalizeName(set.Zone), Name: NormalizeName(set.Name), Type: set.Type,
		Values: normalizedValues(set), TTL: set.TTL,
	})
}

func (w *webhook) DeleteRecords(ctx context.Context, zone, name, rtype string) error {
	set := RecordSet{Zone: NormalizeName(zone), Name: NormalizeName(name), Type: rtype}
	if err := CheckSet(set, true); err != nil {
		return err
	}
	return w.send(ctx, WebhookPayload{Action: "delete", Zone: set.Zone, Name: set.Name, Type: set.Type})
}
