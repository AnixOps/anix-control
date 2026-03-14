package service

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/smtp"
	"strings"
	"time"

	"github.com/anixops/v2board/internal/config"
	"github.com/anixops/v2board/internal/model"
	"gorm.io/gorm"
)

// NotificationService 通知服务
type NotificationService struct {
	db           *gorm.DB
	cfg          *config.Config
	emailConfig  *model.EmailConfig
	httpClient   *http.Client
}

// NewNotificationService 创建服务
func NewNotificationService(db *gorm.DB, cfg *config.Config) *NotificationService {
	return &NotificationService{
		db:   db,
		cfg:  cfg,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SetEmailConfig 设置邮件配置
func (s *NotificationService) SetEmailConfig(cfg *model.EmailConfig) {
	s.emailConfig = cfg
}

// Send 发送通知
func (s *NotificationService) Send(userID *uint, notifyType, event, title, content string, data map[string]interface{}) error {
	// 记录日志
	log := &model.NotificationLog{
		UserID:  userID,
		Type:    notifyType,
		Event:   event,
		Title:   title,
		Content: content,
		Status:  0,
	}
	s.db.Create(log)

	var err error
	now := time.Now()

	switch notifyType {
	case "email":
		if userID != nil {
			err = s.sendEmailNotification(*userID, title, content)
		}
	case "telegram":
		if userID != nil {
			err = s.sendTelegramNotification(*userID, title, content)
		}
	case "webhook":
		err = s.sendWebhookNotification(event, title, content, data)
	}

	// 更新状态
	if err != nil {
		log.Status = 2
		log.Error = err.Error()
	} else {
		log.Status = 1
		log.SentAt = &now
	}
	s.db.Save(log)

	return err
}

// SendEmail 发送邮件
func (s *NotificationService) SendEmail(to, subject, body string) error {
	if s.emailConfig == nil {
		return fmt.Errorf("email not configured")
	}

	// 构建邮件
	from := s.emailConfig.FromAddress
	msg := fmt.Sprintf("From: %s <%s>\r\n", s.emailConfig.FromName, from)
	msg += fmt.Sprintf("To: %s\r\n", to)
	msg += fmt.Sprintf("Subject: %s\r\n", subject)
	msg += "MIME-version: 1.0;\r\nContent-Type: text/html; charset=\"UTF-8\";\r\n\r\n"
	msg += body

	// 发送
	var auth smtp.Auth
	if s.emailConfig.Username != "" {
		auth = smtp.PlainAuth("", s.emailConfig.Username, s.emailConfig.Password, s.emailConfig.Host)
	}

	addr := fmt.Sprintf("%s:%d", s.emailConfig.Host, s.emailConfig.Port)

	// TLS配置
	if s.emailConfig.Encryption == "ssl" || s.emailConfig.Encryption == "tls" {
		return s.sendEmailTLS(addr, auth, from, []string{to}, []byte(msg))
	}

	return smtp.SendMail(addr, auth, from, []string{to}, []byte(msg))
}

// sendEmailTLS 通过TLS发送邮件
func (s *NotificationService) sendEmailTLS(addr string, auth smtp.Auth, from string, to []string, msg []byte) error {
	host := strings.Split(addr, ":")[0]

	conn, err := tls.Dial("tcp", addr, &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         host,
	})
	if err != nil {
		return err
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer client.Close()

	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return err
		}
	}

	if err := client.Mail(from); err != nil {
		return err
	}

	for _, addr := range to {
		if err := client.Rcpt(addr); err != nil {
			return err
		}
	}

	w, err := client.Data()
	if err != nil {
		return err
	}
	defer w.Close()

	_, err = w.Write(msg)
	return err
}

// sendEmailNotification 发送邮件通知
func (s *NotificationService) sendEmailNotification(userID uint, title, content string) error {
	var user model.User
	if err := s.db.First(&user, userID).Error; err != nil {
		return err
	}

	// 使用模板
	tmpl := s.getEmailTemplate(title, content)
	return s.SendEmail(user.Email, title, tmpl)
}

// sendTelegramNotification 发送Telegram通知
func (s *NotificationService) sendTelegramNotification(userID uint, title, content string) error {
	var tgUser model.TelegramUser
	if err := s.db.Where("user_id = ?", userID).First(&tgUser).Error; err != nil {
		// 用户未绑定Telegram
		return nil
	}

	botService := NewTelegramBotService(s.db)
	return botService.SendNotification(tgUser.TelegramID, title, content)
}

