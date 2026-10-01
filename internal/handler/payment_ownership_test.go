package handler

import (
	"bytes"
	"crypto/md5" // #nosec G501 -- EPay's public callback protocol uses MD5 signatures.
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	_ "github.com/AnixOps/anix-control/v4/internal/payment/gateways"
)

// PaymentOwnershipTestSuite holds the regression tests for payments bound
// to the caller's own orders and records.
type PaymentOwnershipTestSuite struct {
	HandlerTestSuite
	buyer, other *model.User
	plan         *model.Plan
	order        *model.Order
	gateway      *model.PaymentGateway
}

const epayTestKey = "epay-merchant-key"

func (s *PaymentOwnershipTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()
	s.buyer = &model.User{Email: "buyer@example.test", Token: "buyer-token", UUID: "buyer-uuid"}
	s.other = &model.User{Email: "other@example.test", Token: "other-token", UUID: "other-uuid"}
	s.Require().NoError(s.db.Create(s.buyer).Error)
	s.Require().NoError(s.db.Create(s.other).Error)
	s.plan = &model.Plan{Name: "Pro", GroupID: 2, TransferEnable: 100}
	s.Require().NoError(s.db.Create(s.plan).Error)
	s.order = &model.Order{TradeNo: "ORDER-BUYER", UserID: s.buyer.ID, PlanID: s.plan.ID, Period: "month", TotalAmount: 10000}
	s.Require().NoError(s.db.Create(s.order).Error)
	s.Require().NoError(s.db.AutoMigrate(&model.Payment{}))
	s.Require().NoError(s.db.Create(&model.Payment{Name: "x402", Method: "crypto", Provider: "x402", Enable: 1}).Error)
	s.gateway = &model.PaymentGateway{
		Name: "EPay", Type: model.PaymentGatewayEPay, Enabled: true, MinAmount: 1, MaxAmount: 10000,
		Config: `{"api_url":"https://pay.example.test","pid":"1001","key":"` + epayTestKey + `"}`,
	}
	s.Require().NoError(s.db.Create(s.gateway).Error)
}

// serve runs one request through handler as caller.
func (s *PaymentOwnershipTestSuite) serve(method, pattern, path string, caller uint, body any, handler gin.HandlerFunc) map[string]any {
	router := gin.New()
	router.Handle(method, pattern, func(c *gin.Context) {
		c.Set("user_id", caller)
		handler(c)
	})
	var reader *bytes.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		s.Require().NoError(err)
		reader = bytes.NewReader(encoded)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	s.Require().Equal(http.StatusOK, w.Code, w.Body.String())
	return decodePanelTestResponse(s.T(), w)
}

func (s *PaymentOwnershipTestSuite) requireError(resp map[string]any, msg string) {
	s.T().Helper()
	s.Equal(float64(-1), resp["code"], "%v", resp)
	s.Equal(msg, resp["msg"])
}

func (s *PaymentOwnershipTestSuite) createPayment(caller uint, body map[string]any) map[string]any {
	return s.serve(http.MethodPost, "/user/payment/create", "/user/payment/create", caller, body, NewPaymentGatewayHandler().CreatePayment)
}

// A payment for an order pays the caller's own pending order in full: it
// may not pay less, nor name another user's order.
func (s *PaymentOwnershipTestSuite) TestCreatePaymentPaysTheCallersOrderInFull() {
	s.requireError(s.createPayment(s.buyer.ID, map[string]any{"gateway_id": s.gateway.ID, "amount": 1, "order_id": s.order.ID}), "支付金额与订单金额不符")
	s.requireError(s.createPayment(s.buyer.ID, map[string]any{"gateway_id": s.gateway.ID, "amount": 1000, "order_id": s.order.ID}), "支付金额与订单金额不符")
	s.requireError(s.createPayment(s.other.ID, map[string]any{"gateway_id": s.gateway.ID, "amount": 100, "order_id": s.order.ID}), "订单不存在")
	s.requireError(s.createPayment(s.buyer.ID, map[string]any{"gateway_id": s.gateway.ID, "amount": 100, "order_id": 999}), "订单不存在")
	var records int64
	s.Require().NoError(s.db.Model(&model.PaymentRecord{}).Count(&records).Error)
	s.Zero(records, "a refused payment records nothing")

	resp := s.createPayment(s.buyer.ID, map[string]any{"gateway_id": s.gateway.ID, "amount": 100, "order_id": s.order.ID})
	s.Equal(float64(0), resp["code"], "%v", resp)
	var record model.PaymentRecord
	s.Require().NoError(s.db.Take(&record, "order_id = ?", s.order.ID).Error)
	s.Equal(s.buyer.ID, record.UserID)
	s.InDelta(100, record.Amount, 0.000001)

	s.Require().NoError(s.db.Model(&model.Order{}).Where("id = ?", s.order.ID).Update("status", 1).Error)
	s.requireError(s.createPayment(s.buyer.ID, map[string]any{"gateway_id": s.gateway.ID, "amount": 100, "order_id": s.order.ID}), "订单已支付或已取消")

	// A payment without an order is a top-up of any allowed amount.
	resp = s.createPayment(s.buyer.ID, map[string]any{"gateway_id": s.gateway.ID, "amount": 1})
	s.Equal(float64(0), resp["code"], "%v", resp)
}

