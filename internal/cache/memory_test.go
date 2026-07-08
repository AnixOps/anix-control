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

func mustHSet(t *testing.T, key, field string, value any) {
	t.Helper()
	require.NoError(t, HSet(key, field, value))
}

func mustGet(t *testing.T, key string) any {
	t.Helper()
	value, err := Get(key)
	require.NoError(t, err)
	return value
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

			got, err := GetString(tt.key)
			require.NoError(t, err)
			assert.Equal(t, tt.value, got)
		})
	}
}

func TestSetWithExpiration(t *testing.T) {
	err := Set("expiring_key", "value", 100*time.Millisecond)
	require.NoError(t, err)

	// 立即获取应该成功
	got, err := GetString("expiring_key")
	require.NoError(t, err)
	assert.Equal(t, "value", got)

	// 等待过期
	time.Sleep(150 * time.Millisecond)

	// 过期后应该找不到
	_, err = GetString("expiring_key")
	assert.Equal(t, ErrKeyNotFound, err)
}

func TestGetNotFound(t *testing.T) {
	_, err := GetString("nonexistent_key")
	assert.Equal(t, ErrKeyNotFound, err)
}

func TestDel(t *testing.T) {
	mustSet(t, "del_key", "value", 0)

	err := Del("del_key")
	require.NoError(t, err)

	_, err = GetString("del_key")
	assert.Equal(t, ErrKeyNotFound, err)
}

func TestDelMultiple(t *testing.T) {
	mustSet(t, "key1", "value1", 0)
	mustSet(t, "key2", "value2", 0)
	mustSet(t, "key3", "value3", 0)

	err := Del("key1", "key2")
	require.NoError(t, err)

	assert.False(t, Exists("key1"))
	assert.False(t, Exists("key2"))
	assert.True(t, Exists("key3"))
}

func TestDelete(t *testing.T) {
	mustSet(t, "delete_key", "value", 0)

	err := Delete("delete_key")
	require.NoError(t, err)

	assert.False(t, Exists("delete_key"))
}

func TestSAddAndSMembers(t *testing.T) {
	err := SAdd("set1", "member1", "member2", "member3")
	require.NoError(t, err)

	members, err := SMembers("set1")
	require.NoError(t, err)
	assert.Len(t, members, 3)
	assert.Contains(t, members, "member1")
	assert.Contains(t, members, "member2")
	assert.Contains(t, members, "member3")
}

func TestSCard(t *testing.T) {
	mustSAdd(t, "set2", "a", "b", "c")

	count, err := SCard("set2")
	require.NoError(t, err)
	assert.Equal(t, int64(3), count)
}

