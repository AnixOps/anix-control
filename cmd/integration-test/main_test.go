package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPrintJSON_Map(t *testing.T) {
	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	data := map[string]any{"key": "value", "num": 42}
	printJSON(data)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

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

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printJSON(Item{Name: "test", Value: 99})

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	var parsed map[string]any
	assert.NoError(t, json.Unmarshal([]byte(output), &parsed))
	assert.Equal(t, "test", parsed["name"])
	assert.Equal(t, float64(99), parsed["value"])
}

func TestPrintJSON_Nil(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printJSON(nil)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	assert.Equal(t, "null\n", output)
}
