package native

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	kernelorderv1 "github.com/AnixOps/anix-control/sdk/api/kernelorder/v1"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// The callback routes' ids.
const (
	CallbackRouteID       = "payment.callback"
	X402CallbackRouteID   = "payment.payment.x402.callback.post"
	StripeWebhookRouteID  = "payment.payment.stripe.webhook.post"
	PayPalWebhookRouteID  = "payment.payment.paypal.webhook.post"
	textContentType       = "text/plain; charset=utf-8"
	jsonContentType       = "application/json; charset=utf-8"
	x402AmountDecimals    = 8
	minX402Confirmations  = 1
	paymentMatchTolerance = 0.01
)

// Provider configurations, with the kernel model's fields and types
// (model.EPayConfig, StripeConfig, PayPalConfig, X402Config), so that a
// configuration the kernel cannot read is unreadable here too.
type ePayConfig struct {
	APIURL    string `json:"api_url"`
	PID       string `json:"pid"`
	Key       string `json:"key"`
	NotifyURL string `json:"notify_url"`
	ReturnURL string `json:"return_url"`
}

type stripeConfig struct {
	PublishableKey string `json:"publishable_key"`
	SecretKey      string `json:"secret_key"`
	WebhookSecret  string `json:"webhook_secret"`
	Currency       string `json:"currency"`
}

type payPalConfig struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	WebhookID    string `json:"webhook_id"`
	SandboxMode  bool   `json:"sandbox_mode"`
	Currency     string `json:"currency"`
}

type x402Config struct {
	WalletAddress string   `json:"wallet_address"`
	Network       string   `json:"network"`
	AcceptTokens  []string `json:"accept_tokens"`
	TestMode      bool     `json:"test_mode"`
	WebhookSecret string   `json:"webhook_secret"`
	ConfirmBlocks int      `json:"confirm_blocks"`
}

// callbackVerifiers are the gateways /payment/callback/:type serves, by
// type: the kernel's payment registry (internal/payment), whose server
// registers EPay only.
var callbackVerifiers = map[string]func(query url.Values, rawBody []byte, config string) (*epayCallback, error){
	gatewayEPay: verifyEPayCallback,
}

// Answers of the kernel's payment service, in its words.
var (
	errPaymentProcessed      = errors.New("payment already processed")
	errPaymentAmountMismatch = errors.New("payment amount mismatch")
	// errPaymentNotCovered wraps the reason a paid callback does not pay
	// what its record asks for; nothing is written.
	errPaymentNotCovered = errors.New("payment not applied")
)

func rawText(statusCode uint32, body string) pluginhostsdk.NativeResponse {
	return pluginhostsdk.NativeResponse{
		StatusCode: statusCode, Body: []byte(body), Headers: []pluginhostsdk.Header{{Name: "Content-Type", Value: textContentType}},
	}
}

// rawJSON is gin's Context.JSON of a gin.H: the object's keys in order,
// HTML characters escaped.
func rawJSON(statusCode uint32, value map[string]any) (pluginhostsdk.NativeResponse, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return pluginhostsdk.NativeResponse{}, err
	}
	return pluginhostsdk.NativeResponse{
		StatusCode: statusCode, Body: body, Headers: []pluginhostsdk.Header{{Name: "Content-Type", Value: jsonContentType}},
	}, nil
}

// requestHeader is gin's Context.GetHeader on the forwarded headers.
func requestHeader(request pluginhostsdk.NativeRequest, name string) string {
	return http.Header(request.Metadata.Headers).Get(name)
}

// enabledGateway is the kernel's GetByType: the first enabled gateway of a
// type that may be enabled.
func enabledGateway(db *gorm.DB, gatewayType string) (*PaymentGateway, error) {
	if db == nil {
		return nil, errors.New("payment storage is unavailable")
	}
	if !gatewayTypeCanBeEnabled(gatewayType) {
		return nil, fmt.Errorf("payment gateway type %q cannot be enabled until live callback implementation and tests are complete", gatewayType)
	}
	var gateway PaymentGateway
	if err := db.Where("type = ? AND enabled = ?", gatewayType, true).First(&gateway).Error; err != nil {
		return nil, err
	}
	return &gateway, nil
}

// gatewayConfig decodes the enabled gateway's configuration of a type into
// config, as the kernel's ParseConfig does, and reports whether it could.
func gatewayConfig(db *gorm.DB, gatewayType string, config any) bool {
	gateway, err := enabledGateway(db, gatewayType)
	if err != nil {
		return false
	}
	return json.Unmarshal([]byte(gateway.Config), config) == nil
}

