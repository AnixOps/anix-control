package service

// Kept apart from notification_service.go: it belongs to the commercial set
// (config/commercial-boundary.json).

import (
	"fmt"

	"github.com/AnixOps/anix-control/v5/internal/model"
)

// NotifyOrderPaid 订单支付成功通知
func (s *NotificationService) NotifyOrderPaid(order *model.Order, user *model.User) error {
	title := "支付成功通知"
	content := fmt.Sprintf("您的订单 #%s 已支付成功，金额: %.2f 元。", order.TradeNo, float64(order.TotalAmount)/100)

	var tgUser model.TelegramUser
	s.db.Where("user_id = ?", user.ID).First(&tgUser)

	if user.Email != "" {
		s.sendAsync(&user.ID, "email", model.EventOrderPaid, title, content, nil)
	}

	if tgUser.ID > 0 {
		s.sendAsync(&user.ID, "telegram", model.EventOrderPaid, title, content, nil)
	}

	return nil
}
