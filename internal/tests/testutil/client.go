package testutil

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// APITestClient API 测试客户端
type APITestClient struct {
	Router *gin.Engine
	T      *testing.T
	Token  string
}

// NewAPITestClient 创建 API 测试客户端
func NewAPITestClient(t *testing.T, router *gin.Engine) *APITestClient {
	return &APITestClient{
		Router: router,
		T:      t,
	}
}

// SetAuth 设置认证 Token
func (c *APITestClient) SetAuth(token string) *APITestClient {
	c.Token = token
	return c
}

// Request 发送请求
func (c *APITestClient) Request(method, path string, body interface{}) *httptest.ResponseRecorder {
	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			c.T.Fatalf("failed to marshal request body: %v", err)
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequest(method, path, bodyReader)
	if err != nil {
		c.T.Fatalf("failed to create request: %v", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	w := httptest.NewRecorder()
	c.Router.ServeHTTP(w, req)
	return w
}

// GET 发送 GET 请求
func (c *APITestClient) GET(path string) *httptest.ResponseRecorder {
	return c.Request(http.MethodGet, path, nil)
}

// POST 发送 POST 请求
func (c *APITestClient) POST(path string, body interface{}) *httptest.ResponseRecorder {
	return c.Request(http.MethodPost, path, body)
}

// PUT 发送 PUT 请求
func (c *APITestClient) PUT(path string, body interface{}) *httptest.ResponseRecorder {
	return c.Request(http.MethodPut, path, body)
}

// DELETE 发送 DELETE 请求
func (c *APITestClient) DELETE(path string) *httptest.ResponseRecorder {
	return c.Request(http.MethodDelete, path, nil)
}

// AssertStatus 断言状态码
func AssertStatus(t *testing.T, w *httptest.ResponseRecorder, expected int) {
	assert.Equal(t, expected, w.Code, "Response body: %s", w.Body.String())
}

// AssertJSON 断言 JSON 响应
func AssertJSON(t *testing.T, w *httptest.ResponseRecorder, expected map[string]interface{}) {
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	for key, value := range expected {
		assert.Equal(t, value, response[key], "Key: %s", key)
	}
}

// ParseResponse 解析响应
func ParseResponse(t *testing.T, w *httptest.ResponseRecorder, v interface{}) {
	err := json.Unmarshal(w.Body.Bytes(), v)
	assert.NoError(t, err)
}