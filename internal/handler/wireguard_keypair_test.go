package handler

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGenerateWireGuardKeypair(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/wireguard/keypair", NewNodeHandler().GenerateWireGuardKeypair)

	req := httptest.NewRequest(http.MethodPost, "/wireguard/keypair", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	require.Equal(t, http.StatusOK, resp.Code)
	var envelope struct {
		Data struct {
			PrivateKey string `json:"private_key"`
			PublicKey  string `json:"public_key"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &envelope))
	privateKey, err := base64.StdEncoding.DecodeString(envelope.Data.PrivateKey)
	require.NoError(t, err)
	publicKey, err := base64.StdEncoding.DecodeString(envelope.Data.PublicKey)
	require.NoError(t, err)
	require.Len(t, privateKey, 32)
	require.Len(t, publicKey, 32)
}
