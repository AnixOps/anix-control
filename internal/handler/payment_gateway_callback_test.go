package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anixops/v2board/internal/model"
	"github.com/anixops/v2board/internal/payment"
	"github.com/anixops/v2board/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	_ "github.com/anixops/v2board/internal/payment/gateways"
)

type PaymentGatewayCallbackTestSuite struct {
	HandlerTestSuite
}

func (s *PaymentGatewayCallbackTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()
	s.router = gin.New()
}

func (s *PaymentGatewayCallbackTestSuite) createEPayRecord(tradeNo string, actualAmount float64) uint {
	gateway := &model.PaymentGateway{
		Name:    "EPay Callback Gateway",
		Type:    model.PaymentGatewayEPay,
		Enabled: true,
		Config:  `{"api_url":"https://pay.example.com","pid":"1001","key":"testkey123"}`,
	}
	assert.NoError(s.T(), s.db.Create(gateway).Error)

	user := &model.User{
		Email:    tradeNo + "@example.com",
		Password: "hashed",
		Token:    tradeNo + "-token",
		UUID:     tradeNo + "-uuid",
	}
	assert.NoError(s.T(), s.db.Create(user).Error)

	order := &model.Order{
		TradeNo:     tradeNo,
		UserID:      user.ID,
		TotalAmount: int64(actualAmount * 100),
		Status:      0,
	}
	assert.NoError(s.T(), s.db.Create(order).Error)

	record := &model.PaymentRecord{
		GatewayID:    gateway.ID,
		TradeNo:      tradeNo,
		GatewayType:  model.PaymentGatewayEPay,
		UserID:       user.ID,
		Amount:       actualAmount,
		ActualAmount: actualAmount,
		Status:       model.PaymentStatusPending,
		OrderID:      &order.ID,
	}
	assert.NoError(s.T(), s.db.Create(record).Error)

	return order.ID
}

func (s *PaymentGatewayCallbackTestSuite) TestEPayCallbackRejectsSignedAmountMismatch() {
	orderID := s.createEPayRecord("PAY-CB-MISMATCH", 10.00)

	handler := NewPaymentGatewayHandler()
	handler.gatewayService = service.NewPaymentGatewayService(s.db)
	s.router.POST("/payment/callback/:type", handler.PaymentCallback)
	_, registered := payment.Get(model.PaymentGatewayEPay)
	assert.True(s.T(), registered)

	req, _ := http.NewRequest(http.MethodPost, "/payment/callback/epay?pid=1001&out_trade_no=PAY-CB-MISMATCH&trade_no=EP-CB-MISMATCH&trade_status=TRADE_SUCCESS&money=0.01&sign_type=MD5&sign=56e4b691d28f5d436107d4b56d35e1a2", strings.NewReader(""))
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusBadRequest, w.Code)
	assert.Equal(s.T(), "fail", w.Body.String())

	var record model.PaymentRecord
	assert.NoError(s.T(), s.db.Where("trade_no = ?", "PAY-CB-MISMATCH").First(&record).Error)
	assert.Equal(s.T(), model.PaymentStatusPending, record.Status)
	assert.Empty(s.T(), record.GatewayTradeNo)

	var order model.Order
	assert.NoError(s.T(), s.db.First(&order, orderID).Error)
	assert.Equal(s.T(), 0, order.Status)
}

func (s *PaymentGatewayCallbackTestSuite) TestEPayCallbackAcceptsSignedMatchingAmount() {
	orderID := s.createEPayRecord("PAY-CB-MATCH", 10.00)

	handler := NewPaymentGatewayHandler()
	handler.gatewayService = service.NewPaymentGatewayService(s.db)
	s.router.POST("/payment/callback/:type", handler.PaymentCallback)
	_, registered := payment.Get(model.PaymentGatewayEPay)
	assert.True(s.T(), registered)

	req, _ := http.NewRequest(http.MethodPost, "/payment/callback/epay?pid=1001&out_trade_no=PAY-CB-MATCH&trade_no=EP-CB-MATCH&trade_status=TRADE_SUCCESS&money=10.00&sign_type=MD5&sign=69052a8b67ba006adb8b2a5ed79e01d6", strings.NewReader(""))
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
	assert.Equal(s.T(), "success", w.Body.String())

	var record model.PaymentRecord
	assert.NoError(s.T(), s.db.Where("trade_no = ?", "PAY-CB-MATCH").First(&record).Error)
	assert.Equal(s.T(), model.PaymentStatusPaid, record.Status)
	assert.Equal(s.T(), "EP-CB-MATCH", record.GatewayTradeNo)

	var order model.Order
	assert.NoError(s.T(), s.db.First(&order, orderID).Error)
	assert.Equal(s.T(), 1, order.Status)
}

func TestPaymentGatewayCallback(t *testing.T) {
	suite.Run(t, new(PaymentGatewayCallbackTestSuite))
}
