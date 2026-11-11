package packagecompat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/sdk/v2compat"
	"github.com/stretchr/testify/require"
)

// RecordEnvironment makes a route without a legacy handler write its
// answers to the golden files instead of comparing with them.
const RecordEnvironment = "PACKAGECOMPAT_RECORD"

// goldenAnswer is what a route without a legacy handler must answer: the
// answer recorded while the route still matched the kernel's handler.
type goldenAnswer struct {
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body"`
	State  json.RawMessage `json:"state,omitempty"`
}

// requireGolden compares native with testdata/<test>.json, entry <case>.
// SQLite and PostgreSQL share the entry, unless PostgreSQL answers
// otherwise (a database's own error text): its answer is then the entry
// "<case> (postgres)". Bodies compare after v2compat.NormalizeForCompare
// and c.Mask.
func requireGolden(t *testing.T, c Case, native Result) {
	t.Helper()
	for _, name := range c.Headers {
		require.NotEmpty(t, native.Header.Get(name), "native %s header", name)
	}
	body := native.Body
	if len(c.Mask) > 0 {
		body = mask(t, body, c.Mask)
	}
	body = v2compat.NormalizeForCompare(body)
	if !json.Valid(body) {
		body, _ = json.Marshal(string(body))
	}
	got := goldenAnswer{Status: native.StatusCode, Body: body}
	if native.State != nil {
		state, err := json.Marshal(native.State)
		require.NoError(t, err)
		got.State = state
	}
	for _, text := range c.Volatile {
		got.Body = bytes.ReplaceAll(got.Body, []byte(text), []byte("<volatile>"))
		got.State = bytes.ReplaceAll(got.State, []byte(text), []byte("<volatile>"))
	}

	test, key, _ := strings.Cut(t.Name(), "/")
	backend := key[strings.LastIndex(key, "/")+1:]
	key = key[:strings.LastIndex(key, "/")]
	own := key + " (" + backend + ")"
	path := filepath.Join("testdata", test+".json")
	answers := map[string]goldenAnswer{}
	if raw, err := os.ReadFile(path); err == nil {
		require.NoError(t, json.Unmarshal(raw, &answers))
	}
	if os.Getenv(RecordEnvironment) != "" {
		if backend == backendSQLite {
			answers[key] = got
		} else if delete(answers, own); compact(t, answers[key]) != compact(t, got) {
			answers[own] = got
		}
		// One case per line, sorted, so a change shows as that case's line.
		keys := make([]string, 0, len(answers))
		for name := range answers {
			keys = append(keys, name)
		}
		sort.Strings(keys)
		var out bytes.Buffer
		for i, name := range keys {
			quoted, _ := json.Marshal(name)
			fmt.Fprintf(&out, "%s%s: %s", map[bool]string{true: "{\n", false: ",\n"}[i == 0], quoted, compact(t, answers[name]))
		}
		out.WriteString("\n}\n")
		require.NoError(t, os.MkdirAll("testdata", 0o755))
		require.NoError(t, os.WriteFile(path, out.Bytes(), 0o644))
		return
	}
	want, ok := answers[own]
	if !ok {
		want, ok = answers[key]
	}
	require.True(t, ok, "no recorded answer for %q in %s", key, path)
	require.Equal(t, compact(t, want), compact(t, got))
}

func compact(t *testing.T, answer goldenAnswer) string {
	t.Helper()
	raw, err := json.Marshal(answer)
	require.NoError(t, err)
	var out bytes.Buffer
	require.NoError(t, json.Compact(&out, raw))
	return out.String()
}
