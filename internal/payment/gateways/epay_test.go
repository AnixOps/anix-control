package gateways

import (
	"net/url"
	"testing"

	"github.com/anixops/v2board/internal/payment"
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

func TestEpayRegistered(t *testing.T) {
	gw, ok := payment.Get("epay")
	if !ok {
		t.Fatal("expected epay gateway to be registered via init()")
	}
	if gw.Type() != "epay" {
		t.Fatalf("expected type epay, got %s", gw.Type())
	}
}