func TestSClear(t *testing.T) {
	mustSAdd(t, "set3", "x", "y", "z")

	err := SClear("set3")
	require.NoError(t, err)

	count, err := SCard("set3")
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

func TestHSetAndHGet(t *testing.T) {
	err := HSet("hash1", "field1", "value1")
	require.NoError(t, err)

	got, err := HGet("hash1", "field1")
	require.NoError(t, err)
	assert.Equal(t, "value1", got)
}

func TestHGetAll(t *testing.T) {
	mustHSet(t, "hash2", "f1", "v1")
	mustHSet(t, "hash2", "f2", "v2")
	mustHSet(t, "hash2", "f3", "v3")

	all, err := HGetAll("hash2")
	require.NoError(t, err)
	assert.Len(t, all, 3)
	assert.Equal(t, "v1", all["f1"])
	assert.Equal(t, "v2", all["f2"])
	assert.Equal(t, "v3", all["f3"])
}

func TestHDel(t *testing.T) {
	mustHSet(t, "hash3", "f1", "v1")
	mustHSet(t, "hash3", "f2", "v2")

	err := HDel("hash3", "f1")
	require.NoError(t, err)

	_, err = HGet("hash3", "f1")
	assert.Equal(t, ErrKeyNotFound, err)

	got, err := HGet("hash3", "f2")
	require.NoError(t, err)
	assert.Equal(t, "v2", got)
}

func TestKeys(t *testing.T) {
	// Clear previous data
	Clear()

	mustSet(t, "test_key1", "v1", 0)
	mustSet(t, "test_key2", "v2", 0)
	mustSet(t, "other_key", "v3", 0)

	keys, err := Keys("test_*")
	require.NoError(t, err)
	assert.Len(t, keys, 2)
	assert.Contains(t, keys, "test_key1")
	assert.Contains(t, keys, "test_key2")
}

func TestExists(t *testing.T) {
	mustSet(t, "exists_key", "value", 0)

	assert.True(t, Exists("exists_key"))
	assert.False(t, Exists("not_exists_key"))
}

func TestIncr(t *testing.T) {
	val := Incr("counter")
	assert.Equal(t, int64(1), val)

	val = Incr("counter")
	assert.Equal(t, int64(2), val)

	val = Incr("counter")
	assert.Equal(t, int64(3), val)
}

func TestIncrBy(t *testing.T) {
	val := IncrBy("counter2", 5)
	assert.Equal(t, int64(5), val)

	val = IncrBy("counter2", 10)
	assert.Equal(t, int64(15), val)

	val = IncrBy("counter2", -3)
	assert.Equal(t, int64(12), val)
}

func TestDecr(t *testing.T) {
	IncrBy("decr_counter", 10)

	val := Decr("decr_counter")
	assert.Equal(t, int64(9), val)
}

func TestSetIntAndGetInt(t *testing.T) {
	err := SetInt("int_key", 42, 0)
	require.NoError(t, err)

	val, err := GetInt("int_key")
	require.NoError(t, err)
	assert.Equal(t, int64(42), val)
}

func TestExpire(t *testing.T) {
	mustSet(t, "expire_key", "value", 0)

	err := Expire("expire_key", 100*time.Millisecond)
	require.NoError(t, err)

	// 立即应该存在
	assert.True(t, Exists("expire_key"))

	// 等待过期
	time.Sleep(150 * time.Millisecond)

	assert.False(t, Exists("expire_key"))
}

func TestClear(t *testing.T) {
	mustSet(t, "clear1", "v1", 0)
	mustSet(t, "clear2", "v2", 0)
	mustSAdd(t, "clear_set", "m1", "m2")

	Clear()

	assert.False(t, Exists("clear1"))
	assert.False(t, Exists("clear2"))
	count, err := SCard("clear_set")
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
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
	_, _ = GetString("key")
	_, _ = Get("key")
	_ = Del("key")
	_ = SAdd("set", "member")
	_, _ = SMembers("set")
	_, _ = SCard("set")
	_ = HSet("hash", "field", "value")
	_, _ = HGet("hash", "field")
	_, _ = HGetAll("hash")
	_ = HDel("hash", "field")
	_, _ = Keys("*")
	_ = Exists("key")
	_ = Incr("counter")
	_ = IncrBy("counter", 5)
	_ = Decr("counter")
	assert.False(t, Exists("any_key"))
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
	assert.Equal(t, 5, Len())

	// 访问 key_A 使其成为最近使用
	mustGet(t, "key_A")

	// 插入第 6 个条目，应淘汰最久未使用的 key_B
	mustSet(t, "key_F", 5, 0)

	// 容量应保持为 5
	assert.Equal(t, 5, Len())

	// key_B 应被淘汰（最久未使用）
	assert.False(t, Exists("key_B"))
	// key_A 应仍然存在（被访问过）
	assert.True(t, Exists("key_A"))
	// key_F 应存在
	assert.True(t, Exists("key_F"))
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
	assert.False(t, Exists("exp1"))
	assert.False(t, Exists("exp2"))
	assert.True(t, Exists("perm"))
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

func TestCloseMemoryIsIdempotentAndRestartable(t *testing.T) {
	restore := saveAndRestoreMemoryCache(t)

	InitMemoryWithSize(2)
	current := memCache
	require.NotNil(t, current)

	CloseMemory()
	assertClosed(t, current.stopCh)
	assertClosed(t, current.doneCh)
	assert.NotPanics(t, CloseMemory)

	StartCleanup()
	restarted := memCache
	require.NotNil(t, restarted)
	assertOpen(t, restarted.stopCh)
	assertOpen(t, restarted.doneCh)

	CloseMemory()
	assertClosed(t, restarted.stopCh)
	assertClosed(t, restarted.doneCh)

	restore()
}

func saveAndRestoreMemoryCache(t *testing.T) func() {
	t.Helper()

	oldCache := memCache
	restored := false
	restore := func() {
		if restored {
			return
		}
		restored = true
		CloseMemory()
		memCache = oldCache
		StartCleanup()
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
