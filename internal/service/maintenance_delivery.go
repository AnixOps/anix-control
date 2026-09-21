package service

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// MaintenanceSender is injectable: tests never contact real recipients. External
// transports are at-least-once; a crash after send/before commit can redeliver.
type MaintenanceSender interface {
	Send(context.Context, model.MaintenanceDelivery) error
}
type MaintenanceTransportConfig struct {
	SMTPHost      string
	SMTPPort      int
	SMTPFrom      string
	SMTPUsername  string
	SMTPPassword  string
	TelegramToken string
}

type MaintenanceTransport struct {
	config          MaintenanceTransportConfig
	httpClient      *http.Client
	telegramAPIBase string
}

func NewMaintenanceTransport(config MaintenanceTransportConfig) MaintenanceTransport {
	return MaintenanceTransport{
		config: config,
		httpClient: &http.Client{
			Timeout:       15 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		telegramAPIBase: "https://api.telegram.org",
	}
}

func (t MaintenanceTransport) Send(ctx context.Context, d model.MaintenanceDelivery) error {
	if d.Destination == "" {
		return errors.New("channel_unconfigured")
	}
	switch d.Channel {
	case "telegram":
		token := strings.TrimSpace(t.config.TelegramToken)
		if token == "" {
			return errors.New("channel_unconfigured")
		}
		body, _ := json.Marshal(map[string]string{"chat_id": d.Destination, "text": d.Body})
		// #nosec G704 -- only the path contains the deployment secret; scheme and host are fixed to Telegram.
		baseURL := strings.TrimRight(t.telegramAPIBase, "/")
		if baseURL == "" {
			baseURL = "https://api.telegram.org"
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/bot"+token+"/sendMessage", bytes.NewReader(body))
		if err != nil {
			return errors.New("delivery_failed")
		}
		req.Header.Set("Content-Type", "application/json")
		client := t.httpClient
		if client == nil {
			client = NewMaintenanceTransport(t.config).httpClient
		}
		// #nosec G704 -- req always targets the fixed Telegram HTTPS host above and redirects are disabled.
		resp, err := client.Do(req)
		if err != nil {
			return errors.New("delivery_failed")
		}
		defer func() { _ = resp.Body.Close() }()
		var result struct {
			OK bool `json:"ok"`
		}
		err = json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&result)
		if err != nil || resp.StatusCode != http.StatusOK || !result.OK {
			return errors.New("delivery_failed")
		}
		return nil
	case "email":
		return sendMaintenanceEmail(ctx, t.config, d)
	default:
		return errors.New("invalid_channel")
	}
}
func sendMaintenanceEmail(ctx context.Context, config MaintenanceTransportConfig, d model.MaintenanceDelivery) error {
	host := strings.TrimSpace(config.SMTPHost)
	port := config.SMTPPort
	if port == 0 {
		port = 587
	}
	from := strings.TrimSpace(config.SMTPFrom)
	if host == "" || from == "" {
		return errors.New("channel_unconfigured")
	}
	for _, address := range []string{from, d.Destination} {
		parsed, err := mail.ParseAddress(address)
		if err != nil || parsed.Address != address || strings.ContainsAny(address, "\r\n") {
			return errors.New("invalid_address")
		}
	}
	connection, err := (&net.Dialer{Timeout: 15 * time.Second}).DialContext(ctx, "tcp", net.JoinHostPort(host, fmt.Sprint(port)))
	if err != nil {
		return errors.New("delivery_failed")
	}
	defer func() { _ = connection.Close() }()
	deadline := time.Now().Add(15 * time.Second)
	if end, ok := ctx.Deadline(); ok && end.Before(deadline) {
		deadline = end
	}
	if err := connection.SetDeadline(deadline); err != nil {
		return errors.New("delivery_failed")
	}
	stop := context.AfterFunc(ctx, func() { _ = connection.Close() })
	defer stop()
	client, err := smtp.NewClient(connection, host)
	if err != nil {
		return errors.New("delivery_failed")
	}
	defer func() { _ = client.Close() }()
	// Always require authenticated server TLS before credentials or message data.
	if ok, _ := client.Extension("STARTTLS"); !ok {
		return errors.New("smtp_tls_required")
	}
	if err := client.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
		return errors.New("delivery_failed")
	}
	username, password := config.SMTPUsername, config.SMTPPassword
	if username != "" {
		if err := client.Auth(smtp.PlainAuth("", username, password, host)); err != nil {
			return errors.New("delivery_failed")
		}
	}
	// #nosec G707 -- mail.ParseAddress exact-match and CR/LF rejection above prevent SMTP command injection.
	if err := client.Mail(from); err != nil {
		return errors.New("delivery_failed")
	}
	if err := client.Rcpt(d.Destination); err != nil {
		return errors.New("delivery_failed")
	}
	writer, err := client.Data()
	if err != nil {
		return errors.New("delivery_failed")
	}
	_, err = fmt.Fprintf(writer, "From: %s\r\nTo: %s\r\nSubject: AnixOps operations #%d\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s\r\n", from, d.Destination, d.TicketID, strings.ReplaceAll(d.Body, "\n", "\r\n"))
	if err != nil {
		return errors.New("delivery_failed")
	}
	if err := writer.Close(); err != nil {
		return errors.New("delivery_failed")
	}
	return nil
}

