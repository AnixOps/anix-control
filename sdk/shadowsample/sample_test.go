package shadowsample

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func diffJSON(t *testing.T, entries []DiffEntry) string {
	t.Helper()
	encoded, err := json.Marshal(entries)
	require.NoError(t, err)
	return string(encoded)
}

func TestDiff(t *testing.T) {
	cases := []struct {
		name           string
		legacy, native string
		want           string
	}{
		{"equal", `{"code":0,"data":{"a":1},"ts":1}`, `{"ts":2,"data":{"a":1},"code":0}`, `[]`},
		{
			"changed scalar with masked values",
			`{"data":{"email":"alice@example.com","ip":"10.2.3.4"}}`,
			`{"data":{"email":"alicia@example.com","ip":"10.2.3.5"}}`,
			`[{"path":"$.data.email","kind":"changed","legacy":"a***@example.com","native":"a***@example.com"},
			  {"path":"$.data.ip","kind":"changed","legacy":"10.2.*.*","native":"10.2.*.*"}]`,
		},
		{
			"sensitive key differs, values masked",
			`{"data":{"token":"aaa","nested":{"password":"x"}}}`,
			`{"data":{"token":"bbb","nested":{"password":"y"}}}`,
			`[{"path":"$.data.nested.password","kind":"changed","legacy":"***","native":"***"},
			  {"path":"$.data.token","kind":"changed","legacy":"***","native":"***"}]`,
		},
		{
			"missing and extra keys",
			`{"data":{"a":1,"secret_b":{"k":"v"}}}`,
			`{"data":{"c":"` + testJWT + `"}}`,
			`[{"path":"$.data.a","kind":"legacy_only","legacy":1},
			  {"path":"$.data.c","kind":"native_only","native":"***"},
			  {"path":"$.data.secret_b","kind":"legacy_only","legacy":"***"}]`,
		},
		{
			"arrays of different length",
			`{"data":[{"id":1},{"id":2}]}`,
			`{"data":[{"id":1}]}`,
			`[{"path":"$.data[1]","kind":"legacy_only","legacy":{"id":2}}]`,
		},
		{
			"type change",
			`{"data":{"n":1}}`,
			`{"data":{"n":"1"}}`,
			`[{"path":"$.data.n","kind":"type","legacy":1,"native":"1"}]`,
		},
		{
			"secret-looking keys are masked in paths",
			`{"data":{"7b3f6c1e-2d4a-4b8f-9c0d-1e2f3a4b5c6d":{"n":1},"a.b":1}}`,
			`{"data":{"7b3f6c1e-2d4a-4b8f-9c0d-1e2f3a4b5c6d":{"n":2},"a.b":2}}`,
			`[{"path":"$.data.***.n","kind":"changed","legacy":1,"native":2},{"path":"$.data[\"a.b\"]","kind":"changed","legacy":1,"native":2}]`,
		},
		{
			"sealed handles match any string",
			`{"data":{"password_visible":"plain","v":"plain"}}`,
			`{"data":{"password_visible":"anix-sealed:v1:Zk3q9v_8xN2mT4pL6rW1yB5cH7jF0dAa1b2c3d4e5f6","v":"anix-sealed:v1:Zk3q9v_8xN2mT4pL6rW1yB5cH7jF0dAa1b2c3d4e5f6"}}`,
			`[]`,
		},
		{
			"non-JSON bodies are described by size",
			`password=hunter2`,
			`{"ok":true}`,
			`[{"path":"$","kind":"body","legacy":"<non-JSON body, 16 bytes>","native":"<object, 1 keys>"}]`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			entries, truncated := Diff([]byte(tc.legacy), []byte(tc.native))
			require.False(t, truncated)
			require.JSONEq(t, tc.want, diffJSON(t, entries))
		})
	}
}

func TestDiffIsCapped(t *testing.T) {
	legacy, native := map[string]any{}, map[string]any{}
	for index := 0; index < 500; index++ {
		key := "field_" + strconv.Itoa(index)
		legacy[key] = strings.Repeat("a", 100)
		native[key] = strings.Repeat("b", 100)
	}
	legacyJSON, _ := json.Marshal(legacy)
	nativeJSON, _ := json.Marshal(native)
	entries, truncated := Diff(legacyJSON, nativeJSON)
	require.True(t, truncated)
	require.LessOrEqual(t, len(entries), MaxDiffEntries)
	require.LessOrEqual(t, len(diffJSON(t, entries)), MaxDiffBytes)

	// Large values are described, not copied.
	big := `{"data":{"list":[` + strings.Repeat(`"x",`, 400) + `"x"]}}`
	entries, _ = Diff([]byte(big), []byte(`{"data":{}}`))
	require.JSONEq(t, `[{"path":"$.data.list","kind":"legacy_only","legacy":"<array, 401 items>"}]`, diffJSON(t, entries))
}

func TestSanitizeSample(t *testing.T) {
	raw := Sample{
		ID: "0123456789abcdef0123456789abcdef", RouteID: "user.plan.fetch", Method: "get",
		Path: "/api/v2/user/7b3f6c1e-2d4a-4b8f-9c0d-1e2f3a4b5c6d/{token}?token=abc&email=a@b.co", LegacyStatus: 200, NativeStatus: 500,
		Diff: []DiffEntry{
			{Path: "$.data.token", Kind: DiffChanged, Legacy: json.RawMessage(`"plaintext"`), Native: json.RawMessage(`"other"`)},
			{Path: "$.data.note", Kind: DiffChanged, Legacy: json.RawMessage(`"` + testJWT + `"`), Native: json.RawMessage(`{"ip":"10.1.2.3"}`)},
			{Path: "$.data.x", Kind: "bogus"},
			{Path: "$.data.y", Kind: DiffChanged, Legacy: json.RawMessage(`not json`)},
		},
		RequestID: testJWT, ObservedAt: 1700000000,
	}
	clean := Sanitize(raw)
	require.Equal(t, "GET", clean.Method)
	require.Equal(t, "/api/v2/user/***/{token}?email=***&token=***", clean.Path)
	require.Equal(t, Mask, clean.RequestID)
	require.JSONEq(t, `[
		{"path":"$.data.token","kind":"changed","legacy":"***","native":"***"},
		{"path":"$.data.note","kind":"changed","legacy":"***","native":{"ip":"10.1.*.*"}},
		{"path":"$.data.y","kind":"changed","legacy":"***"}
	]`, diffJSON(t, clean.Diff))
	require.Equal(t, clean, Sanitize(clean), "Sanitize is idempotent")

	require.Equal(t, "7b3f6c1e-2d4a-4b8f-9c0d-1e2f3a4b5c6d", SanitizeRequestID("7B3F6C1E-2D4A-4B8F-9C0D-1E2F3A4B5C6D"))
	require.Equal(t, "req-1", SanitizeRequestID("req-1"))
	require.Equal(t, "", SanitizeRequestID("has space"))
	require.True(t, ValidID(raw.ID))
	require.False(t, ValidID("../etc"))
}

func TestSanitizeSampleCapsDiff(t *testing.T) {
	sample := Sample{ID: "0123456789abcdef0123456789abcdef", RouteID: "r", Method: "GET"}
	for index := 0; index < 200; index++ {
		sample.Diff = append(sample.Diff, DiffEntry{Path: "$.f" + strconv.Itoa(index), Kind: DiffChanged, Legacy: json.RawMessage(`"` + strings.Repeat("a", 200) + `"`)})
	}
	clean := Sanitize(sample)
	require.True(t, clean.DiffTruncated)
	require.LessOrEqual(t, len(diffJSON(t, clean.Diff)), MaxDiffBytes)
}
