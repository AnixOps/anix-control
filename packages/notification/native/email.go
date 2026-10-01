package native

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/smtp"
	"strconv"
	"strings"

	kernelsettingsv1 "github.com/AnixOps/anix-control/sdk/api/kernelsettings/v1"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/gin-gonic/gin/binding"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

// Settings is the part of the kernel's KernelSettings contract the native
// routes call.
type Settings interface {
	GetSettings(ctx context.Context, in *kernelsettingsv1.GetSettingsRequest, opts ...grpc.CallOption) (*kernelsettingsv1.GetSettingsResponse, error)
	PutSettings(ctx context.Context, in *kernelsettingsv1.PutSettingsRequest, opts ...grpc.CallOption) (*kernelsettingsv1.PutSettingsResponse, error)
}

const (
	// mailNamespace is the KernelSettings namespace of the SMTP delivery
	// configuration.
	mailNamespace = "mail"
	// emailConfigKey holds the configuration as one JSON value, the SMTP
	// password included.
	emailConfigKey = "notification.email.config"
	// secretPlaceholder is the kernel's service.SensitiveSystemConfigPlaceholder:
	// administrators see it instead of the SMTP password, and a value sent
	// with it keeps the stored password.
	secretPlaceholder = "********"
)

// maskPassword is the SMTP password as an administrator's answer shows it:
// the placeholder when one is set, else "".
func maskPassword(password string) string {
	if strings.TrimSpace(password) == "" {
		return ""
	}
	return secretPlaceholder
}

// newPassword is the password an update sets: a string that is neither
// blank nor the placeholder. Anything else keeps the stored one.
func newPassword(req map[string]any) (string, bool) {
	password, ok := req["password"].(string)
	if !ok || strings.TrimSpace(password) == "" || password == secretPlaceholder {
		return "", false
	}
	return password, true
}

// Route ids of the routes that read or write the e-mail configuration.
const (
	EmailConfigGetRouteID = "notification.admin.notification.email.config.get"
	EmailConfigPutRouteID = "notification.admin.notification.email.config.put"
	TestSendRouteID       = "notification.admin.notification.test.post"
)

// EmailConfig is the kernel's model.EmailConfig: the stored JSON value has
// its fields in this order.
type EmailConfig struct {
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	FromName    string `json:"from_name"`
	FromAddress string `json:"from_address"`
	Encryption  string `json:"encryption"`
}

// EmailConfigRequestID names an e-mail configuration update in the kernel's
// settings request ledger, so a retried request is applied once. token
// identifies the HTTP request: its Idempotency-Key, else its request id,
// else a fresh value.
func EmailConfigRequestID(token string) string {
	sum := sha256.Sum256([]byte(token))
	return fmt.Sprintf("notification.email_config:%x", sum[:12])
}

// requestToken is what identifies the request for EmailConfigRequestID.
func (s *Service) requestToken(request pluginhostsdk.NativeRequest) string {
	for _, name := range []string{"Idempotency-Key", "X-Request-Id"} {
		for key, values := range request.Metadata.Headers {
			if strings.EqualFold(key, name) && len(values) > 0 {
				if value := strings.TrimSpace(values[0]); value != "" {
					return value
				}
			}
		}
	}
	if s.NewToken != nil {
		return s.NewToken()
	}
	return uuid.NewString()
}

func parseStringField(raw map[string]any, key string) string {
	v, ok := raw[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func parseIntField(raw map[string]any, key string) int {
	v, ok := raw[key]
	if !ok || v == nil {
		return 0
	}
	switch vv := v.(type) {
	case int:
		return vv
	case int32:
		return int(vv)
	case int64:
		return int(vv)
	case float64:
		return int(vv)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(vv))
		return n
	default:
		return 0
	}
}

func normalizeEmailEncryption(raw any) (string, bool) {
	switch v := raw.(type) {
	case bool:
		if v {
			return "tls", true
		}
		return "none", true
	case float64:
		if int(v) == 1 {
			return "tls", true
		}
		if int(v) == 0 {
			return "none", true
		}
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "", "none", "false", "0", "off", "no":
			return "none", true
		case "tls", "true", "1", "on", "yes", "starttls":
			return "tls", true
		case "ssl":
			return "ssl", true
		}
	}
	return "", false
}

func emailEncryptionBool(enc string) bool {
	return enc == "tls" || enc == "ssl"
}

