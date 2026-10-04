package handler

import (
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/forwardlegacy"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/require"
)

// GET /api/v4/forward/legacy/archive: the kernel serves the newest
// recorded archive to a super administrator and audits every attempt.
func TestForwardLegacyArchiveDownload(t *testing.T) {
	db := newKernelHandlerTestDB(t, &model.User{}, &model.AuditLog{}, &model.ForwardNode{})
	require.NoError(t, db.Create(&model.User{ID: 7, Email: "root@example.test", Password: "x", Token: "t7", UUID: "u7", IsAdmin: 1}).Error)
	require.NoError(t, db.Create(&model.User{ID: 8, Email: "staff@example.test", Password: "x", Token: "t8", UUID: "u8", IsAdmin: 1, IsStaff: 1}).Error)
	require.NoError(t, db.Create(&model.ForwardNode{ID: 1, Name: "hk", Host: "192.0.2.1", APIToken: "secret-node-token-1"}).Error)
	kernel := &KernelHandler{db: db}
	get := func(userID uint) (int, string, http.Header) {
		recorder := performKernelHandlerRequestWithSetup(t, http.MethodGet, "/api/v4/forward/legacy/archive", "", "/api/v4/forward/*route", kernel.ForwardGateway, asAdmin(userID))
		return recorder.Code, recorder.Body.String(), recorder.Header()
	}

	code, body, _ := get(7)
	require.Equal(t, http.StatusNotFound, code, body)
	require.Contains(t, body, "legacy_archive_not_found")

	written, err := forwardlegacy.WriteArchive(t.Context(), db, forwardlegacy.WriteOptions{Dir: t.TempDir(), Trigger: "cli", Now: time.Now()})
	require.NoError(t, err)

	code, body, _ = get(8)
	require.Equal(t, http.StatusForbidden, code, body)
	require.Contains(t, body, "super_admin_required")

	code, body, header := get(7)
	require.Equal(t, http.StatusOK, code, body)
	require.Contains(t, body, forwardlegacy.ArchiveSchema)
	require.NotContains(t, body, "secret-node-token-1")
	require.Equal(t, written.Record.SHA256, header.Get("X-Archive-SHA256"))
	require.Contains(t, header.Get("Content-Disposition"), "attachment")
	require.Equal(t, "no-store", header.Get("Cache-Control"))

	// A changed file is not served.
	require.NoError(t, os.WriteFile(written.Record.Path, []byte("{}"), 0o600))
	code, body, _ = get(7)
	require.Equal(t, http.StatusConflict, code, body)

	var entries []model.AuditLog
	require.NoError(t, db.Order("id").Find(&entries).Error)
	require.Len(t, entries, 4)
	statuses := []int{}
	for _, entry := range entries {
		require.Equal(t, "forward", entry.Module)
		require.Equal(t, "legacy_archive_download", entry.Action)
		statuses = append(statuses, entry.StatusCode)
	}
	require.Equal(t, []int{http.StatusNotFound, http.StatusForbidden, http.StatusOK, http.StatusConflict}, statuses)
	require.Equal(t, uint(7), *entries[2].UserID)
}
