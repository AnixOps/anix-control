package service

import (
	"encoding/json"
	"testing"

	appconfig "github.com/anixops/v2board/internal/config"
	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type ForwardRuntimeBootstrapTestSuite struct {
	ServiceTestSuite
}

func (s *ForwardRuntimeBootstrapTestSuite) SetupSuite() {
	s.ServiceTestSuite.SetupSuite()
	database.AutoMigrate(&model.SystemConfig{})
}

func (s *ForwardRuntimeBootstrapTestSuite) SetupTest() {
	s.ServiceTestSuite.SetupTest()
	database.Get().Exec("DELETE FROM v2_system_config")
	appconfig.Set(nil)
}

func (s *ForwardRuntimeBootstrapTestSuite) TestInitForwardRuntimeSystemConfig_SeedsNodeXFromConfig() {
	appconfig.Set(&appconfig.Config{
		ForwardRuntime: appconfig.ForwardRuntimeConfig{
			Backend: model.ForwardRuntimeBackendGost,
			NodeX: appconfig.ForwardRuntimeNodeXConfig{
				BaseURL:        "http://127.0.0.1:18081",
				Token:          "config-nodex-token",
				TimeoutSeconds: 20,
			},
		},
	})

	err := InitForwardRuntimeSystemConfig(database.Get())
	assert.NoError(s.T(), err)

	configService := NewSystemConfigService(database.Get())
	backend, err := configService.Get(forwardRuntimeBackendConfigKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), model.ForwardRuntimeBackendGost, backend)

	baseURL, err := configService.Get(forwardRuntimeNodeXBaseURLConfigKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "http://127.0.0.1:18081", baseURL)

	token, err := configService.Get(forwardRuntimeNodeXTokenConfigKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "config-nodex-token", token)

	timeout, err := configService.Get(forwardRuntimeNodeXTimeoutSecondsConfigKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "20", timeout)
}

func (s *ForwardRuntimeBootstrapTestSuite) TestInitForwardRuntimeSystemConfig_SeedsAnsibleFromConfig() {
	appconfig.Set(&appconfig.Config{
		ForwardRuntime: appconfig.ForwardRuntimeConfig{
			Backend: model.ForwardRuntimeBackendNftablesAnsible,
			NftablesAnsible: appconfig.ForwardRuntimeAnsibleConfig{
				Inventory:      "config/deploy/ansible/inventory.ini",
				ApplyPlaybook:  "config/deploy/ansible/playbooks/forward_apply_nftables.yml",
				RemovePlaybook: "config/deploy/ansible/playbooks/forward_remove_nftables.yml",
				WorkingDir:     "config/deploy/ansible",
				TargetPattern:  "{{node.host}}",
				TimeoutSeconds: 90,
				Become:         true,
				ExtraVars: map[string]any{
					"retry": 3,
				},
				Environment: map[string]string{
					"ANSIBLE_CONFIG": "config/deploy/ansible/ansible.cfg",
				},
			},
		},
	})

	err := InitForwardRuntimeSystemConfig(database.Get())
	assert.NoError(s.T(), err)

	configService := NewSystemConfigService(database.Get())
	backend, err := configService.Get(forwardRuntimeBackendConfigKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), model.ForwardRuntimeBackendNftablesAnsible, backend)

	localBackend, err := configService.Get(forwardRuntimeLocalBackendConfigKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), model.ForwardRuntimeBackendNftablesAnsible, localBackend)

	ansibleConfig, err := configService.Get(forwardRuntimeAnsibleConfigJSONKey)
	assert.NoError(s.T(), err)

	var cfg panelForwardAnsibleConfig
	assert.NoError(s.T(), json.Unmarshal([]byte(ansibleConfig), &cfg))
	assert.Equal(s.T(), "config/deploy/ansible/inventory.ini", cfg.Inventory)
	assert.Equal(s.T(), "config/deploy/ansible/playbooks/forward_apply_nftables.yml", cfg.ApplyPlaybook)
	assert.Equal(s.T(), "config/deploy/ansible/playbooks/forward_remove_nftables.yml", cfg.RemovePlaybook)
	assert.Equal(s.T(), "config/deploy/ansible", cfg.WorkingDir)
	assert.Equal(s.T(), "{{node.host}}", cfg.TargetPattern)
	assert.Equal(s.T(), 90, cfg.TimeoutSeconds)
	assert.True(s.T(), cfg.Become)
	assert.Equal(s.T(), "config/deploy/ansible/ansible.cfg", cfg.Environment["ANSIBLE_CONFIG"])
	assert.Equal(s.T(), float64(3), cfg.ExtraVars["retry"])
}

