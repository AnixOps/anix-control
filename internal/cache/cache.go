package cache

import "errors"

// ErrKeyNotFound 键不存在错误
var ErrKeyNotFound = errors.New("key not found")

// CacheType 缓存类型
type CacheType string

const (
	CacheTypeMemory CacheType = "memory"
	CacheTypeRedis  CacheType = "redis"
)

var currentCacheType CacheType = CacheTypeMemory

// GetCacheType 获取当前缓存类型
func GetCacheType() CacheType {
	return currentCacheType
}

// IsMemoryCache 是否使用内存缓存
func IsMemoryCache() bool {
	return currentCacheType == CacheTypeMemory
}

// IsRedisCache 是否使用Redis缓存
func IsRedisCache() bool {
	return currentCacheType == CacheTypeRedis
}
