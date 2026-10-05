// Package identityimport copies Control's legacy identity data (v2_user
// identity columns and v2_user_mfa) into the identity module
// with IdentityService.ImportAccounts. The kernel drives it: batches are
// checkpointed in v4_kernel_identity_authority so an interrupted import
// resumes, and a delta import re-sends what changed since the previous one:
// users or their MFA row updated since, and deletions (linked users whose
// v2_user row is gone), which every import finishes with.
// Identity never reads v2_user.password itself. Invite codes stay with Control:
// registration consumes them through KernelIdentity.
package identityimport

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	identityv1 "github.com/AnixOps/anix-control/sdk/api/identity/v1"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// accountNamespace derives the stable account UUID of a legacy user, so a
// repeated import links the same account.
var accountNamespace = uuid.MustParse("5f0f3a6e-1c2d-4b8e-9f7a-3d2c1b0a9e8f")

// AccountUUID is the account UUID of a legacy user without a link yet.
func AccountUUID(userID uint) string {
	return uuid.NewSHA1(accountNamespace, []byte("v2_user:"+strconv.FormatUint(uint64(userID), 10))).String()
}

// Checkpoint is the import progress stored in the authority row.
type Checkpoint struct {
	ImportID string `json:"import_id"`
	Delta    bool   `json:"delta"`
	// UserCursor is the last user id sent.
	UserCursor uint `json:"user_cursor"`
	UsersDone  bool `json:"users_done"`
	// DeletionsDone is set once identity deleted every account whose
	// subscriber is gone.
	DeletionsDone bool `json:"deletions_done"`
	// Since is the lower bound of a delta import: the start of the
	// previous completed import.
	Since       int64  `json:"since"`
	StartedAt   int64  `json:"started_at"`
	CompletedAt int64  `json:"completed_at"`
	Accounts    uint64 `json:"accounts"`
	Deleted     uint64 `json:"deleted"`
	LastError   string `json:"last_error,omitempty"`
}

// Errors of Run.
var (
	ErrRunning        = errors.New("an identity import is already running")
	ErrNotImportable  = errors.New("identity is authoritative: importing would overwrite its accounts")
	ErrNoFullImport   = errors.New("a delta import needs a completed full import")
	ErrNotInitialized = errors.New("the identity importer is not configured")
)

// Importer runs imports, one at a time.
type Importer struct {
	DB      *gorm.DB
	Connect func() (grpc.ClientConnInterface, error)
	// BatchSize defaults to 500.
	BatchSize int
	// Now defaults to time.Now.
	Now func() time.Time

	running sync.Mutex
}

func (i *Importer) now() time.Time {
	if i.Now != nil {
		return i.Now()
	}
	return time.Now()
}

func (i *Importer) batchSize() int {
	if i.BatchSize > 0 {
		return i.BatchSize
	}
	return 500
}

// Status returns the authority state and the import progress.
func Status(ctx context.Context, db *gorm.DB) (string, Checkpoint, error) {
	authority, err := loadAuthority(db.WithContext(ctx))
	if err != nil {
		return "", Checkpoint{}, err
	}
	var checkpoint Checkpoint
	if authority.Checkpoint != "" {
		if err := json.Unmarshal([]byte(authority.Checkpoint), &checkpoint); err != nil {
			return "", Checkpoint{}, fmt.Errorf("identity import checkpoint is malformed: %w", err)
		}
	}
	return authority.State, checkpoint, nil
}

