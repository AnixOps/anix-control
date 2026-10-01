package native

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"gorm.io/gorm"
)

// Route ids of the user's invite codes.
const (
	InviteInfoRouteID     = "affiliate.user.invite.get"
	InviteGenerateRouteID = "affiliate.user.invite.generate.post"
)

// InviteCode is a v2_invite_code row, adopted in place. Its fields and tags
// match the kernel model, so answers are the same. Registration (the
// kernel's and the identity module's account creation through Control)
// consumes a code: it marks it used inside Control's transaction that
// creates the user.
type InviteCode struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	Code      string     `gorm:"size:32;uniqueIndex" json:"code"`
	UserID    *uint      `gorm:"index" json:"user_id"`
	Status    int        `gorm:"default:0" json:"status"`
	UsedBy    *uint      `json:"used_by"`
	UsedAt    *time.Time `json:"used_at"`
	ExpiredAt *time.Time `json:"expired_at"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName is the adopted kernel table.
func (InviteCode) TableName() string { return "v2_invite_code" }

// OrderBilling is the part of a kapi_order_billing_v1 row the invite
// statistics read: whose order it is and its status.
type OrderBilling struct {
	ID     uint
	UserID uint
	Status int
}

// TableName is the kernel view.
func (OrderBilling) TableName() string { return "kapi_order_billing_v1" }

// orderStatusPaid is v2_order's paid status, which the kernel's invite
// statistics count.
const orderStatusPaid = 1

// Answers of the user's invite routes, the kernel's.
const (
	errBalanceUnavailable = "failed to load user commission balance"
	// errInviteCodeLimit is v2board's answer at the limit, with status 500
	// (service.InviteCodeLimitMessage).
	errInviteCodeLimit = "The maximum number of creations has been reached"
)

// defaultInviteCodeLimit is how many unused codes a user may hold when the
// configuration sets no code_count, the kernel's
// service.DefaultInviteCodeLimit.
const defaultInviteCodeLimit = 5

// errCodeLimit stops a generation at the limit.
var errCodeLimit = errors.New("invite code limit reached")

// InviteInfo is GET /api/v2/user/invite: the caller's invite codes newest
// first, their commission balance and their invite statistics.
//   - The codes are rows of the adopted v2_invite_code.
//   - invite_count counts the users the caller invited
//     (kapi_user_referral_v1) and paid_count those of them with a paid order
//     (kapi_order_billing_v1).
//   - total_commission sums the caller's settled and withdrawn commissions
//     (v2_commission_record).
//   - The balance is kapi_subscriber_entitlement_v1's commission_balance.
//
// As in the kernel, a statistic whose query fails stays zero.
func (s *Service) InviteInfo(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	db, err := s.Open(ctx)
	if err != nil {
		return errorAnswer(http.StatusInternalServerError, err.Error())
	}
	userID := request.Principal.ActorID

	var codes []InviteCode
	if err := db.Where("user_id = ?", userID).Order("created_at DESC").Find(&codes).Error; err != nil {
		return errorAnswer(http.StatusInternalServerError, err.Error())
	}

	var inviteCount, paidCount int64
	var totalCommission float64
	if err := db.Model(&ReferralUser{}).Where("invite_user_id = ?", userID).Count(&inviteCount).Error; err != nil {
		log.Printf("failed to count invited users: %v", err)
	}
	if err := db.Table(ReferralUser{}.TableName()+" u").
		Joins("JOIN "+OrderBilling{}.TableName()+" o ON o.user_id = u.id").
		Where("u.invite_user_id = ? AND o.status = ?", userID, orderStatusPaid).
		Distinct("u.id").
		Count(&paidCount).Error; err != nil {
		log.Printf("failed to count paying invited users: %v", err)
	}
	if err := db.Model(&Commission{}).
		Where("user_id = ? AND status IN (1, 2)", userID).
		Select("COALESCE(SUM(amount), 0)").
		Scan(&totalCommission).Error; err != nil {
		log.Printf("failed to sum commission: %v", err)
	}

	var entitlement Entitlement
	if err := db.Select("id", "commission_balance").First(&entitlement, userID).Error; err != nil {
		return errorAnswer(http.StatusInternalServerError, errBalanceUnavailable)
	}

	return s.panel(map[string]any{
		"codes":              codes,
		"commission_balance": entitlement.CommissionBalance,
		"stats": map[string]any{
			"invite_count":     inviteCount,
			"paid_count":       paidCount,
			"total_commission": totalCommission,
		},
	})
}

// GenerateInviteCode is POST /api/v2/user/invite/generate: a new unused
// code for the caller, expiring after the configured code_expire_days. As in
// v2board, a user holds at most code_count (default 5) unused codes; an
// expired one no longer counts.
//
// Concurrent generations of one user count one after another, whichever
// side serves them: on PostgreSQL both take the user's invite code advisory
// lock (LockUserInviteCodes) before counting; on SQLite one writer runs at
// a time and a transaction that read before another's write is retried, as
// the kernel's service.WithRetryableTransaction does.
func (s *Service) GenerateInviteCode(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	db, err := s.Open(ctx)
	if err != nil {
		return errorAnswer(http.StatusInternalServerError, err.Error())
	}
	userID := request.Principal.ActorID

	code, err := newInviteCodeString()
	if err != nil {
		return errorAnswer(http.StatusInternalServerError, err.Error())
	}
	inviteCode := &InviteCode{Code: code, UserID: &userID, Status: 0}
	cfg, err := loadConfig(db)
	if err == nil && cfg.CodeExpireDays > 0 {
		expiredAt := s.now().AddDate(0, 0, cfg.CodeExpireDays)
		inviteCode.ExpiredAt = &expiredAt
	}

	err = retrySQLiteBusy(db, func(tx *gorm.DB) error {
		if err := LockUserInviteCodes(tx, userID); err != nil {
			return err
		}
		var unused int64
		if err := tx.Model(&InviteCode{}).
			Where("user_id = ? AND status = ? AND (expired_at IS NULL OR expired_at > ?)", userID, 0, s.now()).
			Count(&unused).Error; err != nil {
			return err
		}
		if unused >= int64(inviteCodeLimit(tx)) {
			return errCodeLimit
		}
		return tx.Create(inviteCode).Error
	})
	if errors.Is(err, errCodeLimit) {
		return errorAnswer(http.StatusInternalServerError, errInviteCodeLimit)
	}
	if err != nil {
		return errorAnswer(http.StatusInternalServerError, err.Error())
	}
	return s.panel(inviteCode)
}

// inviteCodeLimit is the configuration's code_count, read in the
// transaction; a missing configuration or one below one is the default.
func inviteCodeLimit(db *gorm.DB) int {
	var cfg InviteConfig
	if err := db.Select("id", "code_count").First(&cfg).Error; err != nil || cfg.CodeCount < 1 {
		return defaultInviteCodeLimit
	}
	return cfg.CodeCount
}

// newInviteCodeString is the kernel's generateInviteCodeStr(8): eight hex
// digits from a random source.
func newInviteCodeString() (string, error) {
	const length = 8
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b)[:length], nil
}

// inviteCodeLockClass is the first key of a user's invite code lock, the
// kernel's ("invc" in ASCII; service.InviteCodeLockKeys).
const inviteCodeLockClass int32 = 0x696e7663

// InviteCodeLockKeys are the keys of a user's invite code advisory lock,
// as the kernel's service.InviteCodeLockKeys derives them: the class and the
// user id masked to 31 bits.
func InviteCodeLockKeys(userID uint) (int32, int32) {
	return inviteCodeLockClass, int32(uint64(userID) & math.MaxInt32) // #nosec G115 -- masked to 31 bits.
}

// LockUserInviteCodes takes the user's invite code lock until tx ends, on
// PostgreSQL, where advisory locks need no grant; on SQLite it does nothing.
func LockUserInviteCodes(tx *gorm.DB, userID uint) error {
	if tx.Name() != "postgres" {
		return nil
	}
	class, key := InviteCodeLockKeys(userID)
	return tx.Exec("SELECT pg_advisory_xact_lock(CAST(? AS integer), CAST(? AS integer))", class, key).Error
}

// SQLite contention retries, the kernel's service.WithRetryableTransaction.
const (
	maxSQLiteTransactionAttempts = 8
	sqliteTransactionRetryDelay  = 10 * time.Millisecond
)

// retrySQLiteBusy runs mutation in a transaction and reruns it while SQLite
// reports writer contention, as the kernel does; other errors and other
// databases return at once.
func retrySQLiteBusy(db *gorm.DB, mutation func(*gorm.DB) error) error {
	for attempt := 0; ; attempt++ {
		err := db.Transaction(mutation)
		if err == nil || db.Name() != "sqlite" || !sqliteBusy(err) || attempt+1 >= maxSQLiteTransactionAttempts {
			return err
		}
		time.Sleep(sqliteTransactionRetryDelay * time.Duration(1<<attempt))
	}
}

func sqliteBusy(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "database is locked") || strings.Contains(message, "database table is locked") ||
		strings.Contains(message, "sqlite_busy") || strings.Contains(message, "sqlite_locked")
}
