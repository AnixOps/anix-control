// Package settings keeps named JSON documents of the identity service's
// configuration in a SQL table
//
//	setting(setting_key VARCHAR(64) PK, value TEXT, updated_at BIGINT)
//
// so every replica reads the same configuration.
package settings

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Store reads and writes Table.
type Store struct {
	DB    *gorm.DB
	Table string
}

type row struct {
	Key       string `gorm:"column:setting_key;primaryKey"`
	Value     string `gorm:"column:value"`
	UpdatedAt int64  `gorm:"column:updated_at"`
}

// Get returns a document and whether it exists.
func (s *Store) Get(ctx context.Context, key string) ([]byte, bool, error) {
	var stored row
	err := s.DB.WithContext(ctx).Table(s.Table).Where("setting_key = ?", key).Take(&stored).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return []byte(stored.Value), true, nil
}

// Put writes a document.
func (s *Store) Put(ctx context.Context, key string, value []byte) error {
	return s.DB.WithContext(ctx).Table(s.Table).Clauses(clause.OnConflict{UpdateAll: true}).
		Create(&row{Key: key, Value: string(value), UpdatedAt: time.Now().Unix()}).Error
}
