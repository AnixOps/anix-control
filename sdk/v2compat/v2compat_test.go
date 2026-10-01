package v2compat

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The legacy handlers wrote map[string]any envelopes; the struct must marshal
// to the same bytes for every kind of data.
func TestPanelMarshalsLikeTheLegacyMapEnvelope(t *testing.T) {
	now := time.UnixMilli(1790000000123)
	cases := []any{
		nil, "操作成功", 42, []any{}, []string(nil), map[string]any{"list": []int{1, 2}, "a<b>&c": "x<y>&z"},
		struct {
			ID   uint   `json:"id"`
			Body string `json:"body"`
		}{ID: 7, Body: "<script>"},
	}
	for _, data := range cases {
		legacy, err := json.Marshal(map[string]any{"code": 0, "msg": PanelSuccessMessage, "ts": now.UnixMilli(), "data": data})
		require.NoError(t, err)
		current, err := json.Marshal(PanelSuccess(data, now))
		require.NoError(t, err)
		assert.Equal(t, string(legacy), string(current), "%#v", data)
	}
	legacyError, err := json.Marshal(map[string]any{"code": -1, "msg": "文章不存在", "ts": now.UnixMilli(), "data": nil})
	require.NoError(t, err)
	currentError, err := json.Marshal(PanelError("文章不存在", now))
	require.NoError(t, err)
	assert.Equal(t, string(legacyError), string(currentError))
}

func TestNormalizeForCompareIgnoresTimestampsAndKeyOrder(t *testing.T) {
	assert.True(t, EqualForCompare(
		[]byte(`{"code":0,"ts":1,"data":{"b":2,"a":1.50},"msg":"ok"}`),
		[]byte(` {"msg":"ok","data":{"a":1.50,"b":2},"code":0,"ts":999}`),
	))
	assert.False(t, EqualForCompare([]byte(`{"data":null}`), []byte(`{"data":[]}`)), "null and [] differ")
	assert.False(t, EqualForCompare([]byte(`{"data":{"ts":1}}`), []byte(`{"data":{"ts":2}}`)), "only the top-level ts is volatile")
	assert.False(t, EqualForCompare([]byte(`{"n":1.0}`), []byte(`{"n":1}`)), "number spelling is preserved")
	assert.Equal(t, []byte("plain text"), NormalizeForCompare([]byte("plain text")))
	assert.True(t, EqualForCompare([]byte(`[1,2]`), []byte(`[1, 2]`)))
}

func TestSealedHandleGrammar(t *testing.T) {
	handle := SealedHandlePrefix + "abcdefghijklmnopqrstuvwxyzABCDEFGHIJ-_01234"
	require.Len(t, handle, SealedHandleLength)
	assert.True(t, IsSealedHandle(handle))
	assert.False(t, IsSealedHandle(handle[:len(handle)-1]), "too short")
	assert.False(t, IsSealedHandle(handle+"x"), "too long")
	assert.False(t, IsSealedHandle(SealedHandlePrefix+"abcdefghijklmnopqrstuvwxyzABCDEFGHIJ-_0123="), "padding is not base64url")
	assert.False(t, IsSealedHandle("anix-sealed:v2:abcdefghijklmnopqrstuvwxyzABCDEFGHIJ-_01234"), "another version")
	assert.False(t, IsSealedHandle("********"))
}

// The shadow comparison masks handles on both sides: a native answer holds
// a handle where the legacy answer shows the secret.
func TestEqualForCompareMasksSealedHandles(t *testing.T) {
	first := SealedHandlePrefix + "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	second := SealedHandlePrefix + "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"
	legacy := []byte(`{"code":0,"data":{"node_id":7,"api_key":"real-key","secret":"real-secret"},"msg":"ok","ts":1}`)
	native := []byte(`{"code":0,"data":{"node_id":7,"api_key":"` + first + `","secret":"` + second + `"},"msg":"ok","ts":2}`)
	assert.True(t, EqualForCompare(legacy, native), "only handles differ")
	assert.True(t, EqualForCompare(native, legacy), "on either side")
	assert.True(t, EqualForCompare(
		[]byte(`{"data":{"api_key":"`+first+`"}}`), []byte(`{"data":{"api_key":"`+second+`"}}`),
	), "two answers with different handles")
	assert.False(t, EqualForCompare(legacy, []byte(`{"code":0,"data":{"node_id":8,"api_key":"`+first+`","secret":"`+second+`"},"msg":"ok"}`)),
		"anything else that differs still differs")
	assert.False(t, EqualForCompare([]byte(`{"data":{"api_key":7}}`), []byte(`{"data":{"api_key":"`+first+`"}}`)),
		"a handle matches strings only")
	assert.False(t, EqualForCompare([]byte(`{"data":{"api_key":"k"}}`), []byte(`{"data":{"api_key":"`+first+`x"}}`)),
		"a string that is not a handle is compared as it is")
	assert.Equal(t, `{"data":["`+SealedHandleMask+`"]}`, string(NormalizeForCompare([]byte(`{"data":["`+first+`"]}`))))
}
