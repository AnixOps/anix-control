package service

import (
	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// CLIActor is the actor the command line records in the audit log.
const CLIActor = "system/cli"

// IsSuperAdmin reports whether userID is a super administrator. The kernel
// has no separate super-administrator role, so this is its strictest
// administrator check, read from the database rather than the token: an
// administrator (is_admin) who is not staff and not banned. A demoted or
// banned administrator loses the right at once, not when the token expires.
func IsSuperAdmin(db *gorm.DB, userID uint) (bool, error) {
	if db == nil || userID == 0 {
		return false, nil
	}
	var users []model.User
	if err := db.Select("id", "is_admin", "is_staff", "banned").Where("id = ?", userID).Limit(1).Find(&users).Error; err != nil {
		return false, err
	}
	if len(users) == 0 {
		return false, nil
	}
	user := users[0]
	return user.IsAdmin == 1 && user.IsStaff == 0 && user.Banned == 0, nil
}
