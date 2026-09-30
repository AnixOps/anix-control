// Package native implements the knowledge package's v2 routes in the
// package itself. The package adopts the kernel's v2_knowledge table
// (kernel.storage.adopt:v2_knowledge), so the native routes and the kernel's
// legacy handlers read and write the same rows: switching a route between
// legacy and native needs no data move and can be undone at any time.
// Responses are byte-compatible with the legacy handlers
// (internal/tests/knowledgecompat).
package native

import (
	"context"
	"errors"
	"log"
	"strconv"
	"time"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/sdk/v2compat"
	"github.com/gin-gonic/gin/binding"
	"gorm.io/gorm"
)

// Article is a v2_knowledge row. Its JSON matches the kernel model, which
// the admin create answer returns as is.
type Article struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Category  string    `gorm:"size:64" json:"category"`
	Title     string    `gorm:"size:255" json:"title"`
	Body      string    `gorm:"type:text" json:"body"`
	Sort      int       `gorm:"default:0" json:"sort"`
	Show      int       `gorm:"default:1" json:"show"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName is the adopted kernel table.
func (Article) TableName() string { return "v2_knowledge" }

// Service holds what the native routes need.
type Service struct {
	// Open returns the package's storage connection, on which the adopted
	// v2_knowledge table is visible.
	Open func(ctx context.Context) (*gorm.DB, error)
	// Now defaults to time.Now.
	Now func() time.Time
}

// Handlers returns the native handlers by route id.
func (s *Service) Handlers() map[string]pluginhostsdk.NativeHandler {
	return map[string]pluginhostsdk.NativeHandler{
		"knowledge.admin.knowledge.get":       s.AdminList,
		"knowledge.admin.knowledge.post":      s.AdminCreate,
		"knowledge.admin.knowledge.id.put":    s.AdminUpdate,
		"knowledge.admin.knowledge.id.delete": s.AdminDelete,
		"knowledge.article.list":              s.UserList,
		"knowledge.user.knowledge.id.get":     s.UserGet,
	}
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Service) panel(data any) (pluginhostsdk.NativeResponse, error) {
	return pluginhostsdk.PanelJSON(v2compat.PanelSuccess(data, s.now()))
}

func (s *Service) panelError(message string) (pluginhostsdk.NativeResponse, error) {
	return pluginhostsdk.PanelJSON(v2compat.PanelError(message, s.now()))
}

// AdminList is GET /api/v2/admin/knowledge.
func (s *Service) AdminList(ctx context.Context, _ pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError("获取文章列表失败")
	}
	var articles []Article
	if err := db.Order("sort ASC, created_at DESC").Find(&articles).Error; err != nil {
		log.Printf("admin knowledge list failed: %v", err)
		return s.panelError("获取文章列表失败")
	}
	result := make([]map[string]any, 0, len(articles))
	for _, a := range articles {
		result = append(result, map[string]any{
			"id": a.ID, "category": a.Category, "title": a.Title, "body": a.Body, "sort": a.Sort, "show": a.Show,
			"created_at": a.CreatedAt.Unix(), "updated_at": a.UpdatedAt.Unix(),
		})
	}
	return s.panel(result)
}

// AdminCreate is POST /api/v2/admin/knowledge.
func (s *Service) AdminCreate(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	// An anonymous struct, as in v2: validation messages name its fields
	// without a type.
	var req struct {
		Category string `json:"category"`
		Title    string `json:"title" binding:"required"`
		Body     string `json:"body" binding:"required"`
		Sort     int    `json:"sort"`
		Show     int    `json:"show"`
	}
	if err := binding.JSON.BindBody(request.Body, &req); err != nil {
		return s.panelError("参数错误: " + err.Error())
	}
	if req.Category == "" {
		req.Category = "公告"
	}
	if req.Show == 0 {
		req.Show = 1
	}
	now := s.now()
	article := Article{Category: req.Category, Title: req.Title, Body: req.Body, Sort: req.Sort, Show: req.Show, CreatedAt: now, UpdatedAt: now}
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError("创建失败")
	}
	if err := db.Create(&article).Error; err != nil {
		log.Printf("admin knowledge create failed: %v", err)
		return s.panelError("创建失败")
	}
	return s.panel(map[string]any{"message": "创建成功", "data": article})
}

func parseID(raw string) (uint64, bool) {
	id, err := strconv.ParseUint(raw, 10, 32)
	return id, err == nil
}

// AdminUpdate is PUT /api/v2/admin/knowledge/:id.
func (s *Service) AdminUpdate(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	id, ok := parseID(request.Metadata.PathParams["id"])
	if !ok {
		return s.panelError("无效的文章ID")
	}
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError("文章不存在")
	}
	var article Article
	if err := db.First(&article, id).Error; err != nil {
		return s.panelError("文章不存在")
	}
	var req struct {
		Category string `json:"category" binding:"omitempty,max=64"`
		Title    string `json:"title" binding:"omitempty,min=1,max=255"`
		Body     string `json:"body" binding:"omitempty"`
		Sort     *int   `json:"sort"`
		Show     *int   `json:"show" binding:"omitempty,oneof=0 1"`
	}
	if err := binding.JSON.BindBody(request.Body, &req); err != nil {
		return s.panelError("参数错误: " + err.Error())
	}
	updates := map[string]any{"updated_at": s.now()}
	if req.Category != "" {
		updates["category"] = req.Category
	}
	if req.Title != "" {
		updates["title"] = req.Title
	}
	if req.Body != "" {
		updates["body"] = req.Body
	}
	if req.Sort != nil {
		updates["sort"] = *req.Sort
	}
	if req.Show != nil {
		updates["show"] = *req.Show
	}
	if err := db.Model(&article).Updates(updates).Error; err != nil {
		log.Printf("admin knowledge update failed: %v", err)
		return s.panelError("更新失败")
	}
	return s.panel(map[string]any{"message": "更新成功"})
}

// AdminDelete is DELETE /api/v2/admin/knowledge/:id.
func (s *Service) AdminDelete(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	id, ok := parseID(request.Metadata.PathParams["id"])
	if !ok {
		return s.panelError("无效的文章ID")
	}
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError("文章不存在")
	}
	var article Article
	if err := db.First(&article, id).Error; err != nil {
		return s.panelError("文章不存在")
	}
	if err := db.Delete(&article).Error; err != nil {
		log.Printf("admin knowledge delete failed: %v", err)
		return s.panelError("删除失败")
	}
	return s.panel(map[string]any{"message": "删除成功"})
}

// UserList is GET /api/v2/user/knowledge: visible articles.
func (s *Service) UserList(ctx context.Context, _ pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError("获取知识库失败")
	}
	var articles []Article
	if err := db.Where("show = ?", 1).Order("sort ASC, created_at DESC").Find(&articles).Error; err != nil {
		log.Printf("user knowledge list failed: %v", err)
		return s.panelError("获取知识库失败")
	}
	// v2 answers null, not [], when nothing is visible.
	var result []map[string]any
	for _, a := range articles {
		result = append(result, map[string]any{
			"id": a.ID, "category": a.Category, "title": a.Title, "body": a.Body, "updated_at": a.UpdatedAt.Unix(),
		})
	}
	return s.panel(result)
}

// UserGet is GET /api/v2/user/knowledge/:id.
func (s *Service) UserGet(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	id, ok := parseID(request.Metadata.PathParams["id"])
	if !ok {
		return s.panelError("无效的文章ID")
	}
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError("获取知识库失败")
	}
	var article Article
	if err := db.Where("id = ? AND show = ?", id, 1).First(&article).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return s.panelError("文章不存在")
		}
		log.Printf("user knowledge detail failed: %v", err)
		return s.panelError("获取知识库失败")
	}
	return s.panel(map[string]any{
		"id": article.ID, "category": article.Category, "title": article.Title, "body": article.Body,
		"updated_at": article.UpdatedAt.Unix(),
	})
}
