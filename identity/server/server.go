// Package server serves the identity module's IdentityService contract
// (sdk/api/identity/v1). Products add their own adapters around it; the
// service itself is product-neutral.
package server

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/AnixOps/anix-control/identity/account"
	"github.com/AnixOps/anix-control/identity/signingkey"
	identityv1 "github.com/AnixOps/anix-control/sdk/api/identity/v1"
	"github.com/AnixOps/anix-control/sdk/identitytoken"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// KeySource returns the current signing keys.
type KeySource interface {
	Load(ctx context.Context) ([]signingkey.Key, error)
}

// AccountStore holds the identity accounts.
type AccountStore interface {
	Import(ctx context.Context, importID, checkpoint string, accounts []account.Imported) (account.ImportResult, error)
	Get(ctx context.Context, userIDs []uint64) ([]account.Account, error)
	ChangedAfter(ctx context.Context, version uint64, limit int) ([]account.Account, error)
}

// Limits of one call, so a caller cannot make the service buffer without
// bound.
const (
	MaxImportAccounts  = 5000
	MaxBatchGet        = 1000
	DefaultExportLimit = 500
	MaxExportLimit     = 5000
)

// Server implements IdentityService. Keys and Accounts are nil while no
// key-encryption key is configured: the service then publishes no keys,
// issues nothing and holds no accounts.
type Server struct {
	identityv1.UnimplementedIdentityServiceServer
	Keys     KeySource
	Accounts AccountStore
	Policy   signingkey.Policy
	// Issuer and Audience are the iss and aud claims of the tokens the
	// caller verifies.
	Issuer   string
	Audience string
	// Now defaults to time.Now.
	Now func() time.Time
}

// GetTokenKeys publishes the keys that verify tokens: NEXT, ACTIVE and
// RETIRED keys until they end, and REVOKED keys so verifiers drop them.
func (s *Server) GetTokenKeys(ctx context.Context, _ *identityv1.GetTokenKeysRequest) (*identityv1.GetTokenKeysResponse, error) {
	if s.Keys == nil {
		return nil, status.Error(codes.FailedPrecondition, "identity signing keys are not configured")
	}
	keys, err := s.Keys.Load(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "load signing keys: %v", err)
	}
	now := time.Now()
	if s.Now != nil {
		now = s.Now()
	}
	response := &identityv1.GetTokenKeysResponse{Issuer: s.Issuer, Audience: s.Audience}
	for _, key := range keys {
		notAfter := s.Policy.NotAfter(key)
		if !notAfter.IsZero() && !now.Before(notAfter) {
			continue
		}
		published := &identityv1.TokenKey{
			Kid: key.ID, Alg: identitytoken.Algorithm, PublicKey: key.PublicKey, State: keyState(key.State),
			NotBeforeUnix: unixOrZero(key.CreatedAt),
		}
		if !notAfter.IsZero() {
			published.NotAfterUnix = notAfter.Unix()
		}
		response.Keys = append(response.Keys, published)
	}
	return response, nil
}

func keyState(state signingkey.State) identityv1.TokenKeyState {
	switch state {
	case signingkey.StateNext:
		return identityv1.TokenKeyState_TOKEN_KEY_STATE_NEXT
	case signingkey.StateActive:
		return identityv1.TokenKeyState_TOKEN_KEY_STATE_ACTIVE
	case signingkey.StateRetired:
		return identityv1.TokenKeyState_TOKEN_KEY_STATE_RETIRED
	case signingkey.StateRevoked:
		return identityv1.TokenKeyState_TOKEN_KEY_STATE_REVOKED
	default:
		return identityv1.TokenKeyState_TOKEN_KEY_STATE_UNSPECIFIED
	}
}

func unixOrZero(value time.Time) int64 {
	if value.IsZero() {
		return 0
	}
	return value.Unix()
}

