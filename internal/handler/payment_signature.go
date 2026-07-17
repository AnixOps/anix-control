package handler

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
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

	"github.com/AnixOps/anix-control/v4/internal/model"
)

// 验签相关错误
var (
	errSignatureMissing  = errors.New("missing signature")
	errSignatureMismatch = errors.New("signature mismatch")
	errSignatureExpired  = errors.New("signature timestamp expired")
	errWebhookSecret     = errors.New("webhook secret not configured")
)

// stripeSignatureTolerance Stripe 时间戳容差，防重放。
const stripeSignatureTolerance = 5 * time.Minute

// verifyStripeSignature 按 Stripe 官方算法校验 Webhook 签名。
// 头格式: "t=<timestamp>,v1=<sig>[,v1=<sig>...]"，签名体为 "<timestamp>.<rawBody>"，
// 算法为 HMAC-SHA256，密钥为 Webhook Secret。
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

	// 防重放：拒绝超出容差的时间戳。
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

	// Stripe 可能在轮换密钥期间发送多个 v1 签名，任一匹配即通过。
	for _, sig := range v1Sigs {
		if hmac.Equal([]byte(sig), []byte(expected)) {
			return nil
		}
	}
	return errSignatureMismatch
}

// verifyX402Signature 校验 X402 回调签名。
// 对回调关键字段做确定性拼接后 HMAC-SHA256，避免依赖 JSON 字段顺序。
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

// canonicalizeFields 将字段按 key 升序拼成 "k1=v1&k2=v2" 形式，保证签名体确定性。
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

// computeHMACSHA256 返回十六进制小写 HMAC-SHA256。
func computeHMACSHA256(message, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

// paypalSignatureHeaders 聚合 PayPal 回调用于验签的传输头。
type paypalSignatureHeaders struct {
	AuthAlgo         string
	CertURL          string
	TransmissionID   string
	TransmissionSig  string
	TransmissionTime string
}

// paypalHTTPClient 用于 PayPal API 调用，带超时防止回调阻塞。
var paypalHTTPClient = &http.Client{Timeout: 10 * time.Second}

func paypalAPIBase(sandbox bool) string {
	if sandbox {
		return "https://api-m.sandbox.paypal.com"
	}
	return "https://api-m.paypal.com"
}

// verifyPayPalSignatureRemote 调用 PayPal verify-webhook-signature API 校验回调。
func verifyPayPalSignatureRemote(ctx context.Context, cfg *model.PayPalConfig, h paypalSignatureHeaders, body []byte) error {
	token, err := paypalAccessToken(ctx, cfg)
	if err != nil {
		return fmt.Errorf("paypal token: %w", err)
	}

	// webhook_event 必须为原始 JSON 对象，故用 RawMessage 保留原文。
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

	resp, err := paypalHTTPClient.Do(req)
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

// paypalAccessToken 用 client_credentials 换取 PayPal OAuth2 访问令牌。
func paypalAccessToken(ctx context.Context, cfg *model.PayPalConfig) (string, error) {
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

	resp, err := paypalHTTPClient.Do(req)
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