func (s *ForwardRuntimeBootstrapTestSuite) TestInitForwardRuntimeSystemConfig_NodeXModeCompatibilityOverridesBackend() {
	appconfig.Set(&appconfig.Config{
		ForwardRuntime: appconfig.ForwardRuntimeConfig{
			NodeXMode: boolPtr(true),
			Backend:   model.ForwardRuntimeBackendIptablesAnsible,
			NodeX: appconfig.ForwardRuntimeNodeXConfig{
				BaseURL: "http://127.0.0.1:18081",
				Token:   "nodex-token",
			},
		},
	})

	err := InitForwardRuntimeSystemConfig(database.Get())
	assert.NoError(s.T(), err)

	configService := NewSystemConfigService(database.Get())
	mode, err := configService.Get(forwardRuntimeNodeXModeConfigKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "true", mode)

	backend, err := configService.Get(forwardRuntimeBackendConfigKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), model.ForwardRuntimeBackendGost, backend)
}

func (s *ForwardRuntimeBootstrapTestSuite) TestInitForwardRuntimeSystemConfig_SeedsDefaultLocalRuntimeValuesWhenOmitted() {
	configService := NewSystemConfigService(database.Get())
	assert.NoError(s.T(), configService.Set(forwardRuntimeNodeXModeConfigKey, "true", "bool", forwardRuntimeConfigGroup, forwardRuntimeBootstrapRemark))
	assert.NoError(s.T(), configService.Set(forwardRuntimeBackendConfigKey, model.ForwardRuntimeBackendGost, "string", forwardRuntimeConfigGroup, forwardRuntimeBootstrapRemark))
	assert.NoError(s.T(), configService.Set(forwardRuntimeNodeXBaseURLConfigKey, "http://127.0.0.1:18081", "string", forwardRuntimeConfigGroup, forwardRuntimeBootstrapRemark))
	assert.NoError(s.T(), configService.Set(forwardRuntimeNodeXTokenConfigKey, "old-token", "string", forwardRuntimeConfigGroup, forwardRuntimeBootstrapRemark))
	assert.NoError(s.T(), configService.Set(forwardRuntimeNodeXTimeoutSecondsConfigKey, "20", "int", forwardRuntimeConfigGroup, forwardRuntimeBootstrapRemark))
	assert.NoError(s.T(), configService.Set(forwardRuntimeAnsibleConfigJSONKey, `{"inventory":"old.ini"}`, "json", forwardRuntimeConfigGroup, forwardRuntimeBootstrapRemark))
	assert.NoError(s.T(), configService.Set(forwardRuntimeAnsibleInventoryConfigKey, "old.ini", "string", forwardRuntimeConfigGroup, forwardRuntimeBootstrapRemark))

	appconfig.Set(&appconfig.Config{})

	err := InitForwardRuntimeSystemConfig(database.Get())
	assert.NoError(s.T(), err)

	value, err := configService.Get(forwardRuntimeNodeXModeConfigKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "", value)

	value, err = configService.Get(forwardRuntimeBackendConfigKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "", value)

	value, err = configService.Get(forwardRuntimeLocalBackendConfigKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), model.ForwardRuntimeBackendNftablesAnsible, value)

	value, err = configService.Get(forwardRuntimeNodeXBaseURLConfigKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "", value)

	value, err = configService.Get(forwardRuntimeNodeXTokenConfigKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "", value)

	value, err = configService.Get(forwardRuntimeNodeXTimeoutSecondsConfigKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "", value)

	value, err = configService.Get(forwardRuntimeAnsibleConfigJSONKey)
	assert.NoError(s.T(), err)
	assert.NotEmpty(s.T(), value)

	var cfg panelForwardAnsibleConfig
	assert.NoError(s.T(), json.Unmarshal([]byte(value), &cfg))
	assert.Equal(s.T(), defaultForwardAnsibleInventoryPath, cfg.Inventory)
	assert.Equal(s.T(), defaultForwardNftablesApplyPlaybookPath, cfg.ApplyPlaybook)
	assert.Equal(s.T(), defaultForwardNftablesRemovePlaybookPath, cfg.RemovePlaybook)
	assert.Equal(s.T(), defaultForwardAnsibleWorkingDir, cfg.WorkingDir)
	assert.Equal(s.T(), defaultForwardAnsibleTargetPattern, cfg.TargetPattern)
	assert.Equal(s.T(), int(defaultForwardRuntimeJobTimeout.Seconds()), cfg.TimeoutSeconds)
	assert.Equal(s.T(), defaultForwardAnsibleConfigPath, cfg.Environment["ANSIBLE_CONFIG"])

	value, err = configService.Get(forwardRuntimeAnsibleInventoryConfigKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), defaultForwardAnsibleInventoryPath, value)
}

