// Package identitycutover moves credential authority between the kernel's
// legacy handlers and the identity module, for identity-platform's group A
// (service.IdentityGroupARoutes):
//   - the cutover catches identity up with delta imports, pauses group A,
//     imports the last changes, then switches group A to native and the
//     authority to identity in one transaction, and waits until the
//     identity hosts serve group A natively;
//   - a rollback, possible until finalize, switches back to legacy, and
//     returns identity's account reads (service.IdentityAccountReadRoutes),
//     which the operator may have switched on their own since, to legacy
//     with it;
//   - finalize, a day after the cutover, removes the legacy credentials.
//
// Every change is recorded in v4_kernel_identity_cutover.
package identitycutover

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/identityimport"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/internal/pluginhost"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"gorm.io/gorm"
)

// Errors of the authority changes.
var (
	ErrBusy     = errors.New("an identity cutover, rollback or finalize is already running")
	ErrNotReady = errors.New("identity is not ready for this change")
	ErrTooEarly = errors.New("finalize waits a day after the cutover")
	ErrFinal    = errors.New("identity is finalized: the legacy credentials are gone")
)

const (
	defaultSettle        = 30 * time.Second
	defaultHold          = 6 * time.Second
	defaultDrain         = 30 * time.Second
	defaultFinalizeAfter = 24 * time.Hour
	hostPoll             = time.Second
)

// Hosts reports the health of package hosts.
type Hosts interface {
	Health(ctx context.Context, packageID, version string, generation uint64) (pluginhost.HostHealth, error)
}

// Freeze pauses package routes at the v2 gateway.
type Freeze interface {
	Freeze(packageID string, routes []string)
	Unfreeze(packageID string, routes []string)
	Drain(ctx context.Context, packageID string, routes []string) error
}

// Service changes the identity authority.
type Service struct {
	DB       *gorm.DB
	Importer *identityimport.Importer
	// PublicKey returns the official package trust root.
	PublicKey func() (ed25519.PublicKey, error)
	// RefreshKeys pulls identity's token keys; the cutover needs them so
	// Control verifies the tokens identity issues.
	RefreshKeys func(ctx context.Context) error
	Hosts       Hosts
	Freeze      Freeze
	// Settle bounds the wait for the hosts to apply new route modes
	// (default 30s; hosts poll every 5s).
	Settle time.Duration
	// Hold keeps group A paused after a host confirmed the new modes, so
	// every instance of a remote generation has polled them too (default
	// 6s, one host poll interval and a margin).
	Hold time.Duration
	// DrainTimeout bounds the wait for group A requests in flight
	// (default 30s).
	DrainTimeout time.Duration
	// FinalizeAfter is how long identity must have been authoritative
	// before finalize without force (default 24h, the token lifetime).
	FinalizeAfter time.Duration
	// OnFinalized runs after finalize commits: Control stops accepting
	// legacy HS256 tokens.
	OnFinalized func()
	// Now defaults to time.Now.
	Now func() time.Time

	running sync.Mutex
	opMu    sync.Mutex
	op      *Operation
}

