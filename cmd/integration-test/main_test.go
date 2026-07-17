package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrintJSON_Map(t *testing.T) {
	data := map[string]any{"key": "value", "num": 42}
	output := capturePrintJSON(t, data)

	// Should be valid JSON
	var parsed map[string]any
	assert.NoError(t, json.Unmarshal([]byte(output), &parsed))
	assert.Equal(t, "value", parsed["key"])
	assert.Equal(t, float64(42), parsed["num"])
}

func TestPrintJSON_Struct(t *testing.T) {
	type Item struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	output := capturePrintJSON(t, Item{Name: "test", Value: 99})

	var parsed map[string]any
	assert.NoError(t, json.Unmarshal([]byte(output), &parsed))
	assert.Equal(t, "test", parsed["name"])
	assert.Equal(t, float64(99), parsed["value"])
}

func TestPrintJSON_Nil(t *testing.T) {
	output := capturePrintJSON(t, nil)

	assert.Equal(t, "null\n", output)
}

func capturePrintJSON(t *testing.T, v any) string {
	t.Helper()

	old := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	defer func() {
		os.Stdout = old
	}()

	printJSON(v)
	require.NoError(t, w.Close())

	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)
	require.NoError(t, r.Close())

	return buf.String()
}
