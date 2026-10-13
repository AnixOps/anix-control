package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/handler"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/nodegroup"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"gorm.io/gorm"
)

// joinNotifyTimeout bounds the delivery of one join notification.
const joinNotifyTimeout = time.Minute

// adminJoinNotifier tells the administrators of every machine that joined a
// node group (owner decision D4): an in-app entry for every administrator,
// e-mail when the notification e-mail settings are configured, and Telegram
// for administrators who bound an account, as the alert digests are
// delivered. The join server calls it after the join committed, in its own
// goroutine, so a slow mail server never holds up a join.
type adminJoinNotifier struct {
	db  *gorm.DB
	cfg *config.Config
}

func (n adminJoinNotifier) Notify(joined nodegroup.Joined) {
	if n.db == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), joinNotifyTimeout)
	defer cancel()
	notifications := service.NewNotificationService(n.db.WithContext(ctx), n.cfg)
	if email, err := handler.LoadNotificationEmailConfig(n.db); err == nil && email != nil {
		notifications.SetEmailConfig(email)
	}
	title, content, data := joined.Notification()
	if err := notifications.NotifyAdministrators(model.EventNodeJoined, title, content, data); err != nil {
		slog.Warn("node group: the join notification could not be sent", "component", "node-group", "node", joined.Node.String(), "error", err)
	}
	notifications.Drain()
}
