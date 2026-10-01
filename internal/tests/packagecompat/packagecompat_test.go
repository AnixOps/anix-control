package packagecompat

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/sdk/v2compat"
	"github.com/AnixOps/anix-control/v4/internal/handler"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// The knowledge routes serve as the harness's own fixture: these native
// implementations follow the legacy handlers, including their quirks.

type knowledgeItem struct {
	ID        uint   `json:"id"`
	Category  string `json:"category"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	UpdatedAt int64  `json:"updated_at"`
}

func nativeKnowledgeList(db *gorm.DB) pluginhostsdk.NativeHandler {
	return func(ctx context.Context, _ pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
		var articles []model.Knowledge
		if err := db.WithContext(ctx).Where("show = ?", 1).Order("sort ASC, created_at DESC").Find(&articles).Error; err != nil {
			return pluginhostsdk.PanelJSON(v2compat.PanelError("获取知识库失败", time.Now()))
		}
		// The legacy handler returns data: null for an empty list.
		var items []knowledgeItem
		for _, article := range articles {
			items = append(items, knowledgeItem{article.ID, article.Category, article.Title, article.Body, article.UpdatedAt.Unix()})
		}
		return pluginhostsdk.PanelJSON(v2compat.PanelSuccess(items, time.Now()))
	}
}

func nativeKnowledgeDetail(db *gorm.DB) pluginhostsdk.NativeHandler {
	return func(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
		id, err := strconv.ParseUint(request.Metadata.PathParams["id"], 10, 32)
		if err != nil {
			return pluginhostsdk.PanelJSON(v2compat.PanelError("无效的文章ID", time.Now()))
		}
		var article model.Knowledge
		if err := db.WithContext(ctx).Where("id = ? AND show = ?", id, 1).First(&article).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return pluginhostsdk.PanelJSON(v2compat.PanelError("文章不存在", time.Now()))
			}
			return pluginhostsdk.PanelJSON(v2compat.PanelError("获取知识库失败", time.Now()))
		}
		return pluginhostsdk.PanelJSON(v2compat.PanelSuccess(
			knowledgeItem{article.ID, article.Category, article.Title, article.Body, article.UpdatedAt.Unix()}, time.Now()))
	}
}

func nativeKnowledgeDelete(db *gorm.DB) pluginhostsdk.NativeHandler {
	return func(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
		id, err := strconv.ParseUint(request.Metadata.PathParams["id"], 10, 32)
		if err != nil {
			return pluginhostsdk.PanelJSON(v2compat.PanelError("无效的文章ID", time.Now()))
		}
		var article model.Knowledge
		if err := db.WithContext(ctx).First(&article, id).Error; err != nil {
			return pluginhostsdk.PanelJSON(v2compat.PanelError("文章不存在", time.Now()))
		}
		if err := db.WithContext(ctx).Delete(&article).Error; err != nil {
			return pluginhostsdk.PanelJSON(v2compat.PanelError("删除失败", time.Now()))
		}
		return pluginhostsdk.PanelJSON(v2compat.PanelSuccess(map[string]string{"message": "删除成功"}, time.Now()))
	}
}

func seedKnowledge(t testing.TB, db *gorm.DB) {
	t.Helper()
	updated := time.Unix(1_790_000_000, 0)
	for _, article := range []model.Knowledge{
		{ID: 1, Category: "公告", Title: "Welcome", Body: "hello", Sort: 2, Show: 1, CreatedAt: updated, UpdatedAt: updated},
		{ID: 2, Category: "教程", Title: "Setup", Body: "steps", Sort: 1, Show: 1, CreatedAt: updated, UpdatedAt: updated},
		{ID: 3, Category: "公告", Title: "Hidden", Body: "secret", Sort: 0, Show: 1, CreatedAt: updated, UpdatedAt: updated},
	} {
		require.NoError(t, db.Create(&article).Error)
	}
	require.NoError(t, db.Model(&model.Knowledge{}).Where("id = ?", 3).Update("show", 0).Error)
}

func knowledgeIDs(t testing.TB, db *gorm.DB) any {
	t.Helper()
	var ids []uint
	require.NoError(t, db.Model(&model.Knowledge{}).Order("id").Pluck("id", &ids).Error)
	return ids
}

var member = pluginhostsdk.Principal{ActorID: 7}

func TestRunReadComparesKnowledgeRoutes(t *testing.T) {
	list := Route{
		Method: http.MethodGet, Pattern: "/api/v2/user/knowledge", RouteID: "knowledge.article.list",
		Legacy: handler.NewKnowledgeHandler().GetArticles, Native: nativeKnowledgeList, Models: []any{&model.Knowledge{}},
	}
	RunRead(t, list, Case{Name: "visible articles", Path: "/api/v2/user/knowledge", Principal: member, Seed: seedKnowledge})
	RunRead(t, list, Case{Name: "empty list is null", Path: "/api/v2/user/knowledge", Principal: member})

	detail := Route{
		Method: http.MethodGet, Pattern: "/api/v2/user/knowledge/:id", RouteID: "knowledge.user.knowledge.id.get",
		Legacy: handler.NewKnowledgeHandler().GetArticle, Native: nativeKnowledgeDetail, Models: []any{&model.Knowledge{}},
	}
	for name, path := range map[string]string{
		"visible": "/api/v2/user/knowledge/1", "hidden": "/api/v2/user/knowledge/3",
		"missing": "/api/v2/user/knowledge/99", "invalid": "/api/v2/user/knowledge/abc",
	} {
		RunRead(t, detail, Case{Name: name, Path: path, Principal: member, Seed: seedKnowledge})
	}
}

func TestRunWriteComparesDatabaseState(t *testing.T) {
	remove := Route{
		Method: http.MethodDelete, Pattern: "/api/v2/admin/knowledge/:id", RouteID: "knowledge.admin.knowledge.id.delete",
		Legacy: handler.NewAdminKnowledgeHandler().DeleteArticle, Native: nativeKnowledgeDelete, Models: []any{&model.Knowledge{}},
	}
	admin := pluginhostsdk.Principal{ActorID: 1, Admin: true}
	RunWrite(t, remove, Case{Name: "delete", Path: "/api/v2/admin/knowledge/2", Principal: admin, Seed: seedKnowledge, Snapshot: knowledgeIDs})
	RunWrite(t, remove, Case{Name: "delete missing", Path: "/api/v2/admin/knowledge/42", Principal: admin, Seed: seedKnowledge, Snapshot: knowledgeIDs})
}

// Both sides see the case's request headers: the legacy handler on the
// request, the native one in the forwarded metadata.
func TestRequestHeadersReachBothSides(t *testing.T) {
	echo := Route{
		Method: http.MethodGet, Pattern: "/api/v2/echo", RouteID: "echo",
		Legacy: func(c *gin.Context) {
			c.JSON(http.StatusOK, v2compat.PanelSuccess(c.GetHeader("Idempotency-Key"), time.Now()))
		},
		Native: func(*gorm.DB) pluginhostsdk.NativeHandler {
			return func(_ context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
				return pluginhostsdk.PanelJSON(v2compat.PanelSuccess(request.Metadata.Headers["Idempotency-Key"][0], time.Now()))
			}
		},
	}
	RunRead(t, echo, Case{Name: "idempotency key", Path: "/api/v2/echo", RequestHeaders: map[string]string{"idempotency-key": "k-1"}})
	legacy, native := run(t, openSQLite, echo, Case{Path: "/api/v2/echo", RequestHeaders: map[string]string{"Idempotency-Key": "k-1"}})
	require.NoError(t, CompareAnswers(legacy, native))
	require.Contains(t, string(native.Body), `"data":"k-1"`, "the header is not lost on both sides")
}

func TestCompareAnswersReportsDifferences(t *testing.T) {
	legacy := Result{StatusCode: 200, Body: []byte(`{"code":0,"data":null,"msg":"操作成功","ts":1}`)}
	require.NoError(t, CompareAnswers(legacy, Result{StatusCode: 200, Body: []byte(`{"ts":2,"msg":"操作成功","data":null,"code":0}`)}),
		"ts and key order are ignored")
	require.ErrorContains(t, CompareAnswers(legacy, Result{StatusCode: 200, Body: []byte(`{"code":0,"data":[],"msg":"操作成功","ts":1}`)}),
		"normalized body differs")
	require.ErrorContains(t, CompareAnswers(legacy, Result{StatusCode: 500, Body: legacy.Body}), "status code differs")
}
