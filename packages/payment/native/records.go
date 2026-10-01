package native

import (
	"context"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"gorm.io/gorm"
)

// Pagination of the administrator's record list, as the kernel's
// ClampPagination.
const (
	defaultPageSize = 20
	maxPageSize     = 100
)

func clampPagination(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	return page, pageSize
}

func recordStatusText(status int) string {
	switch status {
	case PaymentStatusPaid:
		return "paid"
	case PaymentStatusRefunded:
		return "refunded"
	case PaymentStatusCancelled:
		return "failed"
	default:
		return "pending"
	}
}

func recordStatusFromText(status string) (int, bool) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "pending":
		return PaymentStatusPending, true
	case "paid":
		return PaymentStatusPaid, true
	case "failed", "cancelled":
		return PaymentStatusCancelled, true
	case "refunded":
		return PaymentStatusRefunded, true
	default:
		return 0, false
	}
}

// AdminRecords is GET /api/v2/admin/payment/records: newest first, filtered
// by status (a number or its name) and gateway type.
func (s *Service) AdminRecords(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	page, _ := strconv.Atoi(defaultQuery(request, "page", "1"))
	pageSize, _ := strconv.Atoi(defaultQuery(request, "page_size", "20"))
	page, pageSize = clampPagination(page, pageSize)
	gatewayType := queryValue(request, "gateway_type")
	var status *int
	if raw := queryValue(request, "status"); raw != "" {
		if numeric, err := strconv.Atoi(raw); err == nil {
			status = &numeric
		} else if mapped, ok := recordStatusFromText(raw); ok {
			status = &mapped
		}
	}
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	query := db.Model(&PaymentRecord{})
	if status != nil {
		query = query.Where("status = ?", *status)
	}
	if gatewayType != "" {
		query = query.Where("gateway_type = ?", gatewayType)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return s.panelError(err.Error())
	}
	var records []*PaymentRecord
	if err := query.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&records).Error; err != nil {
		return s.panelError(err.Error())
	}
	list := make([]map[string]any, 0, len(records))
	for _, record := range records {
		list = append(list, map[string]any{
			"id":               record.ID,
			"trade_no":         record.TradeNo,
			"gateway_id":       record.GatewayID,
			"gateway_type":     record.GatewayType,
			"gateway_trade_no": record.GatewayTradeNo,
			"user_id":          record.UserID,
			"amount":           record.Amount,
			"fee_amount":       record.FeeAmount,
			"actual_amount":    record.ActualAmount,
			"currency":         record.Currency,
			"status":           recordStatusText(record.Status),
			"status_code":      record.Status,
			"paid_at":          record.PaidAt,
			"cancelled_at":     record.CancelledAt,
			"refunded_at":      record.RefundedAt,
			"client_ip":        record.ClientIP,
			"created_at":       record.CreatedAt,
			"updated_at":       record.UpdatedAt,
		})
	}
	return s.panel(map[string]any{"list": list, "total": total, "page": page, "page_size": pageSize})
}

// AdminStats is GET /api/v2/admin/payment/stats for the dates start to end
// (default: the last 30 days), as the kernel's GetPaymentStats and
// PaymentGatewayService.GetStats; failed sums count as zero, as there.
func (s *Service) AdminStats(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	now := s.now()
	start, err := time.Parse("2006-01-02", defaultQuery(request, "start", now.AddDate(0, 0, -30).Format("2006-01-02")))
	if err != nil {
		return s.panelError("invalid start date")
	}
	end, err := time.Parse("2006-01-02", defaultQuery(request, "end", now.Format("2006-01-02")))
	if err != nil {
		return s.panelError("invalid end date")
	}
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	paid := func() *gorm.DB {
		return db.Model(&PaymentRecord{}).Where("status = ? AND paid_at BETWEEN ? AND ?", PaymentStatusPaid, start, end)
	}
	pending := func() *gorm.DB {
		return db.Model(&PaymentRecord{}).Where("status = ? AND created_at BETWEEN ? AND ?", PaymentStatusPending, start, end)
	}
	var totalAmount, pendingAmount float64
	var totalCount, pendingCount int64
	paid().Select("COALESCE(SUM(actual_amount), 0)").Scan(&totalAmount)
	paid().Count(&totalCount)
	pending().Select("COALESCE(SUM(amount), 0)").Scan(&pendingAmount)
	pending().Count(&pendingCount)

	var totalOrders int64
	if err := db.Model(&PaymentRecord{}).Where("created_at BETWEEN ? AND ?", start, end).Count(&totalOrders).Error; err != nil {
		log.Printf("failed to count total orders: %v", err)
	}
	var successOrders int64
	if err := paid().Count(&successOrders).Error; err != nil {
		log.Printf("failed to count success orders: %v", err)
	}
	successRate := float64(0)
	if totalOrders > 0 {
		successRate = float64(successOrders) * 100 / float64(totalOrders)
	}
	var byGatewayRows []struct {
		GatewayType string
		Amount      float64
		Count       int64
	}
	if err := paid().Select("gateway_type, COALESCE(SUM(actual_amount), 0) AS amount, COUNT(*) AS count").
		Group("gateway_type").Scan(&byGatewayRows).Error; err != nil {
		log.Printf("failed to scan gateway rows: %v", err)
	}
	byGateway := make(map[string]map[string]any, len(byGatewayRows))
	for _, row := range byGatewayRows {
		byGateway[row.GatewayType] = map[string]any{"amount": row.Amount, "count": row.Count}
	}
	return s.panel(map[string]any{
		"total_amount":   totalAmount,
		"total_count":    totalCount,
		"pending_amount": pendingAmount,
		"pending_count":  pendingCount,
		"total_orders":   totalOrders,
		"success_orders": successOrders,
		"success_rate":   successRate,
		"by_gateway":     byGateway,
	})
}

// UserRecords is GET /api/v2/user/payment/records: the caller's records,
// newest first. The page is not clamped, as in the kernel.
func (s *Service) UserRecords(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	page, _ := strconv.Atoi(defaultQuery(request, "page", "1"))
	pageSize, _ := strconv.Atoi(defaultQuery(request, "page_size", "20"))
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	query := db.Model(&PaymentRecord{}).Where("user_id = ?", request.Principal.ActorID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return s.panelError(err.Error())
	}
	var records []*PaymentRecord
	if err := query.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&records).Error; err != nil {
		return s.panelError(err.Error())
	}
	return s.panel(map[string]any{"list": records, "total": total, "page": page, "page_size": pageSize})
}
