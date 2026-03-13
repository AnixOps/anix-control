package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestParseTrafficData_Extended 扩展流量数据解析测试
func TestParseTrafficData_Extended(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectedLen int
		expectError bool
	}{
		{
			name:        "normal traffic data",
			input:       `{"1": [1024, 2048], "2": [512, 1024]}`,
			expectedLen: 2,
			expectError: false,
		},
		{
			name:        "empty data",
			input:       `{}`,
			expectedLen: 0,
			expectError: false,
		},
		{
			name:        "large numbers",
			input:       `{"1": [1073741824, 2147483648]}`,
			expectedLen: 1,
			expectError: false,
		},
		{
			name:        "zero traffic",
			input:       `{"1": [0, 0], "2": [0, 0]}`,
			expectedLen: 2,
			expectError: false,
		},
		{
			name:        "single user",
			input:       `{"12345": [1000000, 2000000]}`,
			expectedLen: 1,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var data map[string]interface{}
			err := json.Unmarshal([]byte(tt.input), &data)
			assert.NoError(t, err)

			result, err := ParseTrafficData(data)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedLen, len(result))
			}
		})
	}
}

// TestParseOnlineData_Extended 扩展在线数据解析测试
func TestParseOnlineData_Extended(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectedLen int
		expectError bool
	}{
		{
			name:        "normal online data",
			input:       `{"1": ["192.168.1.100", "10.0.0.50"], "2": ["172.16.0.1"]}`,
			expectedLen: 2,
			expectError: false,
		},
		{
			name:        "empty data",
			input:       `{}`,
			expectedLen: 0,
			expectError: false,
		},
		{
			name:        "single user single ip",
			input:       `{"1": ["192.168.1.1"]}`,
			expectedLen: 1,
			expectError: false,
		},
		{
			name:        "ipv6 addresses",
			input:       `{"1": ["::1", "2001:db8::1"]}`,
			expectedLen: 1,
			expectError: false,
		},
		{
			name:        "mixed ipv4 and ipv6",
			input:       `{"1": ["192.168.1.1", "::1", "2001:db8::1"]}`,
			expectedLen: 1,
			expectError: false,
		},
		{
			name:        "multiple users multiple ips",
			input:       `{"1": ["192.168.1.1", "192.168.1.2"], "2": ["10.0.0.1"], "3": ["172.16.0.1", "172.16.0.2", "172.16.0.3"]}`,
			expectedLen: 3,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var data map[string]interface{}
			err := json.Unmarshal([]byte(tt.input), &data)
			assert.NoError(t, err)

			result, err := ParseOnlineData(data)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedLen, len(result))
			}
		})
	}
}

// TestToInt64_EdgeCases toInt64 边界情况测试
func TestToInt64_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected int64
		ok       bool
	}{
		{
			name:     "zero float64",
			input:    float64(0),
			expected: 0,
			ok:       true,
		},
		{
			name:     "negative number",
			input:    float64(-1024),
			expected: -1024,
			ok:       true,
		},
		{
			name:     "large int64",
			input:    int64(9223372036854775807), // max int64
			expected: 9223372036854775807,
			ok:       true,
		},
		{
			name:     "json.Number large",
			input:    json.Number("9999999999999999"),
			expected: 9999999999999999,
			ok:       true,
		},
		{
			name:     "nil value",
			input:    nil,
			expected: 0,
			ok:       false,
		},
		{
			name:     "bool value (unsupported)",
			input:    true,
			expected: 0,
			ok:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, ok := toInt64(tt.input)
			assert.Equal(t, tt.expected, result)
			assert.Equal(t, tt.ok, ok)
		})
	}
}

// BenchmarkParseTrafficData 基准测试
func BenchmarkParseTrafficData(b *testing.B) {
	input := `{"1": [1024, 2048], "2": [512, 1024], "3": [100, 200]}`
	var data map[string]interface{}
	json.Unmarshal([]byte(input), &data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ParseTrafficData(data)
	}
}

// BenchmarkParseOnlineData 基准测试
func BenchmarkParseOnlineData(b *testing.B) {
	input := `{"1": ["192.168.1.100", "10.0.0.50"], "2": ["172.16.0.1"]}`
	var data map[string]interface{}
	json.Unmarshal([]byte(input), &data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ParseOnlineData(data)
	}
}