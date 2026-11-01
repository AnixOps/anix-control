package trafficqos

import (
	"path/filepath"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/destinationstats"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "control.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.SystemConfig{}, &model.OperationLog{}, &model.TrafficCategoryRule{}, &model.TrafficCategorySource{}))
	return db
}

func classesOf(t *testing.T, member map[string]any) map[string]map[string]any {
	t.Helper()
	out := map[string]map[string]any{}
	for _, class := range member["classes"].([]map[string]any) {
		out[class["name"].(string)] = class
	}
	return out
}

func TestDefaultsCapSteamAndLowerDownloads(t *testing.T) {
	db := openDB(t)
	settings, err := Load(db)
	require.NoError(t, err)
	assert.True(t, settings.Enabled)
	assert.Equal(t, PriorityLow, settings.Priorities[destinationstats.CategoryDownload])
	require.Len(t, settings.Caps, 1)
	assert.Equal(t, 5, settings.Caps[0].LimitMbps)

	member, err := NodeMember(db, 57)
	require.NoError(t, err)
	require.NotNil(t, member)
	assert.Equal(t, 0, member["egress_mbps"])
	assert.Equal(t, DefaultMinShare, member["min_share_percent"])
	classes := classesOf(t, member)
	steam := classes["cap:Steam 下载"]
	require.NotNil(t, steam)
	assert.Equal(t, 5, steam["limit_mbps"])
	assert.Contains(t, steam["domains"], "steamcontent.com")
	download := classes["download"]
	require.NotNil(t, download)
	assert.Equal(t, PriorityLow, download["priority"])
	// The capped domains stay out of the category class, so the cap wins.
	assert.NotContains(t, download["domains"], "steamcontent.com")
	assert.Contains(t, download["domains"], "mega.nz")
	assert.Equal(t, []string{"bittorrent"}, download["protocols"])
}

func TestSettingsAreCheckedSavedAndPerNode(t *testing.T) {
	db := openDB(t)
	for _, bad := range []Settings{
		{Priorities: map[string]string{"web": PriorityLow}},
		{Priorities: map[string]string{"video": "urgent"}},
		{MinSharePercent: 95},
		{NodeEgressMbps: map[string]int{"x": 10}},
		{Caps: []Cap{{Name: "a", Domains: []string{"bad domain"}}}},
		{Caps: []Cap{{Name: "a", Domains: []string{"a.example"}}, {Name: "a", Domains: []string{"b.example"}}}},
		{Caps: []Cap{{Name: "a"}}},
	} {
		_, err := Save(db, bad, Actor{})
		require.ErrorIs(t, err, ErrInvalid, "%+v", bad)
	}
	saved, err := Save(db, Settings{
		Enabled:           true,
		Priorities:        map[string]string{"messaging": PriorityHigh, "video": PriorityNormal},
		DefaultEgressMbps: 1000,
		NodeEgressMbps:    map[string]int{"57": 200},
		Caps:              []Cap{{Name: "files", LimitMbps: 10, Domains: []string{"*.Files.Example"}}},
	}, Actor{UserID: 1, Name: "admin@example.com"})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"messaging": PriorityHigh}, saved.Priorities)
	assert.Equal(t, PriorityLow, saved.Caps[0].Priority)
	assert.Equal(t, []string{"files.example"}, saved.Caps[0].Domains)
	loaded, err := Load(db)
	require.NoError(t, err)
	assert.Equal(t, saved, loaded)

	member, err := NodeMember(db, 57)
	require.NoError(t, err)
	assert.Equal(t, 200, member["egress_mbps"])
	classes := classesOf(t, member)
	assert.Contains(t, classes, "messaging")
	assert.NotContains(t, classes, "download")
	other, err := NodeMember(db, 58)
	require.NoError(t, err)
	assert.Equal(t, 1000, other["egress_mbps"])

	_, err = Save(db, Settings{Enabled: false}, Actor{})
	require.NoError(t, err)
	member, err = NodeMember(db, 57)
	require.NoError(t, err)
	assert.Nil(t, member)
	var audits int64
	require.NoError(t, db.Model(&model.OperationLog{}).Where("module = ?", "traffic_qos").Count(&audits).Error)
	assert.Equal(t, int64(2), audits)
}