// epaySigned is an EPay callback query signed with the merchant key.
func epaySigned(params map[string]string, key string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	query := url.Values{}
	for _, k := range keys {
		pairs = append(pairs, k+"="+params[k])
		query.Set(k, params[k])
	}
	sum := md5.Sum([]byte(strings.Join(pairs, "&") + key)) // #nosec G401 -- EPay's public callback protocol uses MD5 signatures.
	query.Set("sign", hex.EncodeToString(sum[:]))
	query.Set("sign_type", "MD5")
	return query.Encode()
}

// A record created before payments were bound to their orders may still
// name an order it does not pay. Its paid callback records the payment but
// leaves the order unpaid and its plan unassigned.
func (s *PaymentOwnershipTestSuite) TestCallbackDoesNotPayAnOrderTheRecordDoesNotCover() {
	for _, tc := range []struct {
		name   string
		user   uint
		amount float64
	}{
		{"an amount below the order's total", s.buyer.ID, 1},
		{"another user's order", s.other.ID, 100},
	} {
		s.Run(tc.name, func() {
			tradeNo := "PAY-LEGACY-" + strconv.FormatUint(uint64(tc.user), 10) + "-" + strconv.FormatFloat(tc.amount, 'f', 0, 64)
			s.Require().NoError(s.db.Create(&model.PaymentRecord{
				TradeNo: tradeNo, GatewayID: s.gateway.ID, GatewayType: model.PaymentGatewayEPay, UserID: tc.user,
				Amount: tc.amount, ActualAmount: tc.amount, Status: model.PaymentStatusPending, OrderID: &s.order.ID,
			}).Error)
			query := epaySigned(map[string]string{
				"pid": "1001", "out_trade_no": tradeNo, "trade_no": "EP-" + tradeNo, "trade_status": "TRADE_SUCCESS",
				"money": strconv.FormatFloat(tc.amount, 'f', 2, 64),
			}, epayTestKey)
			router := gin.New()
			router.POST("/payment/callback/:type", NewPaymentGatewayHandler().PaymentCallback)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/payment/callback/epay?"+query, nil))
			s.Equal(http.StatusOK, w.Code)
			s.Equal("success", w.Body.String())

			var record model.PaymentRecord
			s.Require().NoError(s.db.Take(&record, "trade_no = ?", tradeNo).Error)
			s.Equal(model.PaymentStatusPaid, record.Status, "the payment itself is recorded")
			var order model.Order
			s.Require().NoError(s.db.Take(&order, s.order.ID).Error)
			s.Equal(0, order.Status, "the order is not paid")
			s.Nil(order.PaidAt)
			var buyer model.User
			s.Require().NoError(s.db.Take(&buyer, s.buyer.ID).Error)
			s.Nil(buyer.PlanID, "no plan is assigned")
		})
	}

	// The record that pays the order in full still activates it.
	s.Require().NoError(s.db.Create(&model.PaymentRecord{
		TradeNo: "PAY-FULL", GatewayID: s.gateway.ID, GatewayType: model.PaymentGatewayEPay, UserID: s.buyer.ID,
		Amount: 100, ActualAmount: 100, Status: model.PaymentStatusPending, OrderID: &s.order.ID,
	}).Error)
	query := epaySigned(map[string]string{
		"pid": "1001", "out_trade_no": "PAY-FULL", "trade_no": "EP-FULL", "trade_status": "TRADE_SUCCESS", "money": "100.00",
	}, epayTestKey)
	router := gin.New()
	router.POST("/payment/callback/:type", NewPaymentGatewayHandler().PaymentCallback)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/payment/callback/epay?"+query, nil))
	s.Equal("success", w.Body.String())
	var order model.Order
	s.Require().NoError(s.db.Take(&order, s.order.ID).Error)
	s.Equal(3, order.Status)
	var buyer model.User
	s.Require().NoError(s.db.Take(&buyer, s.buyer.ID).Error)
	s.Require().NotNil(buyer.PlanID)
	s.Equal(s.plan.ID, *buyer.PlanID)
}

