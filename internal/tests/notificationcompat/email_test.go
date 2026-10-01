package notificationcompat

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/internal/handler"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/AnixOps/anix-control/v4/internal/tests/packagecompat"
	"github.com/AnixOps/anix-control/v4/packages/notification/native"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// notificationHost is the identity the kernel serves the notification host
// as; its signed release declares the mail namespace's read, write and
// secrets capabilities.
var notificationHost = packagebridge.HostIdentity{PackageID: "notification", Version: "4.1.0", Generation: 1}

// mailRoute runs a route whose native side reads and writes the e-mail
// configuration through the real KernelSettings server on its database.
func mailRoute(t *testing.T, method, pattern, routeID string, legacy gin.HandlerFunc) packagecompat.Route {
	return packagecompat.Route{
		Method: method, Pattern: pattern, RouteID: routeID, Legacy: legacy,
		Models: []any{&model.SystemConfig{}, &model.OperationLog{}, &model.SettingsRequest{}, &model.User{}},
		Native: func(db *gorm.DB) pluginhostsdk.NativeHandler {
			grants := packagecompat.SettingsGrants{Host: notificationHost}
			for _, access := range []string{service.SettingsAccessRead, service.SettingsAccessWrite, service.SettingsAccessSecrets} {
				grants.Capabilities = append(grants.Capabilities, service.SettingsCapability(service.SettingsNamespaceMail, access))
			}
			service := &native.Service{
				Open:     func(ctx context.Context) (*gorm.DB, error) { return db.WithContext(ctx), nil },
				Settings: packagecompat.KernelSettings(t, db, grants),
			}
			return service.Handlers()[routeID]
		},
	}
}

// seedAdmin writes the administrator the requests run as: the audit
// entries name them by e-mail.
func seedAdmin(t testing.TB, db *gorm.DB) {
	require.NoError(t, db.Create(&model.User{ID: 1, Email: "admin@example.test", Token: "t1", UUID: "u1", IsAdmin: 1}).Error)
}

// emailConfig seeds the e-mail configuration row with value, and a row of
// another namespace that must stay as it is.
func emailConfig(value string) func(testing.TB, *gorm.DB) {
	return func(t testing.TB, db *gorm.DB) {
		seedAdmin(t, db)
		require.NoError(t, db.Create(&[]model.SystemConfig{
			{ID: 1, Key: "notification.email.config", Value: value, Type: "json", Group: "notification", Remark: "Email notification config", CreatedAt: seeded, UpdatedAt: seeded},
			{ID: 2, Key: "site.name", Value: "Anix", Type: "string", Group: "site", CreatedAt: seeded, UpdatedAt: seeded},
		}).Error)
		syncConfigSequence(t, db)
	}
}

// otherSettings seeds only the unrelated row.
func otherSettings(t testing.TB, db *gorm.DB) {
	seedAdmin(t, db)
	require.NoError(t, db.Create(&model.SystemConfig{ID: 2, Key: "site.name", Value: "Anix", Type: "string", Group: "site", CreatedAt: seeded, UpdatedAt: seeded}).Error)
	syncConfigSequence(t, db)
}

func syncConfigSequence(t testing.TB, db *gorm.DB) {
	if db.Name() == "postgres" {
		require.NoError(t, db.Exec("SELECT setval(pg_get_serial_sequence('v2_system_config', 'id'), COALESCE((SELECT MAX(id) FROM v2_system_config), 0) + 1, false)").Error)
	}
}

// settingsState is v2_system_config and the system audit trail after the
// request, without the times taken from the clock. An audit entry never
// holds the SMTP password.
func settingsState(t testing.TB, db *gorm.DB) any {
	var configs []struct {
		ID     uint
		Key    string
		Value  string
		Type   string
		Group  string
		Remark string
	}
	require.NoError(t, db.Model(&model.SystemConfig{}).Order("id").Find(&configs).Error)
	var audit []struct {
		ID         uint
		UserID     *uint
		Username   string
		Action     string
		Module     string
		TargetType string
		TargetID   *uint
		Content    string
		IP         string
		UserAgent  string
		Status     int
	}
	require.NoError(t, db.Model(&model.OperationLog{}).Order("id").Find(&audit).Error)
	for _, entry := range audit {
		for _, secret := range []string{"smtp-secret", "rotated", "p<w>&d"} {
			require.NotContains(t, entry.Content, secret)
		}
	}
	return map[string]any{"configs": configs, "audit": audit}
}