// sendWebhookNotification 发送Webhook通知
func (s *NotificationService) sendWebhookNotification(event, title, content string, data map[string]interface{}) error {
	// 获取Webhook配置
	// TODO: 从配置或数据库读取Webhook URL
	webhookURL := "" // 配置的Webhook地址
	if webhookURL == "" {
		return nil
	}

	payload := map[string]interface{}{
		"event":     event,
		"title":     title,
		"content":   content,
		"timestamp": time.Now().Unix(),
		"data":      data,
	}

	jsonData, _ := json.Marshal(payload)
	resp, err := s.httpClient.Post(webhookURL, "application/json", bytes.NewReader(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return nil
}

// getEmailTemplate 获取邮件模板
func (s *NotificationService) getEmailTemplate(title, content string) string {
	htmlTmpl := `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>{{.Title}}</title>
</head>
<body style="margin: 0; padding: 0; background-color: #f5f5f5;">
    <table width="100%" cellpadding="0" cellspacing="0" style="padding: 40px 0;">
        <tr>
            <td align="center">
                <table width="600" cellpadding="0" cellspacing="0" style="background-color: #ffffff; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1);">
                    <tr>
                        <td style="padding: 30px; background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); border-radius: 8px 8px 0 0;">
                            <h1 style="margin: 0; color: #ffffff; font-size: 24px;">{{.Title}}</h1>
                        </td>
                    </tr>
                    <tr>
                        <td style="padding: 30px; color: #333333; line-height: 1.6;">
                            {{.Content}}
                        </td>
                    </tr>
                    <tr>
                        <td style="padding: 20px 30px; background-color: #f8f9fa; border-radius: 0 0 8px 8px; text-align: center; color: #666666; font-size: 12px;">
                            此邮件由系统自动发送，请勿直接回复。
                        </td>
                    </tr>
                </table>
            </td>
        </tr>
    </table>
</body>
</html>`

	tmpl, _ := template.New("email").Parse(htmlTmpl)
	var buf bytes.Buffer
	tmpl.Execute(&buf, map[string]string{
		"Title":   title,
		"Content": content,
	})
	return buf.String()
}

// ========== 事件触发方法 ==========

// NotifyUserExpire 用户到期通知
func (s *NotificationService) NotifyUserExpire(user *model.User, daysLeft int) error {
	title := "账户即将到期提醒"
	content := fmt.Sprintf("您的账户将在 %d 天后到期，请及时续费以继续使用服务。", daysLeft)

	// 检查用户通知设置
	var tgUser model.TelegramUser
	s.db.Where("user_id = ? AND notify_expire = ?", user.ID, true).First(&tgUser)

	// 发送邮件
	if user.Email != "" {
		go s.Send(&user.ID, "email", model.EventUserExpire, title, content, nil)
	}

	// 发送Telegram
	if tgUser.ID > 0 {
		go s.Send(&user.ID, "telegram", model.EventUserExpire, title, content, nil)
	}

	return nil
}

// NotifyTrafficLow 流量不足通知
func (s *NotificationService) NotifyTrafficLow(user *model.User, percentLeft float64) error {
	title := "流量不足提醒"
	content := fmt.Sprintf("您的流量剩余 %.1f%%，请及时充值或升级套餐。", percentLeft)

	var tgUser model.TelegramUser
	s.db.Where("user_id = ? AND notify_traffic = ?", user.ID, true).First(&tgUser)

	if user.Email != "" {
		go s.Send(&user.ID, "email", model.EventUserTrafficLow, title, content, nil)
	}

	if tgUser.ID > 0 {
		go s.Send(&user.ID, "telegram", model.EventUserTrafficLow, title, content, nil)
	}

	return nil
}

// NotifyTicketReply 工单回复通知
func (s *NotificationService) NotifyTicketReply(ticket *model.Ticket, replyBy string) error {
	var user model.User
	if err := s.db.First(&user, ticket.UserID).Error; err != nil {
		return err
	}

	title := "工单回复通知"
	content := fmt.Sprintf("您的工单 #%d 收到了来自 %s 的回复，请登录查看。", ticket.ID, replyBy)

	var tgUser model.TelegramUser
	s.db.Where("user_id = ? AND notify_ticket = ?", user.ID, true).First(&tgUser)

	if user.Email != "" {
		go s.Send(&user.ID, "email", model.EventTicketReplied, title, content, nil)
	}

	if tgUser.ID > 0 {
		go s.Send(&user.ID, "telegram", model.EventTicketReplied, title, content, nil)
	}

	return nil
}

// NotifyOrderPaid 订单支付成功通知
func (s *NotificationService) NotifyOrderPaid(order *model.Order, user *model.User) error {
	title := "支付成功通知"
	content := fmt.Sprintf("您的订单 #%s 已支付成功，金额: %.2f 元。", order.TradeNo, float64(order.TotalAmount)/100)

	var tgUser model.TelegramUser
	s.db.Where("user_id = ?", user.ID).First(&tgUser)

	if user.Email != "" {
		go s.Send(&user.ID, "email", model.EventOrderPaid, title, content, nil)
	}

	if tgUser.ID > 0 {
		go s.Send(&user.ID, "telegram", model.EventOrderPaid, title, content, nil)
	}

	return nil
}

// NotifyNodeOffline 节点离线通知 (管理员)
func (s *NotificationService) NotifyNodeOffline(node *model.Node) error {
	title := "节点离线告警"
	content := fmt.Sprintf("节点 [%s] 已离线，请及时检查。", node.Name)

	// 发送给所有管理员
	var admins []model.User
	s.db.Where("is_admin = ?", true).Find(&admins)

	for _, admin := range admins {
		go s.Send(&admin.ID, "email", model.EventNodeOffline, title, content, map[string]interface{}{
			"node_id":   node.ID,
			"node_name": node.Name,
		})

		var tgUser model.TelegramUser
		if s.db.Where("user_id = ?", admin.ID).First(&tgUser).Error == nil {
			go s.Send(&admin.ID, "telegram", model.EventNodeOffline, title, content, nil)
		}
	}

	return nil
}

// Broadcast 广播通知 (所有用户)
func (s *NotificationService) Broadcast(title, content string) error {
	// 分批发送，避免内存溢出
	batchSize := 100
	offset := 0

	for {
		var users []model.User
		result := s.db.Where("banned = ?", false).Offset(offset).Limit(batchSize).Find(&users)
		if result.RowsAffected == 0 {
			break
		}

		for _, user := range users {
			go s.Send(&user.ID, "email", model.EventSystemBroadcast, title, content, nil)
		}

		offset += batchSize
	}

	return nil
}

// ========== 模板管理 ==========

// GetTemplate 获取通知模板
func (s *NotificationService) GetTemplate(notifyType, event string) (*model.NotificationTemplate, error) {
	var tmpl model.NotificationTemplate
	err := s.db.Where("type = ? AND event = ? AND enabled = ?", notifyType, event, true).First(&tmpl).Error
	if err != nil {
		return nil, err
	}
	return &tmpl, nil
}

// CreateTemplate 创建模板
func (s *NotificationService) CreateTemplate(tmpl *model.NotificationTemplate) error {
	return s.db.Create(tmpl).Error
}

// UpdateTemplate 更新模板
func (s *NotificationService) UpdateTemplate(tmpl *model.NotificationTemplate) error {
	return s.db.Save(tmpl).Error
}

// DeleteTemplate 删除模板
func (s *NotificationService) DeleteTemplate(id uint) error {
	return s.db.Delete(&model.NotificationTemplate{}, id).Error
}

// ListTemplates 获取模板列表
func (s *NotificationService) ListTemplates() ([]*model.NotificationTemplate, error) {
	var templates []*model.NotificationTemplate
	err := s.db.Order("type, event").Find(&templates).Error
	return templates, err
}

// GetLogs 获取通知日志
func (s *NotificationService) GetLogs(page, pageSize int, notifyType string) ([]*model.NotificationLog, int64, error) {
	var logs []*model.NotificationLog
	var total int64

	query := s.db.Model(&model.NotificationLog{})
	if notifyType != "" {
		query = query.Where("type = ?", notifyType)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	err := query.Order("id DESC").Offset(offset).Limit(pageSize).Find(&logs).Error
	return logs, total, err
}