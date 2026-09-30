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
