package service

import (
	"testing"
	"time"

	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type ForwardFlowResetWorkerTestSuite struct {
	ServiceTestSuite
	worker *ForwardFlowResetWorker
}

func (s *ForwardFlowResetWorkerTestSuite) SetupTest() {
	s.ServiceTestSuite.SetupTest()
	s.worker = NewForwardFlowResetWorker(database.Get())
}

func (s *ForwardFlowResetWorkerTestSuite) TestRunOnce_ResetsMatchingUserTunnelDayOnly() {
	db := database.Get()

	user := &model.User{
		Email:          "worker-reset-day@example.com",
		Password:       "hash",
		Token:          "worker-reset-day-token",
		UUID:           "worker-reset-day-uuid",
		TransferEnable: 1073741824,
	}
	tunnelA := &model.ForwardTunnel{Name: "Reset Day Tunnel A", InIP: "10.40.0.1", Status: model.ForwardTunnelStatusActive}
	tunnelB := &model.ForwardTunnel{Name: "Reset Day Tunnel B", InIP: "10.40.0.2", Status: model.ForwardTunnelStatusActive}
	assert.NoError(s.T(), db.Create(user).Error)
	assert.NoError(s.T(), db.Create(tunnelA).Error)
	assert.NoError(s.T(), db.Create(tunnelB).Error)

	match := &model.ForwardUserTunnel{
		UserID:        user.ID,
		TunnelID:      tunnelA.ID,
		FlowResetTime: 6,
		InFlow:        100,
		OutFlow:       200,
		Status:        model.ForwardUserTunnelStatusActive,
	}
	other := &model.ForwardUserTunnel{
		UserID:        user.ID,
		TunnelID:      tunnelB.ID,
		FlowResetTime: 7,
		InFlow:        300,
		OutFlow:       400,
		Status:        model.ForwardUserTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(match).Error)
	assert.NoError(s.T(), db.Create(other).Error)

	assert.NoError(s.T(), db.Create(&model.Forward{
		UserID:     user.ID,
		UserName:   user.Email,
		Name:       "Match Forward",
		TunnelID:   tunnelA.ID,
		InPort:     15001,
		RemoteAddr: "match.example:443",
		InFlow:     100,
		OutFlow:    200,
		Status:     model.ForwardStatusPaused,
	}).Error)
	assert.NoError(s.T(), db.Create(&model.Forward{
		UserID:     user.ID,
		UserName:   user.Email,
		Name:       "Other Forward",
		TunnelID:   tunnelB.ID,
		InPort:     15002,
		RemoteAddr: "other.example:443",
		InFlow:     300,
		OutFlow:    400,
		Status:     model.ForwardStatusPaused,
	}).Error)

	assert.NoError(s.T(), s.worker.RunOnce(time.Date(2026, time.April, 6, 0, 0, 5, 0, time.Local)))

	var reloadedMatch model.ForwardUserTunnel
	var reloadedOther model.ForwardUserTunnel
	assert.NoError(s.T(), db.First(&reloadedMatch, match.ID).Error)
	assert.NoError(s.T(), db.First(&reloadedOther, other.ID).Error)
	assert.Equal(s.T(), int64(0), reloadedMatch.InFlow)
	assert.Equal(s.T(), int64(0), reloadedMatch.OutFlow)
	assert.Equal(s.T(), int64(300), reloadedOther.InFlow)
	assert.Equal(s.T(), int64(400), reloadedOther.OutFlow)

	var matchForward model.Forward
	var otherForward model.Forward
	assert.NoError(s.T(), db.Where("tunnel_id = ?", tunnelA.ID).First(&matchForward).Error)
	assert.NoError(s.T(), db.Where("tunnel_id = ?", tunnelB.ID).First(&otherForward).Error)
	assert.Equal(s.T(), int64(0), matchForward.InFlow)
	assert.Equal(s.T(), int64(0), matchForward.OutFlow)
	assert.Equal(s.T(), int64(300), otherForward.InFlow)
	assert.Equal(s.T(), int64(400), otherForward.OutFlow)
}

func (s *ForwardFlowResetWorkerTestSuite) TestRunOnce_ResetsMonthEndOverflowDays() {
	db := database.Get()

	user := &model.User{
		Email:          "worker-reset-month-end@example.com",
		Password:       "hash",
		Token:          "worker-reset-month-end-token",
		UUID:           "worker-reset-month-end-uuid",
		TransferEnable: 1073741824,
	}
	tunnelA := &model.ForwardTunnel{Name: "Month End Tunnel A", InIP: "10.40.1.1", Status: model.ForwardTunnelStatusActive}
	tunnelB := &model.ForwardTunnel{Name: "Month End Tunnel B", InIP: "10.40.1.2", Status: model.ForwardTunnelStatusActive}
	tunnelC := &model.ForwardTunnel{Name: "Month End Tunnel C", InIP: "10.40.1.3", Status: model.ForwardTunnelStatusActive}
	assert.NoError(s.T(), db.Create(user).Error)
	assert.NoError(s.T(), db.Create(tunnelA).Error)
	assert.NoError(s.T(), db.Create(tunnelB).Error)
	assert.NoError(s.T(), db.Create(tunnelC).Error)

	reset30 := &model.ForwardUserTunnel{UserID: user.ID, TunnelID: tunnelA.ID, FlowResetTime: 30, InFlow: 10, OutFlow: 20, Status: model.ForwardUserTunnelStatusActive}
	reset31 := &model.ForwardUserTunnel{UserID: user.ID, TunnelID: tunnelB.ID, FlowResetTime: 31, InFlow: 30, OutFlow: 40, Status: model.ForwardUserTunnelStatusActive}
	keep29 := &model.ForwardUserTunnel{UserID: user.ID, TunnelID: tunnelC.ID, FlowResetTime: 29, InFlow: 50, OutFlow: 60, Status: model.ForwardUserTunnelStatusActive}
	assert.NoError(s.T(), db.Create(reset30).Error)
	assert.NoError(s.T(), db.Create(reset31).Error)
	assert.NoError(s.T(), db.Create(keep29).Error)

	assert.NoError(s.T(), s.worker.RunOnce(time.Date(2026, time.April, 30, 0, 0, 5, 0, time.Local)))

	var reloaded30 model.ForwardUserTunnel
	var reloaded31 model.ForwardUserTunnel
	var reloaded29 model.ForwardUserTunnel
	assert.NoError(s.T(), db.First(&reloaded30, reset30.ID).Error)
	assert.NoError(s.T(), db.First(&reloaded31, reset31.ID).Error)
	assert.NoError(s.T(), db.First(&reloaded29, keep29.ID).Error)
	assert.Equal(s.T(), int64(0), reloaded30.InFlow)
	assert.Equal(s.T(), int64(0), reloaded30.OutFlow)
	assert.Equal(s.T(), int64(0), reloaded31.InFlow)
	assert.Equal(s.T(), int64(0), reloaded31.OutFlow)
	assert.Equal(s.T(), int64(50), reloaded29.InFlow)
	assert.Equal(s.T(), int64(60), reloaded29.OutFlow)
}

func (s *ForwardFlowResetWorkerTestSuite) TestRunOnce_ResetsUserTrafficWhenFlowResetTimeColumnExists() {
	db := database.Get()

	user30 := &model.User{
		Email:          "worker-user-reset-30@example.com",
		Password:       "hash",
		Token:          "worker-user-reset-30-token",
		UUID:           "worker-user-reset-30-uuid",
		TransferEnable: 1073741824,
		FlowResetTime:  30,
		U:              1000,
		D:              2000,
	}
	user31 := &model.User{
		Email:          "worker-user-reset-31@example.com",
		Password:       "hash",
		Token:          "worker-user-reset-31-token",
		UUID:           "worker-user-reset-31-uuid",
		TransferEnable: 1073741824,
		FlowResetTime:  31,
		U:              3000,
		D:              4000,
	}
	user29 := &model.User{
		Email:          "worker-user-reset-29@example.com",
		Password:       "hash",
		Token:          "worker-user-reset-29-token",
		UUID:           "worker-user-reset-29-uuid",
		TransferEnable: 1073741824,
		FlowResetTime:  29,
		U:              5000,
		D:              6000,
	}
	assert.NoError(s.T(), db.Create(user30).Error)
	assert.NoError(s.T(), db.Create(user31).Error)
	assert.NoError(s.T(), db.Create(user29).Error)

	assert.NoError(s.T(), s.worker.RunOnce(time.Date(2026, time.April, 30, 0, 0, 5, 0, time.Local)))

	var reloaded30 model.User
	var reloaded31 model.User
	var reloaded29 model.User
	assert.NoError(s.T(), db.First(&reloaded30, user30.ID).Error)
	assert.NoError(s.T(), db.First(&reloaded31, user31.ID).Error)
	assert.NoError(s.T(), db.First(&reloaded29, user29.ID).Error)
	assert.Equal(s.T(), int64(0), reloaded30.U)
	assert.Equal(s.T(), int64(0), reloaded30.D)
	assert.Equal(s.T(), int64(0), reloaded31.U)
	assert.Equal(s.T(), int64(0), reloaded31.D)
	assert.Equal(s.T(), int64(5000), reloaded29.U)
	assert.Equal(s.T(), int64(6000), reloaded29.D)
}

func (s *ForwardFlowResetWorkerTestSuite) TestRunOnce_DisablesExpiredUserTunnelAndPausesItsActiveForwards() {
	db := database.Get()
	now := time.Date(2026, time.April, 6, 0, 0, 5, 0, time.Local)

	user := &model.User{
		Email:          "worker-expired-permission@example.com",
		Password:       "hash",
		Token:          "worker-expired-permission-token",
		UUID:           "worker-expired-permission-uuid",
		TransferEnable: 1073741824,
	}
	expiredTunnel := &model.ForwardTunnel{Name: "Expired Permission Tunnel", InIP: "10.40.2.1", Status: model.ForwardTunnelStatusActive}
	validTunnel := &model.ForwardTunnel{Name: "Valid Permission Tunnel", InIP: "10.40.2.2", Status: model.ForwardTunnelStatusActive}
	assert.NoError(s.T(), db.Create(user).Error)
	assert.NoError(s.T(), db.Create(expiredTunnel).Error)
	assert.NoError(s.T(), db.Create(validTunnel).Error)

	expiredPermission := &model.ForwardUserTunnel{
		UserID:   user.ID,
		TunnelID: expiredTunnel.ID,
		ExpTime:  now.Add(-time.Minute).UnixMilli(),
		Status:   model.ForwardUserTunnelStatusActive,
	}
	validPermission := &model.ForwardUserTunnel{
		UserID:   user.ID,
		TunnelID: validTunnel.ID,
		ExpTime:  now.Add(time.Hour).UnixMilli(),
		Status:   model.ForwardUserTunnelStatusActive,
	}
	assert.NoError(s.T(), db.Create(expiredPermission).Error)
	assert.NoError(s.T(), db.Create(validPermission).Error)

	expiredActiveForward := &model.Forward{
		UserID:     user.ID,
		UserName:   user.Email,
		Name:       "Expired Permission Active Forward",
		TunnelID:   expiredTunnel.ID,
		InPort:     15101,
		RemoteAddr: "expired-perm-active.example:443",
		Status:     model.ForwardStatusActive,
	}
	expiredPausedForward := &model.Forward{
		UserID:     user.ID,
		UserName:   user.Email,
		Name:       "Expired Permission Paused Forward",
		TunnelID:   expiredTunnel.ID,
		InPort:     15102,
		RemoteAddr: "expired-perm-paused.example:443",
		Status:     model.ForwardStatusPaused,
	}
	validActiveForward := &model.Forward{
		UserID:     user.ID,
		UserName:   user.Email,
		Name:       "Valid Permission Active Forward",
		TunnelID:   validTunnel.ID,
		InPort:     15103,
		RemoteAddr: "valid-perm-active.example:443",
		Status:     model.ForwardStatusActive,
	}
	assert.NoError(s.T(), db.Create(expiredActiveForward).Error)
	assert.NoError(s.T(), db.Create(expiredPausedForward).Error)
	assert.NoError(s.T(), db.Create(validActiveForward).Error)

	assert.NoError(s.T(), s.worker.RunOnce(now))

	var reloadedExpiredPermission model.ForwardUserTunnel
	var reloadedValidPermission model.ForwardUserTunnel
	assert.NoError(s.T(), db.First(&reloadedExpiredPermission, expiredPermission.ID).Error)
	assert.NoError(s.T(), db.First(&reloadedValidPermission, validPermission.ID).Error)
	assert.Equal(s.T(), model.ForwardUserTunnelStatusDisabled, reloadedExpiredPermission.Status)
	assert.Equal(s.T(), model.ForwardUserTunnelStatusActive, reloadedValidPermission.Status)

	var reloadedExpiredActive model.Forward
	var reloadedExpiredPaused model.Forward
	var reloadedValidActive model.Forward
	assert.NoError(s.T(), db.First(&reloadedExpiredActive, expiredActiveForward.ID).Error)
	assert.NoError(s.T(), db.First(&reloadedExpiredPaused, expiredPausedForward.ID).Error)
	assert.NoError(s.T(), db.First(&reloadedValidActive, validActiveForward.ID).Error)
	assert.Equal(s.T(), model.ForwardStatusPaused, reloadedExpiredActive.Status)
	assert.Equal(s.T(), model.ForwardStatusPaused, reloadedExpiredPaused.Status)
	assert.Equal(s.T(), model.ForwardStatusActive, reloadedValidActive.Status)

	var pauseJobs []model.ForwardRuntimeJob
	assert.NoError(s.T(), db.Where("action = ? AND forward_id = ?", model.ForwardRuntimeJobActionPause, expiredActiveForward.ID).Find(&pauseJobs).Error)
	assert.Len(s.T(), pauseJobs, 1)
}

func (s *ForwardFlowResetWorkerTestSuite) TestRunOnce_PausesActiveForwardsForExpiredUserWithoutBanning() {
	db := database.Get()
	now := time.Date(2026, time.April, 6, 0, 0, 5, 0, time.Local)

	expiredAt := now.Add(-time.Hour).Unix()
	expiredUser := &model.User{
		Email:          "worker-expired-user@example.com",
		Password:       "hash",
		Token:          "worker-expired-user-token",
		UUID:           "worker-expired-user-uuid",
		TransferEnable: 1073741824,
		ExpiredAt:      &expiredAt,
		Banned:         0,
	}
	activeUser := &model.User{
		Email:          "worker-active-user@example.com",
		Password:       "hash",
		Token:          "worker-active-user-token",
		UUID:           "worker-active-user-uuid",
		TransferEnable: 1073741824,
		Banned:         0,
	}
	tunnel := &model.ForwardTunnel{Name: "Expired User Tunnel", InIP: "10.40.3.1", Status: model.ForwardTunnelStatusActive}
	assert.NoError(s.T(), db.Create(expiredUser).Error)
	assert.NoError(s.T(), db.Create(activeUser).Error)
	assert.NoError(s.T(), db.Create(tunnel).Error)

	expiredUserActiveForward := &model.Forward{
		UserID:     expiredUser.ID,
		UserName:   expiredUser.Email,
		Name:       "Expired User Active Forward",
		TunnelID:   tunnel.ID,
		InPort:     15201,
		RemoteAddr: "expired-user-active.example:443",
		Status:     model.ForwardStatusActive,
	}
	activeUserForward := &model.Forward{
		UserID:     activeUser.ID,
		UserName:   activeUser.Email,
		Name:       "Active User Forward",
		TunnelID:   tunnel.ID,
		InPort:     15202,
		RemoteAddr: "active-user-forward.example:443",
		Status:     model.ForwardStatusActive,
	}
	assert.NoError(s.T(), db.Create(expiredUserActiveForward).Error)
	assert.NoError(s.T(), db.Create(activeUserForward).Error)

	assert.NoError(s.T(), s.worker.RunOnce(now))

	var reloadedExpiredForward model.Forward
	var reloadedActiveForward model.Forward
	var reloadedExpiredUser model.User
	assert.NoError(s.T(), db.First(&reloadedExpiredForward, expiredUserActiveForward.ID).Error)
	assert.NoError(s.T(), db.First(&reloadedActiveForward, activeUserForward.ID).Error)
	assert.NoError(s.T(), db.First(&reloadedExpiredUser, expiredUser.ID).Error)
	assert.Equal(s.T(), model.ForwardStatusPaused, reloadedExpiredForward.Status)
	assert.Equal(s.T(), model.ForwardStatusActive, reloadedActiveForward.Status)
	assert.Equal(s.T(), 0, reloadedExpiredUser.Banned)

	var pauseJobs []model.ForwardRuntimeJob
	assert.NoError(s.T(), db.Where("action = ? AND forward_id = ?", model.ForwardRuntimeJobActionPause, expiredUserActiveForward.ID).Find(&pauseJobs).Error)
	assert.Len(s.T(), pauseJobs, 1)
}

func TestForwardFlowResetWorker(t *testing.T) {
	suite.Run(t, new(ForwardFlowResetWorkerTestSuite))
}