const storedEmail = `{"host":"smtp.example.test","port":465,"username":"mailer","password":"smtp-secret","from_name":"Anix Mail","from_address":"noreply@example.test","encryption":"ssl"}`

func mailCases(t *testing.T, r packagecompat.Route, cases []packagecompat.Case) {
	for _, c := range cases {
		c.Principal = admin
		c.Snapshot = settingsState
		packagecompat.RunWrite(t, r, c)
	}
}

func TestEmailConfigReadRouteParity(t *testing.T) {
	path := "/api/v2/admin/notification/email/config"
	mailCases(t, mailRoute(t, "GET", path, native.EmailConfigGetRouteID, notifications((*handler.NotificationHandler).GetEmailConfig)), []packagecompat.Case{
		{Name: "stored configuration, password masked", Path: path, Seed: emailConfig(storedEmail)},
		{Name: "a blank password reads empty", Path: path, Seed: emailConfig(`{"host":"h","password":"   "}`)},
		{Name: "a stored placeholder reads masked", Path: path, Seed: emailConfig(`{"host":"h","password":"********"}`)},
		{Name: "no configuration: the defaults", Path: path, Seed: otherSettings},
		{Name: "a blank value: the defaults", Path: path, Seed: emailConfig("   ")},
		{Name: "encryption_type wins over encryption", Path: path, Seed: emailConfig(`{"host":" h ","port":"25","encryption":"ssl","encryption_type":false}`)},
		{Name: "an invalid encryption_type falls back to encryption", Path: path, Seed: emailConfig(`{"encryption_type":"rot13","encryption":1,"port":-3,"from_name":"  "}`)},
		{Name: "fields of another type", Path: path, Seed: emailConfig(`{"host":5,"port":true,"username":null,"password":7,"from_address":["x"]}`)},
		{Name: "a value that is not JSON", Path: path, Seed: emailConfig(`{"host":`)},
		{Name: "a JSON value that is not an object", Path: path, Seed: emailConfig(`[1,2]`)},
	})
}

