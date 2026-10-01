package native

// The kernel's tests of the ported verification (internal/handler/payment_signature_test.go), as is.

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestVerifyStripeSignature(t *testing.T) {
	secret := "whsec_test_secret"
	payload := []byte(`{"id":"evt_1","type":"checkout.session.completed"}`)
	now := time.Unix(1700000000, 0)
	ts := "1700000000"
	validSig := computeHMACSHA256(ts+"."+string(payload), secret)

	tests := []struct {
		name    string
		header  string
		secret  string
		now     time.Time
		wantErr error
	}{
		{
			name:   "valid signature",
			header: "t=" + ts + ",v1=" + validSig,
			secret: secret,
			now:    now,
		},
		{
			name:    "missing secret fails closed",
			header:  "t=" + ts + ",v1=" + validSig,
			secret:  "",
			now:     now,
			wantErr: errWebhookSecret,
		},
		{
			name:    "missing header",
			header:  "",
			secret:  secret,
			now:     now,
			wantErr: errSignatureMissing,
		},
		{
			name:    "tampered signature rejected",
			header:  "t=" + ts + ",v1=deadbeef",
			secret:  secret,
			now:     now,
			wantErr: errSignatureMismatch,
		},
		{
			name:    "expired timestamp rejected (replay)",
			header:  "t=" + ts + ",v1=" + validSig,
			secret:  secret,
			now:     now.Add(10 * time.Minute),
			wantErr: errSignatureExpired,
		},
		{
			name:   "multiple v1 sigs, one valid",
			header: "t=" + ts + ",v1=deadbeef,v1=" + validSig,
			secret: secret,
			now:    now,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := verifyStripeSignature(payload, tt.header, tt.secret, tt.now)
			if tt.wantErr == nil && err != nil {
				t.Fatalf("expected success, got %v", err)
			}
			if tt.wantErr != nil && err != tt.wantErr {
				t.Fatalf("expected %v, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestVerifyX402Signature(t *testing.T) {
	secret := "x402_webhook_secret"
	fields := map[string]string{
		"trade_no":      "X402202601010000",
		"tx_hash":       "0xabc123",
		"block_number":  "100",
		"confirmations": "3",
		"status":        "confirmed",
		"amount":        "0.01",
		"token":         "ETH",
	}
	validSig := computeHMACSHA256(canonicalizeFields(fields), secret)

	t.Run("valid signature", func(t *testing.T) {
		if err := verifyX402Signature(fields, validSig, secret); err != nil {
			t.Fatalf("expected success, got %v", err)
		}
	})

	t.Run("uppercase signature accepted", func(t *testing.T) {
		upper := ""
		for _, r := range validSig {
			if r >= 'a' && r <= 'f' {
				upper += string(r - 32)
			} else {
				upper += string(r)
			}
		}
		if err := verifyX402Signature(fields, upper, secret); err != nil {
			t.Fatalf("expected success for uppercase, got %v", err)
		}
	})

	t.Run("missing secret fails closed", func(t *testing.T) {
		if err := verifyX402Signature(fields, validSig, ""); err != errWebhookSecret {
			t.Fatalf("expected errWebhookSecret, got %v", err)
		}
	})

	t.Run("forged signature rejected", func(t *testing.T) {
		if err := verifyX402Signature(fields, "00000000", secret); err != errSignatureMismatch {
			t.Fatalf("expected errSignatureMismatch, got %v", err)
		}
	})

	t.Run("tampered amount rejected", func(t *testing.T) {
		tampered := map[string]string{}
		for k, v := range fields {
			tampered[k] = v
		}
		tampered["amount"] = "9999.0"
		if err := verifyX402Signature(tampered, validSig, secret); err != errSignatureMismatch {
			t.Fatalf("expected errSignatureMismatch for tampered amount, got %v", err)
		}
	})
}

func TestCanonicalizeFieldsDeterministic(t *testing.T) {
	a := map[string]string{"b": "2", "a": "1", "c": "3"}
	want := "a=1&b=2&c=3"
	if got := canonicalizeFields(a); got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

const epayTestConfig = `{"api_url":"https://pay.example.com","pid":"1001","key":"testkey123"}`

func buildEpayQuery(key string) url.Values {
	q := url.Values{}
	q.Set("pid", "1001")
	q.Set("out_trade_no", "PAY20260101")
	q.Set("trade_no", "EP998877")
	q.Set("trade_status", "TRADE_SUCCESS")
	q.Set("money", "10.00")
	q.Set("sign_type", "MD5")
	q.Set("sign", epaySign(q, key))
	return q
}

// The kernel's EPay verifier tests (internal/payment/gateways/epay_test.go)
// on the ported verifier.
func TestVerifyEPayCallback(t *testing.T) {
	result, err := verifyEPayCallback(buildEpayQuery("testkey123"), []byte("raw"), epayTestConfig)
	if err != nil {
		t.Fatalf("expected valid callback, got error: %v", err)
	}
	if !result.Paid || result.TradeNo != "PAY20260101" || result.GatewayTradeNo != "EP998877" || result.Raw != "raw" {
		t.Fatalf("unexpected result %#v", result)
	}
	if result.Amount == nil || *result.Amount != 10.00 {
		t.Fatalf("expected amount 10.00, got %#v", result.Amount)
	}

	upper := buildEpayQuery("testkey123")
	upper.Set("sign", strings.ToUpper(upper.Get("sign")))
	if _, err := verifyEPayCallback(upper, nil, epayTestConfig); err != nil {
		t.Fatalf("expected uppercase sign to be accepted, got error: %v", err)
	}

	pending := buildEpayQuery("testkey123")
	pending.Set("trade_status", "WAIT_BUYER_PAY")
	pending.Set("sign", epaySign(pending, "testkey123"))
	if result, err := verifyEPayCallback(pending, nil, epayTestConfig); err != nil || result.Paid {
		t.Fatalf("expected a signed pending callback, got %#v, %v", result, err)
	}

	for name, mutate := range map[string]func(url.Values) (url.Values, string){
		"forged sign": func(q url.Values) (url.Values, string) {
			q.Set("sign", "deadbeefdeadbeefdeadbeefdeadbeef")
			return q, epayTestConfig
		},
		"tampered amount":  func(q url.Values) (url.Values, string) { q.Set("money", "0.01"); return q, epayTestConfig },
		"missing key":      func(q url.Values) (url.Values, string) { return q, `{"pid":"1001"}` },
		"no configuration": func(q url.Values) (url.Values, string) { return q, "" },
		"broken config":    func(q url.Values) (url.Values, string) { return q, `{"key":1}` },
		"missing sign":     func(q url.Values) (url.Values, string) { q.Del("sign"); return q, epayTestConfig },
		"missing trade no": func(q url.Values) (url.Values, string) {
			q.Del("out_trade_no")
			q.Set("sign", epaySign(q, "testkey123"))
			return q, epayTestConfig
		},
		"missing gateway no": func(q url.Values) (url.Values, string) {
			q.Del("trade_no")
			q.Set("sign", epaySign(q, "testkey123"))
			return q, epayTestConfig
		},
		"invalid money": func(q url.Values) (url.Values, string) {
			q.Set("money", "not-a-number")
			q.Set("sign", epaySign(q, "testkey123"))
			return q, epayTestConfig
		},
		"no money": func(q url.Values) (url.Values, string) {
			q.Del("money")
			q.Set("sign", epaySign(q, "testkey123"))
			return q, epayTestConfig
		},
	} {
		query, config := mutate(buildEpayQuery("testkey123"))
		if _, err := verifyEPayCallback(query, nil, config); err == nil {
			t.Fatalf("%s: expected the callback to be rejected", name)
		}
	}
}