func DeliverMaintenanceNotifications(ctx context.Context, db *gorm.DB, sender MaintenanceSender, now time.Time, limit int) (int, error) {
	if sender == nil {
		return 0, errors.New("notification sender unavailable")
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	// Expired worker claims are retried after restart; committed sent rows are never
	// claimed again, even by a second Control process.
	if err := db.WithContext(ctx).Model(&model.MaintenanceDelivery{}).Where("status = ? AND lease_until <= ?", "sending", now).Updates(map[string]any{"status": "pending", "lease_owner": "", "lease_until": nil}).Error; err != nil {
		return 0, err
	}
	var deliveries []model.MaintenanceDelivery
	if err := db.WithContext(ctx).Where("status = ? AND next_attempt_at <= ?", "pending", now).Order("id").Limit(limit).Find(&deliveries).Error; err != nil {
		return 0, err
	}
	sent := 0
	var errs []error
	for _, d := range deliveries {
		if ctx.Err() != nil {
			return sent, ctx.Err()
		}
		owner := uuid.NewString()
		claimTime := time.Now()
		if now.After(claimTime) {
			claimTime = now
		}
		lease := claimTime.Add(time.Minute)
		claim := db.WithContext(ctx).Model(&model.MaintenanceDelivery{}).Where("id = ? AND status = ? AND next_attempt_at <= ?", d.ID, "pending", now).Updates(map[string]any{"status": "sending", "lease_owner": owner, "lease_until": lease, "attempts": gorm.Expr("attempts + 1")})
		if claim.Error != nil {
			errs = append(errs, claim.Error)
			continue
		}
		if claim.RowsAffected == 0 {
			continue
		}
		settings, err := GetMaintenanceSettings(db.WithContext(ctx))
		cancel := err == nil && (settings.Revision != d.SettingsRevision || (d.Reason != "verification" && !settings.Enabled))
		if err == nil && !cancel && d.TicketID != 0 && d.Reason != "recovered" {
			var ticket model.MaintenanceTicket
			err = db.WithContext(ctx).First(&ticket, d.TicketID).Error
			if err == nil && ticket.Status != "open" {
				cancel = true
			}
		}
		if cancel {
			result := db.WithContext(ctx).Model(&model.MaintenanceDelivery{}).Where("id = ? AND lease_owner = ?", d.ID, owner).Updates(map[string]any{"status": "cancelled", "body": "", "lease_owner": "", "lease_until": nil})
			if result.Error != nil {
				errs = append(errs, result.Error)
			}
			continue
		}
		if err == nil {
			sendCtx, stop := context.WithTimeout(ctx, 15*time.Second)
			err = sender.Send(sendCtx, d)
			stop()
		}
		updates := map[string]any{"lease_owner": "", "lease_until": nil}
		if err != nil {
			updates["status"] = "pending"
			updates["last_error"] = "delivery_failed"
			updates["next_attempt_at"] = now.Add(time.Second * time.Duration(min(30*(1<<min(d.Attempts, 6)), 1800)))
		} else {
			updates["status"] = "sent"
			updates["last_error"] = ""
			updates["sent_at"] = now
			updates["body"] = ""
			sent++
		}
		// Even if cancellation interrupted the send, release the claim with a short
		// independent DB deadline so graceful shutdown doesn't strand it indefinitely.
		finishCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		result := db.WithContext(finishCtx).Model(&model.MaintenanceDelivery{}).Where("id = ? AND lease_owner = ? AND status = ?", d.ID, owner, "sending").Updates(updates)
		stop()
		if result.Error != nil {
			errs = append(errs, result.Error)
		}
	}
	return sent, errors.Join(errs...)
}
