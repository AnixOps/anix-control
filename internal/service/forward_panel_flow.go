package service

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/anixops/v2board/internal/model"
	"gorm.io/gorm"
)

const (
	defaultFluxForwardFlowBackend = model.ForwardRuntimeBackendGost
	webAPIServiceName             = "web_api"
)

var forwardTrafficLocks sync.Map

type PanelForwardFlowData struct {
	N string `json:"n"`
	U int64  `json:"u"`
	D int64  `json:"d"`
}

type PanelForwardTrafficRecord struct {
	ForwardID uint  `json:"forwardId"`
	Upload    int64 `json:"upload"`
	Download  int64 `json:"download"`
}

type PanelForwardTrafficSnapshot struct {
	ForwardID     uint   `json:"forwardId"`
	Backend       string `json:"backend"`
	UploadTotal   int64  `json:"uploadTotal"`
	DownloadTotal int64  `json:"downloadTotal"`
}

type panelForwardFlowRef struct {
	ForwardID    uint
	UserID       uint
	UserTunnelID uint
}

type panelForwardTrafficScope struct {
	UserID   uint
	TunnelID uint
}

func (s *PanelForwardService) UploadFluxForwardFlow(data PanelForwardFlowData) error {
	if strings.TrimSpace(data.N) == "" || strings.EqualFold(strings.TrimSpace(data.N), webAPIServiceName) {
		return nil
	}

	ref, err := parsePanelForwardFlowRef(data.N)
	if err != nil {
		return err
	}

	forward, err := s.loadForwardWithTunnel(ref.ForwardID)
	if err != nil {
		return err
	}

	upload, download := applyPanelForwardTrafficRatio(forward.Tunnel, data.U, data.D)
	return s.recordForwardTrafficDelta(ref.ForwardID, upload, download)
}

func (s *PanelForwardService) RecordForwardTraffic(records []PanelForwardTrafficRecord) error {
	for _, record := range records {
		if err := s.recordForwardTrafficDelta(record.ForwardID, record.Upload, record.Download); err != nil {
			return err
		}
	}
	return nil
}

func (s *PanelForwardService) ApplyForwardTrafficSnapshots(records []PanelForwardTrafficSnapshot) error {
	for _, record := range records {
		if err := s.applyForwardTrafficSnapshot(record); err != nil {
			return err
		}
	}
	return nil
}

func (s *PanelForwardService) applyForwardTrafficSnapshot(record PanelForwardTrafficSnapshot) error {
	if record.ForwardID == 0 {
		return errors.New("forwardId is required")
	}
	if record.UploadTotal < 0 || record.DownloadTotal < 0 {
		return errors.New("traffic totals must be non-negative")
	}

	backend := strings.TrimSpace(record.Backend)
	if backend == "" {
		backend = defaultFluxForwardFlowBackend
	}

	lock := panelForwardTrafficLock(record.ForwardID)
	lock.Lock()
	defer lock.Unlock()

	var (
		uploadDelta   int64
		downloadDelta int64
		scope         panelForwardTrafficScope
	)

	err := s.db.Transaction(func(tx *gorm.DB) error {
		var forward model.Forward
		if err := tx.Select("id", "user_id", "tunnel_id").First(&forward, record.ForwardID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				_ = tx.Where("forward_id = ? AND backend = ?", record.ForwardID, backend).Delete(&model.ForwardTrafficCursor{}).Error
				return nil
			}
			return err
		}

		scope = panelForwardTrafficScope{
			UserID:   forward.UserID,
			TunnelID: forward.TunnelID,
		}

		var cursor model.ForwardTrafficCursor
		err := tx.Where("forward_id = ? AND backend = ?", record.ForwardID, backend).First(&cursor).Error
		switch {
		case err == nil:
			uploadDelta = record.UploadTotal - cursor.UploadTotal
			downloadDelta = record.DownloadTotal - cursor.DownloadTotal
			if uploadDelta < 0 {
				uploadDelta = record.UploadTotal
			}
			if downloadDelta < 0 {
				downloadDelta = record.DownloadTotal
			}
			if err := tx.Model(&model.ForwardTrafficCursor{}).
				Where("id = ?", cursor.ID).
				Updates(map[string]interface{}{
					"upload_total":   record.UploadTotal,
					"download_total": record.DownloadTotal,
				}).Error; err != nil {
				return err
			}
		case errors.Is(err, gorm.ErrRecordNotFound):
			uploadDelta = record.UploadTotal
			downloadDelta = record.DownloadTotal
			if err := tx.Create(&model.ForwardTrafficCursor{
				ForwardID:     record.ForwardID,
				Backend:       backend,
				UploadTotal:   record.UploadTotal,
				DownloadTotal: record.DownloadTotal,
			}).Error; err != nil {
				return err
			}
		default:
			return err
		}

		if uploadDelta == 0 && downloadDelta == 0 {
			return nil
		}
		return s.recordForwardTrafficDeltaTx(tx, record.ForwardID, uploadDelta, downloadDelta)
	})
	if err != nil {
		return err
	}
	if scope.UserID == 0 {
		return nil
	}
	return s.reconcileTrafficScope(scope)
}

