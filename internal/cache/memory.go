package cache

import (
	"strings"
	"sync"
	"time"
)

// MemoryCache 内存缓存实现
type MemoryCache struct {
	data   map[string]*cacheItem
	sets   map[string]map[string]struct{} // 集合数据
	mu     sync.RWMutex
	stopCh chan struct{}
}

type cacheItem struct {
	value     interface{}
	expiresAt time.Time
}

var memCache *MemoryCache

// InitMemory 初始化内存缓存
func InitMemory() {
	memCache = &MemoryCache{
		data:   make(map[string]*cacheItem),
		sets:   make(map[string]map[string]struct{}),
		stopCh: make(chan struct{}),
	}

	// 启动过期清理协程
	go memCache.cleanupLoop()
}

// cleanupLoop 定期清理过期数据
func (c *MemoryCache) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.cleanup()
		case <-c.stopCh:
			return
		}
	}
}

// cleanup 清理过期项
func (c *MemoryCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for key, item := range c.data {
		if !item.expiresAt.IsZero() && now.After(item.expiresAt) {
			delete(c.data, key)
		}
	}
}

// CloseMemory 关闭内存缓存
func CloseMemory() {
	if memCache != nil {
		close(memCache.stopCh)
	}
}

// Set 设置缓存
func Set(key string, value interface{}, expiration time.Duration) error {
	if memCache == nil {
		return nil // 缓存未初始化，直接返回
	}
	memCache.mu.Lock()
	defer memCache.mu.Unlock()

	var expiresAt time.Time
	if expiration > 0 {
		expiresAt = time.Now().Add(expiration)
	}

	memCache.data[key] = &cacheItem{
		value:     value,
		expiresAt: expiresAt,
	}
	return nil
}

// GetString 获取字符串缓存
func GetString(key string) (string, error) {
	if memCache == nil {
		return "", ErrKeyNotFound
	}
	memCache.mu.RLock()
	defer memCache.mu.RUnlock()

	item, ok := memCache.data[key]
	if !ok {
		return "", ErrKeyNotFound
	}

	if !item.expiresAt.IsZero() && time.Now().After(item.expiresAt) {
		return "", ErrKeyNotFound
	}

	if str, ok := item.value.(string); ok {
		return str, nil
	}
	return "", ErrKeyNotFound
}

// Get 获取缓存值
func Get(key string) (interface{}, error) {
	if memCache == nil {
		return nil, ErrKeyNotFound
	}
	memCache.mu.RLock()
	defer memCache.mu.RUnlock()

	item, ok := memCache.data[key]
	if !ok {
		return nil, ErrKeyNotFound
	}

	if !item.expiresAt.IsZero() && time.Now().After(item.expiresAt) {
		return nil, ErrKeyNotFound
	}

	return item.value, nil
}

// Delete 删除单个缓存 (别名)
func Delete(key string) error {
	return Del(key)
}

// Del 删除缓存
func Del(keys ...string) error {
	if memCache == nil {
		return nil // 缓存未初始化，直接返回
	}
	memCache.mu.Lock()
	defer memCache.mu.Unlock()

	for _, key := range keys {
		delete(memCache.data, key)
		delete(memCache.sets, key)
	}
	return nil
}

// SAdd 集合添加
func SAdd(key string, members ...interface{}) error {
	if memCache == nil {
		return nil
	}
	memCache.mu.Lock()
	defer memCache.mu.Unlock()

	if memCache.sets[key] == nil {
		memCache.sets[key] = make(map[string]struct{})
	}

	for _, member := range members {
		if str, ok := member.(string); ok {
			memCache.sets[key][str] = struct{}{}
		}
	}
	return nil
}

// SMembers 获取集合成员
func SMembers(key string) ([]string, error) {
	if memCache == nil {
		return []string{}, nil
	}
	memCache.mu.RLock()
	defer memCache.mu.RUnlock()

	set, ok := memCache.sets[key]
	if !ok {
		return []string{}, nil
	}

	result := make([]string, 0, len(set))
	for member := range set {
		result = append(result, member)
	}
	return result, nil
}

// SCard 获取集合大小
func SCard(key string) (int64, error) {
	if memCache == nil {
		return 0, nil
	}
	memCache.mu.RLock()
	defer memCache.mu.RUnlock()

	set, ok := memCache.sets[key]
	if !ok {
		return 0, nil
	}
	return int64(len(set)), nil
}

// Expire 设置过期时间
func Expire(key string, expiration time.Duration) error {
	if memCache == nil {
		return nil
	}
	memCache.mu.Lock()
	defer memCache.mu.Unlock()

	if item, ok := memCache.data[key]; ok {
		item.expiresAt = time.Now().Add(expiration)
	}
	return nil
}

