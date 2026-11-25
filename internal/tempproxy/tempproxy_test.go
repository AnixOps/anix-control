package tempproxy

import (
	"context"
	"errors"
	"net/netip"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "tmp.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.NodeTemporaryProxy{}, &model.NodeProtocol{}))
	return db
}

func TestIsInternal(t *testing.T) {
	for _, address := range []string{"10.1.2.3", "172.16.0.1", "172.31.255.255", "192.168.1.1", "100.64.0.1", "100.127.255.254",
		"169.254.1.1", "127.0.0.1", "fd00::1", "fe80::1", "::1", "::ffff:10.0.0.1"} {
		require.True(t, IsInternal(netip.MustParseAddr(address)), address)
	}
	for _, address := range []string{"0.0.0.0", "::", "8.8.8.8", "172.32.0.1", "100.128.0.1", "2001:db8::1", "::ffff:8.8.8.8"} {
		require.False(t, IsInternal(netip.MustParseAddr(address)), address)
	}
}

func TestOpenRefusesInvalidRequests(t *testing.T) {
	db := openDB(t)
	now := time.Date(2026, 11, 25, 8, 0, 0, 0, time.UTC)
	for _, request := range []Request{
		{Listen: "0.0.0.0", Port: 1080},
		{Listen: "::", Port: 1080},
		{Listen: "203.0.113.5", Port: 1080},
		{Listen: "mesh.local", Port: 1080},
		{Listen: "fe80::1%eth0", Port: 1080},
		{Listen: "10.0.0.1", Port: 0},
		{Listen: "10.0.0.1", Port: 65536},
		{Listen: "10.0.0.1", Port: 1080, Duration: 30 * time.Second},
		{Listen: "10.0.0.1", Port: 1080, Duration: 25 * time.Hour},
	} {
		_, err := Open(context.Background(), db, 3, request, now, nil)
		require.ErrorIs(t, err, ErrInvalid, "%+v", request)
	}
	var count int64
	require.NoError(t, db.Model(&model.NodeTemporaryProxy{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestOpenListCloseAndExpiry(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 25, 8, 0, 0, 0, time.UTC)
	var notified []uint
	cancel := OnChange(func(nodeID uint) { notified = append(notified, nodeID) })
	defer cancel()
	var audited []uint
	audit := func(tx *gorm.DB, row model.NodeTemporaryProxy) error {
		audited = append(audited, row.ID)
		return nil
	}

	creator := uint(7)
	opened, err := Open(ctx, db, 3, Request{Listen: "::ffff:10.126.126.1", Port: 1080, CreatedBy: &creator}, now, audit)
	require.NoError(t, err)
	require.Equal(t, "10.126.126.1", opened.Listen)
	require.Equal(t, now.Add(DefaultDuration), opened.ExpiresAt)
	require.Len(t, opened.Password, passwordLength)
	require.True(t, strings.HasPrefix(opened.Username, "tmp-"))
	require.Len(t, opened.Username, len("tmp-")+8)
	require.Equal(t, &creator, opened.CreatedBy)
	require.Equal(t, []uint{3}, notified)
	require.Equal(t, []uint{opened.ID}, audited)

	// Each proxy gets its own credentials.
	second, err := Open(ctx, db, 3, Request{Listen: "10.126.126.1", Port: 1081, Duration: time.Hour}, now, nil)
	require.NoError(t, err)
	require.NotEqual(t, opened.Password, second.Password)
	require.NotEqual(t, opened.Username, second.Username)

	rows, err := List(ctx, db, 3, now)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, opened.ID, rows[0].ID, "the soonest to end first")
	other, err := List(ctx, db, 4, now)
	require.NoError(t, err)
	require.Empty(t, other)

	member, err := NodeMember(db, 3, now)
	require.NoError(t, err)
	require.Equal(t, []map[string]any{
		{"id": opened.ID, "listen": "10.126.126.1", "port": 1080, "username": opened.Username,
			"password": opened.Password, "expires_at_unix_ms": now.Add(DefaultDuration).UnixMilli()},
		{"id": second.ID, "listen": "10.126.126.1", "port": 1081, "username": second.Username,
			"password": second.Password, "expires_at_unix_ms": now.Add(time.Hour).UnixMilli()},
	}, member)
	member, err = NodeMember(db, 4, now)
	require.NoError(t, err)
	require.Nil(t, member)

	// Expired: neither listed nor in the configuration, and not closable.
	later := now.Add(DefaultDuration)
	rows, err = List(ctx, db, 3, later)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, second.ID, rows[0].ID)
	_, err = Close(ctx, db, 3, opened.ID, later, nil)
	require.ErrorIs(t, err, ErrNotFound)

	// Closed early: on another node it is not found.
	_, err = Close(ctx, db, 4, second.ID, now, nil)
	require.ErrorIs(t, err, ErrNotFound)
	notified = nil
	audited = nil
	closed, err := Close(ctx, db, 3, second.ID, now.Add(time.Minute), audit)
	require.NoError(t, err)
	require.NotNil(t, closed.ClosedAt)
	require.Equal(t, now.Add(time.Minute), closed.ClosedAt.UTC())
	require.Equal(t, []uint{3}, notified)
	require.Equal(t, []uint{second.ID}, audited)
	_, err = Close(ctx, db, 3, second.ID, now.Add(time.Minute), nil)
	require.ErrorIs(t, err, ErrNotFound)
	member, err = NodeMember(db, 3, now.Add(time.Minute))
	require.NoError(t, err)
	require.Len(t, member, 1)
	require.Equal(t, opened.ID, member[0]["id"])

	// The port of a proxy that ended is free again.
	_, err = Open(ctx, db, 3, Request{Listen: "10.126.126.1", Port: 1081}, now.Add(time.Minute), nil)
	require.NoError(t, err)
}

func TestOpenRefusesTakenPortsAndTooMany(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 25, 8, 0, 0, 0, time.UTC)

	_, err := Open(ctx, db, 3, Request{Listen: "10.0.0.1", Port: 1080}, now, nil)
	require.NoError(t, err)
	_, err = Open(ctx, db, 3, Request{Listen: "192.168.1.1", Port: 1080}, now, nil)
	require.ErrorIs(t, err, ErrPortTaken)
	// Another node may use the port.
	_, err = Open(ctx, db, 4, Request{Listen: "10.0.0.1", Port: 1080}, now, nil)
	require.NoError(t, err)

	// An enabled protocol of the node takes the port; a disabled one does not.
	require.NoError(t, db.Create(&model.NodeProtocol{NodeID: 3, Name: "ss", Type: "shadowsocks", Port: 443, Enable: 1}).Error)
	disabled := model.NodeProtocol{NodeID: 3, Name: "old", Type: "shadowsocks", Port: 8443, Enable: 1}
	require.NoError(t, db.Create(&disabled).Error)
	require.NoError(t, db.Model(&disabled).Update("enable", 0).Error)
	_, err = Open(ctx, db, 3, Request{Listen: "10.0.0.1", Port: 443}, now, nil)
	require.ErrorIs(t, err, ErrPortTaken)
	_, err = Open(ctx, db, 3, Request{Listen: "10.0.0.1", Port: 8443}, now, nil)
	require.NoError(t, err)

	for port := 2000; port < 2000+MaxOpenPerNode-2; port++ {
		_, err = Open(ctx, db, 3, Request{Listen: "10.0.0.1", Port: port}, now, nil)
		require.NoError(t, err)
	}
	_, err = Open(ctx, db, 3, Request{Listen: "10.0.0.1", Port: 3000}, now, nil)
	require.ErrorIs(t, err, ErrTooMany)
	// Expired ones do not count.
	_, err = Open(ctx, db, 3, Request{Listen: "10.0.0.1", Port: 3000}, now.Add(DefaultDuration), nil)
	require.NoError(t, err)
}

