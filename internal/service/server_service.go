package service

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/agentreports"
	"github.com/AnixOps/anix-control/v5/internal/cache"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/subscriber"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ServerService 服务器服务
type ServerService struct {
	db *gorm.DB
}

// NewServerService 创建服务器服务
func NewServerService() *ServerService {
	return &ServerService{
		db: database.Get(),
	}
}

// RecordTrafficLog 记录流量日志
func (s *ServerService) RecordTrafficLog(serverType model.ServerType, serverID uint, userID uint, upload, download int64, rate float64) error {
	if err := ValidateTrafficDelta(upload, download); err != nil {
		return err
	}
	rate = normalizeTrafficRate(rate)
	log := model.TrafficLog{
		UserID:     userID,
		ServerID:   serverID,
		ServerType: string(serverType),
		U:          upload,
		D:          download,
		Rate:       rate,
		LogAt:      model.TrafficLogBucketStart(time.Now().Unix()),
	}
	return addTrafficLogs(s.db, []model.TrafficLog{log})
}

// BatchRecordTrafficLog 批量记录流量日志
func (s *ServerService) BatchRecordTrafficLog(serverType model.ServerType, serverID uint, traffics map[uint][2]int64, rate float64) error {
	if len(traffics) == 0 {
		return nil
	}
	for _, traffic := range traffics {
		if err := ValidateTrafficDelta(traffic[0], traffic[1]); err != nil {
			return err
		}
	}
	rate = normalizeTrafficRate(rate)

	logs := make([]model.TrafficLog, 0, len(traffics))
	now := model.TrafficLogBucketStart(time.Now().Unix())

	for userID, traffic := range traffics {
		logs = append(logs, model.TrafficLog{
			UserID:     userID,
			ServerID:   serverID,
			ServerType: string(serverType),
			U:          traffic[0],
			D:          traffic[1],
			Rate:       rate,
			LogAt:      now,
		})
	}

	return addTrafficLogs(s.db, logs)
}

// trafficLogBatch is the number of v2_server_log rows of one INSERT.
const trafficLogBatch = 100

// addTrafficLogs adds traffic to hourly buckets of the traffic log: a row
// of a bucket that exists gets the bytes added to its u and d, any other is
// inserted. The rows' LogAt is the start of their hour
// (model.TrafficLogBucketStart) and the key of one call is unique per user,
// as PostgreSQL does not update a row twice in one statement.
func addTrafficLogs(tx *gorm.DB, logs []model.TrafficLog) error {
	if len(logs) == 0 {
		return nil
	}
	table := model.TrafficLog{}.TableName()
	columns := make([]clause.Column, len(model.TrafficLogBucketColumns))
	for i, name := range model.TrafficLogBucketColumns {
		columns[i] = clause.Column{Name: name}
	}
	// The table's name qualifies the old value: PostgreSQL rejects the bare
	// column as ambiguous with excluded. created_at moves to the report's
	// time, so a bucket's created_at is its latest report (the dashboard's
	// "latest report" reads it, GetTrafficLogMeta).
	return tx.Clauses(clause.OnConflict{
		Columns: columns,
		DoUpdates: clause.Set{
			{Column: clause.Column{Name: "u"}, Value: gorm.Expr(table + ".u + excluded.u")},
			{Column: clause.Column{Name: "d"}, Value: gorm.Expr(table + ".d + excluded.d")},
			{Column: clause.Column{Name: "created_at"}, Value: gorm.Expr("excluded.created_at")},
		},
	}).CreateInBatches(logs, trafficLogBatch).Error
}

// RecordNodeTrafficReport atomically applies a node traffic report. Node
// clients retry a report when the panel returns an error, so logs, node
// counters, daily/monthly stats, and user counters must commit together.
func (s *ServerService) RecordNodeTrafficReport(serverType model.ServerType, nodeID uint, traffics map[uint][2]int64, rate float64) error {
	if len(traffics) == 0 {
		return nil
	}
	now := time.Now()
	plan, err := planNodeTrafficReport(serverType, nodeID, traffics, rate, now)
	if err != nil {
		return err
	}
	if err := s.db.Transaction(plan.applyTx); err != nil {
		return err
	}
	subscriber.RecordOnline(s.db, trafficUsers(traffics), now)
	return nil
}

// trafficUsers lists the users a traffic report carried traffic for: the ones
// seen online.
func trafficUsers(traffics map[uint][2]int64) []uint {
	users := make([]uint, 0, len(traffics))
	for userID, traffic := range traffics {
		if traffic[0]+traffic[1] > 0 {
			users = append(users, userID)
		}
	}
	return users
}