func (s *PanelForwardService) recordForwardTrafficDelta(forwardID uint, upload, download int64) error {
	if forwardID == 0 {
		return errors.New("forwardId is required")
	}
	if upload < 0 || download < 0 {
		return errors.New("traffic values must be non-negative")
	}
	if upload == 0 && download == 0 {
		return nil
	}

	lock := panelForwardTrafficLock(forwardID)
	lock.Lock()
	defer lock.Unlock()

	scope := panelForwardTrafficScope{}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var forward model.Forward
		if err := tx.Select("id", "user_id", "tunnel_id").First(&forward, forwardID).Error; err != nil {
			return err
		}
		scope = panelForwardTrafficScope{
			UserID:   forward.UserID,
			TunnelID: forward.TunnelID,
		}
		return s.recordForwardTrafficDeltaTx(tx, forwardID, upload, download)
	})
	if err != nil {
		return err
	}
	return s.reconcileTrafficScope(scope)
}

func (s *PanelForwardService) recordForwardTrafficDeltaTx(tx *gorm.DB, forwardID uint, upload, download int64) error {
	if upload == 0 && download == 0 {
		return nil
	}

	var forward model.Forward
	if err := tx.Select("id", "user_id", "tunnel_id").First(&forward, forwardID).Error; err != nil {
		return err
	}

	if err := tx.Model(&model.Forward{}).
		Where("id = ?", forwardID).
		Updates(map[string]interface{}{
			"in_flow":  gorm.Expr("in_flow + ?", download),
			"out_flow": gorm.Expr("out_flow + ?", upload),
		}).Error; err != nil {
		return err
	}

	if err := tx.Model(&model.User{}).
		Where("id = ?", forward.UserID).
		Updates(map[string]interface{}{
			"u": gorm.Expr("u + ?", upload),
			"d": gorm.Expr("d + ?", download),
		}).Error; err != nil {
		return err
	}

	if err := tx.Model(&model.ForwardUserTunnel{}).
		Where("user_id = ? AND tunnel_id = ?", forward.UserID, forward.TunnelID).
		Updates(map[string]interface{}{
			"in_flow":  gorm.Expr("in_flow + ?", download),
			"out_flow": gorm.Expr("out_flow + ?", upload),
		}).Error; err != nil {
		return err
	}

	return nil
}

func (s *PanelForwardService) reconcileTrafficScope(scope panelForwardTrafficScope) error {
	if scope.UserID == 0 {
		return nil
	}
	if err := s.reconcileUserTrafficForwards(scope.UserID); err != nil {
		return err
	}
	if scope.TunnelID == 0 {
		return nil
	}

	var permission model.ForwardUserTunnel
	if err := s.db.Where("user_id = ? AND tunnel_id = ?", scope.UserID, scope.TunnelID).First(&permission).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	return s.reconcileUserTunnelForwards(&permission)
}

func (s *PanelForwardService) reconcileUserTrafficForwards(userID uint) error {
	var user model.User
	if err := s.db.First(&user, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if user.IsValid() && user.HasTraffic() {
		return nil
	}

	var forwards []model.Forward
	if err := s.db.Where("user_id = ? AND status = ?", userID, model.ForwardStatusActive).Order("id ASC").Find(&forwards).Error; err != nil {
		return err
	}

	var firstErr error
	for i := range forwards {
		if err := s.pauseManagedForward(&forwards[i]); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (s *PanelForwardService) loadForwardWithTunnel(forwardID uint) (*model.Forward, error) {
	var forward model.Forward
	if err := s.db.Preload("Tunnel").First(&forward, forwardID).Error; err != nil {
		return nil, err
	}
	return &forward, nil
}

func parsePanelForwardFlowRef(raw string) (*panelForwardFlowRef, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, errors.New("service name is required")
	}

	parts := strings.Split(trimmed, "_")
	if len(parts) == 3 {
		forwardID, err := parsePanelForwardIDPart(parts[0], "forward")
		if err != nil {
			return nil, err
		}
		userID, err := parsePanelForwardIDPart(parts[1], "user")
		if err != nil {
			return nil, err
		}
		userTunnelID, err := parsePanelForwardIDPart(parts[2], "userTunnel")
		if err != nil {
			return nil, err
		}
		return &panelForwardFlowRef{
			ForwardID:    forwardID,
			UserID:       userID,
			UserTunnelID: userTunnelID,
		}, nil
	}

	base := strings.TrimSuffix(trimmed, "-udp")
	if strings.HasPrefix(base, "panel-forward-") {
		forwardID, err := parsePanelForwardIDPart(strings.TrimPrefix(base, "panel-forward-"), "forward")
		if err != nil {
			return nil, err
		}
		return &panelForwardFlowRef{ForwardID: forwardID}, nil
	}

	return nil, fmt.Errorf("unsupported panel forward service name: %s", raw)
}

func parsePanelForwardIDPart(raw, field string) (uint, error) {
	value, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid %s id", field)
	}
	return uint(value), nil
}

func applyPanelForwardTrafficRatio(tunnel *model.ForwardTunnel, upload, download int64) (int64, int64) {
	if tunnel == nil {
		return upload, download
	}

	ratio := tunnel.TrafficRatio
	if ratio <= 0 {
		ratio = 1
	}

	flowType := tunnel.Flow
	if flowType <= 0 {
		flowType = 2
	}

	scaledUpload := int64(float64(upload) * ratio)
	scaledDownload := int64(float64(download) * ratio)
	return scaledUpload * int64(flowType), scaledDownload * int64(flowType)
}

func panelForwardTrafficLock(forwardID uint) *sync.Mutex {
	lock, _ := forwardTrafficLocks.LoadOrStore(forwardID, &sync.Mutex{})
	return lock.(*sync.Mutex)
}
