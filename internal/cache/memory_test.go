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
	Set("del_key", "value", 0)

	err := Del("del_key")
	require.NoError(t, err)

	_, err = GetString("del_key")
	assert.Equal(t, ErrKeyNotFound, err)
}

func TestDelMultiple(t *testing.T) {
	Set("key1", "value1", 0)
	Set("key2", "value2", 0)
	Set("key3", "value3", 0)

	err := Del("key1", "key2")
	require.NoError(t, err)

	assert.False(t, Exists("key1"))
	assert.False(t, Exists("key2"))
	assert.True(t, Exists("key3"))
}

func TestDelete(t *testing.T) {
	Set("delete_key", "value", 0)

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
	SAdd("set2", "a", "b", "c")

	count, err := SCard("set2")
	require.NoError(t, err)
	assert.Equal(t, int64(3), count)
}

func TestSClear(t *testing.T) {
	SAdd("set3", "x", "y", "z")

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
	HSet("hash2", "f1", "v1")
	HSet("hash2", "f2", "v2")
	HSet("hash2", "f3", "v3")

	all, err := HGetAll("hash2")
	require.NoError(t, err)
	assert.Len(t, all, 3)
	assert.Equal(t, "v1", all["f1"])
	assert.Equal(t, "v2", all["f2"])
	assert.Equal(t, "v3", all["f3"])
}

func TestHDel(t *testing.T) {
	HSet("hash3", "f1", "v1")
	HSet("hash3", "f2", "v2")

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

	Set("test_key1", "v1", 0)
	Set("test_key2", "v2", 0)
	Set("other_key", "v3", 0)

	keys, err := Keys("test_*")
	require.NoError(t, err)
	assert.Len(t, keys, 2)
	assert.Contains(t, keys, "test_key1")
	assert.Contains(t, keys, "test_key2")
}

func TestExists(t *testing.T) {
	Set("exists_key", "value", 0)

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
	Set("expire_key", "value", 0)

	err := Expire("expire_key", 100*time.Millisecond)
	require.NoError(t, err)

	// 立即应该存在
	assert.True(t, Exists("expire_key"))

	// 等待过期
	time.Sleep(150 * time.Millisecond)

	assert.False(t, Exists("expire_key"))
}

func TestClear(t *testing.T) {
	Set("clear1", "v1", 0)
	Set("clear2", "v2", 0)
	SAdd("clear_set", "m1", "m2")

	Clear()

	assert.False(t, Exists("clear1"))
	assert.False(t, Exists("clear2"))
	count, _ := SCard("clear_set")
	assert.Equal(t, int64(0), count)
}

func TestGetGeneric(t *testing.T) {
	Set("generic_key", "string_value", 0)

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

	// 并发写入
	for i := 0; i < workers; i++ {
		go func(id int) {
			for j := 0; j < iterations; j++ {
				key := "concurrent_" + string(rune('A'+id))
				Set(key, j, 0)
				GetString(key)
				Del(key)
			}
			done <- true
		}(i)
	}

	// 等待所有 goroutine 完成
	for i := 0; i < workers; i++ {
		<-done
	}
}