func TestEmailConfigUpdateRouteParity(t *testing.T) {
	path := "/api/v2/admin/notification/email/config"
	mailCases(t, mailRoute(t, "PUT", path, native.EmailConfigPutRouteID, notifications((*handler.NotificationHandler).UpdateEmailConfig)), []packagecompat.Case{
		{Name: "a new configuration", Path: path, Seed: otherSettings, Body: []byte(`{"host":" smtp.new.test ","port":587,"username":"u","password":"p",
			"from_address":"a@new.test","from_name":"New","encryption_type":"starttls"}`)},
		{Name: "a blank password keeps the stored one", Path: path, Seed: emailConfig(storedEmail), Body: []byte(`{"host":"smtp.example.test","port":"2525","password":"  ","from_address":"b@example.test"}`)},
		{Name: "no password, no sender name, no encryption: the stored ones", Path: path, Seed: emailConfig(storedEmail), Body: []byte(`{"host":"h","port":25,"from_address":"c@example.test"}`)},
		{Name: "the placeholder keeps the stored password", Path: path, Seed: emailConfig(storedEmail), Body: []byte(`{"host":"h","port":25,"password":"********","from_address":"c@example.test"}`)},
		{Name: "a new password replaces the stored one", Path: path, Seed: emailConfig(storedEmail), Body: []byte(`{"host":"h","port":25,"password":"rotated <&>","from_address":"c@example.test"}`)},
		{Name: "the placeholder with no stored password stores none", Path: path, Seed: otherSettings, Body: []byte(`{"host":"h","port":25,"password":"********","from_address":"c@example.test"}`)},
		{Name: "a kept password is stored as the handler writes it", Path: path, Seed: emailConfig(`{"from_address":"x@example.test", "password" : "p<w>&d"}`), Body: []byte(`{"host":"h","port":25,"from_address":"c@example.test"}`)},
		{Name: "a password of another type is no password", Path: path, Seed: emailConfig(`{"password":7}`), Body: []byte(`{"host":"h","port":25,"password":"********","from_address":"c@example.test"}`)},
		{Name: "a blank stored password is kept blank", Path: path, Seed: emailConfig(`{"password":"  "}`), Body: []byte(`{"host":"h","port":25,"from_address":"c@example.test"}`)},
		{Name: "the request's user agent in the audit", Path: path, Seed: emailConfig(storedEmail), Body: []byte(`{"host":"h","port":25,"from_address":"c@example.test"}`),
			RequestHeaders: map[string]string{"User-Agent": "parity-agent/1.0"}},
		{Name: "an unknown administrator is audited without a username", Path: path, Body: []byte(`{"host":"h","port":25,"password":"p","from_address":"c@example.test"}`)},
		{Name: "encryption as a boolean", Path: path, Seed: emailConfig(storedEmail), Body: []byte(`{"host":"h","port":25,"from_address":"c@example.test","encryption":false}`)},
		{Name: "an HTML-looking sender name", Path: path, Seed: otherSettings, Body: []byte(`{"host":"h","port":25,"from_address":"d@example.test","from_name":"<Anix & Co>"}`)},
		{Name: "an invalid encryption_type", Path: path, Seed: emailConfig(storedEmail), Body: []byte(`{"host":"h","port":25,"from_address":"e@example.test","encryption_type":"rot13"}`)},
		{Name: "an invalid encryption", Path: path, Seed: emailConfig(storedEmail), Body: []byte(`{"host":"h","port":25,"from_address":"e@example.test","encryption":2}`)},
		{Name: "no host", Path: path, Seed: emailConfig(storedEmail), Body: []byte(`{"port":25,"from_address":"f@example.test"}`)},
		{Name: "no port", Path: path, Seed: emailConfig(storedEmail), Body: []byte(`{"host":"h","port":"x","from_address":"f@example.test"}`)},
		{Name: "no sender address", Path: path, Seed: emailConfig(storedEmail), Body: []byte(`{"host":"h","port":25,"from_address":"  "}`)},
		{Name: "a stored value that is not JSON", Path: path, Seed: emailConfig(`nope`), Body: []byte(`{"host":"h","port":25,"from_address":"g@example.test"}`)},
		{Name: "a body that is not an object", Path: path, Seed: otherSettings, Body: []byte(`"text"`)},
		{Name: "a null body", Path: path, Seed: otherSettings, Body: []byte(`null`)},
	})
}

// smtpServer is a minimal SMTP server that accepts mail for every
// recipient but refused@example.test, and keeps what it receives.
type smtpServer struct {
	address string
	mu      sync.Mutex
	mail    []string
}

func newSMTPServer(t *testing.T) *smtpServer {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	server := &smtpServer{address: listener.Addr().String()}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go server.serve(conn)
		}
	}()
	return server
}

func (s *smtpServer) serve(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	reader := bufio.NewReader(conn)
	reply := func(line string) { _, _ = fmt.Fprintf(conn, "%s\r\n", line) }
	reply("220 fake ESMTP")
	var transcript []string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		command := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(command, "EHLO"):
			reply("250-fake")
			reply("250 AUTH PLAIN")
		case strings.HasPrefix(command, "AUTH PLAIN"):
			transcript = append(transcript, line)
			reply("235 2.7.0 accepted")
		case strings.HasPrefix(command, "MAIL FROM"):
			transcript = append(transcript, line)
			reply("250 ok")
		case strings.HasPrefix(command, "RCPT TO"):
			if strings.Contains(line, "refused@example.test") {
				reply("550 5.1.1 no such user")
				continue
			}
			transcript = append(transcript, line)
			reply("250 ok")
		case command == "DATA":
			reply("354 go ahead")
			var body []string
			for {
				data, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				if data == ".\r\n" {
					break
				}
				body = append(body, data)
			}
			transcript = append(transcript, strings.Join(body, ""))
			s.mu.Lock()
			s.mail = append(s.mail, strings.Join(transcript, "\n"))
			s.mu.Unlock()
			reply("250 queued")
		case command == "QUIT":
			reply("221 bye")
			return
		default:
			reply("250 ok")
		}
	}
}

