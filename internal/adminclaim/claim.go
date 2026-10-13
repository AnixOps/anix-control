package adminclaim

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/database/migrations"
	"github.com/AnixOps/anix-control/v5/internal/identityimport"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Audit row of a claim: the action, the email and the source address only.
const (
	AuditModule = "admin"
	AuditAction = "admin_claimed"
	// AuditPath is the claim's route, recorded as the audit row's path.
	AuditPath = "/api/v5/claim"
)

// IdentityAccounts hands identity an account the kernel created while
// identity holds the credentials (identityimport.Runner).
type IdentityAccounts interface {
	ImportAccount(ctx context.Context, user model.User, accountUUID string) error
}

// Request is a claim: the token from the link, and the first
// administrator's email and password. SourceIP is where it came from, for
// the audit row.
type Request struct {
	Token    string
	Email    string
	Password string
	SourceIP string
}

// Result is the administrator a claim created.
type Result struct {
	UserID uint
	Email  string
	// Identity is true when the account was created through identity.
	Identity bool
}

// Claimer creates the first administrator from a claim.
type Claimer struct {
	DB *gorm.DB
	// Identity creates the account in identity when identity holds the
	// credentials (the authority is identity or finalized). Without it such
	// a claim answers ErrIdentityUnavailable and changes nothing.
	Identity IdentityAccounts
	// Now defaults to time.Now.
	Now func() time.Time
	// Cost is the bcrypt cost (bcrypt.DefaultCost when 0).
	Cost int
}

func (c *Claimer) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// identityAuthority is the credential authority, "kernel" when its table
// does not exist (a database that never had identity).
func identityAuthority(db *gorm.DB) (string, error) {
	if !db.Migrator().HasTable(&model.IdentityAuthority{}) {
		return model.IdentityAuthorityKernel, nil
	}
	return service.IdentityAuthorityState(db)
}

