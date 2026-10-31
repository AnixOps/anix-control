package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
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

// paypalHTTPClient 用于 PayPal API 调用，带超时防止回调阻塞。
