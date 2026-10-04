package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/forwardlegacy"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
)

// The v4.2 upgrade's cleaners (forward-sdk.md section 10, F5c): Control
// removes the old forward runtime from the hosts the Agent's installer does
// not reach. NodeX hosts are cleaned through NodeX's HTTP API (its delete
// calls, one per flux forward or legacy rule on the node); hosts on the
// Ansible fallback with a cleanup playbook over SSH that deletes the old
// tables and chains and then lists them to verify they are gone.

// LegacyForwardNodeXCleaner deletes a node's flux gost services through
// NodeX. NodeX has no listing call: a delete is idempotent, so running the
// check again is the re-check.
type LegacyForwardNodeXCleaner struct {
	DB *gorm.DB
	// HTTPClient replaces the NodeX client's HTTP client (tests).
	HTTPClient interface {
		Do(*http.Request) (*http.Response, error)
	}
}

var _ forwardlegacy.Cleaner = (*LegacyForwardNodeXCleaner)(nil)

func (c *LegacyForwardNodeXCleaner) client() *nodeXForwardRuntimeClient {
	client := newNodeXForwardRuntimeClient(NewSystemConfigService(c.DB))
	if c.HTTPClient != nil {
		client.httpClient = c.HTTPClient
	}
	return client
}

// CleanNode deletes, through NodeX, every flux forward whose tunnel enters
// at the node on the gost backend, and every legacy rule relayed or exited
// by the node.
func (c *LegacyForwardNodeXCleaner) CleanNode(ctx context.Context, node forwardlegacy.NodeInfo) forwardlegacy.PathResult {
	result := forwardlegacy.PathResult{Path: forwardlegacy.PathNodeX}
	runtime := NewPanelForwardRuntimeService(c.DB)
	client := c.client()
	runtime.client = client
	type item struct {
		name string
		run  func() error
	}
	var items []item
	if c.DB.Migrator().HasTable(&model.Forward{}) && c.DB.Migrator().HasTable(&model.ForwardTunnel{}) {
		var tunnels []model.ForwardTunnel
		if err := c.DB.WithContext(ctx).Where("in_node_id = ?", node.ID).Order("id").Find(&tunnels).Error; err != nil {
			return failedPath(result, err)
		}
		for i := range tunnels {
			tunnel := tunnels[i]
			var forwards []model.Forward
			if err := c.DB.WithContext(ctx).Where("tunnel_id = ?", tunnel.ID).Order("id").Find(&forwards).Error; err != nil {
				return failedPath(result, err)
			}
			for j := range forwards {
				forward := forwards[j]
				backend, err := runtime.BackendForForward(&forward)
				if err != nil || backend != model.ForwardRuntimeBackendGost {
					continue
				}
				items = append(items, item{name: fmt.Sprintf("forward %d", forward.ID), run: func() error {
					req, _, err := runtime.buildExecuteRequest(forwardNodeRoleIngress, backend, model.ForwardRuntimeJobActionDelete, &forward, &tunnel)
					if err != nil {
						return err
					}
					if req, err = withForwardNodeTokens(c.DB, req); err != nil {
						return err
					}
					return checkNodeXResult(client.Execute(ctx, req))
				}})
			}
		}
	}
	if c.DB.Migrator().HasTable(&model.ForwardRule{}) {
		var rules []model.ForwardRule
		if err := c.DB.WithContext(ctx).Where("relay_node_id = ? OR exit_node_id = ?", node.ID, node.ID).Order("id").Find(&rules).Error; err != nil {
			return failedPath(result, err)
		}
		provider := &nodeXForwardRuntimeProvider{db: c.DB, client: client}
		for i := range rules {
			rule := rules[i]
			items = append(items, item{name: fmt.Sprintf("legacy rule %d", rule.ID), run: func() error {
				req, err := provider.buildRequest(model.ForwardRuntimeJobActionDelete, &rule)
				if err != nil {
					return err
				}
				if req, err = withForwardNodeTokens(c.DB, req); err != nil {
					return err
				}
				return checkNodeXResult(client.Execute(ctx, req))
			}})
		}
	}
	if len(items) == 0 {
		result.State = forwardlegacy.PathNotNeeded
		result.Detail = "no flux forward or legacy rule on gost references this node"
		return result
	}
	if settings, err := client.loadSettings(); err != nil || strings.TrimSpace(settings.BaseURL) == "" || strings.TrimSpace(settings.Token) == "" {
		result.State = forwardlegacy.StateUnreachable
		result.Detail = fmt.Sprintf("%d gost resource(s) to delete, but NodeX is not configured (%s, %s)", len(items),
			forwardRuntimeNodeXBaseURLConfigKey, forwardRuntimeNodeXTokenConfigKey)
		return result
	}
	var dirty, unreachable []string
	for _, entry := range items {
		if err := entry.run(); err != nil {
			line := entry.name + ": " + err.Error()
			if nodeXUnreachable(err) {
				unreachable = append(unreachable, line)
			} else {
				dirty = append(dirty, line)
			}
		}
	}
	result.Items = len(items)
	switch {
	case len(dirty) > 0:
		result.State = forwardlegacy.StateDirty
		result.Detail = "NodeX refused deletes: " + strings.Join(append(dirty, unreachable...), "; ")
	case len(unreachable) > 0:
		result.State = forwardlegacy.StateUnreachable
		result.Detail = "NodeX not reachable: " + strings.Join(unreachable, "; ")
	default:
		result.State = forwardlegacy.StateClean
		result.Detail = fmt.Sprintf("NodeX deleted %d gost resource(s)", len(items))
	}
	return result
}

