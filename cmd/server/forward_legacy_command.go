package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/forwardlegacy"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"gorm.io/gorm"
)

const forwardLegacyUsage = `usage:
  anix-control forward legacy archive [-o <file|directory>]
  anix-control forward legacy check [--node <node>]... [--json]
  anix-control forward legacy status [--json]
  anix-control forward legacy abandon <node> --reason <text>
  anix-control forward legacy drop --confirm "` + forwardlegacy.ConfirmPhrase + `" [--backup-taken <path>]

The v4.2 forwarding upgrade (docs/UPGRADE.md, "Forwarding: Archive, Clean
The Nodes, Drop The Old Tables"; forward-sdk.md section 10). The v4.1 flux
forwarding data is not migrated: forwarding is reconfigured as v4 routes.

archive writes the flux forwarding tables to one JSON file (mode 0600,
node tokens and other secrets left out) and records it. It never
overwrites a file: -o names a new file or a directory that receives a
timestamped one; without -o the file goes to the data directory's
forward-legacy/ (Control also writes one there at its first v4.2 start).

check verifies each forward node (all, or the --node ones; a node is its
name or forward-<id>) and records the result: an enrolled Agent was cleaned
by its installer; Control deletes the node's flux gost services through
NodeX's HTTP API and runs the cleanup playbook on Ansible hosts. A node is
clean, dirty (the old runtime is still there) or unreachable.

status shows each node's standing and what the drop still needs.

abandon accepts an unreachable node as gone for good, with a reason; the
drop then no longer waits for it. Its old runtime, if any, stays on it.

drop is IRREVERSIBLE (gate H15). It drops the flux tables in one
transaction and refuses unless: the phrase is exact; the latest archive is
readable, unchanged and current; every forward node is clean or abandoned;
no installed package release still adopts one of the tables (its storage
lease would fail); no Control process runs on the database; and a database backup of the last
24 hours exists (Control's own on SQLite, or --backup-taken <path>, a
pg_dump file on PostgreSQL). v2_forward_node and v2_forward_clean_agent
are kept. After the drop, rolling back to 4.1 needs that backup. There is
no button for it in the web UI on purpose.

Every command is written to the audit log as system/cli. The config file
comes from ANIX_CONTROL_CONFIG or config/config.yaml.`

// forwardLegacyDataDir is Control's data directory, set by run.
var forwardLegacyDataDir string

// resolveDataDir is Control's data directory: the SQLite database's
// directory, else config/data resolved like the SQLite path.
func resolveDataDir(cfg *config.Config, resolvedConfigPath string) string {
	driver := strings.ToLower(cfg.Database.Driver)
	if (driver == "" || driver == "sqlite" || driver == "sqlite3") && cfg.Database.Database != "" {
		return filepath.Dir(cfg.Database.Database)
	}
	return resolveRuntimePath("config/data", resolvedConfigPath)
}

// forwardLegacyArchiveDir is where archives go without -o.
func forwardLegacyArchiveDir(cfg *config.Config) string {
	dir := forwardLegacyDataDir
	if dir == "" && cfg != nil {
		dir = resolveDataDir(cfg, "")
	}
	return filepath.Join(dir, forwardlegacy.ArchiveDirName)
}

func forwardLegacyUsageError() error {
	return fmt.Errorf("invalid forward legacy command\n%s", forwardLegacyUsage)
}

// forwardLegacyCLI runs the upgrade's commands.
type forwardLegacyCLI struct {
	ctx    context.Context
	cfg    *config.Config
	db     *gorm.DB
	stdout io.Writer
	now    func() time.Time
	// nodeX and ansible are the cleaners check runs; tests replace them.
	nodeX   forwardlegacy.Cleaner
	ansible forwardlegacy.Cleaner
}

// forwardLegacyCleaners builds the cleaners check runs; tests replace it.
var forwardLegacyCleaners = func(db *gorm.DB) (forwardlegacy.Cleaner, forwardlegacy.Cleaner) {
	return &service.LegacyForwardNodeXCleaner{DB: db}, &service.LegacyForwardAnsibleCleaner{DB: db}
}

// runForwardLegacyCommand runs `forward legacy ...`.
func runForwardLegacyCommand(ctx context.Context, cfg *config.Config, db *gorm.DB, arguments []string, stdout io.Writer) error {
	if len(arguments) == 0 {
		return forwardLegacyUsageError()
	}
	nodeX, ansible := forwardLegacyCleaners(db)
	cli := &forwardLegacyCLI{ctx: ctx, cfg: cfg, db: db, stdout: stdout, now: time.Now, nodeX: nodeX, ansible: ansible}
	switch arguments[0] {
	case "archive":
		return cli.archive(arguments[1:])
	case "check":
		return cli.check(arguments[1:])
	case "status":
		return cli.status(arguments[1:])
	case "abandon":
		return cli.abandon(arguments[1:])
	case "drop":
		return cli.drop(arguments[1:])
	}
	return forwardLegacyUsageError()
}