// X402 and fiat payments are created only for the caller's own orders.
func (s *PaymentOwnershipTestSuite) TestCryptoAndFiatPaymentsNeedTheCallersOrder() {
	payments := NewPaymentHandler()
	s.requireError(s.serve(http.MethodPost, "/x402/create", "/x402/create", s.other.ID,
		map[string]any{"order_id": s.order.ID, "token": "ETH", "network": "sepolia"}, payments.X402CreatePayment), "订单不存在")
	s.requireError(s.serve(http.MethodPost, "/fiat/create", "/fiat/create", s.other.ID,
		map[string]any{"order_id": s.order.ID, "provider": "stripe"}, payments.FiatCreatePayment), "订单不存在")
	var records int64
	s.Require().NoError(s.db.Model(&model.PaymentRecord{}).Count(&records).Error)
	s.Zero(records)

	resp := s.serve(http.MethodPost, "/x402/create", "/x402/create", s.buyer.ID,
		map[string]any{"order_id": s.order.ID, "token": "ETH", "network": "sepolia"}, payments.X402CreatePayment)
	s.Equal(float64(0), resp["code"], "%v", resp)
}

// A user sees only their own payment records.
func (s *PaymentOwnershipTestSuite) TestPaymentStatusIsTheOwnersOnly() {
	record := &model.PaymentRecord{TradeNo: "PAY-OWNED", UserID: s.buyer.ID, Amount: 100, ActualAmount: 100, Status: model.PaymentStatusPaid}
	s.Require().NoError(s.db.Create(record).Error)
	payments := NewPaymentHandler()
	gateways := NewPaymentGatewayHandler()
	id := strconv.FormatUint(uint64(record.ID), 10)
	for _, path := range []string{"/x402/check/PAY-OWNED", "/x402/check/" + id} {
		s.requireError(s.serve(http.MethodGet, "/x402/check/:id", path, s.other.ID, nil, payments.X402CheckPayment), "支付记录不存在")
		resp := s.serve(http.MethodGet, "/x402/check/:id", path, s.buyer.ID, nil, payments.X402CheckPayment)
		s.Equal(float64(0), resp["code"], "%v", resp)
	}
	s.requireError(s.serve(http.MethodGet, "/user/payment/status/:trade_no", "/user/payment/status/PAY-OWNED", s.other.ID, nil, gateways.GetPaymentStatus), "支付记录不存在")
	resp := s.serve(http.MethodGet, "/user/payment/status/:trade_no", "/user/payment/status/PAY-OWNED", s.buyer.ID, nil, gateways.GetPaymentStatus)
	s.Equal(float64(0), resp["code"], "%v", resp)
}

