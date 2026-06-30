package gateways

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
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
	if !strings.EqualFold(sign, expected) {
		return nil, fmt.Errorf("epay: sign mismatch")
	}

	status := payment.StatusPending
	if params.Get("trade_status") == "TRADE_SUCCESS" {
		status = payment.StatusPaid
	}

	return &payment.CallbackResult{
		TradeNo:        params.Get("out_trade_no"),
		GatewayTradeNo: params.Get("trade_no"),
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

	sum := md5.Sum([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

func init() {
	payment.Register(epayGateway{})
}