// Report describes a finished cutover or rollback.
type Report struct {
	Action string `json:"action"`
	// Revision is the identity-platform configuration revision written.
	Revision int64 `json:"revision"`
	// Imported counts the accounts the catch-up and final delta imports
	// sent, and Deleted the deletions.
	Imported uint64 `json:"imported,omitempty"`
	Deleted  uint64 `json:"deleted,omitempty"`
	// Frozen is how long group A answered 503.
	FrozenMillis int64 `json:"frozen_ms"`
	// Warning is set when a rollback committed but the hosts did not
	// confirm legacy mode in time; they follow within a poll interval.
	Warning string `json:"warning,omitempty"`
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func duration(value, fallback time.Duration) time.Duration {
	if value > 0 {
		return value
	}
	return fallback
}

// authorityModes are the route modes an authority change writes: group A in
// mode and, when the change goes back to legacy, identity's account reads
// too, which leave legacy only while identity is authoritative. A cutover
// leaves the reads as they are; the operator switches them.
func authorityModes(mode string) map[string]string {
	modes := make(map[string]string, len(service.IdentityGroupARoutes)+len(service.IdentityAccountReadRoutes))
	for _, route := range service.IdentityGroupARoutes {
		modes[route] = mode
	}
	if mode == packagebridge.RouteModeLegacy {
		for _, route := range service.IdentityAccountReadRoutes {
			modes[route] = mode
		}
	}
	return modes
}

// Cutover hands credential authority to identity. It needs a completed
// full import and identity's token keys, and leaves nothing changed when it
// fails.
func (s *Service) Cutover(ctx context.Context, actorID uint) (Report, error) {
	report := Report{Action: model.IdentityCutoverActionCutover}
	if !s.running.TryLock() {
		return report, ErrBusy
	}
	defer s.running.Unlock()
	state, checkpoint, err := identityimport.Status(ctx, s.DB)
	if err != nil {
		return report, err
	}
	switch {
	case service.IdentityAuthoritative(state):
		return report, fmt.Errorf("%w: identity is already authoritative (state %q)", ErrNotReady, state)
	case state != model.IdentityAuthorityImporting || (checkpoint.CompletedAt == 0 && !checkpoint.Delta):
		return report, fmt.Errorf("%w: run a full account import first (POST /api/v4/kernel/identity/import)", ErrNotReady)
	}
	installation, publicKey, err := s.target(ctx)
	if err != nil {
		return report, err
	}
	if s.RefreshKeys == nil {
		return report, fmt.Errorf("%w: identity token keys cannot be pulled", ErrNotReady)
	}
	if err := s.RefreshKeys(ctx); err != nil {
		return report, fmt.Errorf("%w: Control cannot verify identity tokens: %v", ErrNotReady, err)
	}
	catchUp, err := s.Importer.Run(ctx, true)
	if err != nil {
		return report, fmt.Errorf("catch-up import: %w", err)
	}
	report.Imported, report.Deleted = catchUp.Accounts, catchUp.Deleted

	frozenAt := s.now()
	s.Freeze.Freeze(service.IdentityPlatformPackageID, service.IdentityGroupARoutes)
	defer func() {
		s.Freeze.Unfreeze(service.IdentityPlatformPackageID, service.IdentityGroupARoutes)
		report.FrozenMillis = s.now().Sub(frozenAt).Milliseconds()
	}()
	if err := s.drain(ctx); err != nil {
		return report, err
	}
	final, err := s.Importer.Run(ctx, true)
	if err != nil {
		return report, fmt.Errorf("final import: %w", err)
	}
	report.Imported += final.Accounts
	report.Deleted += final.Deleted
	detail, _ := json.Marshal(map[string]any{"imported": report.Imported, "deleted": report.Deleted})
	report.Revision, err = s.switchAuthority(ctx, publicKey, installation.ID, model.IdentityAuthorityIdentity,
		packagebridge.RouteModeNative, actorID, model.IdentityCutoverActionCutover, string(detail))
	if err != nil {
		return report, fmt.Errorf("switch group A to identity: %w", err)
	}
	if err := s.awaitHosts(ctx, installation, report.Revision, packagebridge.RouteModeNative); err != nil {
		cause := fmt.Errorf("the identity hosts did not serve group A natively: %w", err)
		restore := context.WithoutCancel(ctx)
		revision, restoreErr := s.switchAuthority(restore, publicKey, installation.ID, model.IdentityAuthorityImporting,
			packagebridge.RouteModeLegacy, actorID, model.IdentityCutoverActionAborted, cause.Error())
		if restoreErr != nil {
			return report, errors.Join(cause, fmt.Errorf("restore legacy group A: %w", restoreErr))
		}
		report.Revision = revision
		_ = s.awaitHosts(restore, installation, revision, packagebridge.RouteModeLegacy)
		s.hold(restore)
		return report, cause
	}
	s.hold(ctx)
	return report, nil
}

// hold waits Hold, or until ctx ends.
func (s *Service) hold(ctx context.Context) {
	select {
	case <-ctx.Done():
	case <-time.After(duration(s.Hold, defaultHold)):
	}
}

// Rollback hands credential authority back to the kernel's legacy handlers.
// Until finalize identity mirrors credentials to the legacy tables, so the
// legacy handlers serve the same accounts; a later cutover re-imports what
// changed meanwhile.
func (s *Service) Rollback(ctx context.Context, actorID uint) (Report, error) {
	report := Report{Action: model.IdentityCutoverActionRollback}
	if !s.running.TryLock() {
		return report, ErrBusy
	}
	defer s.running.Unlock()
	state, err := service.IdentityAuthorityState(s.DB.WithContext(ctx))
	if err != nil {
		return report, err
	}
	switch state {
	case model.IdentityAuthorityIdentity:
	case model.IdentityAuthorityFinalized:
		return report, ErrFinal
	default:
		return report, fmt.Errorf("%w: identity is not authoritative (state %q)", ErrNotReady, state)
	}
	installation, publicKey, err := s.target(ctx)
	if err != nil {
		return report, err
	}
	frozenAt := s.now()
	s.Freeze.Freeze(service.IdentityPlatformPackageID, service.IdentityGroupARoutes)
	defer func() {
		s.Freeze.Unfreeze(service.IdentityPlatformPackageID, service.IdentityGroupARoutes)
		report.FrozenMillis = s.now().Sub(frozenAt).Milliseconds()
	}()
	if err := s.drain(ctx); err != nil {
		return report, err
	}
	report.Revision, err = s.switchAuthority(ctx, publicKey, installation.ID, model.IdentityAuthorityImporting,
		packagebridge.RouteModeLegacy, actorID, model.IdentityCutoverActionRollback, "")
	if err != nil {
		return report, fmt.Errorf("switch group A to legacy: %w", err)
	}
	if err := s.awaitHosts(ctx, installation, report.Revision, packagebridge.RouteModeLegacy); err != nil {
		report.Warning = "the identity hosts did not confirm legacy mode yet: " + err.Error()
	}
	s.hold(ctx)
	return report, nil
}

// Finalize removes the legacy credentials: v2_user passwords become
// unusable, v2_user_mfa is emptied, identity stops mirroring and Control
// stops accepting legacy HS256 tokens. It waits FinalizeAfter after the
// latest cutover unless forced; tokens issued before the cutover then have
// expired. It cannot be undone.
func (s *Service) Finalize(ctx context.Context, actorID uint, force bool) error {
	if !s.running.TryLock() {
		return ErrBusy
	}
	defer s.running.Unlock()
	db := s.DB.WithContext(ctx)
	state, err := service.IdentityAuthorityState(db)
	if err != nil {
		return err
	}
	switch state {
	case model.IdentityAuthorityIdentity:
	case model.IdentityAuthorityFinalized:
		return ErrFinal
	default:
		return fmt.Errorf("%w: identity is not authoritative (state %q)", ErrNotReady, state)
	}
	var cutover model.IdentityCutoverEvent
	if err := db.Where("action = ?", model.IdentityCutoverActionCutover).Order("created_at DESC, id DESC").Take(&cutover).Error; err != nil {
		return fmt.Errorf("find the cutover: %w", err)
	}
	after := duration(s.FinalizeAfter, defaultFinalizeAfter)
	if wait := cutover.CreatedAt.Add(after).Sub(s.now()); wait > 0 && !force {
		return fmt.Errorf("%w: %s left (or force it)", ErrTooEarly, wait.Round(time.Minute))
	}
	err = service.WithAgentLifecycleTransaction(db, func(tx *gorm.DB) error {
		if err := tx.Model(&model.User{}).Where("1 = 1").UpdateColumns(map[string]any{
			"password": model.UnusableLegacyPassword, "password_algo": nil, "password_salt": nil,
		}).Error; err != nil {
			return fmt.Errorf("clear legacy passwords: %w", err)
		}
		if err := tx.Where("1 = 1").Delete(&model.UserMFA{}).Error; err != nil {
			return fmt.Errorf("clear legacy MFA: %w", err)
		}
		if err := setAuthority(tx, model.IdentityAuthorityFinalized, s.now()); err != nil {
			return err
		}
		detail := ""
		if force {
			detail = "forced"
		}
		return tx.Create(&model.IdentityCutoverEvent{
			Action: model.IdentityCutoverActionFinalize, ActorID: actorID, Detail: detail, CreatedAt: s.now(),
		}).Error
	})
	if err != nil {
		return err
	}
	if s.OnFinalized != nil {
		s.OnFinalized()
	}
	return nil
}

// target returns identity-platform's enabled Control installation and the
// trust root that verifies its configuration.
func (s *Service) target(ctx context.Context) (model.PluginInstallation, ed25519.PublicKey, error) {
	var installation model.PluginInstallation
	err := s.DB.WithContext(ctx).Where("plugin_id = ? AND target = ?", service.IdentityPlatformPackageID, "control").Take(&installation).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && !installation.Enabled) {
		return installation, nil, fmt.Errorf("%w: identity-platform is not installed and enabled", ErrNotReady)
	}
	if err != nil {
		return installation, nil, err
	}
	if s.PublicKey == nil {
		return installation, nil, service.ErrPluginTrustRootRequired
	}
	publicKey, err := s.PublicKey()
	if err != nil {
		return installation, nil, err
	}
	return installation, publicKey, nil
}

