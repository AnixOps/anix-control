package service

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/anixops/v2board/internal/cache"
	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
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

// GetServerByTypeAndID 根据类型和ID获取服务器
func (s *ServerService) GetServerByTypeAndID(serverType model.ServerType, serverID uint) (any, error) {
	switch serverType {
	case model.ServerTypeVMess:
		var server model.ServerVMess
		if err := s.db.First(&server, serverID).Error; err != nil {
			return nil, err
		}
		return &server, nil
	case model.ServerTypeVLESS:
		var server model.ServerVLESS
		if err := s.db.First(&server, serverID).Error; err != nil {
			return nil, err
		}
		return &server, nil
	case model.ServerTypeTrojan:
		var server model.ServerTrojan
		if err := s.db.First(&server, serverID).Error; err != nil {
			return nil, err
		}
		return &server, nil
	case model.ServerTypeShadowsocks:
		var server model.ServerShadowsocks
		if err := s.db.First(&server, serverID).Error; err != nil {
			return nil, err
		}
		return &server, nil
	case model.ServerTypeHysteria, model.ServerTypeHysteria2:
		var server model.ServerHysteria
		if err := s.db.First(&server, serverID).Error; err != nil {
			return nil, err
		}
		return &server, nil
	case model.ServerTypeTUIC:
		var server model.ServerTUIC
		if err := s.db.First(&server, serverID).Error; err != nil {
			return nil, err
		}
		return &server, nil
	case model.ServerTypeAnyTLS:
		var server model.ServerAnyTLS
		if err := s.db.First(&server, serverID).Error; err != nil {
			return nil, err
		}
		return &server, nil
	}
	return nil, fmt.Errorf("unsupported server type: %s", serverType)
}

// GetRoutesByIDs 根据ID列表获取路由
func (s *ServerService) GetRoutesByIDs(ids []uint) ([]model.ServerRoute, error) {
	var routes []model.ServerRoute
	if len(ids) == 0 {
		return routes, nil
	}
	err := s.db.Where("id IN ?", ids).Find(&routes).Error
	return routes, err
}

// GetServerUsers 获取服务器可用用户列表
func (s *ServerService) GetServerUsers(serverType model.ServerType, serverID uint) ([]model.User, error) {
	// 获取服务器信息
	server, err := s.GetServerByTypeAndID(serverType, serverID)
	if err != nil {
		return nil, err
	}

	// 获取服务器分组IDs
	var groupIDs []uint
	switch sv := server.(type) {
	case *model.ServerVMess:
		groupIDs = sv.GetGroupIDs()
	case *model.ServerVLESS:
		groupIDs = sv.GetGroupIDs()
	case *model.ServerTrojan:
		groupIDs = sv.GetGroupIDs()
	case *model.ServerShadowsocks:
		groupIDs = sv.GetGroupIDs()
	case *model.ServerHysteria:
		groupIDs = sv.GetGroupIDs()
	case *model.ServerTUIC:
		groupIDs = sv.GetGroupIDs()
	case *model.ServerAnyTLS:
		groupIDs = sv.GetGroupIDs()
	}

	// 获取有效用户
	var users []model.User
	now := time.Now().Unix()
	query := s.db.Preload("Plan").
		Where("banned = 0").
		Where("(expired_at IS NULL OR expired_at > ?)", now).
		Where("(u + d) < transfer_enable")

	if len(groupIDs) > 0 {
		query = query.Where("group_id IN ?", groupIDs)
	}

	err = query.Find(&users).Error
	return users, err
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
		LogAt:      time.Now().Unix(),
	}
	return s.db.Create(&log).Error
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
	now := time.Now().Unix()

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

	return s.db.CreateInBatches(logs, 100).Error
}

