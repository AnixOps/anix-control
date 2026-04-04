package router

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type frontendAPIEndpoint struct {
	Method string
	Path   string
}

func TestFrontendAPIEndpoints_AreRegistered(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter()

	apiFiles := []string{
		"web/src/api/admin.js",
		"web/src/api/user.js",
		"web/src/api/auth.js",
	}

	endpoints, err := loadFrontendAPIEndpoints(apiFiles)
	require.NoError(t, err)
	require.NotEmpty(t, endpoints)

	for _, ep := range endpoints {
		fullPath := "/api/v2" + ep.Path
		if !strings.HasPrefix(ep.Path, "/") {
			fullPath = "/api/v2/" + ep.Path
		}

		var body *bytes.Reader
		if ep.Method == http.MethodPost || ep.Method == http.MethodPut {
			body = bytes.NewReader([]byte("{}"))
		} else {
			body = bytes.NewReader(nil)
		}

		req, reqErr := http.NewRequest(ep.Method, fullPath, body)
		require.NoError(t, reqErr)
		if ep.Method == http.MethodPost || ep.Method == http.MethodPut {
			req.Header.Set("Content-Type", "application/json")
		}

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.NotEqualf(t, http.StatusNotFound, w.Code, "%s %s is not registered in backend router", ep.Method, fullPath)
		assert.NotEqualf(t, http.StatusMethodNotAllowed, w.Code, "%s %s method mismatch between frontend and backend", ep.Method, fullPath)
	}
}

func loadFrontendAPIEndpoints(relPaths []string) ([]frontendAPIEndpoint, error) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		return nil, os.ErrInvalid
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))

	blockRe := regexp.MustCompile(`(?s)return\s+request\(\{\s*(.*?)\s*\}\)`)
	singleQuoteURLRe := regexp.MustCompile(`url:\s*'([^']+)'`)
	templateURLRe := regexp.MustCompile("url:\\s*`([^`]+)`")
	methodRe := regexp.MustCompile(`method:\s*'([a-z]+)'`)
	templateVarRe := regexp.MustCompile(`\$\{[^}]+\}`)

	seen := make(map[string]struct{})
	var endpoints []frontendAPIEndpoint

	for _, relPath := range relPaths {
		fullPath := filepath.Join(repoRoot, filepath.FromSlash(relPath))
		content, err := os.ReadFile(fullPath)
		if err != nil {
			return nil, err
		}

		matches := blockRe.FindAllStringSubmatch(string(content), -1)
		for _, m := range matches {
			if len(m) < 2 {
				continue
			}
			block := m[1]

			urlValue := ""
			if urlMatch := singleQuoteURLRe.FindStringSubmatch(block); len(urlMatch) >= 2 {
				urlValue = urlMatch[1]
			} else if urlMatch := templateURLRe.FindStringSubmatch(block); len(urlMatch) >= 2 {
				urlValue = urlMatch[1]
			}
			methodMatch := methodRe.FindStringSubmatch(block)
			if urlValue == "" || len(methodMatch) < 2 {
				continue
			}

			urlPath := templateVarRe.ReplaceAllString(urlValue, "1")
			method := strings.ToUpper(methodMatch[1])

			key := method + " " + urlPath
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}

			endpoints = append(endpoints, frontendAPIEndpoint{
				Method: method,
				Path:   urlPath,
			})
		}
	}

	sort.Slice(endpoints, func(i, j int) bool {
		if endpoints[i].Method == endpoints[j].Method {
			return endpoints[i].Path < endpoints[j].Path
		}
		return endpoints[i].Method < endpoints[j].Method
	})

	return endpoints, nil
}
