package cache

import (
	"container/list"
	"strings"
	"sync"
	"time"
)

// cacheEntry 缓存条目
type cacheEntry struct {
	value      any
	expiresAt  time.Time
	lastAccess time.Time
}

// MemoryCache 内存缓存实现（支持 TTL、LRU 淘汰、定期清理）
type MemoryCache struct {
	mu      sync.RWMutex
	items   map[string]*cacheEntry
	sets    map[string]map[string]struct{} // 集合数据
	lruList *list.List                     // LRU 链表，尾部是最久未使用的
	keyMap  map[string]*list.Element       // key -> *list.Element (指向 lruList 中的 *lruNode)
	maxSize int                            // 最大缓存条目数，0 表示无限制
	stopCh  chan struct{}
}

// lruNode LRU 链表节点
type lruNode struct {
	key string
}

// DefaultMaxSize 默认最大缓存条目数
const DefaultMaxSize = 10000

var memCache *MemoryCache

// InitMemory 初始化内存缓存
func InitMemory() {
	InitMemoryWithSize(DefaultMaxSize)
}

// InitMemoryWithSize 初始化指定最大容量的内存缓存
func InitMemoryWithSize(maxSize int) {
	if maxSize <= 0 {
		maxSize = DefaultMaxSize
	}
	memCache = &MemoryCache{
		items:   make(map[string]*cacheEntry),
		sets:    make(map[string]map[string]struct{}),
		lruList: list.New(),
		keyMap:  make(map[string]*list.Element),
		maxSize: maxSize,
		stopCh:  make(chan struct{}),
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
	for key, item := range c.items {
		if !item.expiresAt.IsZero() && now.After(item.expiresAt) {
			c.deleteInternal(key)
		}
	}
}

// deleteInternal 内部删除方法（调用者需持有写锁）
func (c *MemoryCache) deleteInternal(key string) {
	delete(c.items, key)
	delete(c.sets, key)
	if elem, ok := c.keyMap[key]; ok {
		c.lruList.Remove(elem)
		delete(c.keyMap, key)
	}
}

// evict 当缓存超出最大容量时淘汰最久未使用的条目（调用者需持有写锁）
func (c *MemoryCache) evict() {
	for c.lruList.Len() > c.maxSize && c.lruList.Len() > 0 {
		// 从链表尾部移除最久未使用的条目
		elem := c.lruList.Back()
		if elem != nil {
			node, ok := elem.Value.(*lruNode)
			if ok {
				c.deleteInternal(node.key)
			} else {
				// 异常情况：直接移除尾部元素并重新检查
				c.lruList.Remove(elem)
			}
		}
	}
}

// moveToFront 将访问的键移到 LRU 链表头部（调用者需持有写锁）
func (c *MemoryCache) moveToFront(key string) {
	if elem, ok := c.keyMap[key]; ok {
		c.lruList.MoveToFront(elem)
	}
}

// addToFront 将新键添加到 LRU 链表头部（调用者需持有写锁）
func (c *MemoryCache) addToFront(key string) {
	elem := c.lruList.PushFront(&lruNode{key: key})
	c.keyMap[key] = elem
}

// CloseMemory 关闭内存缓存
func CloseMemory() {
	if memCache != nil {
		select {
		case <-memCache.stopCh:
			// 已经关闭
		default:
			close(memCache.stopCh)
		}
	}
}

// Set 设置缓存（保持兼容原有签名）
func Set(key string, value any, expiration time.Duration) error {
	if memCache == nil {
		return nil // 缓存未初始化，直接返回
	}
	memCache.mu.Lock()
	defer memCache.mu.Unlock()

	var expiresAt time.Time
	if expiration > 0 {
		expiresAt = time.Now().Add(expiration)
	}

	now := time.Now()
	if _, exists := memCache.items[key]; !exists {
		// 新键，先检查是否需要淘汰
		memCache.items[key] = &cacheEntry{
			value:      value,
			expiresAt:  expiresAt,
			lastAccess: now,
		}
		memCache.addToFront(key)
		memCache.evict()
	} else {
		// 更新已有键
		memCache.items[key] = &cacheEntry{
			value:      value,
			expiresAt:  expiresAt,
			lastAccess: now,
		}
		memCache.moveToFront(key)
	}
	return nil
}

// GetString 获取字符串缓存
func GetString(key string) (string, error) {
	if memCache == nil {
		return "", ErrKeyNotFound
	}
	memCache.mu.Lock() // 需要写锁来更新 LRU
	defer memCache.mu.Unlock()

	item, ok := memCache.items[key]
	if !ok {
		return "", ErrKeyNotFound
	}

	if !item.expiresAt.IsZero() && time.Now().After(item.expiresAt) {
		memCache.deleteInternal(key)
		return "", ErrKeyNotFound
	}

	// 更新访问时间
	item.lastAccess = time.Now()
	memCache.moveToFront(key)

	if str, ok := item.value.(string); ok {
		return str, nil
	}
	return "", ErrKeyNotFound
}

// Get 获取缓存值
func Get(key string) (any, error) {
	if memCache == nil {
		return nil, ErrKeyNotFound
	}
	memCache.mu.Lock() // 需要写锁来更新 LRU
	defer memCache.mu.Unlock()

	item, ok := memCache.items[key]
	if !ok {
		return nil, ErrKeyNotFound
	}

	if !item.expiresAt.IsZero() && time.Now().After(item.expiresAt) {
		memCache.deleteInternal(key)
		return nil, ErrKeyNotFound
	}

	// 更新访问时间
	item.lastAccess = time.Now()
	memCache.moveToFront(key)

	return item.value, nil
}

// Delete 删除缓存（别名）
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
		memCache.deleteInternal(key)
	}
	return nil
}