// RecordNodeTrafficReport atomically applies a node traffic report. Node
// clients retry a report when the panel returns an error, so logs, node
// counters, daily/monthly stats, and user counters must commit together.
func (s *ServerService) RecordNodeTrafficReport(serverType model.ServerType, nodeID uint, traffics map[uint][2]int64, rate float64) error {
	if len(traffics) == 0 {
		return nil
	}
	rate = normalizeTrafficRate(rate)

	logs := make([]model.TrafficLog, 0, len(traffics))
	userTraffics := make(map[uint][2]int64, len(traffics))
	var totalUpload, totalDownload int64
	now := time.Now().Unix()
	for userID, traffic := range traffics {
		if err := ValidateTrafficDelta(traffic[0], traffic[1]); err != nil {
			return err
		}
		upload := int64(float64(traffic[0]) * rate)
		download := int64(float64(traffic[1]) * rate)
		userTraffics[userID] = [2]int64{upload, download}
		totalUpload += upload
		totalDownload += download
		logs = append(logs, model.TrafficLog{
			UserID:     userID,
			ServerID:   nodeID,
			ServerType: string(serverType),
			U:          traffic[0],
			D:          traffic[1],
			Rate:       rate,
			LogAt:      now,
		})
	}

	dayStart := time.Unix(now, 0).In(time.Local)
	dayStart = time.Date(dayStart.Year(), dayStart.Month(), dayStart.Day(), 0, 0, 0, 0, time.Local)
	monthStart := time.Date(dayStart.Year(), dayStart.Month(), 1, 0, 0, 0, 0, time.Local)
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.CreateInBatches(logs, 100).Error; err != nil {
			return err
		}
		if totalUpload > 0 || totalDownload > 0 {
			if err := accumulateTrafficTx(tx, nodeID, totalUpload, totalDownload, false); err != nil {
				return err
			}
			if err := recordServerStatTx(tx, serverType, nodeID, totalUpload, totalDownload, "d", dayStart.Unix()); err != nil {
				return err
			}
			if err := recordServerStatTx(tx, serverType, nodeID, totalUpload, totalDownload, "m", monthStart.Unix()); err != nil {
				return err
			}
		}
		for userID, traffic := range userTraffics {
			if err := tx.Model(&model.User{}).Where("id = ?", userID).
				Updates(map[string]any{
					"u": gorm.Expr("u + ?", traffic[0]),
					"d": gorm.Expr("d + ?", traffic[1]),
				}).Error; err != nil {
				return err
			}
		}
		return nil
	})
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

// GetAllUsersOnlineCount 获取所有用户的在线IP数量
func (s *ServerService) GetAllUsersOnlineCount() (map[string]int, error) {
	result := make(map[string]int)

	// 获取所有在线记录的键
	keys, err := cache.Keys("online:*:*:*")
	if err != nil {
		return result, err
	}

	// 统计每个用户的在线IP数
	for _, key := range keys {
		parts := strings.Split(key, ":")
		if len(parts) < 4 {
			continue
		}
		userID := parts[len(parts)-1]
		count, err := cache.SCard(key)
		if err == nil {
			result[userID] += int(count)
		}
	}

	return result, nil
}

