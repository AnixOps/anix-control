package handler

import (
	"crypto/rand"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v4/internal/agentpki"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/modulepki"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func agentPKITestService(t *testing.T) (*agentpki.Service, *gorm.DB, model.Node) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "kernel.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ServiceCA{}, &model.AgentEnrollment{}, &model.AgentCertificate{}, &model.Node{},
		&model.ForwardNode{}, &model.OperationLog{}))
	kek := make([]byte, 32)
	_, err = rand.Read(kek)
	require.NoError(t, err)
	authority, err := modulepki.New(modulepki.Options{DB: db, Cluster: "prod", KEK: kek})
	require.NoError(t, err)
	require.NoError(t, authority.Ensure(t.Context()))
	pki, err := agentpki.New(agentpki.Options{DB: db, Authority: authority})
	require.NoError(t, err)
	node := model.Node{Name: "proxy", APIKeyHash: strings.Repeat("a", 64), Status: model.NodeStatusOnline}
	require.NoError(t, db.Create(&node).Error)
	return pki, db, node
}

func newAgentPKITestRouter(pki func() (*agentpki.Service, error)) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", uint(7))
		c.Set("email", "admin@example.com")
	})
	agents := &AgentPKIHandler{pki: pki}
	router.POST("/enrollment-tokens", agents.CreateEnrollmentToken)
	return router
}

func TestAgentEnrollmentTokenAdministration(t *testing.T) {
	pki, db, node := agentPKITestService(t)
	router := newAgentPKITestRouter(func() (*agentpki.Service, error) { return pki, nil })
	nodeName := "proxy-" + strconv.FormatUint(uint64(node.ID), 10)

	response := serveModuleRequest(router, http.MethodPost, "/enrollment-tokens", `{"node":"`+nodeName+`","ttl_seconds":3600}`)
	require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
	assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	var created struct {
		Data struct {
			Enrollment map[string]any `json:"enrollment"`
			Credential string         `json:"credential"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &created))
	assert.True(t, strings.HasPrefix(created.Data.Credential, agentcontrol.EnrollmentCredentialPrefix))
	assert.Equal(t, "proxy", created.Data.Enrollment["node_kind"])
	assert.Equal(t, model.AgentEnrollmentMethodCredential, created.Data.Enrollment["method"])
	assert.NotContains(t, created.Data.Enrollment, "credential_hash")
	assert.EqualValues(t, 7, created.Data.Enrollment["created_by"])

	var stored model.AgentEnrollment
	require.NoError(t, db.First(&stored).Error)
	require.NotNil(t, stored.CredentialHash)
	assert.NotContains(t, *stored.CredentialHash, created.Data.Credential)
	var audit model.OperationLog
	require.NoError(t, db.Where("action = ?", agentpki.AuditActionTokenIssue).First(&audit).Error)
	assert.Equal(t, "admin@example.com", audit.Username)
	assert.NotContains(t, audit.Content, created.Data.Credential)

	for body, code := range map[string]int{
		`{"node":"` + nodeName + `"}`: http.StatusCreated,
		`{"node":"proxy-4242"}`:       http.StatusNotFound,
		`{"node":"module-1"}`:         http.StatusBadRequest,
		`{}`:                          http.StatusBadRequest,
		`{"node":"` + nodeName + `","ttl_seconds":604801}`:      http.StatusBadRequest,
		`{"node":"` + nodeName + `","ttl_seconds":-1}`:          http.StatusBadRequest,
		`{"node":"` + nodeName + `","ttl_seconds":"a"}`:         http.StatusBadRequest,
		`{"node":"` + nodeName + `","ttl_seconds":604800}`:      http.StatusCreated,
		`{"node":"forward-` + strconv.Itoa(int(node.ID)) + `"}`: http.StatusNotFound,
	} {
		response := serveModuleRequest(router, http.MethodPost, "/enrollment-tokens", body)
		assert.Equal(t, code, response.Code, body)
	}
}

func TestAgentEnrollmentTokenWithoutBuiltinPKI(t *testing.T) {
	router := newAgentPKITestRouter(func() (*agentpki.Service, error) { return nil, agentpki.ErrDisabled })
	response := serveModuleRequest(router, http.MethodPost, "/enrollment-tokens", `{"node":"proxy-1"}`)
	assert.Equal(t, http.StatusConflict, response.Code)
	assert.Contains(t, response.Body.String(), "agent_pki_disabled")
}