// Run imports everything (delta false) or what changed since the previous
// completed import (delta true). An unfinished import of the same kind
// resumes from its checkpoint. The authority state becomes importing; the
// cutover, not the import, hands authority to identity.
func (i *Importer) Run(ctx context.Context, delta bool) (Checkpoint, error) {
	if i == nil || i.DB == nil || i.Connect == nil {
		return Checkpoint{}, ErrNotInitialized
	}
	if !i.running.TryLock() {
		return Checkpoint{}, ErrRunning
	}
	defer i.running.Unlock()

	state, previous, err := Status(ctx, i.DB)
	if err != nil {
		return Checkpoint{}, err
	}
	if state != model.IdentityAuthorityKernel && state != model.IdentityAuthorityImporting {
		return Checkpoint{}, ErrNotImportable
	}
	checkpoint := previous
	if previous.ImportID == "" || previous.CompletedAt != 0 || previous.Delta != delta {
		if delta && (previous.ImportID == "" || previous.CompletedAt == 0) {
			return Checkpoint{}, ErrNoFullImport
		}
		checkpoint = Checkpoint{ImportID: uuid.NewString(), Delta: delta, StartedAt: i.now().Unix()}
		if delta {
			checkpoint.Since = previous.StartedAt
		}
	}
	checkpoint.LastError = ""
	if err := i.save(ctx, checkpoint); err != nil {
		return checkpoint, err
	}
	conn, err := i.Connect()
	if err != nil {
		return i.fail(ctx, checkpoint, fmt.Errorf("reach the identity module: %w", err))
	}
	client := identityv1.NewIdentityServiceClient(conn)
	for !checkpoint.UsersDone {
		done, err := i.userBatch(ctx, client, &checkpoint)
		if err != nil {
			return i.fail(ctx, checkpoint, err)
		}
		checkpoint.UsersDone = done
		if err := i.save(ctx, checkpoint); err != nil {
			return checkpoint, err
		}
	}
	for !checkpoint.DeletionsDone {
		done, err := i.deletionBatch(ctx, client, &checkpoint)
		if err != nil {
			return i.fail(ctx, checkpoint, err)
		}
		checkpoint.DeletionsDone = done
		if err := i.save(ctx, checkpoint); err != nil {
			return checkpoint, err
		}
	}
	checkpoint.CompletedAt = i.now().Unix()
	return checkpoint, i.save(ctx, checkpoint)
}

func (i *Importer) fail(ctx context.Context, checkpoint Checkpoint, err error) (Checkpoint, error) {
	checkpoint.LastError = err.Error()
	if saveErr := i.save(context.WithoutCancel(ctx), checkpoint); saveErr != nil {
		return checkpoint, errors.Join(err, saveErr)
	}
	return checkpoint, err
}

func (i *Importer) save(ctx context.Context, checkpoint Checkpoint) error {
	encoded, err := json.Marshal(checkpoint)
	if err != nil {
		return err
	}
	row := model.IdentityAuthority{ID: 1, State: model.IdentityAuthorityImporting, ImportID: checkpoint.ImportID, Checkpoint: string(encoded), UpdatedAt: i.now()}
	return i.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}}, DoUpdates: clause.AssignmentColumns([]string{"state", "import_id", "checkpoint", "updated_at"}),
	}).Create(&row).Error
}

func loadAuthority(db *gorm.DB) (model.IdentityAuthority, error) {
	var authority model.IdentityAuthority
	err := db.Where("id = ?", 1).Take(&authority).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.IdentityAuthority{ID: 1, State: model.IdentityAuthorityKernel}, nil
	}
	return authority, err
}

// userBatch sends the next users; it reports true when none are left.
func (i *Importer) userBatch(ctx context.Context, client identityv1.IdentityServiceClient, checkpoint *Checkpoint) (bool, error) {
	query := i.DB.WithContext(ctx).Model(&model.User{}).
		Select("id", "email", "password", "password_algo", "password_salt", "is_admin", "is_staff", "banned", "invite_user_id", "created_at", "updated_at").
		Where("id > ?", checkpoint.UserCursor)
	if checkpoint.Delta {
		// Legacy MFA changes touch only v2_user_mfa; disabling MFA deletes
		// its row and touches v2_user.
		since := time.Unix(checkpoint.Since, 0)
		changedMFA := i.DB.Model(&model.UserMFA{}).Select("user_id").Where("updated_at >= ?", since)
		query = query.Where("(updated_at >= ? OR id IN (?))", since, changedMFA)
	}
	var users []model.User
	if err := query.Order("id").Limit(i.batchSize()).Find(&users).Error; err != nil {
		return false, fmt.Errorf("read users: %w", err)
	}
	if len(users) == 0 {
		return true, nil
	}
	ids := make([]uint, 0, len(users))
	for _, user := range users {
		ids = append(ids, user.ID)
	}
	links := map[uint]string{}
	var linkRows []model.IdentityAccountLink
	if err := i.DB.WithContext(ctx).Where("user_id IN ?", ids).Find(&linkRows).Error; err != nil {
		return false, fmt.Errorf("read account links: %w", err)
	}
	for _, link := range linkRows {
		links[link.UserID] = link.AccountUUID
	}
	mfas := map[uint]model.UserMFA{}
	var mfaRows []model.UserMFA
	if err := i.DB.WithContext(ctx).Where("user_id IN ?", ids).Find(&mfaRows).Error; err != nil {
		return false, fmt.Errorf("read MFA: %w", err)
	}
	for _, mfa := range mfaRows {
		mfas[mfa.UserID] = mfa
	}

	stream, err := client.ImportAccounts(ctx)
	if err != nil {
		return false, fmt.Errorf("open import: %w", err)
	}
	if err := stream.Send(header(checkpoint)); err != nil {
		return false, sendFailure(stream, "send import header", err)
	}
	newLinks := make([]model.IdentityAccountLink, 0, len(users))
	for _, user := range users {
		accountUUID, linked := links[user.ID]
		if !linked {
			accountUUID = AccountUUID(user.ID)
			newLinks = append(newLinks, model.IdentityAccountLink{UserID: user.ID, AccountUUID: accountUUID})
		}
		imported, err := importedAccount(user, accountUUID, mfas)
		if err != nil {
			return false, err
		}
		if err := stream.Send(&identityv1.ImportAccountsRequest{Value: &identityv1.ImportAccountsRequest_Account{Account: imported}}); err != nil {
			return false, sendFailure(stream, fmt.Sprintf("send account %d", user.ID), err)
		}
	}
	response, err := stream.CloseAndRecv()
	if err != nil {
		return false, fmt.Errorf("import accounts: %w", err)
	}
	if response.GetAccounts() != uint64(len(users)) {
		return false, fmt.Errorf("identity stored %d of %d accounts", response.GetAccounts(), len(users))
	}
	// Links exist only for accounts identity holds.
	if len(newLinks) > 0 {
		if err := i.DB.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&newLinks).Error; err != nil {
			return false, fmt.Errorf("link imported accounts: %w", err)
		}
	}
	checkpoint.UserCursor = ids[len(ids)-1]
	checkpoint.Accounts += response.GetAccounts()
	return false, nil
}

