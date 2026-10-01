package native

import (
	"context"
	"errors"
	"strings"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/packages/subscription/native/model"
	"github.com/gin-gonic/gin/binding"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GroupTemplates is GET /api/v2/admin/subscription/groups/:id/templates.
func (s *Service) GroupTemplates(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	id, ok := pathUint(request, "id")
	if !ok {
		return s.panelError(invalidID)
	}
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	var templates []*model.SubscriptionTemplate
	if err := db.Where("group_id = ?", id).Order("sort ASC, id ASC").Find(&templates).Error; err != nil {
		return s.panelError(err.Error())
	}
	return s.panel(templates)
}

// CreateTemplate is POST /api/v2/admin/subscription/groups/:id/templates.
// As in the kernel, only the template's own columns are written, in the
// group the path names: a nested "group" is dropped.
func (s *Service) CreateTemplate(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	groupID, ok := pathUint(request, "id")
	if !ok {
		return s.panelError("分组 ID 无效")
	}
	var template model.SubscriptionTemplate
	if err := binding.JSON.BindBody(request.Body, &template); err != nil {
		return s.panelError(err.Error())
	}
	template.GroupID = groupID
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	template.Group = nil
	if err := db.Omit(clause.Associations).Create(&template).Error; err != nil {
		return s.panelError(err.Error())
	}
	return s.panel(template)
}

// Template is GET /api/v2/admin/subscription/templates/:id.
func (s *Service) Template(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	id, ok := pathUint(request, "id")
	if !ok {
		return s.panelError(invalidID)
	}
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError(templateNotFound)
	}
	var template model.SubscriptionTemplate
	if err := db.First(&template, id).Error; err != nil {
		return s.panelError(templateNotFound)
	}
	return s.panel(&template)
}

// UpdateTemplate is PUT /api/v2/admin/subscription/templates/:id: a
// partial update by column, as the kernel's UpdateTemplateFields does,
// answered with the stored template.
func (s *Service) UpdateTemplate(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	id, ok := pathUint(request, "id")
	if !ok {
		return s.panelError(invalidID)
	}
	var updates map[string]any
	if err := binding.JSON.BindBody(request.Body, &updates); err != nil {
		return s.panelError(err.Error())
	}
	if len(updates) == 0 {
		return s.panelError("参数错误")
	}
	normalizeTemplateUpdate(updates)
	for key := range updates {
		if isProtectedTemplateKey(key) {
			delete(updates, key)
		}
	}
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		var template model.SubscriptionTemplate
		if err := tx.First(&template, id).Error; err != nil {
			return err
		}
		return tx.Model(&template).Updates(updates).Error
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return s.panelError(templateNotFound)
		}
		return s.panelError(err.Error())
	}
	var template model.SubscriptionTemplate
	if err := db.First(&template, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return s.panelError(templateNotFound)
		}
		return s.panelError(err.Error())
	}
	return s.panel(&template)
}

// normalizeTemplateUpdate is the kernel's normalizeTemplateUpdatePayload:
// JSON numbers and booleans of integer columns become ints.
func normalizeTemplateUpdate(updates map[string]any) {
	for _, field := range []string{"group_id", "port", "tls", "enable", "sort"} {
		value, exists := updates[field]
		if !exists {
			continue
		}
		switch v := value.(type) {
		case float64:
			updates[field] = int(v)
		case bool:
			if v {
				updates[field] = 1
			} else {
				updates[field] = 0
			}
		}
	}
}

// isProtectedTemplateKey is the kernel's: an update never writes the id or
// the timestamps, whichever spelling GORM or SQLite would accept for them.
func isProtectedTemplateKey(key string) bool {
	switch strings.ToLower(strings.ReplaceAll(key, "_", "")) {
	case "id", "createdat", "updatedat":
		return true
	}
	return false
}

// DeleteTemplate is DELETE /api/v2/admin/subscription/templates/:id.
func (s *Service) DeleteTemplate(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	id, ok := pathUint(request, "id")
	if !ok {
		return s.panelError(invalidID)
	}
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		var template model.SubscriptionTemplate
		if err := tx.First(&template, id).Error; err != nil {
			return err
		}
		return tx.Delete(&template).Error
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return s.panelError(templateNotFound)
		}
		return s.panelError(err.Error())
	}
	return s.panel(map[string]any{"message": "删除成功"})
}