// BuildNodeConfig 构建节点配置
func (s *ServerService) BuildNodeConfig(serverType model.ServerType, serverID uint) (map[string]any, error) {
	server, err := s.GetServerByTypeAndID(serverType, serverID)
	if err != nil {
		return nil, err
	}

	config := make(map[string]any)

	// V2bX 必需字段：node_type 和 type
	nodeTypeStr := string(serverType)
	config["node_type"] = nodeTypeStr
	config["type"] = nodeTypeStr
	config["send_through"] = "0.0.0.0"

	switch sv := server.(type) {
	case *model.ServerVMess:
		config["host"] = sv.Host
		config["server_port"] = sv.ServerPort
		config["server_name"] = sv.Host
		config["tls"] = sv.TLS
		config["network"] = sv.Network
		if sv.TLSSettings != nil {
			var tlsSettings map[string]any
			if err := decodeConfigJSON("tls_settings", *sv.TLSSettings, &tlsSettings); err != nil {
				return nil, err
			}
			config["tls_settings"] = tlsSettings
		}
		if sv.NetworkSettings != nil {
			var networkSettings map[string]any
			if err := decodeConfigJSON("network_settings", *sv.NetworkSettings, &networkSettings); err != nil {
				return nil, err
			}
			config["network_settings"] = networkSettings
		}
		if err := s.addRoutesAndBaseConfig(config, sv.GetRouteIDs()); err != nil {
			return nil, err
		}

	case *model.ServerVLESS:
		config["host"] = sv.Host
		config["server_port"] = sv.ServerPort
		config["server_name"] = sv.Host
		config["tls"] = sv.TLS
		config["network"] = sv.Network
		config["flow"] = sv.Flow
		if sv.TLSSettings != nil {
			var tlsSettings map[string]any
			if err := decodeConfigJSON("tls_settings", *sv.TLSSettings, &tlsSettings); err != nil {
				return nil, err
			}
			config["tls_settings"] = tlsSettings
		}
		if sv.NetworkSettings != nil {
			var networkSettings map[string]any
			if err := decodeConfigJSON("network_settings", *sv.NetworkSettings, &networkSettings); err != nil {
				return nil, err
			}
			config["network_settings"] = networkSettings
		}
		if sv.Encryption != nil {
			config["encryption"] = *sv.Encryption
		}
		if sv.EncryptionSettings != nil {
			var encSettings map[string]any
			if err := decodeConfigJSON("encryption_settings", *sv.EncryptionSettings, &encSettings); err != nil {
				return nil, err
			}
			config["encryption_settings"] = encSettings
		}
		if err := s.addRoutesAndBaseConfig(config, sv.GetRouteIDs()); err != nil {
			return nil, err
		}

	case *model.ServerTrojan:
		config["host"] = sv.Host
		config["server_port"] = sv.ServerPort
		if sv.ServerName != nil {
			config["server_name"] = *sv.ServerName
		} else {
			config["server_name"] = sv.Host
		}
		config["network"] = sv.Network
		if sv.NetworkSettings != nil {
			var networkSettings map[string]any
			if err := decodeConfigJSON("network_settings", *sv.NetworkSettings, &networkSettings); err != nil {
				return nil, err
			}
			config["networkSettings"] = networkSettings
		}
		if err := s.addRoutesAndBaseConfig(config, sv.GetRouteIDs()); err != nil {
			return nil, err
		}

	case *model.ServerShadowsocks:
		config["host"] = sv.Host
		config["server_port"] = sv.ServerPort
		config["server_name"] = sv.Host
		config["cipher"] = sv.Cipher
		if sv.ServerKey != nil {
			config["server_key"] = *sv.ServerKey
		}
		if err := s.addRoutesAndBaseConfig(config, sv.GetRouteIDs()); err != nil {
			return nil, err
		}

	case *model.ServerHysteria:
		config["host"] = sv.Host
		config["server_port"] = sv.ServerPort
		if sv.ServerName != nil {
			config["server_name"] = *sv.ServerName
		} else {
			config["server_name"] = sv.Host
		}
		if sv.Version == 2 {
			config["ignore_client_bandwidth"] = sv.IgnoreClientBandwidth == 1
			config["up_mbps"] = sv.UpMbps
			config["down_mbps"] = sv.DownMbps
			config["obfs"] = sv.Obfs
			if sv.ObfsPassword != nil {
				config["obfs-password"] = *sv.ObfsPassword
			}
		} else {
			config["up_mbps"] = sv.UpMbps
			config["down_mbps"] = sv.DownMbps
			config["obfs"] = sv.Obfs
		}
		if err := s.addRoutesAndBaseConfig(config, sv.GetRouteIDs()); err != nil {
			return nil, err
		}

	case *model.ServerTUIC:
		config["host"] = sv.Host
		config["server_port"] = sv.ServerPort
		if sv.ServerName != nil {
			config["server_name"] = *sv.ServerName
		} else {
			config["server_name"] = sv.Host
		}
		config["congestion_control"] = sv.CongestionControl
		config["zero_rtt_handshake"] = sv.ZeroRTTHandshake == 1
		if err := s.addRoutesAndBaseConfig(config, sv.GetRouteIDs()); err != nil {
			return nil, err
		}

	case *model.ServerAnyTLS:
		config["host"] = sv.Host
		config["server_port"] = sv.ServerPort
		if sv.ServerName != nil {
			config["server_name"] = *sv.ServerName
		} else {
			config["server_name"] = sv.Host
		}
		if sv.PaddingScheme != nil {
			var paddingScheme []string
			if err := decodeConfigJSON("padding_scheme", *sv.PaddingScheme, &paddingScheme); err != nil {
				return nil, err
			}
			config["padding_scheme"] = paddingScheme
		}
		if err := s.addRoutesAndBaseConfig(config, sv.GetRouteIDs()); err != nil {
			return nil, err
		}
	}

	return config, nil
}

