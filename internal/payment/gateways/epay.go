package gateways

import (
	"crypto/md5" // #nosec G501 -- EPay's public callback protocol uses MD5 signatures.
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/anixops/v2board/internal/model"
	"github.com/anixops/v2board/internal/payment"
)

// epayGateway 实现 EPay 通用支付的回调验签（MD5 签名约定）。
type epayGateway struct{}

func (epayGateway) Type() string { return model.PaymentGatewayEPay }

// VerifyCallback 校验 EPay 回调的 MD5 签名并解析结果。
// EPay 约定：对除 sign/sign_type 外的非空参数按 key 升序拼接，
// 末尾追加商户密钥后做 MD5，与回调带的 sign 比对。
func (epayGateway) VerifyCallback(ctx *payment.CallbackContext) (*payment.CallbackResult, error) {
	var cfg model.EPayConfig
	if ctx.Config != "" {
		if err := json.Unmarshal([]byte(ctx.Config), &cfg); err != nil {
			return nil, fmt.Errorf("epay: invalid config: %w", err)
		}
	}
	if cfg.Key == "" {
		return nil, fmt.Errorf("epay: merchant key not configured")
	}

	params := ctx.Query
	sign := params.Get("sign")
	if sign == "" {
		return nil, fmt.Errorf("epay: missing sign")
	}

	expected := epaySign(params, cfg.Key)
	if !epaySignEqual(sign, expected) {
		return nil, fmt.Errorf("epay: sign mismatch")
	}

	status := payment.StatusPending
	if params.Get("trade_status") == "TRADE_SUCCESS" {
		status = payment.StatusPaid
	}

	amount, err := parseEpayAmount(params.Get("money"))
	if status == payment.StatusPaid && err != nil {
		return nil, err
	}
	if status == payment.StatusPaid && params.Get("out_trade_no") == "" {
		return nil, fmt.Errorf("epay: missing out_trade_no")
	}
	if status == payment.StatusPaid && params.Get("trade_no") == "" {
		return nil, fmt.Errorf("epay: missing trade_no")
	}

	return &payment.CallbackResult{
		TradeNo:        params.Get("out_trade_no"),
		GatewayTradeNo: params.Get("trade_no"),
		Amount:         amount,
		Status:         status,
		Raw:            string(ctx.RawBody),
	}, nil
}

// epaySign 按 EPay 约定计算 MD5 签名。
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

func init() {
	payment.Register(epayGateway{})
}
