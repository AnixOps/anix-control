// Package knowledgecompat proves the knowledge package's native routes
// answer exactly as the kernel's legacy handlers, on SQLite and PostgreSQL:
// the same bytes (tokens of time masked) and the same resulting rows.
package knowledgecompat

import (
	"context"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/internal/handler"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/tests/packagecompat"
	"github.com/AnixOps/anix-control/v4/packages/knowledge/native"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func route(method, pattern, routeID string, legacy gin.HandlerFunc) packagecompat.Route {
	return packagecompat.Route{
		Method: method, Pattern: pattern, RouteID: routeID, Legacy: legacy, Models: []any{&model.Knowledge{}},
		Native: func(db *gorm.DB) pluginhostsdk.NativeHandler {
			service := &native.Service{Open: func(ctx context.Context) (*gorm.DB, error) { return db.WithContext(ctx), nil }}
			return service.Handlers()[routeID]
		},
	}
}

var seeded = time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)

// seed writes visible and hidden articles in an order the sort must fix.
func seed(t testing.TB, db *gorm.DB) {
	require.NoError(t, db.Create(&[]model.Knowledge{
		{ID: 1, Category: "公告", Title: "second", Body: "b1", Sort: 2, Show: 1, CreatedAt: seeded, UpdatedAt: seeded},
		{ID: 2, Category: "帮助", Title: "first", Body: "b2", Sort: 1, Show: 1, CreatedAt: seeded.Add(time.Hour), UpdatedAt: seeded.Add(time.Hour)},
		{ID: 3, Category: "公告", Title: "hidden", Body: "b3", Sort: 0, Show: 0, CreatedAt: seeded, UpdatedAt: seeded},
		{ID: 4, Category: "帮助", Title: "tie", Body: "b4", Sort: 1, Show: 1, CreatedAt: seeded.Add(2 * time.Hour), UpdatedAt: seeded},
	}).Error)
	syncSequence(t, db)
}

// syncSequence moves PostgreSQL's id sequence past explicitly seeded ids, so
// both sides create the next article with the same id.
func syncSequence(t testing.TB, db *gorm.DB) {
	if db.Name() == "postgres" {
		require.NoError(t, db.Exec("SELECT setval(pg_get_serial_sequence('v2_knowledge', 'id'), (SELECT MAX(id) FROM v2_knowledge))").Error)
	}
}

func onlyHidden(t testing.TB, db *gorm.DB) {
	require.NoError(t, db.Create(&model.Knowledge{ID: 9, Title: "hidden", Body: "b", Show: 0, CreatedAt: seeded, UpdatedAt: seeded}).Error)
	syncSequence(t, db)
}

// rows is the table state after a write, without the times the handlers set
// from their clock.
func rows(t testing.TB, db *gorm.DB) any {
	var articles []struct {
		ID       uint
		Category string
		Title    string
		Body     string
		Sort     int
		Show     int
	}
	require.NoError(t, db.Model(&model.Knowledge{}).Order("id").Find(&articles).Error)
	return articles
}

func TestUserRoutesParity(t *testing.T) {
	user := handler.NewKnowledgeHandler()
	list := route("GET", "/api/v2/user/knowledge", "knowledge.article.list", user.GetArticles)
	packagecompat.RunRead(t, list, packagecompat.Case{Name: "visible articles in order", Path: "/api/v2/user/knowledge", Seed: seed})
	packagecompat.RunRead(t, list, packagecompat.Case{Name: "nothing visible answers null", Path: "/api/v2/user/knowledge", Seed: onlyHidden})

	get := route("GET", "/api/v2/user/knowledge/:id", "knowledge.user.knowledge.id.get", user.GetArticle)
	for _, c := range []packagecompat.Case{
		{Name: "visible", Path: "/api/v2/user/knowledge/2"},
		{Name: "hidden", Path: "/api/v2/user/knowledge/3"},
		{Name: "missing", Path: "/api/v2/user/knowledge/99"},
		{Name: "invalid id", Path: "/api/v2/user/knowledge/abc"},
	} {
		c.Seed = seed
		packagecompat.RunRead(t, get, c)
	}
}

func TestAdminRoutesParity(t *testing.T) {
	admin := handler.NewAdminKnowledgeHandler()
	principal := pluginhostsdk.Principal{ActorID: 1, Admin: true}
	list := route("GET", "/api/v2/admin/knowledge", "knowledge.admin.knowledge.get", admin.GetArticles)
	packagecompat.RunRead(t, list, packagecompat.Case{Name: "every article in order", Path: "/api/v2/admin/knowledge", Seed: seed, Principal: principal})
	packagecompat.RunRead(t, list, packagecompat.Case{Name: "empty answers []", Path: "/api/v2/admin/knowledge", Principal: principal})

	write := func(r packagecompat.Route, cases []packagecompat.Case) {
		for _, c := range cases {
			c.Principal, c.Snapshot = principal, rows
			if c.Seed == nil {
				c.Seed = seed
			}
			packagecompat.RunWrite(t, r, c)
		}
	}
	created := []string{"data.data.created_at", "data.data.updated_at"}
	write(route("POST", "/api/v2/admin/knowledge", "knowledge.admin.knowledge.post", admin.CreateArticle), []packagecompat.Case{
		{Name: "create", Path: "/api/v2/admin/knowledge", Mask: created, Body: []byte(`{"category":"帮助","title":"t","body":"b","sort":3,"show":1}`)},
		{Name: "create with defaults", Path: "/api/v2/admin/knowledge", Mask: created, Body: []byte(`{"title":"t","body":"b"}`)},
		{Name: "missing title", Path: "/api/v2/admin/knowledge", Body: []byte(`{"body":"b"}`)},
		{Name: "invalid JSON", Path: "/api/v2/admin/knowledge", Body: []byte(`{"title":`)},
	})
	write(route("PUT", "/api/v2/admin/knowledge/:id", "knowledge.admin.knowledge.id.put", admin.UpdateArticle), []packagecompat.Case{
		{Name: "partial update", Path: "/api/v2/admin/knowledge/1", Body: []byte(`{"title":"renamed","show":0,"sort":7}`)},
		{Name: "empty update", Path: "/api/v2/admin/knowledge/1", Body: []byte(`{}`)},
		{Name: "show out of range", Path: "/api/v2/admin/knowledge/1", Body: []byte(`{"show":2}`)},
		{Name: "missing", Path: "/api/v2/admin/knowledge/99", Body: []byte(`{"title":"x"}`)},
		{Name: "invalid id", Path: "/api/v2/admin/knowledge/abc", Body: []byte(`{"title":"x"}`)},
	})
	write(route("DELETE", "/api/v2/admin/knowledge/:id", "knowledge.admin.knowledge.id.delete", admin.DeleteArticle), []packagecompat.Case{
		{Name: "delete", Path: "/api/v2/admin/knowledge/2"},
		{Name: "missing", Path: "/api/v2/admin/knowledge/99"},
		{Name: "invalid id", Path: "/api/v2/admin/knowledge/abc"},
	})
}
