package native

import (
	"context"
	"database/sql"
	"errors"
	"time"

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

// UserDirectory is the administrator's user directory: Control's
// subscribers with identity's accounts, searched, ordered, paged and counted
// in one query on identity's own storage connection.
//
//   - Control owns the subscribers. Identity reads them through two kernel
//     API views on the same database: kapi_user_directory_v1 (which
//     subscribers exist, since when, and the identity fields Control
//     projects) and kapi_subscriber_entitlement_v1 (plan, group, traffic,
//     limits, reset day, expiry and balances). Neither view shows a
//     subscription token or proxy uuid.
//   - Identity owns the accounts: e-mail, administrator, staff and ban flags
//     come from its account table. A subscriber identity has no account for
//     keeps the fields Control holds, as the user detail does.
//
// Every listed or counted row is a subscriber, so the directory lists what
// v2 listed. A search and its total are read in one read-only transaction
// (a repeatable-read snapshot on PostgreSQL), so a page always agrees with
// its total, and users created in the same instant keep a stable order
// (created_at DESC, id DESC): paging neither repeats nor skips a user.
type UserDirectory struct {
	DB *gorm.DB
	// Accounts is identity's account table, as packagestoresdk names it.
	Accounts string
}

// directory returns the user directory on the stores' connection.
func (s *Stores) directory() UserDirectory {
	return UserDirectory{DB: s.Accounts.DB, Accounts: s.Accounts.Tables.Account}
}

// User statuses of a search, as the v2 list names them. Any other status
// lists every user.
const (
	// UserStatusActive is not banned and not expired.
	UserStatusActive = "active"
	// UserStatusExpired has an expiry that has passed (expired_at <= now).
	UserStatusExpired = "expired"
	// UserStatusBanned is banned.
	UserStatusBanned = "banned"
)

// UserQuery is one search of the directory.
type UserQuery struct {
	// Email, when set, matches e-mails containing it: SQL LIKE '%Email%',
	// whose % and _ match as wildcards, as in v2.
	Email string
	// PlanID, when set, matches the subscribers on that plan.
	PlanID *uint64
	// Status is UserStatusActive, UserStatusExpired, UserStatusBanned, or
	// anything else for every user.
	Status string
	// Now decides expiry.
	Now time.Time
	// Offset and Limit page the result; a negative Offset is 0.
	Offset int
	Limit  int
}

// DirectoryUser is one user of the directory: the v2 list's user. It
// carries no credential; the subscription token and proxy uuid are read
// for one user at a time from the user detail. Its JSON matches the kernel's
// service.UserListItem.
type DirectoryUser struct {
	ID                uint64    `gorm:"column:id" json:"id"`
	Email             string    `gorm:"column:email" json:"email"`
	Balance           int64     `gorm:"column:balance" json:"balance"`
	CommissionBalance int64     `gorm:"column:commission_balance" json:"commission_balance"`
	DeviceLimit       *int64    `gorm:"column:device_limit" json:"device_limit"`
	SpeedLimit        *int64    `gorm:"column:speed_limit" json:"speed_limit"`
	FlowResetTime     int64     `gorm:"column:flow_reset_time" json:"flowResetTime"`
	TransferEnable    int64     `gorm:"column:transfer_enable" json:"transfer_enable"`
	U                 int64     `gorm:"column:u" json:"u"`
	D                 int64     `gorm:"column:d" json:"d"`
	PlanID            *uint64   `gorm:"column:plan_id" json:"plan_id"`
	GroupID           *uint64   `gorm:"column:group_id" json:"group_id"`
	ExpiredAt         *int64    `gorm:"column:expired_at" json:"expired_at"`
	Banned            int       `gorm:"column:banned" json:"banned"`
	IsAdmin           int       `gorm:"column:is_admin" json:"is_admin"`
	IsStaff           int       `gorm:"column:is_staff" json:"is_staff"`
	CreatedAt         time.Time `gorm:"column:created_at" json:"created_at"`
}

// UserCounts are the directory's statistics.
type UserCounts struct {
	Total int64 `gorm:"column:total"`
	// Active are neither banned nor expired.
	Active  int64 `gorm:"column:active"`
	Expired int64 `gorm:"column:expired"`
	Banned  int64 `gorm:"column:banned"`
	// Since were created at or after the count's since time.
	Since int64 `gorm:"column:since"`
}

// The identity fields of a directory row: the account's when identity has
// one, else Control's projection.
const (
	userEmail   = "COALESCE(a.email, d.email)"
	userBanned  = "COALESCE(a.banned, d.banned)"
	userIsAdmin = "COALESCE(a.is_admin, d.is_admin)"
	userIsStaff = "COALESCE(a.is_staff, d.is_staff)"
)

// The status predicates, on a row's identity ban flag and its subscriber's
// expiry (one bound argument: now in Unix seconds, for the expiry ones).
const (
	activePredicate  = userBanned + " = 0 AND (d.expired_at IS NULL OR d.expired_at > ?)"
	expiredPredicate = "d.expired_at IS NOT NULL AND d.expired_at <= ?"
	bannedPredicate  = userBanned + " = 1"
)

// directoryColumns select a DirectoryUser from a directory row.
const directoryColumns = "d.id AS id, " + userEmail + " AS email, e.balance AS balance, " +
	"e.commission_balance AS commission_balance, e.device_limit AS device_limit, e.speed_limit AS speed_limit, " +
	"e.flow_reset_time AS flow_reset_time, e.transfer_enable AS transfer_enable, e.u AS u, e.d AS d, " +
	"e.plan_id AS plan_id, e.group_id AS group_id, e.expired_at AS expired_at, " + userBanned + " AS banned, " +
	userIsAdmin + " AS is_admin, " + userIsStaff + " AS is_staff, d.created_at AS created_at"

// users are the directory's rows: every subscriber, its entitlements and its
// account, if any.
func (d UserDirectory) users(tx *gorm.DB) *gorm.DB {
	return tx.Table("kapi_user_directory_v1 AS d").
		Joins("JOIN kapi_subscriber_entitlement_v1 AS e ON e.id = d.id").
		Joins("LEFT JOIN " + d.Accounts + " AS a ON a.user_id = d.id")
}

// where applies a search's filters.
func (q UserQuery) where(rows *gorm.DB) *gorm.DB {
	if q.Email != "" {
		rows = rows.Where(userEmail+" LIKE ?", "%"+q.Email+"%")
	}
	if q.PlanID != nil {
		rows = rows.Where("e.plan_id = ?", *q.PlanID)
	}
	switch q.Status {
	case UserStatusActive:
		rows = rows.Where(activePredicate, q.Now.Unix())
	case UserStatusExpired:
		rows = rows.Where(expiredPredicate, q.Now.Unix())
	case UserStatusBanned:
		rows = rows.Where(bannedPredicate)
	}
	return rows
}

// Search returns one page of the users a query matches, newest first, and
// how many match in all.
func (d UserDirectory) Search(ctx context.Context, q UserQuery) (int64, []DirectoryUser, error) {
	var total int64
	users := []DirectoryUser{}
	err := d.read(ctx, func(tx *gorm.DB) error {
		if err := q.where(d.users(tx)).Count(&total).Error; err != nil {
			return err
		}
		return q.where(d.users(tx)).
			Select(directoryColumns).
			Order("d.created_at DESC, d.id DESC").
			Offset(max(q.Offset, 0)).
			Limit(q.Limit).
			Scan(&users).Error
	})
	if err != nil {
		return 0, nil, err
	}
	return total, users, nil
}

// Count counts the directory's users: all, active and expired at now,
// banned, and created at or after since, in one statement.
func (d UserDirectory) Count(ctx context.Context, now, since time.Time) (UserCounts, error) {
	var counts UserCounts
	err := d.read(ctx, func(tx *gorm.DB) error {
		return tx.Table("kapi_user_directory_v1 AS d").
			Joins("LEFT JOIN "+d.Accounts+" AS a ON a.user_id = d.id").
			Select("count(*) AS total, "+
				"COALESCE(SUM(CASE WHEN "+activePredicate+" THEN 1 ELSE 0 END), 0) AS active, "+
				"COALESCE(SUM(CASE WHEN "+expiredPredicate+" THEN 1 ELSE 0 END), 0) AS expired, "+
				"COALESCE(SUM(CASE WHEN "+bannedPredicate+" THEN 1 ELSE 0 END), 0) AS banned, "+
				"COALESCE(SUM(CASE WHEN d.created_at >= ? THEN 1 ELSE 0 END), 0) AS since",
				now.Unix(), now.Unix(), since).
			Scan(&counts).Error
	})
	return counts, err
}

var errUnconfiguredDirectory = errors.New("the user directory has no storage")

// read runs fn in a read-only transaction: on PostgreSQL a repeatable-read
// snapshot, so every statement in it sees the same subscribers and
// accounts; SQLite reads one snapshot per transaction anyway.
func (d UserDirectory) read(ctx context.Context, fn func(tx *gorm.DB) error) error {
	if d.DB == nil || d.Accounts == "" {
		return errUnconfiguredDirectory
	}
	db := d.DB.WithContext(ctx)
	if db.Name() == "postgres" {
		return db.Transaction(fn, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	}
	return db.Transaction(fn)
}
