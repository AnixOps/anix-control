package gateways

import (
	"net/url"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v3/internal/payment"
)

const epayTestConfig = `{"api_url":"https://pay.example.com","pid":"1001","key":"testkey123"}`

func buildEpayQuery(key string) url.Values {
	q := url.Values{}
	q.Set("pid", "1001")
	q.Set("out_trade_no", "PAY20260101")
	q.Set("trade_no", "EP998877")
	q.Set("trade_status", "TRADE_SUCCESS")
	q.Set("money", "10.00")
	q.Set("sign_type", "MD5")
	// 用与实现一致的算法生成合法签名。
	q.Set("sign", epaySign(q, key))
	return q
}

func TestEpayVerifyCallback_Valid(t *testing.T) {
	q := buildEpayQuery("testkey123")
	gw := epayGateway{}

	result, err := gw.VerifyCallback(&payment.CallbackContext{
		Query:  q,
		Config: epayTestConfig,
	})
	if err != nil {
		t.Fatalf("expected valid callback, got error: %v", err)
	}
	if result.Status != payment.StatusPaid {
		t.Fatalf("expected paid, got %s", result.Status)
	}
	if result.TradeNo != "PAY20260101" {
		t.Fatalf("expected trade_no PAY20260101, got %s", result.TradeNo)
	}
	if result.GatewayTradeNo != "EP998877" {
		t.Fatalf("expected gateway_trade_no EP998877, got %s", result.GatewayTradeNo)
	}
	if result.Amount == nil || *result.Amount != 10.00 {
		t.Fatalf("expected amount 10.00, got %#v", result.Amount)
	}
}

func TestEpayVerifyCallback_UppercaseSignAccepted(t *testing.T) {
	q := buildEpayQuery("testkey123")
	q.Set("sign", strings.ToUpper(q.Get("sign")))
	gw := epayGateway{}

	_, err := gw.VerifyCallback(&payment.CallbackContext{
		Query:  q,
		Config: epayTestConfig,
	})
	if err != nil {
		t.Fatalf("expected uppercase sign to be accepted, got error: %v", err)
	}
}

func TestEpayVerifyCallback_ForgedSignRejected(t *testing.T) {
	q := buildEpayQuery("testkey123")
	q.Set("sign", "deadbeefdeadbeefdeadbeefdeadbeef") // 篡改签名
	gw := epayGateway{}

	_, err := gw.VerifyCallback(&payment.CallbackContext{
		Query:  q,
		Config: epayTestConfig,
	})
	if err == nil {
		t.Fatal("expected forged sign to be rejected")
	}
}

func TestEpayVerifyCallback_TamperedAmountRejected(t *testing.T) {
	q := buildEpayQuery("testkey123")
	q.Set("money", "0.01") // 签名后篡改金额，签名应失配
	gw := epayGateway{}

	_, err := gw.VerifyCallback(&payment.CallbackContext{
		Query:  q,
		Config: epayTestConfig,
	})
	if err == nil {
		t.Fatal("expected tampered amount to invalidate sign")
	}
}

func TestEpayVerifyCallback_MissingKeyFailsClosed(t *testing.T) {
	q := buildEpayQuery("testkey123")
	gw := epayGateway{}

	_, err := gw.VerifyCallback(&payment.CallbackContext{
		Query:  q,
		Config: `{"pid":"1001"}`, // 无 key
	})
	if err == nil {
		t.Fatal("expected missing key to fail closed")
	}
}

func TestEpayVerifyCallback_MissingTradeNoRejectedForPaid(t *testing.T) {
	q := buildEpayQuery("testkey123")
	q.Del("out_trade_no")
	q.Set("sign", epaySign(q, "testkey123"))
	gw := epayGateway{}

	_, err := gw.VerifyCallback(&payment.CallbackContext{
		Query:  q,
		Config: epayTestConfig,
	})
	if err == nil {
		t.Fatal("expected paid callback without out_trade_no to be rejected")
	}
}

func TestEpayVerifyCallback_InvalidMoneyRejectedForPaid(t *testing.T) {
	q := buildEpayQuery("testkey123")
	q.Set("money", "not-a-number")
	q.Set("sign", epaySign(q, "testkey123"))
	gw := epayGateway{}

	_, err := gw.VerifyCallback(&payment.CallbackContext{
		Query:  q,
		Config: epayTestConfig,
	})
	if err == nil {
		t.Fatal("expected invalid money to be rejected")
	}
}

func TestRegisteredGatewayCallbackSignatureCoverage(t *testing.T) {
	type callbackCase struct {
		valid    func() (*payment.CallbackContext, error)
		tampered func() (*payment.CallbackContext, error)
	}

	cases := map[string]callbackCase{
		"epay": {
			valid: func() (*payment.CallbackContext, error) {
				return &payment.CallbackContext{
					Query:  buildEpayQuery("testkey123"),
					Config: epayTestConfig,
				}, nil
			},
			tampered: func() (*payment.CallbackContext, error) {
				q := buildEpayQuery("testkey123")
				q.Set("money", "0.01")
				return &payment.CallbackContext{
					Query:  q,
					Config: epayTestConfig,
				}, nil
			},
		},
	}

	registered := payment.Types()
	for _, typ := range registered {
		tc, ok := cases[typ]
		if !ok {
			t.Fatalf("registered payment gateway %q has no callback signature coverage case", typ)
		}

		t.Run(typ+"/valid", func(t *testing.T) {
			gw, ok := payment.Get(typ)
			if !ok {
				t.Fatalf("registered payment gateway %q cannot be loaded", typ)
			}
			ctx, err := tc.valid()
			if err != nil {
				t.Fatalf("build valid callback: %v", err)
			}
			if _, err := gw.VerifyCallback(ctx); err != nil {
				t.Fatalf("valid callback rejected: %v", err)
			}
		})

		t.Run(typ+"/tampered", func(t *testing.T) {
			gw, ok := payment.Get(typ)
			if !ok {
				t.Fatalf("registered payment gateway %q cannot be loaded", typ)
			}
			ctx, err := tc.tampered()
			if err != nil {
				t.Fatalf("build tampered callback: %v", err)
			}
			if _, err := gw.VerifyCallback(ctx); err == nil {
				t.Fatal("tampered callback accepted")
			}
		})
	}
}

func TestEpayRegistered(t *testing.T) {
	gw, ok := payment.Get("epay")
	if !ok {
		t.Fatal("expected epay gateway to be registered via init()")
	}
	if gw.Type() != "epay" {
		t.Fatalf("expected type epay, got %s", gw.Type())
	}
}