// addRoutesAndBaseConfig 添加路由和基础配置
func (s *ServerService) addRoutesAndBaseConfig(config map[string]any, routeIDs []uint) error {
	// 添加路由
	routes, err := s.GetRoutesByIDs(routeIDs)
	if err != nil {
		return fmt.Errorf("load server routes: %w", err)
	}
	routeList := make([]map[string]any, 0, len(routes))
	for _, route := range routes {
		routeList = append(routeList, map[string]any{
			"id":           route.ID,
			"match":        route.Match,
			"action":       route.Action,
			"action_value": route.ActionValue,
		})
	}
	config["routes"] = routeList

	// 添加基础配置
	config["base_config"] = map[string]any{
		"push_interval": 60,
		"pull_interval": 60,
	}
	return nil
}

// GetServerRate 获取服务器流量倍率
func (s *ServerService) GetServerRate(serverType model.ServerType, serverID uint) float64 {
	server, err := s.GetServerByTypeAndID(serverType, serverID)
	if err != nil {
		return 1.0
	}

	switch sv := server.(type) {
	case *model.ServerVMess:
		return normalizeTrafficRate(sv.Rate)
	case *model.ServerVLESS:
		return normalizeTrafficRate(sv.Rate)
	case *model.ServerTrojan:
		return normalizeTrafficRate(sv.Rate)
	case *model.ServerShadowsocks:
		return normalizeTrafficRate(sv.Rate)
	case *model.ServerHysteria:
		return normalizeTrafficRate(sv.Rate)
	case *model.ServerTUIC:
		return normalizeTrafficRate(sv.Rate)
	case *model.ServerAnyTLS:
		return normalizeTrafficRate(sv.Rate)
	}
	return 1.0
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

func decodeConfigJSON(field string, raw string, target any) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(raw), target); err != nil {
		return fmt.Errorf("invalid %s JSON: %w", field, err)
	}
	return nil
}

// ParseTrafficData 解析流量数据
func ParseTrafficData(data map[string]any) (map[uint][2]int64, error) {
	result := make(map[uint][2]int64)
	for k, v := range data {
		userID, err := strconv.ParseUint(k, 10, 32)
		if err != nil {
			continue
		}

		traffic, ok := v.([]any)
		if !ok || len(traffic) < 2 {
			continue
		}

		upload, uploadOK := toInt64(traffic[0])
		download, downloadOK := toInt64(traffic[1])
		if !uploadOK || !downloadOK {
			continue
		}
		if err := ValidateTrafficDelta(upload, download); err != nil {
			return nil, err
		}

		result[uint(userID)] = [2]int64{upload, download}
	}
	return result, nil
}

// ParseOnlineData 解析在线数据
func ParseOnlineData(data map[string]any) (map[uint][]string, error) {
	result := make(map[uint][]string)
	for k, v := range data {
		userID, err := strconv.ParseUint(k, 10, 32)
		if err != nil {
			continue
		}

		ipsRaw, ok := v.([]any)
		if !ok {
			continue
		}

		ips := make([]string, 0, len(ipsRaw))
		for _, ip := range ipsRaw {
			if ipStr, ok := ip.(string); ok {
				ips = append(ips, ipStr)
			}
		}

		result[uint(userID)] = ips
	}
	return result, nil
}

func toInt64(v any) (int64, bool) {
	switch val := v.(type) {
	case float64:
		return int64(val), true
	case int64:
		return val, true
	case int:
		return int64(val), true
	case json.Number:
		i, err := val.Int64()
		return i, err == nil
	}
	return 0, false
}