func (s *ForwardRuntimeBootstrapTestSuite) TestInitForwardRuntimeSystemConfig_PreservesUserManagedOverrides() {
	configService := NewSystemConfigService(database.Get())
	assert.NoError(s.T(), configService.Set(forwardRuntimeNodeXModeConfigKey, "false", "bool", forwardRuntimeConfigGroup, "manual override"))
	assert.NoError(s.T(), configService.Set(forwardRuntimeBackendConfigKey, model.ForwardRuntimeBackendNftablesAnsible, "string", forwardRuntimeConfigGroup, "manual override"))
	assert.NoError(s.T(), configService.Set(forwardRuntimeNodeXBaseURLConfigKey, "http://127.0.0.1:19090", "string", forwardRuntimeConfigGroup, "manual override"))
	assert.NoError(s.T(), configService.Set(forwardRuntimeNodeXTokenConfigKey, "manual-token", "string", forwardRuntimeConfigGroup, "manual override"))
	assert.NoError(s.T(), configService.Set(forwardRuntimeAnsibleConfigJSONKey, `{"inventory":"manual.ini"}`, "json", forwardRuntimeConfigGroup, "manual override"))

	appconfig.Set(&appconfig.Config{
		ForwardRuntime: appconfig.ForwardRuntimeConfig{
			Backend: model.ForwardRuntimeBackendGost,
			NodeX: appconfig.ForwardRuntimeNodeXConfig{
				BaseURL: "http://127.0.0.1:18081",
				Token:   "config-token",
			},
		},
	})

	err := InitForwardRuntimeSystemConfig(database.Get())
	assert.NoError(s.T(), err)

	value, err := configService.Get(forwardRuntimeNodeXModeConfigKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "false", value)

	value, err = configService.Get(forwardRuntimeBackendConfigKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), model.ForwardRuntimeBackendNftablesAnsible, value)

	value, err = configService.Get(forwardRuntimeNodeXBaseURLConfigKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "http://127.0.0.1:19090", value)

	value, err = configService.Get(forwardRuntimeNodeXTokenConfigKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "manual-token", value)

	value, err = configService.Get(forwardRuntimeAnsibleConfigJSONKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), `{"inventory":"manual.ini"}`, value)
}

func (s *ForwardRuntimeBootstrapTestSuite) TestInitForwardRuntimeSystemConfig_RejectsInvalidBackend() {
	appconfig.Set(&appconfig.Config{
		ForwardRuntime: appconfig.ForwardRuntimeConfig{
			Backend: "bad-backend",
		},
	})

	err := InitForwardRuntimeSystemConfig(database.Get())
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "forward_runtime.backend")
}

func (s *ForwardRuntimeBootstrapTestSuite) TestInitForwardRuntimeSystemConfig_GostRequiresBaseURL() {
	appconfig.Set(&appconfig.Config{
		ForwardRuntime: appconfig.ForwardRuntimeConfig{
			Backend: model.ForwardRuntimeBackendGost,
			NodeX: appconfig.ForwardRuntimeNodeXConfig{
				Token: "nodex-token",
			},
		},
	})

	err := InitForwardRuntimeSystemConfig(database.Get())
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), forwardRuntimeNodeXBaseURLConfigKey)
}

func (s *ForwardRuntimeBootstrapTestSuite) TestInitForwardRuntimeSystemConfig_GostRequiresToken() {
	appconfig.Set(&appconfig.Config{
		ForwardRuntime: appconfig.ForwardRuntimeConfig{
			Backend: model.ForwardRuntimeBackendGost,
			NodeX: appconfig.ForwardRuntimeNodeXConfig{
				BaseURL: "http://127.0.0.1:18081",
			},
		},
	})

	err := InitForwardRuntimeSystemConfig(database.Get())
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), forwardRuntimeNodeXTokenConfigKey)
}

func boolPtr(v bool) *bool {
	return &v
}

// TestNormalizeBackend_IptablesMapsToNftables 验证 iptables 已下线后归一化兜底:
// 输入 iptables_ansible 一律映射为 nftables_ansible, 旧数据/旧配置不会变成未知后端。
func (s *ForwardRuntimeBootstrapTestSuite) TestNormalizeBackend_IptablesMapsToNftables() {
	backend, ok := normalizeForwardRuntimeBackend(model.ForwardRuntimeBackendIptablesAnsible)
	assert.True(s.T(), ok)
	assert.Equal(s.T(), model.ForwardRuntimeBackendNftablesAnsible, backend)

	local, ok := normalizeForwardRuntimeLocalAnsibleBackend(model.ForwardRuntimeBackendIptablesAnsible)
	assert.True(s.T(), ok)
	assert.Equal(s.T(), model.ForwardRuntimeBackendNftablesAnsible, local)
}