func (s *Service) drain(ctx context.Context) error {
	drainCtx, cancel := context.WithTimeout(ctx, duration(s.DrainTimeout, defaultDrain))
	defer cancel()
	if err := s.Freeze.Drain(drainCtx, service.IdentityPlatformPackageID, service.IdentityGroupARoutes); err != nil {
		return fmt.Errorf("group A requests still in flight: %w", err)
	}
	return nil
}

// switchAuthority sets the authority state and the route modes of
// authorityModes in one transaction, and records the event.
func (s *Service) switchAuthority(ctx context.Context, publicKey ed25519.PublicKey, installationID uint, state, mode string, actorID uint, action, detail string) (int64, error) {
	var revision int64
	err := service.WithAgentLifecycleTransaction(s.DB.WithContext(ctx), func(tx *gorm.DB) error {
		if err := setAuthority(tx, state, s.now()); err != nil {
			return err
		}
		configuration, err := service.SetPackageRouteModesTx(tx, publicKey, installationID, authorityModes(mode), actorID)
		if err != nil {
			return err
		}
		revision = configuration.Revision
		return tx.Create(&model.IdentityCutoverEvent{Action: action, ActorID: actorID, Detail: detail, CreatedAt: s.now()}).Error
	})
	return revision, err
}

func setAuthority(tx *gorm.DB, state string, now time.Time) error {
	result := tx.Model(&model.IdentityAuthority{}).Where("id = ?", 1).Updates(map[string]any{"state": state, "updated_at": now})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("the identity authority row is missing: run an account import first")
	}
	return nil
}