func (c *forwardLegacyCLI) audit(action string, content any) bool {
	encoded, _ := json.Marshal(content)
	err := service.NewOperationLogService(c.db.WithContext(c.ctx)).Record(&service.OperationLogInput{
		Username: service.RouteModeCLIActor, Action: action, Module: "forward", TargetType: "forward_legacy", Content: string(encoded),
	})
	return err == nil
}

func (c *forwardLegacyCLI) printJSON(value any) error {
	encoder := json.NewEncoder(c.stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func (c *forwardLegacyCLI) archive(arguments []string) error {
	set := flag.NewFlagSet("forward legacy archive", flag.ContinueOnError)
	output := set.String("o", "", "archive file or directory")
	positionals, err := parseLegacyFlags(set, arguments)
	if err != nil || len(positionals) != 0 {
		return forwardLegacyUsageError()
	}
	written, err := forwardlegacy.WriteArchive(c.ctx, c.db, forwardlegacy.WriteOptions{
		Output: *output, Dir: forwardLegacyArchiveDir(c.cfg), Trigger: "cli", Actor: service.RouteModeCLIActor,
		ControlVersion: version, Now: c.now(),
	})
	if err != nil {
		return err
	}
	audited := c.audit("forward.legacy_archive", map[string]any{"path": written.Record.Path, "sha256": written.Record.SHA256, "tables": written.Tables})
	return c.printJSON(map[string]any{"archive": written.Record, "tables": written.Tables, "audited": audited})
}

func (c *forwardLegacyCLI) check(arguments []string) error {
	set := flag.NewFlagSet("forward legacy check", flag.ContinueOnError)
	var nodes stringList
	set.Var(&nodes, "node", "a node name or forward-<id>")
	asJSON := set.Bool("json", false, "print JSON")
	positionals, err := parseLegacyFlags(set, arguments)
	if err != nil || len(positionals) != 0 {
		return forwardLegacyUsageError()
	}
	statuses, err := forwardlegacy.Check(c.ctx, c.db, forwardlegacy.CheckOptions{NodeX: c.nodeX, Ansible: c.ansible, Nodes: nodes, Now: c.now()})
	if err != nil {
		return err
	}
	counts := stateCounts(statuses)
	c.audit("forward.legacy_check", map[string]any{"nodes": []string(nodes), "states": counts})
	if *asJSON {
		return c.printJSON(map[string]any{"nodes": statuses, "states": counts})
	}
	return c.printNodes(statuses)
}

func (c *forwardLegacyCLI) status(arguments []string) error {
	set := flag.NewFlagSet("forward legacy status", flag.ContinueOnError)
	asJSON := set.Bool("json", false, "print JSON")
	positionals, err := parseLegacyFlags(set, arguments)
	if err != nil || len(positionals) != 0 {
		return forwardLegacyUsageError()
	}
	pre, err := forwardlegacy.CheckPreconditions(c.ctx, c.db, forwardlegacy.PreconditionOptions{SkipBackup: true, Now: c.now()})
	if err != nil {
		return err
	}
	drop, err := forwardlegacy.LatestDrop(c.db.WithContext(c.ctx))
	if err != nil {
		return err
	}
	if *asJSON {
		return c.printJSON(map[string]any{"preconditions": pre, "states": stateCounts(pre.Nodes), "drop": drop, "confirm_phrase": forwardlegacy.ConfirmPhrase})
	}
	if err := c.printNodes(pre.Nodes); err != nil {
		return err
	}
	out := c.stdout
	if drop != nil {
		_, _ = fmt.Fprintf(out, "\nThe flux tables were dropped at %s (%s). Archive: %s\n", drop.CreatedAt.UTC().Format(time.RFC3339), drop.Dropped, drop.ArchivePath)
		return nil
	}
	if pre.Archive != nil {
		_, _ = fmt.Fprintf(out, "\nArchive: %s (%s, SHA-256 %s)\n", pre.Archive.Path, pre.Archive.CreatedAt.UTC().Format(time.RFC3339), pre.Archive.SHA256)
	}
	_, _ = fmt.Fprintf(out, "Flux tables present: %s\n", dashList(pre.Present))
	if len(pre.Missing) > 0 {
		_, _ = fmt.Fprintf(out, "Flux tables already gone: %s\n", strings.Join(pre.Missing, ", "))
	}
	if len(pre.Blockers) > 0 {
		_, _ = fmt.Fprintln(out, "\nThe drop still needs:")
		for _, blocker := range pre.Blockers {
			_, _ = fmt.Fprintf(out, "  - %s\n", blocker)
		}
		_, _ = fmt.Fprintln(out, "  - a database backup of the last 24 hours")
		return nil
	}
	_, _ = fmt.Fprintf(out, "\nEvery node is clean or abandoned and the archive is current. With a database backup of the last 24 hours, the IRREVERSIBLE drop is:\n  anix-control forward legacy drop --confirm %q --backup-taken <backup file>\n", forwardlegacy.ConfirmPhrase)
	return nil
}

func (c *forwardLegacyCLI) abandon(arguments []string) error {
	set := flag.NewFlagSet("forward legacy abandon", flag.ContinueOnError)
	reason := set.String("reason", "", "why the node is abandoned")
	positionals, err := parseLegacyFlags(set, arguments)
	if err != nil || len(positionals) != 1 {
		return forwardLegacyUsageError()
	}
	status, err := forwardlegacy.Abandon(c.ctx, c.db, positionals[0], *reason, service.RouteModeCLIActor, c.now())
	if err != nil {
		return err
	}
	audited := c.audit("forward.legacy_abandon", map[string]any{"node_ref": status.Ref, "name": status.Name, "reason": status.AbandonReason})
	return c.printJSON(map[string]any{"node": status, "audited": audited})
}

func (c *forwardLegacyCLI) drop(arguments []string) error {
	set := flag.NewFlagSet("forward legacy drop", flag.ContinueOnError)
	confirm := set.String("confirm", "", "the confirmation phrase")
	backup := set.String("backup-taken", "", "the database backup file")
	positionals, err := parseLegacyFlags(set, arguments)
	if err != nil || len(positionals) != 0 {
		return forwardLegacyUsageError()
	}
	result, err := forwardlegacy.Drop(c.ctx, c.db, forwardlegacy.DropOptions{
		Confirm: *confirm, BackupPath: *backup, Actor: service.RouteModeCLIActor, Now: c.now(),
	})
	if err != nil {
		c.audit("forward.legacy_drop_refused", map[string]any{"error": err.Error()})
		return err
	}
	audited := c.audit("forward.legacy_drop", map[string]any{"dropped": result.Dropped, "already_missing": result.AlreadyMissing,
		"archive": result.Record.ArchivePath, "backup": result.Backup})
	return c.printJSON(map[string]any{"drop": result, "audited": audited,
		"next": "Start Control again. Rolling back to 4.1 now needs the database backup " + backupPath(result.Backup) + "."})
}

func backupPath(backup *forwardlegacy.Backup) string {
	if backup == nil {
		return ""
	}
	return backup.Path
}

func (c *forwardLegacyCLI) printNodes(statuses []forwardlegacy.NodeStatus) error {
	writer := tabwriter.NewWriter(c.stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(writer, "NODE\tNAME\tSTATE\tCHECKED\tDETAIL")
	for _, status := range statuses {
		checked := "-"
		if status.CheckedAt != nil {
			checked = status.CheckedAt.UTC().Format(time.RFC3339)
		}
		detail := status.Detail
		if status.State == forwardlegacy.StateAbandoned {
			detail = "abandoned by " + status.AbandonedBy + ": " + status.AbandonReason
		}
		_, _ = fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n", status.Ref, status.Name, status.State, checked, dash(detail))
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	counts := stateCounts(statuses)
	_, _ = fmt.Fprintf(c.stdout, "%d node(s): %d clean, %d dirty, %d unreachable, %d abandoned, %d unchecked.\n", len(statuses),
		counts[forwardlegacy.StateClean], counts[forwardlegacy.StateDirty], counts[forwardlegacy.StateUnreachable],
		counts[forwardlegacy.StateAbandoned], counts[forwardlegacy.StateUnchecked])
	return nil
}

func stateCounts(statuses []forwardlegacy.NodeStatus) map[string]int {
	counts := map[string]int{}
	for _, status := range statuses {
		counts[status.State]++
	}
	return counts
}

func dashList(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, ", ")
}

// parseLegacyFlags is parseFlags with this command's usage error.
func parseLegacyFlags(set *flag.FlagSet, arguments []string) ([]string, error) {
	set.SetOutput(io.Discard)
	var positionals []string
	for {
		if err := set.Parse(arguments); err != nil {
			return nil, forwardLegacyUsageError()
		}
		rest := set.Args()
		if len(rest) == 0 {
			return positionals, nil
		}
		positionals = append(positionals, rest[0])
		arguments = rest[1:]
	}
}
