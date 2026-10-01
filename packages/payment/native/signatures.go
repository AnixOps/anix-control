package native

// Provider signature verification of the callbacks, ported as is from the
// kernel's internal/handler/payment_signature.go (Stripe, x402, PayPal) and
// internal/payment/gateways/epay.go (EPay), which the legacy callbacks
// keep using. The two copies must verify alike: internal/tests/paymentcompat
// sends both the same signed and forged deliveries.

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/md5" // #nosec G501 -- EPay's public callback protocol uses MD5 signatures.
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	errSignatureMissing  = errors.New("missing signature")
	errSignatureMismatch = errors.New("signature mismatch")
	errSignatureExpired  = errors.New("signature timestamp expired")
	errWebhookSecret     = errors.New("webhook secret not configured")
)

// stripeSignatureTolerance bounds a Stripe signature's timestamp, against
// replays.
const stripeSignatureTolerance = 5 * time.Minute

// verifyStripeSignature checks a Stripe webhook signature: the header is
// "t=<timestamp>,v1=<sig>[,v1=<sig>...]", the signed payload
// "<timestamp>.<rawBody>", the algorithm HMAC-SHA256 with the webhook
// secret.
func verifyStripeSignature(payload []byte, sigHeader, secret string, now time.Time) error {
	if secret == "" {
		return errWebhookSecret
	}
	if sigHeader == "" {
		return errSignatureMissing
	}

	var timestamp string
	var v1Sigs []string
	for _, part := range strings.Split(sigHeader, ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			timestamp = kv[1]
		case "v1":
			v1Sigs = append(v1Sigs, kv[1])
		}
	}

	if timestamp == "" || len(v1Sigs) == 0 {
		return errSignatureMissing
	}

	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid timestamp: %w", err)
	}
	diff := now.Unix() - ts
	if diff < 0 {
		diff = -diff
	}
	if time.Duration(diff)*time.Second > stripeSignatureTolerance {
		return errSignatureExpired
	}

	signedPayload := timestamp + "." + string(payload)
	expected := computeHMACSHA256(signedPayload, secret)

	// Stripe may send several v1 signatures while it rotates secrets; any
	// one matching passes.
	for _, sig := range v1Sigs {
		if hmac.Equal([]byte(sig), []byte(expected)) {
			return nil
		}
	}
	return errSignatureMismatch
}

// verifyX402Signature checks an x402 callback signature: HMAC-SHA256 of the
// canonical callback fields, independent of the JSON field order.
func verifyX402Signature(fields map[string]string, signature, secret string) error {
	if secret == "" {
		return errWebhookSecret
	}
	if signature == "" {
		return errSignatureMissing
	}

	expected := computeHMACSHA256(canonicalizeFields(fields), secret)
	if !hmac.Equal([]byte(strings.ToLower(signature)), []byte(expected)) {
		return errSignatureMismatch
	}
	return nil
}

// canonicalizeFields joins the fields as "k1=v1&k2=v2" in key order.
func canonicalizeFields(fields map[string]string) string {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(fields[k])
	}
	return b.String()
}