// openOrNil is the package's storage, or nil when it is unavailable: the
// callbacks then find no gateway configuration and refuse, as the kernel's
// do when its database is.
func (s *Service) openOrNil(ctx context.Context) *gorm.DB {
	db, err := s.Open(ctx)
	if err != nil {
		log.Printf("payment callback: storage unavailable: %v", err)
		return nil
	}
	return db
}

// amountCheck is the kernel's MarkOrderPaidWithAmount check: a callback's
// amount pays a record whose actual amount it matches within a cent.
func amountCheck(paid *float64) func(PaymentRecord) error {
	if paid == nil {
		return nil
	}
	return func(record PaymentRecord) error {
		if math.Abs(record.ActualAmount-*paid) > paymentMatchTolerance {
			return errPaymentAmountMismatch
		}
		return nil
	}
}

// payRecord is the first of a paid callback's two steps, the kernel's
// markOrderPaid on the package's own tables: in one transaction, under
// the record's row lock, a pending record that check accepts is marked
// paid with the gateway's trade number and the notification, and added to
// its gateway's statistics. It returns the record as read, and
// errPaymentProcessed when it was not pending.
func (s *Service) payRecord(db *gorm.DB, tradeNo, gatewayTradeNo, notifyData string, check func(PaymentRecord) error) (PaymentRecord, error) {
	var record PaymentRecord
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("trade_no = ?", tradeNo).First(&record).Error; err != nil {
			return err
		}
		if record.Status != PaymentStatusPending {
			return errPaymentProcessed
		}
		if check != nil {
			if err := check(record); err != nil {
				return err
			}
		}
		now := s.now()
		if err := tx.Model(&PaymentRecord{}).Where("id = ?", record.ID).Updates(map[string]any{
			"status":           PaymentStatusPaid,
			"gateway_trade_no": gatewayTradeNo,
			"notify_data":      notifyData,
			"paid_at":          now,
		}).Error; err != nil {
			return err
		}
		if record.GatewayID != 0 {
			if err := tx.Model(&PaymentGateway{}).Where("id = ?", record.GatewayID).Updates(map[string]any{
				"total_orders": gorm.Expr("total_orders + 1"),
				"total_amount": gorm.Expr("total_amount + ?", record.ActualAmount),
			}).Error; err != nil {
				return err
			}
		}
		record.Status, record.PaidAt = PaymentStatusPaid, &now
		return nil
	})
	return record, err
}

// completeOrder is a paid callback's second step: the kernel applies a
// paid record to the order it names (KernelOrder.CompleteOrderPayment),
// re-checking that the record pays it, marking it paid and completing it
// in one transaction, once per trade number. It runs after payRecord
// committed, and again for every repeat of a callback whose record is
// already paid, so a failure between the two steps converges on the
// provider's next delivery. A record that is not paid or names no order
// needs nothing. A refusal is final: the payment stays recorded and the
// callback is answered as the kernel's is. Any other failure is returned
// and fails the callback (the gateway answers 502), so the provider
// delivers it again.
func (s *Service) completeOrder(ctx context.Context, record PaymentRecord) error {
	if record.Status != PaymentStatusPaid || record.OrderID == nil {
		return nil
	}
	response, err := s.Orders.CompleteOrderPayment(ctx, &kernelorderv1.CompleteOrderPaymentRequest{
		TradeNo: record.TradeNo, OrderId: uint64(*record.OrderID),
	})
	if err != nil {
		return fmt.Errorf("payment %s: completing order %d: %w", record.TradeNo, *record.OrderID, err)
	}
	switch response.GetOutcome() {
	case kernelorderv1.OrderPaymentOutcome_ORDER_PAYMENT_OUTCOME_REFUSED:
		log.Printf("payment %s does not pay order %d: %s; the order is left unchanged", record.TradeNo, *record.OrderID, response.GetReason())
	case kernelorderv1.OrderPaymentOutcome_ORDER_PAYMENT_OUTCOME_PAID:
		log.Printf("payment %s: order %d is paid but was not activated", record.TradeNo, *record.OrderID)
	}
	return nil
}

// orderCompletionError is a failed completeOrder: the callback fails
// instead of answering.
type orderCompletionError struct{ err error }

