package cache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	InitMemory()
	defer CloseMemory()
	m.Run()
}

func mustSet(t *testing.T, key string, value any, expiration time.Duration) {
	t.Helper()
	require.NoError(t, Set(key, value, expiration))
}

func mustSAdd(t *testing.T, key string, members ...any) {
	t.Helper()
	require.NoError(t, SAdd(key, members...))
}

func mustGet(t *testing.T, key string) any {
	t.Helper()
	value, err := Get(key)
	require.NoError(t, err)
	return value
}

func assertPresent(t *testing.T, key string) {
	t.Helper()
	_, err := Get(key)
	assert.NoError(t, err, key)
}

func assertMissing(t *testing.T, key string) {
	t.Helper()
	_, err := Get(key)
	assert.ErrorIs(t, err, ErrKeyNotFound, key)
}

// cacheLen reports the number of stored entries for LRU assertions.
func cacheLen() int {
	if memCache == nil {
		return 0
	}
	memCache.mu.RLock()
	defer memCache.mu.RUnlock()
	return len(memCache.items)
}

func TestSetAndGet(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{"simple string", "key1", "value1"},
		{"empty value", "key2", ""},
		{"special chars", "key3", "hello@world!#$%"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Set(tt.key, tt.value, 0)
			require.NoError(t, err)

			assert.Equal(t, tt.value, mustGet(t, tt.key))
		})
	}
}

func TestSetWithExpiration(t *testing.T) {
	err := Set("expiring_key", "value", 100*time.Millisecond)
	require.NoError(t, err)

	// 立即获取应该成功
	assert.Equal(t, "value", mustGet(t, "expiring_key"))

	// 等待过期
	time.Sleep(150 * time.Millisecond)

	// 过期后应该找不到
	_, err = Get("expiring_key")
	assert.Equal(t, ErrKeyNotFound, err)
}

func TestGetNotFound(t *testing.T) {
	_, err := Get("nonexistent_key")
	assert.Equal(t, ErrKeyNotFound, err)
}

func TestDel(t *testing.T) {
	mustSet(t, "del_key", "value", 0)

	err := Del("del_key")
	require.NoError(t, err)

	_, err = Get("del_key")
	assert.Equal(t, ErrKeyNotFound, err)
}

func TestDelMultiple(t *testing.T) {
	mustSet(t, "key1", "value1", 0)
	mustSet(t, "key2", "value2", 0)
	mustSet(t, "key3", "value3", 0)

	err := Del("key1", "key2")
	require.NoError(t, err)

	assertMissing(t, "key1")
	assertMissing(t, "key2")
	assertPresent(t, "key3")
}

func TestDelete(t *testing.T) {
	mustSet(t, "delete_key", "value", 0)

	err := Delete("delete_key")
	require.NoError(t, err)

	assertMissing(t, "delete_key")
}

func TestSCard(t *testing.T) {
	mustSAdd(t, "set2", "a", "b", "c")

	count, err := SCard("set2")
	require.NoError(t, err)
	assert.Equal(t, int64(3), count)
}

func TestKeys(t *testing.T) {
	mustSet(t, "keys_test_key1", "v1", 0)
	mustSet(t, "keys_test_key2", "v2", 0)
	mustSet(t, "keys_other_key", "v3", 0)

	keys, err := Keys("keys_test_*")
	require.NoError(t, err)
	assert.Len(t, keys, 2)
	assert.Contains(t, keys, "keys_test_key1")
	assert.Contains(t, keys, "keys_test_key2")
}

func TestExpire(t *testing.T) {
	mustSet(t, "expire_key", "value", 0)

	err := Expire("expire_key", 100*time.Millisecond)
	require.NoError(t, err)

	// 立即应该存在
	assertPresent(t, "expire_key")

	// 等待过期
	time.Sleep(150 * time.Millisecond)

	assertMissing(t, "expire_key")
}

func TestGetGeneric(t *testing.T) {
	mustSet(t, "generic_key", "string_value", 0)

	val, err := Get("generic_key")
	require.NoError(t, err)
	assert.Equal(t, "string_value", val)
}

func TestNilCacheSafety(t *testing.T) {
	// 保存当前缓存
	oldCache := memCache
	memCache = nil
	defer func() { memCache = oldCache }()

	// 所有操作应该安全返回而不 panic
	_ = Set("key", "value", 0)
	_, _ = Get("key")
	_ = Del("key")
	_ = SAdd("set", "member")
	_, _ = SCard("set")
	_, _ = Keys("*")
	_, err := Get("any_key")
	assert.ErrorIs(t, err, ErrKeyNotFound)
}