// loadEmailConfig is the kernel's NotificationHandler.loadEmailConfig on
// the value KernelSettings answers, in clear (kernel.settings.mail.secrets.v1):
// the defaults, overlaid with the stored fields.
func (s *Service) loadEmailConfig(ctx context.Context) (*EmailConfig, error) {
	cfg := &EmailConfig{Port: 587, FromName: controlName, Encryption: "tls"}
	response, err := s.Settings.GetSettings(ctx, &kernelsettingsv1.GetSettingsRequest{Namespace: mailNamespace, Keys: []string{emailConfigKey}})
	if err != nil {
		return nil, errors.New(status.Convert(err).Message())
	}
	value := ""
	for _, setting := range response.GetSettings() {
		if setting.GetKey() == emailConfigKey {
			if setting.GetMasked() {
				return nil, errors.New("the e-mail configuration is masked: the package needs kernel.settings.mail.secrets.v1")
			}
			value = setting.GetValue()
		}
	}
	if strings.TrimSpace(value) == "" {
		return cfg, nil
	}

	var raw map[string]any
	if err := json.Unmarshal([]byte(value), &raw); err != nil {
		return nil, err
	}
	if host := parseStringField(raw, "host"); host != "" {
		cfg.Host = host
	}
	if port := parseIntField(raw, "port"); port > 0 {
		cfg.Port = port
	}
	if username := parseStringField(raw, "username"); username != "" {
		cfg.Username = username
	}
	if password, ok := raw["password"]; ok {
		if p, ok := password.(string); ok {
			cfg.Password = p
		}
	}
	if fromAddress := parseStringField(raw, "from_address"); fromAddress != "" {
		cfg.FromAddress = fromAddress
	}
	if fromName := parseStringField(raw, "from_name"); fromName != "" {
		cfg.FromName = fromName
	}
	if enc, ok := raw["encryption_type"]; ok {
		if normalized, valid := normalizeEmailEncryption(enc); valid {
			cfg.Encryption = normalized
			return cfg, nil
		}
	}
	if enc, ok := raw["encryption"]; ok {
		if normalized, valid := normalizeEmailEncryption(enc); valid {
			cfg.Encryption = normalized
		}
	}
	return cfg, nil
}

