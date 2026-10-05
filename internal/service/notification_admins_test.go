package service

import (
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// phaseMessage is a real phase alert: table names, settings and a command in
// backticks, none of which legacy Telegram Markdown accepts unpaired.
const phaseMessage = "warning: The node credential split of table v2_authorized_key is in phase dual_write. " +
	"Verify and finalize it (`anix-control node-secrets finalize -confirm v2_authorized_key`), or set alerts.phase_stuck_after to 0. [see *docs*]"

func TestTelegramMarkdownSafe(t *testing.T) {
	safe := telegramMarkdownSafe(phaseMessage)
	for _, forbidden := range []string{"_", "*", "`", "[", "]"} {
		assert.NotContains(t, safe, forbidden)
	}
	assert.Contains(t, safe, "v2-authorized-key", "readable after the replacement")
	assert.Contains(t, safe, "alerts.phase-stuck-after")
	assert.Contains(t, safe, "(see -docs-)")
	assert.Equal(t, "plain text, 1.5 and (x)\nnext line", telegramMarkdownSafe("plain text, 1.5 and (x)\nnext line"), "everything else is kept, newlines included")
	assert.Equal(t, "", telegramMarkdownSafe(""))
}

func (s *NotificationServiceTestSuite) TestNotifyAdministratorsReachesUnbannedAdministratorsOnly() {
	db := database.Get()
	newUser := func(email string, admin, banned int) model.User {
		user := model.User{Email: email, Password: "hash", Token: "tok-" + email, UUID: "uuid-" + email, TransferEnable: 1, IsAdmin: admin, Banned: banned}
		require.NoError(s.T(), db.Create(&user).Error)
		return user
	}
	admin := newUser("admin@example.com", 1, 0)
	bound := newUser("bound@example.com", 1, 0)
	banned := newUser("banned@example.com", 1, 1)
	require.NoError(s.T(), db.Create(&model.TelegramUser{UserID: bound.ID, TelegramID: 4242}).Error)

	require.NoError(s.T(), s.svc.NotifyAdministrators(model.EventSystemAlert, "AnixOps alert: 1 need attention", phaseMessage, map[string]any{"alerts": 1}))
	s.svc.Drain()

	var logs []model.NotificationLog
	require.NoError(s.T(), db.Where("event = ?", model.EventSystemAlert).Order("id").Find(&logs).Error)
	byUser := map[uint][]string{}
	for _, log := range logs {
		require.NotNil(s.T(), log.UserID)
		byUser[*log.UserID] = append(byUser[*log.UserID], log.Type)
		if log.Type == "telegram" {
			assert.Equal(s.T(), telegramMarkdownSafe(phaseMessage), log.Content, "Telegram gets the Markdown-safe text")
		} else {
			assert.Equal(s.T(), phaseMessage, log.Content, "in-app and e-mail keep the exact text")
		}
	}
	assert.ElementsMatch(s.T(), []string{"inapp"}, byUser[admin.ID], "every administrator gets the in-app entry, e-mail only when configured")
	assert.ElementsMatch(s.T(), []string{"inapp", "telegram"}, byUser[bound.ID], "a bound administrator is also told by Telegram")
	assert.Empty(s.T(), byUser[banned.ID], "a banned administrator is not told")
	assert.Empty(s.T(), byUser[s.testUser.ID], "a member is not told")
	for _, log := range logs {
		if log.Type == "inapp" {
			assert.Equal(s.T(), 1, log.Status, "the in-app entry counts as sent")
		}
	}

	// With e-mail settings, administrators are also e-mailed (the send itself
	// fails without an SMTP server, which the log records).
	s.svc.SetEmailConfig(&model.EmailConfig{Host: "127.0.0.1", Port: 1, FromAddress: "a@example.com", FromName: "A", Encryption: "none"})
	require.NoError(s.T(), s.svc.NotifyAdministrators(model.EventSystemAlert, "second", "again", nil))
	s.svc.Drain()
	var emails int64
	require.NoError(s.T(), db.Model(&model.NotificationLog{}).Where("event = ? AND type = ? AND title = ?", model.EventSystemAlert, "email", "second").Count(&emails).Error)
	assert.EqualValues(s.T(), 2, emails)
}
