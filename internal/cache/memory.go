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
	doneCh  chan struct{}
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
	CloseMemory()
	cache := &MemoryCache{
		items:   make(map[string]*cacheEntry),
		sets:    make(map[string]map[string]struct{}),
		lruList: list.New(),
		keyMap:  make(map[string]*list.Element),
		maxSize: maxSize,
		stopCh:  make(chan struct{}),
		doneCh:  make(chan struct{}),
	}
	memCache = cache

	// 启动过期清理协程
	go cache.cleanupLoop()
}

// cleanupLoop 定期清理过期数据
func (c *MemoryCache) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	defer close(c.doneCh)

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
		memCache.close()
	}
}

func (c *MemoryCache) close() {
	select {
	case <-c.stopCh:
		// 已经关闭
	default:
		close(c.stopCh)
	}
	<-c.doneCh
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