func (e *orderCompletionError) Error() string { return e.err.Error() }
func (e *orderCompletionError) Unwrap() error { return e.err }

// payAndComplete runs both steps of a paid callback. It returns payRecord's
// error, which the callback answers as the kernel's does, or an
// *orderCompletionError, which fails the callback.
func (s *Service) payAndComplete(ctx context.Context, db *gorm.DB, tradeNo, gatewayTradeNo, notifyData string, check func(PaymentRecord) error) error {
	record, err := s.payRecord(db, tradeNo, gatewayTradeNo, notifyData, check)
	if err == nil || errors.Is(err, errPaymentProcessed) {
		if failed := s.completeOrder(ctx, record); failed != nil {
			return &orderCompletionError{err: failed}
		}
	}
	return err
}

// completionFailed reports whether err fails the callback.
func completionFailed(err error) bool {
	var failed *orderCompletionError
	return errors.As(err, &failed)
}

// PaymentCallback is POST /api/v2/payment/callback/:type: a registered
// gateway's signed notification (EPay's, in its query) marks the payment
// paid and completes its order. It answers "success", or "fail" with 400.
func (s *Service) PaymentCallback(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	fail := rawText(http.StatusBadRequest, "fail")
	gatewayType := request.Metadata.PathParams["type"]
	verify, ok := callbackVerifiers[gatewayType]
	if !ok {
		log.Printf("payment callback: no registered gateway for type=%s", gatewayType)
		return fail, nil
	}
	db := s.openOrNil(ctx)
	config := ""
	if gateway, err := enabledGateway(db, gatewayType); err == nil {
		config = gateway.Config
	}
	result, err := verify(url.Values(request.Metadata.Query), request.Body, config)
	if err != nil {
		log.Printf("payment callback verification failed for type=%s: %v", gatewayType, err)
		return fail, nil
	}
	if result.Paid {
		if err := s.payAndComplete(ctx, db, result.TradeNo, result.GatewayTradeNo, result.Raw, amountCheck(result.Amount)); err != nil {
			if completionFailed(err) {
				return pluginhostsdk.NativeResponse{}, err
			}
			log.Printf("payment callback: mark paid failed for trade_no=%s: %v", result.TradeNo, err)
			return fail, nil
		}
	}
	return rawText(http.StatusOK, "success"), nil
}