// TestMigrateIptablesForwardBackend_NormalizesExistingRows 验证存量数据迁移:
// Forward / 未完成 Job / Cursor / systemconfig 的 iptables_ansible 都被改成 nftables_ansible,
// 已完成 Job 保留审计原值, 且 Cursor 唯一索引冲突时正确去重。
func (s *ForwardRuntimeBootstrapTestSuite) TestMigrateIptablesForwardBackend_NormalizesExistingRows() {
	db := database.Get()
	const ipt = model.ForwardRuntimeBackendIptablesAnsible
	const nft = model.ForwardRuntimeBackendNftablesAnsible

	// Forward
	fwd := &model.Forward{UserID: 1, Name: "ipt-fwd", TunnelID: 1, InPort: 30001, RemoteAddr: "10.0.0.9:80", Status: model.ForwardStatusActive, RuntimeBackend: ipt}
	assert.NoError(s.T(), db.Create(fwd).Error)

	// 未完成 job (应迁移) + 已完成 job (保留原值)
	pendingJob := &model.ForwardRuntimeJob{Backend: ipt, Action: model.ForwardRuntimeJobActionCreate, ForwardID: uintPtr(fwd.ID), Status: model.ForwardRuntimeJobStatusPending}
	doneJob := &model.ForwardRuntimeJob{Backend: ipt, Action: model.ForwardRuntimeJobActionCreate, ForwardID: uintPtr(fwd.ID), Status: model.ForwardRuntimeJobStatusSuccess}
	assert.NoError(s.T(), db.Create(pendingJob).Error)
	assert.NoError(s.T(), db.Create(doneJob).Error)

	// Cursor 冲突场景: forward_id=fwd.ID 同时有 ipt 和 nft 行 -> ipt 行应被删除
	assert.NoError(s.T(), db.Create(&model.ForwardTrafficCursor{ForwardID: fwd.ID, Backend: nft, UploadTotal: 100}).Error)
	assert.NoError(s.T(), db.Create(&model.ForwardTrafficCursor{ForwardID: fwd.ID, Backend: ipt, UploadTotal: 200}).Error)
	// Cursor 非冲突场景: 另一个 forward 只有 ipt 行 -> 应被迁移为 nft
	assert.NoError(s.T(), db.Create(&model.ForwardTrafficCursor{ForwardID: 999, Backend: ipt, UploadTotal: 300}).Error)

	// systemconfig
	configService := NewSystemConfigService(db)
	assert.NoError(s.T(), configService.Set(forwardRuntimeBackendConfigKey, ipt, "string", forwardRuntimeConfigGroup, "legacy"))

	// 执行迁移
	assert.NoError(s.T(), migrateIptablesForwardBackend(db))

	// Forward 被迁移
	var gotFwd model.Forward
	assert.NoError(s.T(), db.First(&gotFwd, fwd.ID).Error)
	assert.Equal(s.T(), nft, gotFwd.RuntimeBackend)

	// pending job 迁移, done job 保留
	var gotPending, gotDone model.ForwardRuntimeJob
	assert.NoError(s.T(), db.First(&gotPending, pendingJob.ID).Error)
	assert.NoError(s.T(), db.First(&gotDone, doneJob.ID).Error)
	assert.Equal(s.T(), nft, gotPending.Backend)
	assert.Equal(s.T(), ipt, gotDone.Backend)

	// Cursor: 冲突的 ipt 行被删, 只剩原 nft 行; 非冲突的被迁移为 nft
	var cursors []model.ForwardTrafficCursor
	assert.NoError(s.T(), db.Where("forward_id = ?", fwd.ID).Find(&cursors).Error)
	assert.Len(s.T(), cursors, 1)
	assert.Equal(s.T(), nft, cursors[0].Backend)
	assert.Equal(s.T(), int64(100), cursors[0].UploadTotal) // 保留的是原 nft 行

	var cursor999 model.ForwardTrafficCursor
	assert.NoError(s.T(), db.Where("forward_id = ?", 999).First(&cursor999).Error)
	assert.Equal(s.T(), nft, cursor999.Backend)

	// systemconfig 被迁移
	val, err := configService.Get(forwardRuntimeBackendConfigKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), nft, val)

	// 幂等: 再跑一次不报错
	assert.NoError(s.T(), migrateIptablesForwardBackend(db))
}

func TestForwardRuntimeBootstrap(t *testing.T) {
	suite.Run(t, new(ForwardRuntimeBootstrapTestSuite))
}