// Administrators' gateway responses carry no secret; a configuration sent
// back with the placeholder keeps the stored secrets.
func (s *PaymentOwnershipTestSuite) TestGatewaySecretsAreRedactedAndKept() {
	handler := NewPaymentGatewayHandler()
	list := s.serve(http.MethodGet, "/admin/payment/gateways", "/admin/payment/gateways", 1, nil, handler.ListGateways)
	encoded, err := json.Marshal(list)
	s.Require().NoError(err)
	s.NotContains(string(encoded), epayTestKey)
	gateways := list["data"].(map[string]any)["list"].([]any)
	s.JSONEq(`{"api_url":"https://pay.example.test","key":"********","pid":"1001"}`, gateways[0].(map[string]any)["config"].(string))

	path := "/admin/payment/gateways/" + strconv.FormatUint(uint64(s.gateway.ID), 10)
	resp := s.serve(http.MethodPut, "/admin/payment/gateways/:id", path, 1,
		map[string]any{"config": map[string]any{"api_url": "https://pay2.example.test", "pid": "1001", "key": "********"}}, handler.UpdateGateway)
	s.Equal(float64(0), resp["code"], "%v", resp)
	s.NotContains(resp["data"].(map[string]any)["config"], epayTestKey)
	var stored model.PaymentGateway
	s.Require().NoError(s.db.Take(&stored, s.gateway.ID).Error)
	s.JSONEq(`{"api_url":"https://pay2.example.test","key":"`+epayTestKey+`","pid":"1001"}`, stored.Config)

	resp = s.serve(http.MethodPut, "/admin/payment/gateways/:id", path, 1,
		map[string]any{"config": map[string]any{"api_url": "https://pay2.example.test", "pid": "1001", "key": "rotated"}}, handler.UpdateGateway)
	s.Equal(float64(0), resp["code"], "%v", resp)
	s.Require().NoError(s.db.Take(&stored, s.gateway.ID).Error)
	s.JSONEq(`{"api_url":"https://pay2.example.test","key":"rotated","pid":"1001"}`, stored.Config)

	resp = s.serve(http.MethodPost, "/admin/payment/gateways", "/admin/payment/gateways", 1,
		map[string]any{"name": "Stripe", "type": "stripe", "config": map[string]any{"publishable_key": "pk", "secret_key": "sk", "webhook_secret": "********"}}, handler.CreateGateway)
	s.Equal(float64(0), resp["code"], "%v", resp)
	s.JSONEq(`{"publishable_key":"pk","secret_key":"********","webhook_secret":""}`, resp["data"].(map[string]any)["config"].(string))
	var created model.PaymentGateway
	s.Require().NoError(s.db.Take(&created, "name = ?", "Stripe").Error)
	s.JSONEq(`{"publishable_key":"pk","secret_key":"sk","webhook_secret":""}`, created.Config, "a placeholder on create stores no secret")
}

func TestPaymentOwnership(t *testing.T) {
	suite.Run(t, new(PaymentOwnershipTestSuite))
}

func TestGatewayConfigRedaction(t *testing.T) {
	for _, key := range []string{"key", "secret_key", "webhook_secret", "client_secret", "private_key", "api_key", "api_v3_key", "sign_key", "access_token", "password"} {
		assert.True(t, gatewaySecretKey(key), key)
	}
	for _, key := range []string{"public_key", "publishable_key", "client_id", "webhook_id", "app_id", "pid", "api_url", "key_path", "wallet_address", "network"} {
		assert.False(t, gatewaySecretKey(key), key)
	}
	require.Equal(t, "", redactGatewayConfig(""))
	require.Equal(t, `{"pid":"1"}`, redactGatewayConfig(`{"pid":"1"}`), "a configuration without a secret is unchanged")
	require.Equal(t, `{"key":""}`, redactGatewayConfig(`{"key":""}`), "an empty secret is shown as empty")
	require.Equal(t, `{"confirm_blocks":12345678901234567890,"nested":{"api_key":"********"},"url":"a?b=1&c=2"}`,
		redactGatewayConfig(`{"url":"a?b=1&c=2","confirm_blocks":12345678901234567890,"nested":{"api_key":"k"}}`))
	require.Equal(t, gatewaySecretPlaceholder, redactGatewayConfig(`key=secret`), "an unreadable configuration is hidden whole")
	require.Equal(t, gatewaySecretPlaceholder, redactGatewayConfig(`{"key":"a"} trailing`))

	require.Equal(t, `{"key":"old","pid":"2"}`, keepGatewaySecrets(`{"pid":"2","key":"********"}`, `{"key":"old","pid":"1"}`))
	require.Equal(t, `{"pid":"2","key":"new"}`, keepGatewaySecrets(`{"pid":"2","key":"new"}`, `{"key":"old"}`), "a new secret is stored as sent")
	require.Equal(t, `{"note":"********"}`, keepGatewaySecrets(`{"note":"********"}`, `{"note":"x"}`), "only secrets are restored")
	require.Equal(t, `{"key":""}`, keepGatewaySecrets(`{"key":"********"}`, `not json`))
	require.Equal(t, `not json`, keepGatewaySecrets(`not json`, `{"key":"old"}`))
	require.Equal(t, service.SensitiveSystemConfigPlaceholder, gatewaySecretPlaceholder)
}