// x402Decimal is a callback amount: a plain non-negative decimal number.
var x402Decimal = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`)

// x402PaymentCovers is the kernel's check of a confirmed x402 callback
// against its pending record: the callback pays in the record's token (any
// case), one the gateway accepts when its accept_tokens is set, and at
// least the record's actual amount as X402CreatePayment shows it, eight
// decimals, compared as exact decimals.
func x402PaymentCovers(token, amount string, acceptTokens []string) func(PaymentRecord) error {
	token = strings.TrimSpace(token)
	amount = strings.TrimSpace(amount)
	return func(record PaymentRecord) error {
		expectedToken := strings.TrimSpace(record.Currency)
		if expectedToken == "" || !strings.EqualFold(token, expectedToken) {
			return fmt.Errorf("%w: token %q is not the payment's %q", errPaymentNotCovered, token, expectedToken)
		}
		if len(acceptTokens) > 0 && !containsFold(acceptTokens, token) {
			return fmt.Errorf("%w: token %q is not accepted by the x402 gateway", errPaymentNotCovered, token)
		}
		if !x402Decimal.MatchString(amount) {
			return fmt.Errorf("%w: amount %q is not a decimal number", errPaymentNotCovered, amount)
		}
		paid, ok := new(big.Rat).SetString(amount)
		if !ok {
			return fmt.Errorf("%w: amount %q is not a decimal number", errPaymentNotCovered, amount)
		}
		expected := strconv.FormatFloat(record.ActualAmount, 'f', x402AmountDecimals, 64)
		want, ok := new(big.Rat).SetString(expected)
		if !ok {
			return fmt.Errorf("%w: the payment's amount %v is not a number", errPaymentNotCovered, record.ActualAmount)
		}
		if paid.Cmp(want) < 0 {
			return fmt.Errorf("%w: amount %s is below the payment's %s %s", errPaymentNotCovered, amount, expected, expectedToken)
		}
		return nil
	}
}

func containsFold(values []string, value string) bool {
	for _, v := range values {
		if strings.EqualFold(strings.TrimSpace(v), value) {
			return true
		}
	}
	return false
}

// X402Callback is POST /api/v2/payment/x402/callback: the confirmation
// service's signed report on a crypto payment. A confirmed transfer of the
// payment's token and amount marks it paid and completes its order; a
// failed one cancels the payment.
func (s *Service) X402Callback(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	var callbackData struct {
		TradeNo       string `json:"trade_no"`
		TxHash        string `json:"tx_hash"`
		BlockNumber   int64  `json:"block_number"`
		Confirmations int    `json:"confirmations"`
		Status        string `json:"status"`
		Amount        string `json:"amount"`
		Token         string `json:"token"`
		Signature     string `json:"signature"`
	}
	if err := json.Unmarshal(request.Body, &callbackData); err != nil {
		return rawJSON(http.StatusBadRequest, map[string]any{"message": "无效的回调数据", "error": err.Error()})
	}

	db := s.openOrNil(ctx)
	var config x402Config
	secret := ""
	if gatewayConfig(db, gatewayX402, &config) {
		secret = config.WebhookSecret
	}
	signFields := map[string]string{
		"trade_no":      callbackData.TradeNo,
		"tx_hash":       callbackData.TxHash,
		"block_number":  strconv.FormatInt(callbackData.BlockNumber, 10),
		"confirmations": strconv.Itoa(callbackData.Confirmations),
		"status":        callbackData.Status,
		"amount":        callbackData.Amount,
		"token":         callbackData.Token,
	}
	if err := verifyX402Signature(signFields, callbackData.Signature, secret); err != nil {
		log.Printf("X402 callback signature verification failed for trade_no=%s: %v", callbackData.TradeNo, err)
		return rawJSON(http.StatusBadRequest, map[string]any{"message": "签名验证失败"})
	}

	payment, err := recordByTradeNo(db, callbackData.TradeNo)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return rawJSON(http.StatusNotFound, map[string]any{"message": "支付记录不存在"})
		}
		return rawJSON(http.StatusInternalServerError, map[string]any{"message": "数据库错误"})
	}
	if payment.Status != PaymentStatusPending {
		if err := s.completeOrder(ctx, *payment); err != nil {
			return pluginhostsdk.NativeResponse{}, err
		}
		return rawJSON(http.StatusOK, map[string]any{"status": "ok", "message": "already processed"})
	}
	if callbackData.Confirmations < minX402Confirmations {
		return rawJSON(http.StatusOK, map[string]any{"status": "ok", "message": "insufficient confirmations"})
	}

	rawBody := string(request.Body)
	txHash := callbackData.TxHash
	switch callbackData.Status {
	case "confirmed", "success":
		// The gateway's accepted tokens, none when its configuration cannot
		// be read, as the kernel's x402AcceptTokens.
		var acceptTokens []string
		var accepted x402Config
		if gatewayConfig(db, gatewayX402, &accepted) {
			acceptTokens = accepted.AcceptTokens
		}
		covers := x402PaymentCovers(callbackData.Token, callbackData.Amount, acceptTokens)
		if err := s.payAndComplete(ctx, db, callbackData.TradeNo, txHash, rawBody, covers); err != nil {
			if completionFailed(err) {
				return pluginhostsdk.NativeResponse{}, err
			}
			if errors.Is(err, errPaymentNotCovered) {
				// Refused for good: acknowledged, so it is not redelivered.
				log.Printf("X402 callback for trade_no=%s: %v", callbackData.TradeNo, err)
				return rawJSON(http.StatusOK, map[string]any{"status": "ok", "message": err.Error()})
			}
			return rawJSON(http.StatusInternalServerError, map[string]any{"message": "更新支付状态失败", "error": err.Error()})
		}
		return rawJSON(http.StatusOK, map[string]any{"status": "ok", "message": "payment confirmed"})
	case "failed":
		now := s.now()
		if err := db.Model(&PaymentRecord{}).Where("trade_no = ?", callbackData.TradeNo).Updates(map[string]any{
			"status":       PaymentStatusCancelled,
			"tx_hash":      &txHash,
			"notify_data":  &rawBody,
			"cancelled_at": &now,
		}).Error; err != nil {
			log.Printf("failed to mark payment failed for trade_no=%s: %v", callbackData.TradeNo, err)
		}
		return rawJSON(http.StatusOK, map[string]any{"status": "ok", "message": "payment failed"})
	default:
		return rawJSON(http.StatusOK, map[string]any{"status": "ok", "message": "pending"})
	}
}

// endPayment sets a payment's final status from a provider event, whatever
// its current one, as the kernel's webhooks do; a failure is only logged.
func (s *Service) endPayment(db *gorm.DB, tradeNo string, paymentStatus int, rawBody, what string) {
	now := s.now()
	if err := db.Model(&PaymentRecord{}).Where("trade_no = ?", tradeNo).Updates(map[string]any{
		"status":       paymentStatus,
		"notify_data":  &rawBody,
		"cancelled_at": &now,
	}).Error; err != nil {
		log.Printf("failed to mark payment %s for trade_no=%s: %v", what, tradeNo, err)
	}
}

// StripeWebhook is POST /api/v2/payment/stripe/webhook: a Stripe event,
// signed with the gateway's webhook secret within five minutes. A completed
// checkout marks the payment paid and completes its order; an expired or
// failed one ends the payment.
func (s *Service) StripeWebhook(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	db := s.openOrNil(ctx)
	var config stripeConfig
	secret := ""
	if gatewayConfig(db, gatewayStripe, &config) {
		secret = config.WebhookSecret
	}
	if err := verifyStripeSignature(request.Body, requestHeader(request, "Stripe-Signature"), secret, s.now()); err != nil {
		log.Printf("Stripe webhook signature verification failed: %v", err)
		return rawJSON(http.StatusBadRequest, map[string]any{"message": "签名验证失败"})
	}

	var event struct {
		ID   string `json:"id"`
		Type string `json:"type"`
		Data struct {
			Object struct {
				ID                string `json:"id"`
				Status            string `json:"status"`
				AmountTotal       int64  `json:"amount_total"`
				Currency          string `json:"currency"`
				ClientReferenceID string `json:"client_reference_id"`
				Metadata          struct {
					TradeNo string `json:"trade_no"`
					OrderID string `json:"order_id"`
				} `json:"metadata"`
			} `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(request.Body, &event); err != nil {
		return rawJSON(http.StatusBadRequest, map[string]any{"message": "无效的事件数据"})
	}

	rawBody := string(request.Body)
	tradeNo := event.Data.Object.Metadata.TradeNo
	if tradeNo == "" {
		tradeNo = event.Data.Object.ClientReferenceID
	}
	switch event.Type {
	case "checkout.session.completed":
		if tradeNo == "" {
			return rawJSON(http.StatusOK, map[string]any{"received": true, "message": "no trade_no in metadata"})
		}
		if err := s.payAndComplete(ctx, db, tradeNo, event.Data.Object.ID, rawBody, nil); err != nil {
			if completionFailed(err) {
				return pluginhostsdk.NativeResponse{}, err
			}
			return rawJSON(http.StatusOK, map[string]any{"received": true, "message": "already processed or error: " + err.Error()})
		}
		return rawJSON(http.StatusOK, map[string]any{"received": true})
	case "checkout.session.expired":
		if tradeNo != "" {
			s.endPayment(db, tradeNo, PaymentStatusExpired, rawBody, "expired")
		}
		return rawJSON(http.StatusOK, map[string]any{"received": true})
	case "checkout.session.async_payment_failed":
		if tradeNo != "" {
			s.endPayment(db, tradeNo, PaymentStatusCancelled, rawBody, "failed")
		}
		return rawJSON(http.StatusOK, map[string]any{"received": true})
	default:
		return rawJSON(http.StatusOK, map[string]any{"received": true, "message": "event type not handled: " + event.Type})
	}
}