// computeHMACSHA256 is the lower-case hex HMAC-SHA256 of message.
func computeHMACSHA256(message, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

// paypalSignatureHeaders are a PayPal delivery's transmission headers.
type paypalSignatureHeaders struct {
	AuthAlgo         string
	CertURL          string
	TransmissionID   string
	TransmissionSig  string
	TransmissionTime string
}

// defaultPayPalClient calls PayPal's API with the kernel's timeout, so a
// slow PayPal cannot hold a callback.
var defaultPayPalClient = &http.Client{Timeout: 10 * time.Second}

func paypalAPIBase(sandbox bool) string {
	if sandbox {
		return "https://api-m.sandbox.paypal.com"
	}
	return "https://api-m.paypal.com"
}

// verifyPayPalSignatureRemote asks PayPal's verify-webhook-signature API
// whether a delivery is genuine.
func verifyPayPalSignatureRemote(ctx context.Context, client *http.Client, cfg *payPalConfig, h paypalSignatureHeaders, body []byte) error {
	token, err := paypalAccessToken(ctx, client, cfg)
	if err != nil {
		return fmt.Errorf("paypal token: %w", err)
	}

	// webhook_event must be the delivery's JSON object as received.
	reqBody := map[string]any{
		"auth_algo":         h.AuthAlgo,
		"cert_url":          h.CertURL,
		"transmission_id":   h.TransmissionID,
		"transmission_sig":  h.TransmissionSig,
		"transmission_time": h.TransmissionTime,
		"webhook_id":        cfg.WebhookID,
		"webhook_event":     json.RawMessage(body),
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	base := paypalAPIBase(cfg.SandboxMode)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		base+"/v1/notifications/verify-webhook-signature", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer closePayPalResponseBody(resp)

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("paypal verify status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		VerificationStatus string `json:"verification_status"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return err
	}
	if result.VerificationStatus != "SUCCESS" {
		return errSignatureMismatch
	}
	return nil
}

// paypalAccessToken exchanges the client credentials for an OAuth2 access
// token.
func paypalAccessToken(ctx context.Context, client *http.Client, cfg *payPalConfig) (string, error) {
	form := url.Values{}
	form.Set("grant_type", "client_credentials")

	base := paypalAPIBase(cfg.SandboxMode)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		base+"/v1/oauth2/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(cfg.ClientID, cfg.ClientSecret)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer closePayPalResponseBody(resp)

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("paypal oauth status %d: %s", resp.StatusCode, string(respBody))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		return "", err
	}
	if tokenResp.AccessToken == "" {
		return "", errors.New("empty paypal access token")
	}
	return tokenResp.AccessToken, nil
}

func closePayPalResponseBody(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	if err := resp.Body.Close(); err != nil {
		log.Printf("PayPal response body close failed: %v", err)
	}
}

// epayCallback is a verified EPay notification.
type epayCallback struct {
	TradeNo        string
	GatewayTradeNo string
	Amount         *float64
	Paid           bool
	Raw            string
}

// verifyEPayCallback checks an EPay notification's MD5 signature and reads
// it: the non-empty parameters other than sign and sign_type, in key order,
// joined and followed by the merchant key. config is the enabled EPay
// gateway's configuration, empty without one.
func verifyEPayCallback(query url.Values, rawBody []byte, config string) (*epayCallback, error) {
	var cfg ePayConfig
	if config != "" {
		if err := json.Unmarshal([]byte(config), &cfg); err != nil {
			return nil, fmt.Errorf("epay: invalid config: %w", err)
		}
	}
	if cfg.Key == "" {
		return nil, fmt.Errorf("epay: merchant key not configured")
	}

	sign := query.Get("sign")
	if sign == "" {
		return nil, fmt.Errorf("epay: missing sign")
	}

	expected := epaySign(query, cfg.Key)
	if !epaySignEqual(sign, expected) {
		return nil, fmt.Errorf("epay: sign mismatch")
	}

	paid := query.Get("trade_status") == "TRADE_SUCCESS"

	amount, err := parseEpayAmount(query.Get("money"))
	if paid && err != nil {
		return nil, err
	}
	if paid && query.Get("out_trade_no") == "" {
		return nil, fmt.Errorf("epay: missing out_trade_no")
	}
	if paid && query.Get("trade_no") == "" {
		return nil, fmt.Errorf("epay: missing trade_no")
	}

	return &epayCallback{
		TradeNo:        query.Get("out_trade_no"),
		GatewayTradeNo: query.Get("trade_no"),
		Amount:         amount,
		Paid:           paid,
		Raw:            string(rawBody),
	}, nil
}

// epaySign is EPay's MD5 signature of the parameters.
func epaySign(params map[string][]string, key string) string {
	keys := make([]string, 0, len(params))
	for k, vals := range params {
		if k == "sign" || k == "sign_type" || len(vals) == 0 || vals[0] == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(params[k][0])
	}
	b.WriteString(key)

	sum := md5.Sum([]byte(b.String())) // #nosec G401 -- EPay's public callback protocol uses MD5 signatures.
	return hex.EncodeToString(sum[:])
}

func epaySignEqual(got, expected string) bool {
	gotBytes, err := hex.DecodeString(strings.TrimSpace(got))
	if err != nil {
		return false
	}
	expectedBytes, err := hex.DecodeString(expected)
	if err != nil {
		return false
	}
	if len(gotBytes) != len(expectedBytes) {
		return false
	}
	return subtle.ConstantTimeCompare(gotBytes, expectedBytes) == 1
}

func parseEpayAmount(raw string) (*float64, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("epay: missing money")
	}
	amount, err := strconv.ParseFloat(raw, 64)
	if err != nil || amount <= 0 {
		return nil, fmt.Errorf("epay: invalid money")
	}
	return &amount, nil
}