// deletionBatch tells identity about linked users whose v2_user row is gone
// and drops their links; it reports true when none are left.
func (i *Importer) deletionBatch(ctx context.Context, client identityv1.IdentityServiceClient, checkpoint *Checkpoint) (bool, error) {
	var ids []uint
	if err := i.DB.WithContext(ctx).Model(&model.IdentityAccountLink{}).
		Where("user_id NOT IN (?)", i.DB.Model(&model.User{}).Select("id")).
		Order("user_id").Limit(i.batchSize()).Pluck("user_id", &ids).Error; err != nil {
		return false, fmt.Errorf("read deleted users: %w", err)
	}
	if len(ids) == 0 {
		return true, nil
	}
	stream, err := client.ImportAccounts(ctx)
	if err != nil {
		return false, fmt.Errorf("open import: %w", err)
	}
	if err := stream.Send(header(checkpoint)); err != nil {
		return false, sendFailure(stream, "send import header", err)
	}
	for _, id := range ids {
		if err := stream.Send(&identityv1.ImportAccountsRequest{Value: &identityv1.ImportAccountsRequest_DeletedUserId{DeletedUserId: uint64(id)}}); err != nil {
			return false, sendFailure(stream, fmt.Sprintf("send deletion of %d", id), err)
		}
	}
	response, err := stream.CloseAndRecv()
	if err != nil {
		return false, fmt.Errorf("import deletions: %w", err)
	}
	if response.GetDeleted() != uint64(len(ids)) {
		return false, fmt.Errorf("identity applied %d of %d deletions", response.GetDeleted(), len(ids))
	}
	if err := i.DB.WithContext(ctx).Where("user_id IN ?", ids).Delete(&model.IdentityAccountLink{}).Error; err != nil {
		return false, fmt.Errorf("unlink deleted users: %w", err)
	}
	checkpoint.Deleted += response.GetDeleted()
	return false, nil
}

func header(checkpoint *Checkpoint) *identityv1.ImportAccountsRequest {
	position := fmt.Sprintf("users:%d", checkpoint.UserCursor)
	if checkpoint.UsersDone {
		position = "deletions"
	}
	return &identityv1.ImportAccountsRequest{Value: &identityv1.ImportAccountsRequest_Header{Header: &identityv1.ImportHeader{
		ImportId: checkpoint.ImportID, Delta: checkpoint.Delta, Checkpoint: position,
	}}}
}

