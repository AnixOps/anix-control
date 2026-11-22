package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	_ "github.com/AnixOps/anix-control/v5/docs"
	"github.com/stretchr/testify/assert"
)

// TestSwaggerDocComesFromTheBinary: /swagger/doc.json is the compiled-in
// docs package, so a binary install without a docs directory serves it.
func TestSwaggerDocComesFromTheBinary(t *testing.T) {
	r, _ := setupTestRouter(t)
	defer teardownTestRouter(t)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/swagger/doc.json", nil))
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"title": "AnixOps Control API"`)
	assert.Contains(t, w.Body.String(), `"/admin/nodes": {`)
}