// verifyPayPalWebhook asks PayPal whether a delivery is genuine, with the
// enabled PayPal gateway's client credentials and webhook id; without them
// it refuses.
func (s *Service) verifyPayPalWebhook(ctx context.Context, db *gorm.DB, request pluginhostsdk.NativeRequest) error {
	gateway, err := enabledGateway(db, gatewayPayPal)
	if err != nil {
		return errWebhookSecret
	}
	var cfg payPalConfig
	if err := json.Unmarshal([]byte(gateway.Config), &cfg); err != nil {
		return err
	}
	if cfg.ClientID == "" || cfg.ClientSecret == "" || cfg.WebhookID == "" {
		return errWebhookSecret
	}
	headers := paypalSignatureHeaders{
		AuthAlgo:         requestHeader(request, "Paypal-Auth-Algo"),
		CertURL:          requestHeader(request, "Paypal-Cert-Url"),
		TransmissionID:   requestHeader(request, "Paypal-Transmission-Id"),
		TransmissionSig:  requestHeader(request, "Paypal-Transmission-Sig"),
		TransmissionTime: requestHeader(request, "Paypal-Transmission-Time"),
	}
	if headers.TransmissionID == "" || headers.TransmissionSig == "" {
		return errSignatureMissing
	}
	client := s.HTTPClient
	if client == nil {
		client = defaultPayPalClient
	}
	return verifyPayPalSignatureRemote(ctx, client, &cfg, headers, request.Body)
}