// Claim checks the request and creates the administrator. The token is
// consumed in the transaction that creates the account, and only if it is
// still the pending one, so of concurrent claims with the same token
// exactly one succeeds.
//
// The checks run in this order, so the answer reveals nothing to a caller
// without the token: an administrator exists (ErrAdministratorExists); the
// token is not the pending claim's (ErrInvalidToken) or expired
// (ErrExpired); then the email and the password.
//
// While the kernel holds the credentials (authority kernel or importing)
// the administrator is a v2_user row with the bcrypt hash, as the kernel
// creates administrators; a later identity import copies it. Once identity
// holds them the row is a subscriber linked to an identity account
// (the password mirrored until finalize, as KernelIdentity does), and the
// account is handed to identity after the commit; if identity refuses, the
// row and the link are removed and the token restored.
func (c *Claimer) Claim(ctx context.Context, request Request) (Result, error) {
	db := c.DB.WithContext(ctx)
	now := c.now()
	exists, err := HasAdministrator(ctx, db)
	if err != nil {
		return Result{}, err
	}
	if exists {
		return Result{}, ErrAdministratorExists
	}
	if _, err := check(db, request.Token, now); err != nil {
		return Result{}, err
	}
	email, err := NormalizeEmail(request.Email)
	if err != nil {
		return Result{}, err
	}
	if err := CheckPassword(request.Password); err != nil {
		return Result{}, err
	}
	authority, err := identityAuthority(db)
	if err != nil {
		return Result{}, fmt.Errorf("read the identity authority: %w", err)
	}
	viaIdentity := service.IdentityAuthoritative(authority)
	if viaIdentity && c.Identity == nil {
		return Result{}, ErrIdentityUnavailable
	}
	cost := c.Cost
	if cost == 0 {
		cost = bcrypt.DefaultCost
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(request.Password), cost)
	if err != nil {
		return Result{}, fmt.Errorf("hash the password: %w", err)
	}

	var (
		user        model.User
		accountUUID string
		audit       model.AuditLog
		stored      string
	)
	err = db.Transaction(func(tx *gorm.DB) error {
		// Everything again, now inside the write transaction.
		exists, err := HasAdministrator(ctx, tx)
		if err != nil {
			return err
		}
		if exists {
			return ErrAdministratorExists
		}
		if stored, err = check(tx, request.Token, now); err != nil {
			return err
		}
		var taken int64
		if err := tx.Model(&model.User{}).Where("email = ?", email).Count(&taken).Error; err != nil {
			return fmt.Errorf("check the email: %w", err)
		}
		if taken > 0 {
			return ErrEmailTaken
		}
		// Consume the token: only the claim that deletes this very row
		// goes on.
		deleted := tx.Where("key = ? AND value = ?", MetaKey, stored).Delete(&migrations.MetaEntry{})
		if deleted.Error != nil {
			return fmt.Errorf("consume the claim: %w", deleted.Error)
		}
		if deleted.RowsAffected != 1 {
			return ErrInvalidToken
		}
		user = model.User{Email: email, Password: string(hash), IsAdmin: 1, CreatedAt: now, UpdatedAt: now}
		if viaIdentity && authority == model.IdentityAuthorityFinalized {
			user.Password = model.UnusableLegacyPassword
		}
		user.UUID, user.Token = uuid.NewString(), uuid.NewString()
		if err := tx.Create(&user).Error; err != nil {
			return fmt.Errorf("create the administrator: %w", err)
		}
		if viaIdentity {
			accountUUID = identityimport.AccountUUID(user.ID)
			link := model.IdentityAccountLink{UserID: user.ID, AccountUUID: accountUUID, CreatedAt: now, UpdatedAt: now}
			if err := tx.Create(&link).Error; err != nil {
				return fmt.Errorf("link the identity account: %w", err)
			}
		}
		id := user.ID
		audit = model.AuditLog{
			UserID: &id, Email: email, Method: "POST", Path: AuditPath, Module: AuditModule, Action: AuditAction,
			IP: request.SourceIP, StatusCode: 201, CreatedAt: now,
		}
		if err := tx.Create(&audit).Error; err != nil {
			return fmt.Errorf("audit the claim: %w", err)
		}
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	if !viaIdentity {
		return Result{UserID: user.ID, Email: email}, nil
	}
	account := user
	account.Password = string(hash)
	if pushErr := c.Identity.ImportAccount(ctx, account, accountUUID); pushErr != nil {
		if undoErr := c.undo(context.WithoutCancel(ctx), user.ID, audit.ID, stored); undoErr != nil {
			slog.Error("first administrator claim: identity refused the account and the claim could not be undone",
				slog.Uint64("user_id", uint64(user.ID)), slog.String("error", undoErr.Error()))
			return Result{}, errors.Join(ErrIdentityUnavailable, pushErr, undoErr)
		}
		return Result{}, fmt.Errorf("%w: %v", ErrIdentityUnavailable, pushErr)
	}
	return Result{UserID: user.ID, Email: email, Identity: true}, nil
}

// undo removes what a claim committed when identity did not take the
// account, and restores the pending claim unless a newer link replaced it.
func (c *Claimer) undo(ctx context.Context, userID, auditID uint, stored string) error {
	return c.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", userID).Delete(&model.IdentityAccountLink{}).Error; err != nil {
			return err
		}
		if err := tx.Where("id = ?", userID).Delete(&model.User{}).Error; err != nil {
			return err
		}
		if err := tx.Where("id = ?", auditID).Delete(&model.AuditLog{}).Error; err != nil {
			return err
		}
		entry := migrations.MetaEntry{Key: MetaKey, Value: stored, UpdatedAt: c.now().UTC()}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&entry).Error
	})
}

// IdentityFromImporter adapts the server's identity importer; nil when the
// process runs no package hosts.
func IdentityFromImporter(runner *identityimport.Runner) IdentityAccounts {
	if runner == nil {
		return nil
	}
	return runner
}