// RecordAgentTrafficReport applies a TrafficReport batch from the Agent
// Control stream (reports.v1) exactly as RecordNodeTrafficReport applies the
// legacy report of the same node, once per node and batch id: the batch
// record (internal/agentreports) is claimed in the transaction that applies
// the traffic. It reports whether this call applied the batch; false means a
// committed transaction applied it before, and nothing was counted again.
func (s *ServerService) RecordAgentTrafficReport(nodeKind string, nodeID uint, batchID string, traffics map[uint][2]int64, rate float64) (bool, error) {
	now := time.Now()
	var plan *nodeTrafficReportPlan
	if len(traffics) > 0 {
		var err error
		plan, err = planNodeTrafficReport(model.ServerType("node"), nodeID, traffics, rate, now)
		if err != nil {
			return false, err
		}
	}
	applied := false
	err := s.db.Transaction(func(tx *gorm.DB) error {
		seen, err := agentreports.ClaimTx(tx, nodeKind, nodeID, batchID, agentreports.KindTraffic, now)
		if err != nil || seen {
			return err
		}
		applied = true
		if plan == nil {
			return nil
		}
		return plan.applyTx(tx)
	})
	if err == nil && applied {
		subscriber.RecordOnline(s.db, trafficUsers(traffics), now)
	}
	return applied, err
}

// nodeTrafficReportPlan is a validated node traffic report, ready to apply
// in one transaction.
type nodeTrafficReportPlan struct {
	serverType           model.ServerType
	nodeID               uint
	logs                 []model.TrafficLog
	entries              []subscriber.TrafficEntry
	totalUpload          int64
	totalDownload        int64
	now                  int64
	dayStart, monthStart int64
}

func planNodeTrafficReport(serverType model.ServerType, nodeID uint, traffics map[uint][2]int64, rate float64, at time.Time) (*nodeTrafficReportPlan, error) {
	rate = normalizeTrafficRate(rate)
	now := at.Unix()
	plan := &nodeTrafficReportPlan{serverType: serverType, nodeID: nodeID, now: now}
	bucket := model.TrafficLogBucketStart(now)
	plan.logs = make([]model.TrafficLog, 0, len(traffics))
	plan.entries = make([]subscriber.TrafficEntry, 0, len(traffics))
	for userID, traffic := range traffics {
		if err := ValidateTrafficDelta(traffic[0], traffic[1]); err != nil {
			return nil, err
		}
		upload := int64(float64(traffic[0]) * rate)
		download := int64(float64(traffic[1]) * rate)
		plan.entries = append(plan.entries, subscriber.TrafficEntry{UserID: userID, Upload: upload, Download: download})
		plan.totalUpload += upload
		plan.totalDownload += download
		plan.logs = append(plan.logs, model.TrafficLog{
			UserID:     userID,
			ServerID:   nodeID,
			ServerType: string(serverType),
			U:          traffic[0],
			D:          traffic[1],
			Rate:       rate,
			LogAt:      bucket,
		})
	}
	dayStart := time.Unix(now, 0).In(time.Local)
	dayStart = time.Date(dayStart.Year(), dayStart.Month(), dayStart.Day(), 0, 0, 0, 0, time.Local)
	plan.dayStart = dayStart.Unix()
	plan.monthStart = time.Date(dayStart.Year(), dayStart.Month(), 1, 0, 0, 0, 0, time.Local).Unix()
	return plan, nil
}

func (p *nodeTrafficReportPlan) applyTx(tx *gorm.DB) error {
	if err := addTrafficLogs(tx, p.logs); err != nil {
		return err
	}
	if p.totalUpload > 0 || p.totalDownload > 0 {
		if err := accumulateTrafficTx(tx, p.nodeID, p.totalUpload, p.totalDownload, false); err != nil {
			return err
		}
		if err := recordServerStatTx(tx, p.serverType, p.nodeID, p.totalUpload, p.totalDownload, "d", p.dayStart); err != nil {
			return err
		}
		if err := recordServerStatTx(tx, p.serverType, p.nodeID, p.totalUpload, p.totalDownload, "m", p.monthStart); err != nil {
			return err
		}
	}
	_, err := subscriber.RecordTrafficTx(tx, "", p.entries, time.Unix(p.now, 0))
	return err
}