// paypalCapturedAmount checks a capture's amount and currency against the
// payment record and returns the amount the record must match.
func paypalCapturedAmount(db *gorm.DB, tradeNo, value, currency string) (float64, error) {
	amount, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return 0, fmt.Errorf("capture amount %q is not a number", value)
	}
	record, err := recordByTradeNo(db, tradeNo)
	if err != nil {
		return 0, fmt.Errorf("payment record: %w", err)
	}
	if record.Currency != "" && !strings.EqualFold(record.Currency, strings.TrimSpace(currency)) {
		return 0, fmt.Errorf("capture currency %q is not %s", currency, record.Currency)
	}
	return amount, nil
}

// PayPalWebhook is POST /api/v2/payment/paypal/webhook: a PayPal event that
// PayPal's API confirms. Only a completed capture pays (#74): it marks the
// payment paid when its amount and currency match, and completes its order;
// a denied capture or a processing order cancels the payment.
func (s *Service) PayPalWebhook(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	db := s.openOrNil(ctx)
	if err := s.verifyPayPalWebhook(ctx, db, request); err != nil {
		log.Printf("PayPal webhook signature verification failed: %v", err)
		return rawJSON(http.StatusBadRequest, map[string]any{"message": "签名验证失败"})
	}

	var event struct {
		ID        string `json:"id"`
		EventType string `json:"event_type"`
		Resource  struct {
			ID            string `json:"id"`
			Status        string `json:"status"`
			PurchaseUnits []struct {
				ReferenceID string `json:"reference_id"`
				Payments    struct {
					Captures []struct {
						ID     string `json:"id"`
						Amount struct {
							Value    string `json:"value"`
							Currency string `json:"currency_code"`
						} `json:"amount"`
					} `json:"captures"`
				} `json:"payments"`
			} `json:"purchase_units"`
			CustomID string `json:"custom_id"`
			Amount   struct {
				Value    string `json:"value"`
				Currency string `json:"currency_code"`
			} `json:"amount"`
		} `json:"resource"`
	}
	if err := json.Unmarshal(request.Body, &event); err != nil {
		return rawJSON(http.StatusBadRequest, map[string]any{"message": "无效的事件数据"})
	}

	rawBody := string(request.Body)
	tradeNo := event.Resource.CustomID
	switch event.EventType {
	case "PAYMENT.CAPTURE.COMPLETED":
		if tradeNo == "" {
			return rawJSON(http.StatusOK, map[string]any{"received": true, "message": "no custom_id in resource"})
		}
		paid, err := paypalCapturedAmount(db, tradeNo, event.Resource.Amount.Value, event.Resource.Amount.Currency)
		if err != nil {
			log.Printf("PayPal capture for trade_no=%s not applied: %v", tradeNo, err)
			return rawJSON(http.StatusOK, map[string]any{"received": true, "message": "capture not applied: " + err.Error()})
		}
		if err := s.payAndComplete(ctx, db, tradeNo, event.Resource.ID, rawBody, amountCheck(&paid)); err != nil {
			if completionFailed(err) {
				return pluginhostsdk.NativeResponse{}, err
			}
			return rawJSON(http.StatusOK, map[string]any{"received": true, "message": "already processed or error: " + err.Error()})
		}
		return rawJSON(http.StatusOK, map[string]any{"received": true})
	case "CHECKOUT.ORDER.COMPLETED":
		// Its captures follow as PAYMENT.CAPTURE.COMPLETED.
		return rawJSON(http.StatusOK, map[string]any{"received": true})
	case "CHECKOUT.ORDER.PROCESSING", "PAYMENT.CAPTURE.DENIED":
		if tradeNo != "" {
			s.endPayment(db, tradeNo, PaymentStatusCancelled, rawBody, "denied")
		}
		return rawJSON(http.StatusOK, map[string]any{"received": true})
	default:
		return rawJSON(http.StatusOK, map[string]any{"received": true, "message": "event type not handled: " + event.EventType})
	}
}
