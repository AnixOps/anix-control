package iwanline

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/nodeentry"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newService(t *testing.T) *Service {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "lines.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&model.Node{}, &model.NodeProtocol{}, &model.NodeEntry{}, &model.IWANLine{}, &model.OperationLog{}))
	require.NoError(t, db.Create(&model.Node{ID: 70, Name: "us land", Host: "198.51.100.20"}).Error)
	return New(db)
}

func hongKong() Input {
	return Input{Name: "HK iWAN", Host: "[2001:DB8::1]", Port: 4567, Username: "shared", Password: "secret", MTU: 1400}
}

func TestCreateUpdateKeepsThePasswordOutOfAnswersAndAudit(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	view, err := s.Create(ctx, hongKong(), Actor{UserID: 1, Name: "admin"})
	require.NoError(t, err)
	require.Equal(t, "2001:db8::1", view.Host)
	require.Equal(t, 1400, view.MTU)
	require.False(t, view.Encrypt)

	// An empty password keeps the stored one.
	update := hongKong()
	update.Password = ""
	update.Encrypt = true
	view, err = s.Update(ctx, view.ID, update, Actor{})
	require.NoError(t, err)
	require.True(t, view.Encrypt)
	lines, err := ByID(s.DB, []uint{view.ID})
	require.NoError(t, err)
	require.Equal(t, "secret", lines[view.ID].Password)

	var logs []model.OperationLog
	require.NoError(t, s.DB.Order("id").Find(&logs).Error)
	require.Len(t, logs, 2)
	require.Equal(t, AuditActionCreate, logs[0].Action)
	for _, log := range logs {
		require.NotContains(t, log.Content, "secret")
		require.NotContains(t, log.Content, "shared")
	}
}

func TestValidation(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	for _, change := range []func(*Input){
		func(in *Input) { in.Name = "a|b" },
		func(in *Input) { in.Host = "not a host" },
		func(in *Input) { in.Port = 0 },
		func(in *Input) { in.Username = " " },
		func(in *Input) { in.Password = "" },
		func(in *Input) { in.MTU = 100 },
	} {
		input := hongKong()
		change(&input)
		_, err := s.Create(ctx, input, Actor{})
		require.ErrorIs(t, err, ErrInvalid)
	}
	_, err := s.Create(ctx, hongKong(), Actor{})
	require.NoError(t, err)
	_, err = s.Create(ctx, hongKong(), Actor{})
	require.ErrorIs(t, err, ErrInvalid, "names are unique")
}

func TestALineInUseCannotBeDeleted(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	view, err := s.Create(ctx, hongKong(), Actor{})
	require.NoError(t, err)

	entries := nodeentry.New(s.DB)
	missing := uint(999)
	_, err = entries.Replace(ctx, 70, []nodeentry.Input{{Label: "via HK", Kind: model.NodeEntryIWAN, Host: "198.51.100.20", IWANLineID: &missing}}, nodeentry.Actor{})
	require.ErrorIs(t, err, nodeentry.ErrLineNotFound)
	_, err = entries.Replace(ctx, 70, []nodeentry.Input{{Label: "via HK", Kind: model.NodeEntryIWAN, Host: "198.51.100.20"}}, nodeentry.Actor{})
	require.ErrorIs(t, err, nodeentry.ErrInvalid, "an iWAN entry needs its line")
	saved, err := entries.Replace(ctx, 70, []nodeentry.Input{{Label: "via HK", Kind: model.NodeEntryIWAN, Host: "198.51.100.20", IWANLineID: &view.ID}}, nodeentry.Actor{})
	require.NoError(t, err)
	require.Equal(t, &view.ID, saved[0].IWANLineID)

	lines, err := s.List(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, lines[0].Entries)
	require.True(t, errors.Is(s.Delete(ctx, view.ID, Actor{}), ErrInUse))

	_, err = entries.Replace(ctx, 70, nil, nodeentry.Actor{})
	require.NoError(t, err)
	require.NoError(t, s.Delete(ctx, view.ID, Actor{}))
	require.ErrorIs(t, s.Delete(ctx, view.ID, Actor{}), ErrNotFound)
}