func TestConcurrentAccess(t *testing.T) {
	const workers = 10
	const iterations = 100

	done := make(chan bool, workers)
	errCh := make(chan error, workers*iterations*3)

	// 并发写入
	for i := 0; i < workers; i++ {
		go func(id int) {
			for j := 0; j < iterations; j++ {
				key := "concurrent_" + string(rune('A'+id))
				if err := Set(key, j, 0); err != nil {
					errCh <- err
				}
				if _, err := Get(key); err != nil {
					errCh <- err
				}
				if err := Del(key); err != nil {
					errCh <- err
				}
			}
			done <- true
		}(i)
	}

	// 等待所有 goroutine 完成
	for i := 0; i < workers; i++ {
		<-done
	}
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}
}

func TestLRUEviction(t *testing.T) {
	// 使用小容量缓存测试 LRU 淘汰
	oldCache := memCache
	defer func() { memCache = oldCache }()

	InitMemoryWithSize(5)
	defer CloseMemory()

	// 插入 5 个条目（填满）
	for i := 0; i < 5; i++ {
		mustSet(t, "key_"+string(rune('A'+i)), i, 0)
	}
	assert.Equal(t, 5, cacheLen())

	// 访问 key_A 使其成为最近使用
	mustGet(t, "key_A")

	// 插入第 6 个条目，应淘汰最久未使用的 key_B
	mustSet(t, "key_F", 5, 0)

	// 容量应保持为 5
	assert.Equal(t, 5, cacheLen())

	// key_B 应被淘汰（最久未使用）
	assertMissing(t, "key_B")
	// key_A 应仍然存在（被访问过）
	assertPresent(t, "key_A")
	// key_F 应存在
	assertPresent(t, "key_F")
}

func TestLRUWithExpiration(t *testing.T) {
	oldCache := memCache
	defer func() { memCache = oldCache }()

	InitMemoryWithSize(3)
	defer CloseMemory()

	// 插入 2 个即将过期的条目
	mustSet(t, "exp1", "value1", 50*time.Millisecond)
	mustSet(t, "exp2", "value2", 50*time.Millisecond)

	// 插入 1 个不过期的条目
	mustSet(t, "perm", "permanent", 0)

	// 等待过期
	time.Sleep(100 * time.Millisecond)

	// 过期项应被清理
	assertMissing(t, "exp1")
	assertMissing(t, "exp2")
	assertPresent(t, "perm")
}

func TestInitMemoryWithSize(t *testing.T) {
	oldCache := memCache
	defer func() { memCache = oldCache }()
	defer CloseMemory()

	InitMemoryWithSize(10)

	assert.NotNil(t, memCache)
	assert.Equal(t, 10, memCache.maxSize)
}

func TestDefaultMaxSize(t *testing.T) {
	oldCache := memCache
	defer func() { memCache = oldCache }()
	defer CloseMemory()

	InitMemory()

	assert.NotNil(t, memCache)
	assert.Equal(t, DefaultMaxSize, memCache.maxSize)
}

func TestInitMemoryWithSizeClosesPreviousCleanup(t *testing.T) {
	restore := saveAndRestoreMemoryCache(t)

	InitMemoryWithSize(2)
	first := memCache
	require.NotNil(t, first)
	firstStop := first.stopCh
	firstDone := first.doneCh

	InitMemoryWithSize(3)
	require.NotNil(t, memCache)
	assert.NotSame(t, first, memCache)
	assert.Equal(t, 3, memCache.maxSize)

	assertClosed(t, firstStop)
	assertClosed(t, firstDone)

	restore()
}

func TestCloseMemoryIsIdempotent(t *testing.T) {
	restore := saveAndRestoreMemoryCache(t)

	InitMemoryWithSize(2)
	current := memCache
	require.NotNil(t, current)
	assertOpen(t, current.stopCh)
	assertOpen(t, current.doneCh)

	CloseMemory()
	assertClosed(t, current.stopCh)
	assertClosed(t, current.doneCh)
	assert.NotPanics(t, CloseMemory)

	restore()
}

// saveAndRestoreMemoryCache replaces the shared cache for one test and gives
// the remaining tests a fresh, running cache afterwards.
func saveAndRestoreMemoryCache(t *testing.T) func() {
	t.Helper()

	restored := false
	restore := func() {
		if restored {
			return
		}
		restored = true
		InitMemory()
	}
	t.Cleanup(restore)
	return restore
}

func assertClosed(t *testing.T, ch <-chan struct{}) {
	t.Helper()

	select {
	case <-ch:
	default:
		t.Fatal("channel is open")
	}
}

func assertOpen(t *testing.T, ch <-chan struct{}) {
	t.Helper()

	select {
	case <-ch:
		t.Fatal("channel is closed")
	default:
	}
}
