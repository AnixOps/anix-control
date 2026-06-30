package handler

import (
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