func importedAccount(user model.User, accountUUID string, mfas map[uint]model.UserMFA) (*identityv1.ImportedAccount, error) {
	imported := &identityv1.ImportedAccount{
		Account: &identityv1.Account{
			UserId: uint64(user.ID), AccountUuid: accountUUID, Email: user.Email,
			IsAdmin: user.IsAdmin == 1, IsStaff: user.IsStaff == 1, Banned: user.Banned == 1,
			CreatedAtUnix: user.CreatedAt.Unix(), UpdatedAtUnix: user.UpdatedAt.Unix(),
		},
		PasswordHash: user.Password,
	}
	if user.PasswordAlgo != nil {
		imported.PasswordAlgo = *user.PasswordAlgo
	}
	if user.PasswordSalt != nil {
		imported.PasswordSalt = *user.PasswordSalt
	}
	if user.InviteUserID != nil {
		imported.InviteUserId = uint64(*user.InviteUserID)
	}
	if mfa, ok := mfas[user.ID]; ok {
		hashes, err := backupCodeDigests(mfa.BackupCodes)
		if err != nil {
			return nil, fmt.Errorf("user %d backup codes: %w", user.ID, err)
		}
		imported.Mfa = &identityv1.ImportedMFA{Enabled: mfa.Enabled, TotpSecret: mfa.TOTPSecret, BackupCodeHashes: hashes, LastMethod: mfa.LastMethod}
		if mfa.LastUsed != nil {
			imported.Mfa.LastUsedUnix = mfa.LastUsed.Unix()
		}
		if mfa.Enabled {
			imported.Mfa.EnabledAtUnix = mfa.UpdatedAt.Unix()
		}
	}
	return imported, nil
}

// backupCodeDigests turns the legacy plain-text backup codes (a JSON array,
// compared verbatim at login) into SHA-256 hex digests of the same strings.
func backupCodeDigests(stored string) ([]string, error) {
	if strings.TrimSpace(stored) == "" {
		return nil, nil
	}
	var codes []string
	if err := json.Unmarshal([]byte(stored), &codes); err != nil {
		return nil, err
	}
	digests := make([]string, 0, len(codes))
	for _, code := range codes {
		sum := sha256.Sum256([]byte(code))
		digests = append(digests, hex.EncodeToString(sum[:]))
	}
	return digests, nil
}

// Runner runs imports in the background for the server's lifetime.
type Runner struct {
	importer *Importer
	requests chan bool
	logf     func(string, ...any)
}

// NewRunner returns a runner; call Serve.
func NewRunner(importer *Importer, logf func(string, ...any)) *Runner {
	return &Runner{importer: importer, requests: make(chan bool), logf: logf}
}

// Serve runs requested imports until ctx ends.
func (r *Runner) Serve(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case delta := <-r.requests:
			checkpoint, err := r.importer.Run(ctx, delta)
			if err != nil {
				r.logf("Identity import %s stopped: %v", checkpoint.ImportID, err)
				continue
			}
			r.logf("Identity import %s completed: %d accounts", checkpoint.ImportID, checkpoint.Accounts)
		}
	}
}

// Start requests an import; it fails with ErrRunning while one runs.
func (r *Runner) Start(delta bool) error {
	if r == nil {
		return ErrNotInitialized
	}
	select {
	case r.requests <- delta:
		return nil
	default:
		return ErrRunning
	}
}

var defaultRunner struct {
	sync.RWMutex
	runner *Runner
	db     *gorm.DB
}

// SetDefault installs the server's runner and database for the admin API.
func SetDefault(runner *Runner, db *gorm.DB) {
	defaultRunner.Lock()
	defer defaultRunner.Unlock()
	defaultRunner.runner, defaultRunner.db = runner, db
}

// Default returns the installed runner (nil without package hosts) and
// database.
func Default() (*Runner, *gorm.DB) {
	defaultRunner.RLock()
	defer defaultRunner.RUnlock()
	return defaultRunner.runner, defaultRunner.db
}

// sendFailure explains a failed Send on the ImportAccounts client stream.
// When the identity module ends the stream with an error, gRPC reports it to
// the sender as a bare io.EOF and keeps the real status for CloseAndRecv:
// without reading it, the checkpoint would record "EOF" and the operator
// would never learn why the import stopped.
func sendFailure(stream interface {
	CloseAndRecv() (*identityv1.ImportAccountsResponse, error)
}, what string, err error) error {
	if errors.Is(err, io.EOF) {
		if _, cause := stream.CloseAndRecv(); cause != nil {
			return fmt.Errorf("%s: %w", what, cause)
		}
	}
	return fmt.Errorf("%s: %w", what, err)
}