// RecordServerStat 记录节点汇总统计，用于面板中的服务器统计表。
// recordType: d=日统计, m=月统计。
func (s *ServerService) RecordServerStat(serverType model.ServerType, serverID uint, upload, download int64, recordType string, recordAt int64) error {
	if upload == 0 && download == 0 {
		return nil
	}
	if recordType == "" {
		recordType = "d"
	}
	if recordAt == 0 {
		recordAt = time.Now().Unix()
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		return recordServerStatTx(tx, serverType, serverID, upload, download, recordType, recordAt)
	})
}

func recordServerStatTx(tx *gorm.DB, serverType model.ServerType, serverID uint, upload, download int64, recordType string, recordAt int64) error {
	var stat model.StatServer
	err := tx.Where("server_id = ? AND server_type = ? AND record_type = ? AND record_at = ?",
		serverID, string(serverType), recordType, recordAt).First(&stat).Error
	switch err {
	case nil:
		return tx.Model(&stat).Updates(map[string]any{
			"u": gorm.Expr("u + ?", upload),
			"d": gorm.Expr("d + ?", download),
		}).Error
	case gorm.ErrRecordNotFound:
		stat = model.StatServer{
			ServerID:   serverID,
			ServerType: string(serverType),
			U:          upload,
			D:          download,
			RecordType: recordType,
			RecordAt:   recordAt,
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&stat).Error
	default:
		return err
	}
}

// UpdateOnlineStatus 更新用户在线状态
func (s *ServerService) UpdateOnlineStatus(serverType model.ServerType, serverID uint, userIPs map[uint][]string) error {
	cacheKey := fmt.Sprintf("online:%s:%d", serverType, serverID)

	// 清除旧的在线记录
	oldKeys, err := cache.Keys(cacheKey + ":*")
	if err != nil {
		return fmt.Errorf("list online cache keys: %w", err)
	}
	if len(oldKeys) > 0 {
		if err := cache.Del(oldKeys...); err != nil {
			return fmt.Errorf("delete old online cache keys: %w", err)
		}
	}

	// 记录新的在线IP
	for userID, ips := range userIPs {
		if len(ips) > 0 {
			key := fmt.Sprintf("%s:%d", cacheKey, userID)
			if err := cache.SAdd(key, ipsToInterface(ips)...); err != nil {
				return fmt.Errorf("store online ips for user %d: %w", userID, err)
			}
			if err := cache.Expire(key, 5*time.Minute); err != nil {
				return fmt.Errorf("expire online ips for user %d: %w", userID, err)
			}
		}
	}

	online := make([]uint, 0, len(userIPs))
	for userID, ips := range userIPs {
		if len(ips) > 0 {
			online = append(online, userID)
		}
	}
	subscriber.RecordOnline(s.db, online, time.Now())
	return nil
}

// GetUserOnlineCount 获取用户在线IP数量（跨所有节点）
func (s *ServerService) GetUserOnlineCount(userID uint) (int64, error) {
	pattern := fmt.Sprintf("online:*:*:%d", userID)
	keys, err := cache.Keys(pattern)
	if err != nil {
		return 0, err
	}

	var total int64
	for _, key := range keys {
		count, err := cache.SCard(key)
		if err == nil {
			total += count
		}
	}
	return total, nil
}

// UserAliveCounts counts each user's online IPs across all nodes: the sets
// UpdateOnlineStatus keeps per node and user (online:<type>:<node>:<user>),
// added up per user. Users without any are left out. It is the source of
// the Agent Control stream's AliveList (alive.v1).
func UserAliveCounts() (map[uint64]int, error) {
	result := make(map[uint64]int)
	// "online:*" and not "online:*:*:*": the memory cache matches a
	// pattern by the prefix before its first "*" only.
	keys, err := cache.Keys("online:*")
	if err != nil {
		return result, err
	}
	for _, key := range keys {
		parts := strings.Split(key, ":")
		if len(parts) != 4 || parts[0] != "online" {
			continue
		}
		userID, err := strconv.ParseUint(parts[3], 10, 64)
		if err != nil || userID == 0 {
			continue
		}
		count, err := cache.SCard(key)
		if err == nil && count > 0 {
			result[userID] += int(count)
		}
	}
	return result, nil
}

func normalizeTrafficRate(rate float64) float64 {
	if rate <= 0 {
		return 1.0
	}
	return rate
}

func ipsToInterface(ips []string) []any {
	result := make([]any, len(ips))
	for i, ip := range ips {
		result[i] = ip
	}
	return result
}