// checkNodeXResult turns a NodeX answer that is not a success into an
// error.
func checkNodeXResult(result *nodeXForwardExecuteResult, err error) error {
	if err != nil {
		return err
	}
	if result != nil && result.Status == model.ForwardRuntimeJobStatusFailed {
		message := strings.TrimSpace(result.Message)
		if message == "" {
			message = "NodeX answered failed"
		}
		return errors.New(message)
	}
	return nil
}

// nodeXUnreachable reports whether a NodeX call failed before NodeX
// answered (connection, timeout).
func nodeXUnreachable(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	return strings.Contains(err.Error(), "execute NodeX forward runtime request")
}

func failedPath(result forwardlegacy.PathResult, err error) forwardlegacy.PathResult {
	result.State = forwardlegacy.StateDirty
	result.Detail = err.Error()
	return result
}

// LegacyForwardCleanupPlaybook is the Ansible cleanup playbook, relative to
// the repository root like the other forward playbooks.
const LegacyForwardCleanupPlaybook = "config/deploy/ansible/playbooks/forward_legacy_cleanup.yml"

// LegacyForwardAnsibleCleaner runs the cleanup playbook on a host of the
// Ansible fallback: an Ansible machine, or the execution node of a flux
// forward on an Ansible backend.
type LegacyForwardAnsibleCleaner struct {
	DB *gorm.DB
	// Playbook overrides LegacyForwardCleanupPlaybook.
	Playbook string
	// Run replaces running ansible-playbook (tests).
	Run func(ctx context.Context, command string, args []string, workdir string, env map[string]string) (string, error)
	// Timeout bounds one host's run; 5 minutes when zero.
	Timeout time.Duration
}

var _ forwardlegacy.Cleaner = (*LegacyForwardAnsibleCleaner)(nil)

// CleanNode runs the cleanup playbook limited to the node's host.
func (c *LegacyForwardAnsibleCleaner) CleanNode(ctx context.Context, node forwardlegacy.NodeInfo) forwardlegacy.PathResult {
	result := forwardlegacy.PathResult{Path: forwardlegacy.PathAnsible}
	machine, err := c.isAnsibleMachine(ctx, node.ID)
	if err != nil {
		return failedPath(result, err)
	}
	masquerade, forwards, err := c.ansibleForwards(ctx, node.ID)
	if err != nil {
		return failedPath(result, err)
	}
	if !machine && forwards == 0 {
		result.State = forwardlegacy.PathNotNeeded
		result.Detail = "not an Ansible machine and no flux forward ran on it through Ansible"
		return result
	}
	runtime := NewPanelForwardRuntimeService(c.DB)
	cfg, err := runtime.loadPanelForwardAnsibleConfigForDiagnosticsWithBackend(model.ForwardRuntimeBackendNftablesAnsible)
	if err != nil {
		result.State = forwardlegacy.StateUnreachable
		result.Detail = "the Ansible settings are not readable: " + err.Error()
		return result
	}
	playbook := c.Playbook
	if playbook == "" {
		playbook = LegacyForwardCleanupPlaybook
	}
	payload := &panelForwardAnsibleRuntimePayload{
		Inventory: cfg.Inventory, Playbook: playbook, Become: cfg.Become, Command: cfg.Command,
		WorkingDir: cfg.WorkingDir, TargetPattern: cfg.TargetPattern, Environment: cfg.Environment,
		Node: panelForwardAnsibleNodePayload{ID: node.ID, Name: node.Name, Host: node.Host},
	}
	limit := payload.limitPattern()
	if limit == "" {
		result.State = forwardlegacy.StateUnreachable
		result.Detail = "the node has no host to limit the playbook to"
		return result
	}
	extraVars := fmt.Sprintf(`{"legacy_masquerade": %s}`, jsonStrings(masquerade))
	args := []string{"-i", resolveForwardRuntimeFilePath(payload.WorkingDir, payload.Inventory), "--limit", limit}
	if payload.Become {
		args = append(args, "--become")
	}
	args = append(args, "--extra-vars", extraVars, resolveForwardRuntimeFilePath(payload.WorkingDir, payload.Playbook))
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	run := c.Run
	if run == nil {
		run = osExecPanelForwardRuntimeCommandRunner{}.Run
	}
	output, runErr := run(runCtx, payload.commandName(), args, payload.workingDirectory(), payload.environment())
	result.Items = forwards
	switch {
	case runErr == nil:
		result.State = forwardlegacy.StateClean
		result.Detail = fmt.Sprintf("the cleanup playbook removed and verified the legacy tables and chains on %s", limit)
	case ansibleUnreachable(runErr, output):
		result.State = forwardlegacy.StateUnreachable
		result.Detail = fmt.Sprintf("%s did not answer over SSH: %s", limit, lastLines(output, runErr))
	default:
		result.State = forwardlegacy.StateDirty
		result.Detail = fmt.Sprintf("the cleanup playbook failed on %s: %s", limit, lastLines(output, runErr))
	}
	return result
}