func TestAuditErrorRollsBack(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 25, 8, 0, 0, 0, time.UTC)
	var notified int
	cancel := OnChange(func(uint) { notified++ })
	defer cancel()
	failing := errors.New("audit failed")

	_, err := Open(ctx, db, 3, Request{Listen: "10.0.0.1", Port: 1080}, now, func(*gorm.DB, model.NodeTemporaryProxy) error { return failing })
	require.ErrorIs(t, err, failing)
	rows, err := List(ctx, db, 3, now)
	require.NoError(t, err)
	require.Empty(t, rows)

	opened, err := Open(ctx, db, 3, Request{Listen: "10.0.0.1", Port: 1080}, now, nil)
	require.NoError(t, err)
	notified = 0
	_, err = Close(ctx, db, 3, opened.ID, now, func(*gorm.DB, model.NodeTemporaryProxy) error { return failing })
	require.ErrorIs(t, err, failing)
	require.Zero(t, notified)
	rows, err = List(ctx, db, 3, now)
	require.NoError(t, err)
	require.Len(t, rows, 1)
}

func TestNodeMemberWithoutTable(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "empty.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	member, err := NodeMember(db, 3, time.Now())
	require.NoError(t, err)
	require.Nil(t, member)
}

func TestOnChangeCancel(t *testing.T) {
	var calls int
	cancel := OnChange(func(uint) { calls++ })
	notify(1)
	cancel()
	notify(1)
	require.Equal(t, 1, calls)
}

func TestRandomText(t *testing.T) {
	text, err := randomText(500)
	require.NoError(t, err)
	require.Len(t, text, 500)
	for _, r := range text {
		require.True(t, strings.ContainsRune(alphabet, r))
	}
}