// SAdd 集合添加
func SAdd(key string, members ...any) error {
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

	if item, ok := memCache.items[key]; ok {
		item.expiresAt = time.Now().Add(expiration)
		// 更新访问时间
		item.lastAccess = time.Now()
		memCache.moveToFront(key)
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

	now := time.Now()
	for key, item := range memCache.items {
		// 跳过已过期项
		if !item.expiresAt.IsZero() && now.After(item.expiresAt) {
			continue
		}
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
func HSet(key string, field string, value any) error {
	if memCache == nil {
		return nil
	}
	memCache.mu.Lock()
	defer memCache.mu.Unlock()

	hashKey := key + ":" + field
	if _, exists := memCache.items[hashKey]; !exists {
		memCache.items[hashKey] = &cacheEntry{
			value:      value,
			lastAccess: time.Now(),
		}
		memCache.addToFront(hashKey)
		memCache.evict()
	} else {
		memCache.items[hashKey].value = value
		memCache.items[hashKey].lastAccess = time.Now()
		memCache.moveToFront(hashKey)
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

	now := time.Now()
	for k, item := range memCache.items {
		// 跳过已过期项
		if !item.expiresAt.IsZero() && now.After(item.expiresAt) {
			continue
		}
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
		memCache.deleteInternal(hashKey)
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

	now := time.Now()
	if item, ok := memCache.items[key]; ok {
		if item.expiresAt.IsZero() || now.Before(item.expiresAt) {
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
	if memCache == nil {
		return
	}
	memCache.mu.Lock()
	defer memCache.mu.Unlock()

	memCache.items = make(map[string]*cacheEntry)
	memCache.sets = make(map[string]map[string]struct{})
	memCache.lruList.Init()
	memCache.keyMap = make(map[string]*list.Element)
}

// Incr 自增计数器
func Incr(key string) int64 {
	if memCache == nil {
		return 0
	}
	memCache.mu.Lock()
	defer memCache.mu.Unlock()

	var val int64 = 0
	if item, ok := memCache.items[key]; ok {
		if i, ok := item.value.(int64); ok {
			val = i
		}
	}
	val++
	if _, exists := memCache.items[key]; !exists {
		memCache.items[key] = &cacheEntry{value: val, lastAccess: time.Now()}
		memCache.addToFront(key)
		memCache.evict()
	} else {
		memCache.items[key].value = val
		memCache.items[key].lastAccess = time.Now()
		memCache.moveToFront(key)
	}
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
	if item, ok := memCache.items[key]; ok {
		if i, ok := item.value.(int64); ok {
			val = i
		}
	}
	val += delta
	if _, exists := memCache.items[key]; !exists {
		memCache.items[key] = &cacheEntry{value: val, lastAccess: time.Now()}
		memCache.addToFront(key)
		memCache.evict()
	} else {
		memCache.items[key].value = val
		memCache.items[key].lastAccess = time.Now()
		memCache.moveToFront(key)
	}
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

// Len 返回当前缓存条目数（用于测试和监控）
func Len() int {
	if memCache == nil {
		return 0
	}
	memCache.mu.RLock()
	defer memCache.mu.RUnlock()
	return len(memCache.items)
}

// StartCleanup 启动定期清理（如果尚未启动）
func StartCleanup() {
	// 该函数主要用于在 Stop 后重新启动清理
	// 正常情况下 InitMemory 已启动清理
	if memCache == nil {
		return
	}
	// 如果 stopCh 已关闭，重新创建并启动
	select {
	case <-memCache.stopCh:
		memCache.mu.Lock()
		memCache.stopCh = make(chan struct{})
		memCache.mu.Unlock()
		go memCache.cleanupLoop()
	default:
		// 已经在运行
	}
}

// Stop 停止定期清理
func Stop() {
	CloseMemory()
}
