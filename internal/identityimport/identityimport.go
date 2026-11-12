// Package identityimport hands identity the account of the first
// administrator the claim link creates: the kernel allocates the v2_user
// row, identity owns the credentials. It uses IdentityService.ImportAccounts
// with a one-account import.
package identityimport

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync/atomic"
	"time"

	identityv1 "github.com/AnixOps/anix-control/sdk/api/identity/v1"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/google/uuid"
	"google.golang.org/grpc"
)

// accountNamespace derives the stable account UUID of a kernel user.
var accountNamespace = uuid.MustParse("5f0f3a6e-1c2d-4b8e-9f7a-3d2c1b0a9e8f")

// AccountUUID is the account UUID of a kernel user without a link yet.
func AccountUUID(userID uint) string {
	return uuid.NewSHA1(accountNamespace, []byte("v2_user:"+strconv.FormatUint(uint64(userID), 10))).String()
}

// ErrNotInitialized is returned without an identity connection.
var ErrNotInitialized = errors.New("the identity importer is not configured")

// Importer reaches the identity module.
type Importer struct {
	Connect func() (grpc.ClientConnInterface, error)
}

// accountImportWait is how long ImportAccount retries, well inside the
// server write timeout (30 seconds).
const accountImportWait = 15 * time.Second

// ImportAccount hands identity one account: the account, its password hash
// and its UUID, in an import of its own. Right after a fresh start the
// identity host is still starting and migrating, so it retries for up to
// accountImportWait.
func (i *Importer) ImportAccount(ctx context.Context, user model.User, accountUUID string) error {
	if i == nil || i.Connect == nil {
		return ErrNotInitialized
	}
	deadline := time.Now().Add(accountImportWait)
	for {
		err := i.importAccount(ctx, user, accountUUID)
		if err == nil || ctx.Err() != nil || time.Now().After(deadline) {
			return err
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func (i *Importer) importAccount(ctx context.Context, user model.User, accountUUID string) error {
	conn, err := i.Connect()
	if err != nil {
		return fmt.Errorf("reach the identity module: %w", err)
	}
	stream, err := identityv1.NewIdentityServiceClient(conn).ImportAccounts(ctx)
	if err != nil {
		return fmt.Errorf("open import: %w", err)
	}
	head := &identityv1.ImportAccountsRequest{Value: &identityv1.ImportAccountsRequest_Header{Header: &identityv1.ImportHeader{
		ImportId: "account-" + uuid.NewString(), Delta: true, Checkpoint: fmt.Sprintf("account:%d", user.ID),
	}}}
	if err := stream.Send(head); err != nil {
		return sendFailure(stream, "send import header", err)
	}
	account := &identityv1.ImportedAccount{
		Account: &identityv1.Account{
			UserId: uint64(user.ID), AccountUuid: accountUUID, Email: user.Email,
			IsAdmin: user.IsAdmin == 1, IsStaff: user.IsStaff == 1, Banned: user.Banned == 1,
			CreatedAtUnix: user.CreatedAt.Unix(), UpdatedAtUnix: user.UpdatedAt.Unix(),
		},
		PasswordHash: user.Password,
	}
	if err := stream.Send(&identityv1.ImportAccountsRequest{Value: &identityv1.ImportAccountsRequest_Account{Account: account}}); err != nil {
		return sendFailure(stream, fmt.Sprintf("send account %d", user.ID), err)
	}
	response, err := stream.CloseAndRecv()
	if err != nil {
		return fmt.Errorf("import account: %w", err)
	}
	if response.GetAccounts() != 1 {
		return fmt.Errorf("identity stored %d of 1 account", response.GetAccounts())
	}
	return nil
}

var defaultImporter atomic.Pointer[Importer]

// SetDefault installs the server's importer; nil without package hosts.
func SetDefault(importer *Importer) { defaultImporter.Store(importer) }

// Default returns the installed importer, or nil.
func Default() *Importer { return defaultImporter.Load() }

// sendFailure explains a failed Send on the ImportAccounts client stream.
// When the identity module ends the stream with an error, gRPC reports it to
// the sender as a bare io.EOF and keeps the real status for CloseAndRecv.
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
