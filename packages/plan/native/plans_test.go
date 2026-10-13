package native

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// The user's plan list holds only plans with show = 1, and the admin form
// never sent a show value: a plan created without one is on sale, an
// explicit show keeps its value (as the kernel's handler does,
// internal/tests/plancompat compares the two).
func TestAdminCreateIsOnSaleUnlessShowIsGiven(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "plans.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Plan{}, &Event{}))
	service := &Service{Open: func(ctx context.Context) (*gorm.DB, error) { return db.WithContext(ctx), nil }}

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
			response, err := service.AdminCreate(context.Background(), pluginhostsdk.NativeRequest{Body: []byte(tc.body)})
			require.NoError(t, err)
			require.Contains(t, string(response.Body), `"code":0`)
			var stored Plan
			require.NoError(t, db.Order("id DESC").First(&stored).Error)
			require.Equal(t, tc.want, stored.Show)
		})
	}

	// A body that is not an object is refused as before, whatever its shape.
	for _, body := range []string{``, `[]`, `{"name":`, `{"show":"yes"}`} {
		response, err := service.AdminCreate(context.Background(), pluginhostsdk.NativeRequest{Body: []byte(body)})
		require.NoError(t, err)
		require.NotContains(t, string(response.Body), `"code":0`, body)
	}
	var count int64
	require.NoError(t, db.Model(&Plan{}).Count(&count).Error)
	require.EqualValues(t, 6, count)
}