// hostDetails is the part of a package host's Health details the cutover
// reads (sdk/pluginhostsdk.Router writes them).
type hostDetails struct {
	Config struct {
		Revision int64 `json:"revision"`
	} `json:"config"`
	Routes map[string]struct {
		Effective string `json:"effective"`
	} `json:"routes"`
}

// awaitHosts waits until the identity-platform host reports the
// configuration revision and serves every route the change switched
// (authorityModes) in mode.
func (s *Service) awaitHosts(ctx context.Context, installation model.PluginInstallation, revision int64, mode string) error {
	if s.Hosts == nil {
		return errors.New("package hosts are not running")
	}
	settle, cancel := context.WithTimeout(ctx, duration(s.Settle, defaultSettle))
	defer cancel()
	var last error
	for {
		last = s.hostsApplied(settle, installation, revision, mode)
		if last == nil {
			return nil
		}
		select {
		case <-settle.Done():
			return last
		case <-time.After(hostPoll):
		}
	}
}

func (s *Service) hostsApplied(ctx context.Context, installation model.PluginInstallation, revision int64, mode string) error {
	if installation.LifecycleGeneration <= 0 {
		return errors.New("the identity-platform installation has no running generation")
	}
	generation := uint64(installation.LifecycleGeneration) // #nosec G115 -- positive, checked above.
	health, err := s.Hosts.Health(ctx, installation.PluginID, installation.DesiredVersion, generation)
	if err != nil {
		return err
	}
	if !health.Healthy {
		return errors.New("the identity-platform host is unhealthy")
	}
	var details hostDetails
	if err := json.Unmarshal([]byte(health.DetailsJSON), &details); err != nil {
		return fmt.Errorf("the identity-platform host health details are unreadable: %w", err)
	}
	if details.Config.Revision < revision {
		return fmt.Errorf("the host applies configuration revision %d, not %d yet", details.Config.Revision, revision)
	}
	modes := authorityModes(mode)
	routes := make([]string, 0, len(modes))
	for route := range modes {
		routes = append(routes, route)
	}
	sort.Strings(routes)
	for _, route := range routes {
		effective := details.Routes[route].Effective
		if effective == "" {
			effective = packagebridge.RouteModeLegacy
		}
		if effective != mode {
			return fmt.Errorf("the host serves %s in %s mode", route, effective)
		}
	}
	return nil
}

// Operation is the latest background cutover or rollback.
type Operation struct {
	Action     string     `json:"action"`
	Running    bool       `json:"running"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	Report     *Report    `json:"report,omitempty"`
	Error      string     `json:"error,omitempty"`
}

// Start runs a cutover or rollback in the background; Latest follows it.
func (s *Service) Start(ctx context.Context, action string, actorID uint) error {
	run := s.Cutover
	switch action {
	case model.IdentityCutoverActionCutover:
	case model.IdentityCutoverActionRollback:
		run = s.Rollback
	default:
		return fmt.Errorf("unknown identity authority change %q", action)
	}
	s.opMu.Lock()
	if s.op != nil && s.op.Running {
		s.opMu.Unlock()
		return ErrBusy
	}
	operation := &Operation{Action: action, Running: true, StartedAt: s.now()}
	s.op = operation
	s.opMu.Unlock()
	go func() {
		report, err := run(context.WithoutCancel(ctx), actorID)
		finished := s.now()
		s.opMu.Lock()
		defer s.opMu.Unlock()
		operation.Running, operation.FinishedAt, operation.Report = false, &finished, &report
		if err != nil {
			operation.Error = err.Error()
		}
	}()
	return nil
}

// Latest returns a copy of the latest background change, or nil.
func (s *Service) Latest() *Operation {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if s.op == nil {
		return nil
	}
	copied := *s.op
	return &copied
}

var defaultService struct {
	sync.RWMutex
	service *Service
}

// SetDefault installs the server's service for the admin API.
func SetDefault(service *Service) {
	defaultService.Lock()
	defer defaultService.Unlock()
	defaultService.service = service
}

// Default returns the installed service, nil without package hosts.
func Default() *Service {
	defaultService.RLock()
	defer defaultService.RUnlock()
	return defaultService.service
}