// ImportAccounts stores one batch pushed by the product: a header, then
// accounts, committed together when the stream ends. Invite codes stay with
// the product and are refused.
func (s *Server) ImportAccounts(stream grpc.ClientStreamingServer[identityv1.ImportAccountsRequest, identityv1.ImportAccountsResponse]) error {
	if s.Accounts == nil {
		return status.Error(codes.FailedPrecondition, "identity accounts are not configured")
	}
	first, err := stream.Recv()
	if err != nil {
		return err
	}
	header := first.GetHeader()
	if header == nil || header.GetImportId() == "" {
		return status.Error(codes.InvalidArgument, "an import starts with a header naming the import")
	}
	var accounts []account.Imported
	for {
		message, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		switch value := message.GetValue().(type) {
		case *identityv1.ImportAccountsRequest_Account:
			if len(accounts) == MaxImportAccounts {
				return status.Errorf(codes.ResourceExhausted, "an import batch holds at most %d accounts", MaxImportAccounts)
			}
			accounts = append(accounts, importedAccount(value.Account))
		default:
			return status.Error(codes.InvalidArgument, "an import sends one header, then accounts")
		}
	}
	result, err := s.Accounts.Import(stream.Context(), header.GetImportId(), header.GetCheckpoint(), accounts)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "import: %v", err)
	}
	return stream.SendAndClose(&identityv1.ImportAccountsResponse{
		Accounts: result.Accounts, Checkpoint: header.GetCheckpoint(),
	})
}

func importedAccount(imported *identityv1.ImportedAccount) account.Imported {
	source := imported.GetAccount()
	result := account.Imported{Account: account.Account{
		UserID: source.GetUserId(), AccountUUID: source.GetAccountUuid(), Email: source.GetEmail(),
		PasswordHash: imported.GetPasswordHash(), PasswordAlgo: imported.GetPasswordAlgo(), PasswordSalt: imported.GetPasswordSalt(),
		IsAdmin: source.GetIsAdmin(), IsStaff: source.GetIsStaff(), Banned: source.GetBanned(),
		InviteUserID: imported.GetInviteUserId(), CreatedAt: fromUnix(source.GetCreatedAtUnix()),
	}}
	if mfa := imported.GetMfa(); mfa != nil {
		result.MFA = &account.MFA{
			Enabled: mfa.GetEnabled(), TOTPSecret: mfa.GetTotpSecret(),
			BackupCodeHashes: mfa.GetBackupCodeHashes(), EnabledAt: fromUnix(mfa.GetEnabledAtUnix()),
			LastUsed: fromUnix(mfa.GetLastUsedUnix()), LastMethod: mfa.GetLastMethod(),
		}
	}
	return result
}

// BatchGetAccounts returns the accounts that exist among the ids.
func (s *Server) BatchGetAccounts(ctx context.Context, request *identityv1.BatchGetAccountsRequest) (*identityv1.BatchGetAccountsResponse, error) {
	if s.Accounts == nil {
		return nil, status.Error(codes.FailedPrecondition, "identity accounts are not configured")
	}
	if len(request.GetUserIds()) > MaxBatchGet {
		return nil, status.Errorf(codes.InvalidArgument, "at most %d accounts per call", MaxBatchGet)
	}
	accounts, err := s.Accounts.Get(ctx, request.GetUserIds())
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "load accounts: %v", err)
	}
	response := &identityv1.BatchGetAccountsResponse{}
	for _, a := range accounts {
		response.Accounts = append(response.Accounts, accountMessage(a))
	}
	return response, nil
}

// ExportAccounts streams accounts changed after a version, in version order.
func (s *Server) ExportAccounts(request *identityv1.ExportAccountsRequest, stream grpc.ServerStreamingServer[identityv1.Account]) error {
	if s.Accounts == nil {
		return status.Error(codes.FailedPrecondition, "identity accounts are not configured")
	}
	limit := int(request.GetLimit())
	if limit <= 0 {
		limit = DefaultExportLimit
	}
	limit = min(limit, MaxExportLimit)
	accounts, err := s.Accounts.ChangedAfter(stream.Context(), request.GetAfterVersion(), limit)
	if err != nil {
		return status.Errorf(codes.Unavailable, "load accounts: %v", err)
	}
	for _, a := range accounts {
		if err := stream.Send(accountMessage(a)); err != nil {
			return err
		}
	}
	return nil
}

func accountMessage(a account.Account) *identityv1.Account {
	return &identityv1.Account{
		UserId: a.UserID, AccountUuid: a.AccountUUID, Email: a.Email, IsAdmin: a.IsAdmin, IsStaff: a.IsStaff,
		Banned: a.Banned, TokenVersion: a.TokenVersion, Version: a.Version,
		CreatedAtUnix: unixOrZero(a.CreatedAt), UpdatedAtUnix: unixOrZero(a.UpdatedAt), MfaEnabled: a.MFAEnabled,
	}
}

func fromUnix(value int64) time.Time {
	if value == 0 {
		return time.Time{}
	}
	return time.Unix(value, 0)
}