// Keys 获取匹配的键（支持简单的 * 通配符）
func Keys(pattern string) ([]string, error) {
	if memCache == nil {
		return []string{}, nil
	}
	memCache.mu.RLock()
	defer memCache.mu.RUnlock()

	result := make([]string, 0)

	// 简单的通配符匹配
	prefix := strings.TrimSuffix(pattern, "*")
	hasWildcard := strings.Contains(pattern, "*")

	for key := range memCache.data {
		if hasWildcard {
			if strings.HasPrefix(key, prefix) {
				result = append(result, key)
			}
		} else if key == pattern {
			result = append(result, key)
		}
	}

	for key := range memCache.sets {
		if hasWildcard {
			if strings.HasPrefix(key, prefix) {
				result = append(result, key)
			}
		} else if key == pattern {
			result = append(result, key)
		}
	}

	return result, nil
}

// HSet 设置Hash字段
func HSet(key string, field string, value interface{}) error {
	if memCache == nil {
		return nil
	}
	memCache.mu.Lock()
	defer memCache.mu.Unlock()

	hashKey := key + ":" + field
	memCache.data[hashKey] = &cacheItem{
		value: value,
	}
	return nil
}

// HGet 获取Hash字段
func HGet(key string, field string) (string, error) {
	hashKey := key + ":" + field
	return GetString(hashKey)
}

// HGetAll 获取所有Hash字段
func HGetAll(key string) (map[string]string, error) {
	if memCache == nil {
		return map[string]string{}, nil
	}
	memCache.mu.RLock()
	defer memCache.mu.RUnlock()

	result := make(map[string]string)
	prefix := key + ":"

	for k, item := range memCache.data {
		if strings.HasPrefix(k, prefix) {
			field := strings.TrimPrefix(k, prefix)
			if str, ok := item.value.(string); ok {
				result[field] = str
			}
		}
	}
	return result, nil
}

// HDel 删除Hash字段
func HDel(key string, fields ...string) error {
	if memCache == nil {
		return nil
	}
	memCache.mu.Lock()
	defer memCache.mu.Unlock()

	for _, field := range fields {
		hashKey := key + ":" + field
		delete(memCache.data, hashKey)
	}
	return nil
}

// SClear 清空集合
func SClear(key string) error {
	if memCache == nil {
		return nil
	}
	memCache.mu.Lock()
	defer memCache.mu.Unlock()

	delete(memCache.sets, key)
	return nil
}

// Exists 检查键是否存在
func Exists(key string) bool {
	if memCache == nil {
		return false
	}
	memCache.mu.RLock()
	defer memCache.mu.RUnlock()

	if item, ok := memCache.data[key]; ok {
		if item.expiresAt.IsZero() || time.Now().Before(item.expiresAt) {
			return true
		}
	}
	if _, ok := memCache.sets[key]; ok {
		return true
	}
	return false
}

// Clear 清空所有缓存
func Clear() {
	memCache.mu.Lock()
	defer memCache.mu.Unlock()

	memCache.data = make(map[string]*cacheItem)
	memCache.sets = make(map[string]map[string]struct{})
}

// Incr 自增计数器
func Incr(key string) int64 {
	if memCache == nil {
		return 0
	}
	memCache.mu.Lock()
	defer memCache.mu.Unlock()

	var val int64 = 0
	if item, ok := memCache.data[key]; ok {
		if i, ok := item.value.(int64); ok {
			val = i
		}
	}
	val++
	memCache.data[key] = &cacheItem{value: val}
	return val
}

// IncrBy 自增指定值
func IncrBy(key string, delta int64) int64 {
	if memCache == nil {
		return 0
	}
	memCache.mu.Lock()
	defer memCache.mu.Unlock()

	var val int64 = 0
	if item, ok := memCache.data[key]; ok {
		if i, ok := item.value.(int64); ok {
			val = i
		}
	}
	val += delta
	memCache.data[key] = &cacheItem{value: val}
	return val
}

// Decr 自减计数器
func Decr(key string) int64 {
	return IncrBy(key, -1)
}

// SetInt 设置整数
func SetInt(key string, value int64, expiration time.Duration) error {
	return Set(key, value, expiration)
}

// GetInt 获取整数
func GetInt(key string) (int64, error) {
	val, err := Get(key)
	if err != nil {
		return 0, err
	}
	if i, ok := val.(int64); ok {
		return i, nil
	}
	return 0, ErrKeyNotFound
}
