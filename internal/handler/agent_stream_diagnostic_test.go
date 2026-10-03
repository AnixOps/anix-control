package handler

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/agentstreams"
	"github.com/AnixOps/anix-control/v4/internal/kernelnodeops"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A diagnostic task the legacy routes sent on the Agent Control stream is
// answered as on the WebSocket: refused, failed to send, or sent with the
// agent's acknowledgement and the channel.
func TestAnswerStreamDiagnostic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	answer := func(run *kernelnodeops.AgentDiagnostic, err error) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		answerStreamDiagnostic(c, 7, run, err)
		return recorder
	}
	refused := answer(nil, &kernelnodeops.DiagnosticValidationError{Err: errors.New("action is not allowed")})
	assert.Equal(t, http.StatusBadRequest, refused.Code)
	failed := answer(nil, errors.New("database is down"))
	assert.Equal(t, http.StatusInternalServerError, failed.Code)

	task := agentstreams.DiagnosticTask{ID: "task-1", Type: "diagnostic", Action: "ping"}
	unsent := answer(&kernelnodeops.AgentDiagnostic{Task: task, Dispatch: agentstreams.DiagnosticDispatch{MessageID: "task-1", DispatchError: errors.New("agent nack: busy")}}, nil)
	assert.Equal(t, http.StatusInternalServerError, unsent.Code)
	assert.Contains(t, unsent.Body.String(), `"dispatch_err":"agent nack: busy"`)
	assert.Contains(t, unsent.Body.String(), `"channel":"agent_control"`)

	sent := answer(&kernelnodeops.AgentDiagnostic{Task: task, Dispatch: agentstreams.DiagnosticDispatch{MessageID: "task-1", AckReceived: true}}, nil)
	require.Equal(t, http.StatusOK, sent.Code)
	data := string(rawData(t, sent))
	assert.Contains(t, data, `"task_id":"task-1"`)
	assert.Contains(t, data, `"ack_received":true`)
	assert.Contains(t, data, `"channel":"agent_control"`)
}
