package v2

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/pluginhost"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestDataEnvelopeStripsGatewayOwnedAndHopByHopHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)

	err := writePackageResponse(context, Route{Envelope: EnvelopeData}, pluginhost.DispatchOutput{
		StatusCode: http.StatusOK,
		Body:       []byte(`{"items":[]}`),
		Headers: []pluginhost.Header{
			{Name: "Content-Type", Value: "text/plain"},
			{Name: "Content-Length", Value: "1"},
			{Name: "Content-Encoding", Value: "gzip"},
			{Name: "Connection", Value: "X-Package-Hop"},
			{Name: "X-Package-Hop", Value: "discard"},
			{Name: "X-Package-Trace", Value: "keep"},
		},
	})

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, `{"data":{"items":[]}}`, recorder.Body.String())
	require.Contains(t, recorder.Header().Get("Content-Type"), "application/json")
	require.Empty(t, recorder.Header().Get("Content-Encoding"))
	require.Empty(t, recorder.Header().Get("X-Package-Hop"))
	require.Equal(t, "keep", recorder.Header().Get("X-Package-Trace"))
}

func TestDataEnvelopePreservesAnAlreadyPanelShapedLegacyResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	body := []byte(`{"code":0,"msg":"操作成功","ts":1700000000000,"data":{"list":[]}}`)

	err := writePackageResponse(context, Route{Envelope: EnvelopeData}, pluginhost.DispatchOutput{
		StatusCode: http.StatusOK,
		Body:       body,
	})

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, string(body), recorder.Body.String())
	require.Contains(t, recorder.Header().Get("Content-Type"), "application/json")
}

func TestPanelEnvelopePreservesTheFullLegacyResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	body := []byte(`{"code":0,"msg":"操作成功","ts":1700000000000,"data":{"token":"issued"}}`)

	err := writePackageResponse(context, Route{Envelope: EnvelopePanel}, pluginhost.DispatchOutput{
		StatusCode: http.StatusOK,
		Body:       body,
		Headers:    []pluginhost.Header{{Name: "Content-Type", Value: "text/plain"}},
	})

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, string(body), recorder.Body.String())
	require.Contains(t, recorder.Header().Get("Content-Type"), "application/json")
}

func TestPanelEnvelopeRejectsNonPanelPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)

	err := writePackageResponse(context, Route{Envelope: EnvelopePanel}, pluginhost.DispatchOutput{
		StatusCode: http.StatusOK, Body: []byte(`{"data":{"token":"issued"}}`),
	})

	require.Error(t, err)
}