// isAnsibleMachine reports whether a forward node is in the Ansible
// inventory scope.
func (c *LegacyForwardAnsibleCleaner) isAnsibleMachine(ctx context.Context, id uint) (bool, error) {
	var count int64
	query := applyForwardNodeInventoryScope(c.DB.WithContext(ctx).Model(&model.ForwardNode{}), ForwardNodeInventoryScopeAnsible)
	if err := query.Where("id = ?", id).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// ansibleForwards answers the MASQUERADE rules the iptables runtime added
// for the flux forwards the node executed on an Ansible backend (as
// "proto,host,port"; they carry no marker of their own), and how many
// such forwards there are.
func (c *LegacyForwardAnsibleCleaner) ansibleForwards(ctx context.Context, nodeID uint) ([]string, int, error) {
	if !c.DB.Migrator().HasTable(&model.Forward{}) || !c.DB.Migrator().HasTable(&model.ForwardTunnel{}) {
		return nil, 0, nil
	}
	runtime := NewPanelForwardRuntimeService(c.DB)
	var tunnels []model.ForwardTunnel
	if err := c.DB.WithContext(ctx).Order("id").Find(&tunnels).Error; err != nil {
		return nil, 0, err
	}
	seen := map[string]bool{}
	var masquerade []string
	count := 0
	for i := range tunnels {
		tunnel := tunnels[i]
		if storedPanelTunnelExecutionNodeID(&tunnel) != nodeID {
			continue
		}
		var forwards []model.Forward
		if err := c.DB.WithContext(ctx).Where("tunnel_id = ?", tunnel.ID).Order("id").Find(&forwards).Error; err != nil {
			return nil, 0, err
		}
		for j := range forwards {
			backend, err := runtime.BackendForForward(&forwards[j])
			if err != nil || !isForwardRuntimeLocalAnsibleBackend(backend) {
				continue
			}
			count++
			targets, err := buildPanelForwardAnsibleTargets(forwards[j].RemoteAddr)
			if err != nil {
				continue
			}
			protocols := []string{"tcp"}
			switch normalizePanelRuntimeProtocol(tunnel.Protocol) {
			case "udp":
				protocols = []string{"udp"}
			case "both":
				protocols = []string{"tcp", "udp"}
			}
			for _, target := range targets {
				host, port := target.Host, target.Port
				if host == "" || port == 0 {
					rawHost, rawPort, err := net.SplitHostPort(target.Addr)
					if err != nil {
						continue
					}
					host = rawHost
					port, _ = strconv.Atoi(rawPort)
				}
				for _, protocol := range protocols {
					entry := fmt.Sprintf("%s,%s,%d", protocol, host, port)
					if !seen[entry] {
						seen[entry] = true
						masquerade = append(masquerade, entry)
					}
				}
			}
		}
	}
	return masquerade, count, nil
}

// ansibleUnreachable reports whether ansible-playbook could not reach the
// host (exit status 4) or could not run at all.
func ansibleUnreachable(err error, output string) bool {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 4 {
		return true
	}
	if errors.Is(err, exec.ErrNotFound) {
		return true
	}
	return strings.Contains(output, "UNREACHABLE!")
}

func lastLines(output string, err error) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) > 5 {
		lines = lines[len(lines)-5:]
	}
	text := strings.TrimSpace(strings.Join(lines, " | "))
	if err != nil {
		if text != "" {
			text += " | "
		}
		text += err.Error()
	}
	return text
}

func jsonStrings(values []string) string {
	if values == nil {
		values = []string{}
	}
	encoded, _ := json.Marshal(values)
	return string(encoded)
}
