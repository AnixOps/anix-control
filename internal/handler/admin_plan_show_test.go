package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// The user's plan list holds only plans with show = 1, and the admin form
// never sent a show value. A plan created without one must be on sale; an
// explicit show keeps its value, and an update replaces the stored one.
func TestAdminCreatePlanIsOnSaleUnlessShowIsGiven(t *testing.T) {
	gin.SetMode(gin.TestMode)
	database.Reset()
	require.NoError(t, database.Init(&config.DatabaseConfig{Driver: "sqlite", Database: filepath.Join(t.TempDir(), "plans.db"), LogLevel: "silent"}))
	t.Cleanup(func() {
		_ = database.Close()
		database.Reset()
		service.ResetStatsServiceForTest()
	})
	db := database.GetDB()
	require.NoError(t, db.AutoMigrate(&model.Plan{}, &model.Event{}))

	admin := NewAdminHandler()
	router := gin.New()
	router.POST("/admin/plans", admin.CreatePlan)
	router.PUT("/admin/plans/:id", admin.UpdatePlan)
	router.GET("/user/plan", NewUserPlanHandler().GetPlans)

	send := func(method, path, body string) map[string]any {
		t.Helper()
		request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		var answer map[string]any
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &answer), recorder.Body.String())
		return answer
	}
	create := func(body string) model.Plan {
		t.Helper()
		answer := send(http.MethodPost, "/admin/plans", body)
		require.EqualValues(t, 0, answer["code"], answer)
		data := answer["data"].(map[string]any)
		var stored model.Plan
		require.NoError(t, db.First(&stored, uint(data["id"].(float64))).Error)
		require.EqualValues(t, stored.Show, data["show"], "the answer names the stored value")
		return stored
	}

	for name, tc := range map[string]struct {
		body string
		want int
	}{
		"show omitted":              {`{"name":"Omitted","transfer_enable":10}`, 1},
		"show null":                 {`{"name":"Null","show":null}`, 1},
		"show on":                   {`{"name":"On","show":1}`, 1},
		"show off":                  {`{"name":"Off","show":0}`, 0},
		"show off, other spelling":  {`{"name":"Cased","Show":0}`, 0},
		"other fields without show": {`{"name":"Fields","group_id":2,"renew":0,"month_price":500}`, 1},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, create(tc.body).Show)
		})
	}

	// The user's plan list shows what is on sale, the hidden ones stay out.
	list := send(http.MethodGet, "/user/plan", "")
	names := map[string]bool{}
	for _, item := range list["data"].([]any) {
		names[item.(map[string]any)["name"].(string)] = true
	}
	require.True(t, names["Omitted"] && names["Null"] && names["On"] && names["Fields"], names)
	require.False(t, names["Off"] || names["Cased"], names)

	// An update still writes the body as given: an omitted show is 0 there,
	// which is why the admin form sends the stored plan back with it.
	var off model.Plan
	require.NoError(t, db.Where("name = ?", "Off").First(&off).Error)
	require.EqualValues(t, 0, send(http.MethodPut, "/admin/plans/"+strconv.FormatUint(uint64(off.ID), 10), `{"name":"Off","show":1}`)["code"])
	require.NoError(t, db.First(&off, off.ID).Error)
	require.Equal(t, 1, off.Show)
}
