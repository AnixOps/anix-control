// Package payment 提供可插拔的支付网关注册表。
// 新增支付方式只需实现 Gateway 接口并在 init() 中 Register，
// 无需修改回调分发等核心代码（对标 XBoard 的插件式支付扩展）。
package payment

import (
	"net/http"
	"net/url"
)

// CallbackContext 封装一次支付回调的全部输入，供网关验签与解析。
type CallbackContext struct {
	RawBody []byte      // 原始请求体
	Headers http.Header // 请求头（验签常用）
	Query   url.Values  // 查询参数（部分网关把结果放这里）
	Config  string      // 网关配置 JSON（含密钥等），由调用方从 DB 读取
}

// CallbackResult 是网关验签并解析后的标准化结果。
type CallbackResult struct {
	TradeNo        string // 商户订单号
	GatewayTradeNo string // 第三方订单号
	Amount         *float64
	Status         string // paid / failed / pending
	Raw            string // 原始回调数据，落库备查
}

// 标准化支付状态常量。
const (
	StatusPaid    = "paid"
	StatusFailed  = "failed"
	StatusPending = "pending"
)

// Gateway 是所有支付网关插件需实现的接口。
type Gateway interface {
	// Type 返回网关类型标识（如 "epay"、"alipay"），与路由 :type 对应。
	Type() string
	// VerifyCallback 校验回调真实性并解析出标准化结果。
	// 验签失败必须返回 error，调用方据此拒绝（fail-closed）。
	VerifyCallback(ctx *CallbackContext) (*CallbackResult, error)
}