// AdminEmailConfig is GET /api/v2/admin/notification/email/config: the
// stored configuration over the defaults, with the SMTP password masked as
// the kernel's handler masks it.
func (s *Service) AdminEmailConfig(ctx context.Context, _ pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	cfg, err := s.loadEmailConfig(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	return s.panel(map[string]any{
		"host":            cfg.Host,
		"port":            cfg.Port,
		"username":        cfg.Username,
		"password":        maskPassword(cfg.Password),
		"from_address":    cfg.FromAddress,
		"from_name":       cfg.FromName,
		"encryption":      emailEncryptionBool(cfg.Encryption),
		"encryption_type": cfg.Encryption,
	})
}

// AdminUpdateEmailConfig is PUT /api/v2/admin/notification/email/config:
// host, port and sender address are required; a blank password or the
// placeholder keeps the stored one, and an absent encryption the stored
// one. The kernel writes the value through KernelSettings (namespace mail);
// a kept password is sent as the placeholder, which the kernel replaces
// with the stored one. The kernel records the system configuration audit
// entry its own handler records, in the same transaction.
func (s *Service) AdminUpdateEmailConfig(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	var req map[string]any
	if err := binding.JSON.BindBody(request.Body, &req); err != nil {
		return s.panelError(err.Error())
	}
	host := parseStringField(req, "host")
	if host == "" {
		return s.panelError("host is required")
	}
	port := parseIntField(req, "port")
	if port <= 0 {
		return s.panelError("port is required")
	}
	fromAddress := parseStringField(req, "from_address")
	if fromAddress == "" {
		return s.panelError("from_address is required")
	}
	existing, err := s.loadEmailConfig(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}

	cfg := &EmailConfig{
		Host: host, Port: port, Username: parseStringField(req, "username"),
		FromAddress: fromAddress, FromName: parseStringField(req, "from_name"), Encryption: existing.Encryption,
	}
	if cfg.FromName == "" {
		cfg.FromName = existing.FromName
	}
	if cfg.FromName == "" {
		cfg.FromName = controlName
	}
	if password, ok := newPassword(req); ok {
		cfg.Password = password
	} else if existing.Password != "" {
		// The kernel keeps the stored password.
		cfg.Password = secretPlaceholder
	}
	hasEncryption := false
	if encRaw, ok := req["encryption_type"]; ok {
		normalized, valid := normalizeEmailEncryption(encRaw)
		if !valid {
			return s.panelError("invalid encryption_type")
		}
		cfg.Encryption = normalized
		hasEncryption = true
	}
	if !hasEncryption {
		if encRaw, ok := req["encryption"]; ok {
			normalized, valid := normalizeEmailEncryption(encRaw)
			if !valid {
				return s.panelError("invalid encryption")
			}
			cfg.Encryption = normalized
		}
	}
	if cfg.Encryption == "" {
		cfg.Encryption = "none"
	}

	// The value holds a new password, as the kernel stores it, or the
	// placeholder for the stored one; KernelSettings keeps the key a secret.
	value, err := json.Marshal(cfg) // #nosec G117 -- the SMTP configuration is stored with its password, a declared KernelSettings secret.
	if err != nil {
		return s.panelError(err.Error())
	}
	userID := uint64(request.Principal.ActorID)
	if _, err := s.Settings.PutSettings(ctx, &kernelsettingsv1.PutSettingsRequest{
		Namespace: mailNamespace, RequestId: EmailConfigRequestID(s.requestToken(request)),
		Entries: []*kernelsettingsv1.SettingEntry{{
			Key: emailConfigKey, Value: string(value), Type: "json", Group: "notification", Remark: "Email notification config",
		}},
		Actor: &kernelsettingsv1.Actor{UserId: &userID, ClientIp: request.Metadata.ClientIP, UserAgent: request.Metadata.UserAgent},
	}); err != nil {
		return s.panelError(status.Convert(err).Message())
	}
	return s.panel(map[string]any{"message": "email config updated"})
}

// AdminTestNotification is POST /api/v2/admin/notification/test. An e-mail
// test is sent from the package host with the stored SMTP configuration,
// as the kernel's handler sends it; the Telegram and webhook tests are the
// kernel's stubs, which send nothing and succeed.
func (s *Service) AdminTestNotification(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	// The kernel handler's request type, field for field: binding errors
	// name it.
	var req struct {
		Type      string `json:"type" binding:"required"`
		To        string `json:"to"`
		Recipient string `json:"recipient"`
		Title     string `json:"title"`
		Subject   string `json:"subject"`
		Content   string `json:"content"`
	}
	if err := binding.JSON.BindBody(request.Body, &req); err != nil {
		return s.panelError(err.Error())
	}

	title := req.Title
	if title == "" {
		title = req.Subject
	}
	if title == "" {
		title = "Test Notification"
	}
	recipient := req.To
	if recipient == "" {
		recipient = req.Recipient
	}
	content := req.Content
	if content == "" {
		content = "This is a test notification from " + controlName + "."
	}

	var err error
	switch req.Type {
	case "email":
		if recipient == "" {
			return s.panelError("email address required")
		}
		cfg, cfgErr := s.loadEmailConfig(ctx)
		if cfgErr != nil {
			return s.panelError(cfgErr.Error())
		}
		if cfg.Host == "" || cfg.FromAddress == "" {
			cfg = nil
		}
		err = sendEmail(cfg, recipient, title, content)
	case "telegram":
		log.Printf("[STUB] test notification: Telegram delivery not yet implemented")
	case "webhook":
		log.Printf("[STUB] test notification: Webhook delivery not yet implemented")
	default:
		return s.panelError("invalid notification type")
	}
	if err != nil {
		return s.panelError(err.Error())
	}
	return s.panel(map[string]any{"message": "test notification sent", "success": true})
}

// sendEmail is the kernel's NotificationService.SendEmail: plain SMTP, or
// a TLS connection from the start for tls and ssl.
func sendEmail(cfg *EmailConfig, to, subject, body string) error {
	if cfg == nil {
		return fmt.Errorf("email not configured")
	}
	from := cfg.FromAddress
	msg := fmt.Sprintf("From: %s <%s>\r\n", cfg.FromName, from)
	msg += fmt.Sprintf("To: %s\r\n", to)
	msg += fmt.Sprintf("Subject: %s\r\n", subject)
	msg += "MIME-version: 1.0;\r\nContent-Type: text/html; charset=\"UTF-8\";\r\n\r\n"
	msg += body

	var auth smtp.Auth
	if cfg.Username != "" {
		auth = smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
	}
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	if cfg.Encryption == "ssl" || cfg.Encryption == "tls" {
		return sendEmailTLS(addr, auth, from, []string{to}, []byte(msg))
	}
	return smtp.SendMail(addr, auth, from, []string{to}, []byte(msg))
}

// sendEmailTLS is the kernel's NotificationService.sendEmailTLS.
func sendEmailTLS(addr string, auth smtp.Auth, from string, to []string, msg []byte) (err error) {
	host := strings.Split(addr, ":")[0]
	conn, err := tls.Dial("tcp", addr, &tls.Config{MinVersion: tls.VersionTLS12, ServerName: host})
	if err != nil {
		return err
	}
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return errors.Join(err, conn.Close())
	}
	defer func() {
		if closeErr := client.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close smtp client: %w", closeErr)
		}
	}()
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
	if _, err := w.Write(msg); err != nil {
		return errors.Join(err, w.Close())
	}
	return w.Close()
}
