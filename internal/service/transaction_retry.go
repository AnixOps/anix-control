package service

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	maxSQLiteTransactionAttempts = 8
	sqliteTransactionRetryDelay  = 10 * time.Millisecond
)

// WithRetryableTransaction reruns the whole transaction only when SQLite
// reports transient writer contention. The mutation may run more than once,
// so it must keep side effects inside the transaction. Other databases and
// semantic failures return immediately.
func WithRetryableTransaction(db *gorm.DB, mutation func(*gorm.DB) error) error {
	if db == nil {
		return errors.New("database is not initialized")
	}
	if mutation == nil {
		return errors.New("transaction mutation is required")
	}

	for attempt := 0; ; attempt++ {
		err := db.Transaction(mutation)
		if err == nil || db.Name() != "sqlite" || !isSQLiteBusyError(err) || attempt+1 >= maxSQLiteTransactionAttempts {
			return err
		}
		time.Sleep(sqliteTransactionRetryDelay * time.Duration(1<<attempt))
	}
}

func isSQLiteBusyError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "database is locked") || strings.Contains(message, "database table is locked") ||
		strings.Contains(message, "sqlite_busy") || strings.Contains(message, "sqlite_locked")
}