// received returns and forgets the mail received so far.
func (s *smtpServer) received() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	mail := s.mail
	s.mail = nil
	return mail
}

func TestTestNotificationRouteParity(t *testing.T) {
	path := "/api/v2/admin/notification/test"
	server := newSMTPServer(t)
	host, port, err := net.SplitHostPort(server.address)
	require.NoError(t, err)
	closed, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	_, closedPort, err := net.SplitHostPort(closed.Addr().String())
	require.NoError(t, err)
	require.NoError(t, closed.Close())
	smtpConfig := func(port, encryption string) func(testing.TB, *gorm.DB) {
		return emailConfig(fmt.Sprintf(`{"host":%q,"port":%s,"username":"mailer","password":"smtp-secret","from_name":"Anix","from_address":"noreply@example.test","encryption":%q}`,
			host, port, encryption))
	}
	// Each side's state includes the mail the SMTP server received for it;
	// a delivered case must have delivered one.
	state := func(delivered int) func(testing.TB, *gorm.DB) any {
		return func(t testing.TB, db *gorm.DB) any {
			mail := server.received()
			require.Len(t, mail, delivered)
			return map[string]any{"settings": settingsState(t, db), "mail": mail}
		}
	}
	for _, c := range []packagecompat.Case{
		{Name: "an e-mail, delivered", Seed: smtpConfig(port, "none"), Body: []byte(`{"type":"email","to":"admin@example.test","title":"Hello","content":"<b>hi</b>"}`), Snapshot: state(1)},
		{Name: "an e-mail with the defaults and the recipient field", Seed: smtpConfig(port, "none"), Body: []byte(`{"type":"email","recipient":"ops@example.test","subject":"S"}`), Snapshot: state(1)},
		{Name: "a refused recipient", Seed: smtpConfig(port, "none"), Body: []byte(`{"type":"email","to":"refused@example.test"}`)},
		{Name: "TLS to a plain server", Seed: smtpConfig(port, "ssl"), Body: []byte(`{"type":"email","to":"admin@example.test"}`)},
		{Name: "nobody listening", Seed: smtpConfig(closedPort, "none"), Body: []byte(`{"type":"email","to":"admin@example.test"}`)},
		{Name: "no SMTP host: not configured", Seed: emailConfig(`{"from_address":"noreply@example.test"}`), Body: []byte(`{"type":"email","to":"admin@example.test"}`)},
		{Name: "no configuration at all", Seed: otherSettings, Body: []byte(`{"type":"email","to":"admin@example.test"}`)},
		{Name: "a stored value that is not JSON", Seed: emailConfig(`{`), Body: []byte(`{"type":"email","to":"admin@example.test"}`)},
		{Name: "no recipient", Seed: smtpConfig(port, "none"), Body: []byte(`{"type":"email"}`)},
		{Name: "the Telegram stub", Seed: otherSettings, Body: []byte(`{"type":"telegram"}`)},
		{Name: "the webhook stub", Seed: otherSettings, Body: []byte(`{"type":"webhook","to":"x"}`)},
		{Name: "an unknown type", Seed: otherSettings, Body: []byte(`{"type":"sms"}`)},
		{Name: "no type", Seed: otherSettings, Body: []byte(`{"to":"admin@example.test"}`)},
		{Name: "a field of another type", Seed: otherSettings, Body: []byte(`{"type":"email","to":5}`)},
		{Name: "a body that is not an object", Seed: otherSettings, Body: []byte(`[1]`)},
	} {
		c.Path = path
		c.Principal = admin
		if c.Snapshot == nil {
			c.Snapshot = state(0)
		}
		packagecompat.RunWrite(t, mailRoute(t, "POST", path, native.TestSendRouteID, notifications((*handler.NotificationHandler).SendTestNotification)), c)
	}
}
