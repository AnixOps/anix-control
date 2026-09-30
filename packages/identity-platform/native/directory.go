package native

import (
	"context"

	"gorm.io/gorm"
)

// ViewDirectory reads kapi_user_directory_v1, which identity's storage role
// may read (capability kernel.view:kapi_user_directory_v1).
type ViewDirectory struct {
	DB func(ctx context.Context) (*gorm.DB, error)
}

// ExpiredAt returns the subscriber's expiry, nil when it has none or no row.
func (d ViewDirectory) ExpiredAt(ctx context.Context, userID uint64) (*int64, error) {
	db, err := d.DB(ctx)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		ExpiredAt *int64 `gorm:"column:expired_at"`
	}
	if err := db.WithContext(ctx).Raw("SELECT expired_at FROM kapi_user_directory_v1 WHERE id = ?", userID).Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return rows[0].ExpiredAt, nil
}
