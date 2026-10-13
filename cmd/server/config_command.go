package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/rootkey"
	"gopkg.in/yaml.v3"
)

const configCommandUsage = `usage:
  anix-control config check [--file <path>]
  anix-control config rewrite --in <old> --out <new> [--data-dir <dir>] [--force]

check loads a configuration the way Control does (the file, then the
ANIX_CONTROL_* variables), strictly: it prints every deprecated key with
what happens to it, refuses an unknown key naming the closest one, and
prints the resulting layout and the root key's state. Without --file it
checks ANIX_CONTROL_CONFIG, else config/config.yaml when it exists, else the
built-in values. The exit status is 1 when Control would refuse the
configuration.

rewrite turns an earlier configuration file into the operator keys (domain
from agent_install.public_url, the gRPC certificate files into tls.cert_file
with tls.mode external, grpc.port and subscribe_path when they are not the
defaults, the database section while PostgreSQL is supported) and prints
every key it drops with the reason. With --out equal to --in the old file is
kept as <name>.bak-<UTC time>. The old file's module_runtime.ca_kek and
identity.kek must already be in data_dir/root.key (start Control once with
the old file and it adopts them); --force drops them anyway. It writes no
secret and opens no database.`

// runConfigCommand runs `config ...`; handled is false for any other
// command.
func runConfigCommand(arguments []string, stdout, stderr io.Writer) (code int, handled bool) {
	if len(arguments) == 0 || arguments[0] != "config" {
		return 0, false
	}
	if len(arguments) < 2 {
		_, _ = fmt.Fprintln(stderr, configCommandUsage)
		return 2, true
	}
	switch arguments[1] {
	case "check":
		return runConfigCheck(arguments[2:], os.Environ(), stdout, stderr), true
	case "rewrite":
		return runConfigRewrite(arguments[2:], time.Now(), stdout, stderr), true
	}
	_, _ = fmt.Fprintln(stderr, configCommandUsage)
	return 2, true
}

// runConfigCheck runs `config check`.
func runConfigCheck(arguments []string, environ []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("config check", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	file := flags.String("file", "", "the configuration file")
	if err := flags.Parse(arguments); err != nil || flags.NArg() != 0 {
		_, _ = fmt.Fprintln(stderr, configCommandUsage)
		return 2
	}
	path := strings.TrimSpace(*file)
	if path == "" {
		configured := ""
		for _, entry := range environ {
			if name, value, ok := strings.Cut(entry, "="); ok && name == config.ConfigPathEnv {
				configured = strings.TrimSpace(value)
			}
		}
		resolved, err := selectConfigPath("", false, configured)
		if err != nil {
			_, _ = fmt.Fprintf(stdout, "ERROR %v\n", err)
			return 1
		}
		path = resolved
	}
	source := path
	if source == "" {
		source = "the built-in values (no configuration file)"
	}
	_, _ = fmt.Fprintf(stdout, "Checking %s and the %s* variables.\n", source, config.EnvPrefix)
	cfg, report, err := config.LoadFile(path, environ)
	for _, warning := range report.Warnings() {
		_, _ = fmt.Fprintf(stdout, "WARN  %s\n", strings.TrimPrefix(warning, "Configuration: "))
	}
	if err != nil {
		_, _ = fmt.Fprintf(stdout, "ERROR %v\n", err)
		return 1
	}
	if config.PlaceholderJWTSecret(cfg.JWT.Secret) {
		_, _ = fmt.Fprintln(stdout, "WARN  jwt.secret is a template value and is not used: the root key derives the JWT secret")
		cfg.JWT.Secret = ""
	}
	if err := cfg.ValidateForServer(); err != nil {
		_, _ = fmt.Fprintf(stdout, "ERROR %v\n", err)
		return 1
	}
	rootKeyLine, rootKeyErr := describeRootKeyFile(cfg)
	if rootKeyErr != nil {
		_, _ = fmt.Fprintf(stdout, "ERROR %v\n", rootKeyErr)
		return 1
	}
	panel := cfg.Panel()
	_, _ = fmt.Fprintf(stdout, "OK    %s\n", describeLayout(panel))
	_, _ = fmt.Fprintf(stdout, "OK    subscriptions at /%s/:token; data_dir %s\n", cfg.App.SubscribePath, cfg.DataDir)
	_, _ = fmt.Fprintf(stdout, "OK    %s\n", rootKeyLine)
	return 0
}

// describeRootKeyFile says what the next start does with root.key, without
// writing anything or showing a key. A file that exists but is damaged is
// an error.
func describeRootKeyFile(cfg *config.Config) (string, error) {
	path := rootkey.Path(cfg.DataDir)
	file, err := rootkey.ReadFile(path)
	legacy := strings.TrimSpace(cfg.ModuleRuntime.CAKEK) != ""
	switch {
	case err == nil:
		if legacy {
			configured, parseErr := rootkey.ParseKEK(cfg.ModuleRuntime.CAKEK)
			if parseErr != nil {
				return "", fmt.Errorf("module_runtime.ca_kek: %w", parseErr)
			}
			if !configured.Equal(file.Key) {
				return "", fmt.Errorf("module_runtime.ca_kek and %s hold different keys: Control refuses to start; keep the one that sealed the database", path)
			}
			return "root key " + path + " (module_runtime.ca_kek is the same key: remove it)", nil
		}
		return "root key " + path, nil
	case errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR):
		if legacy {
			return "root key: the next start adopts module_runtime.ca_kek into " + path, nil
		}
		return "root key: the first start generates " + path, nil
	case errors.Is(err, fs.ErrPermission):
		return "root key " + path + " exists but this user cannot read it (Control's user can)", nil
	}
	return "", err
}

// runConfigRewrite runs `config rewrite`.
func runConfigRewrite(arguments []string, now time.Time, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("config rewrite", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	in := flags.String("in", "", "the earlier configuration file")
	out := flags.String("out", "", "the new configuration file (may be --in)")
	dataDir := flags.String("data-dir", "", "the data directory holding root.key (default: the old file's data_dir, else /var/lib/anix-control)")
	force := flags.Bool("force", false, "drop module_runtime.ca_kek and identity.kek even when root.key does not hold them")
	if err := flags.Parse(arguments); err != nil || flags.NArg() != 0 || strings.TrimSpace(*in) == "" || strings.TrimSpace(*out) == "" {
		_, _ = fmt.Fprintln(stderr, configCommandUsage)
		return 2
	}
	fail := func(format string, args ...any) int {
		_, _ = fmt.Fprintf(stderr, "config rewrite: "+format+"\n", args...)
		return 1
	}
	inPath, err := filepath.Abs(*in)
	if err != nil {
		return fail("%v", err)
	}
	outPath, err := filepath.Abs(*out)
	if err != nil {
		return fail("%v", err)
	}
	old, err := os.ReadFile(inPath) // #nosec G304 -- the operator names the file.
	if err != nil {
		return fail("%v", err)
	}
	inInfo, err := os.Stat(inPath)
	if err != nil {
		return fail("%v", err)
	}
	inPlace := inPath == outPath
	if outInfo, statErr := os.Stat(outPath); statErr == nil {
		if !os.SameFile(inInfo, outInfo) {
			return fail("%s exists: choose another --out, or --out %s to rewrite in place (the old file is kept as a .bak copy)", outPath, inPath)
		}
		inPlace = true
	}

	directory := strings.TrimSpace(*dataDir)
	if directory == "" {
		var declared struct {
			DataDir string `yaml:"data_dir"`
		}
		_ = yaml.Unmarshal(old, &declared)
		directory = firstNonEmptyString(strings.TrimSpace(declared.DataDir), config.Defaults().DataDir)
	}
	opts := config.RewriteOptions{
		Source: inPath, Now: now, Force: *force, RootKeyPath: rootkey.Path(directory),
		ResolvePath: func(raw string) string { return resolveRuntimePath(raw, inPath) },
	}
	switch file, readErr := rootkey.ReadFile(opts.RootKeyPath); {
	case readErr == nil:
		opts.RootKey = &file
	case errors.Is(readErr, fs.ErrNotExist) || errors.Is(readErr, syscall.ENOTDIR):
		opts.RootKeyProblem = "it does not exist yet"
	case errors.Is(readErr, fs.ErrPermission):
		opts.RootKeyProblem = "this user cannot read it: run the rewrite as root"
	default:
		opts.RootKeyProblem = readErr.Error()
	}
	result, err := config.Rewrite(old, opts)
	if err != nil {
		return fail("%v", err)
	}
	if len(result.Refused) > 0 && !*force {
		for _, reason := range result.Refused {
			_, _ = fmt.Fprintf(stderr, "config rewrite: %s\n", reason)
		}
		return fail("nothing was written")
	}

	backup := ""
	if inPlace {
		backup = fmt.Sprintf("%s.bak-%s", inPath, now.UTC().Format("20060102T150405Z"))
		if err := copyFileExclusive(inPath, backup, inInfo); err != nil {
			return fail("keep the old file as %s: %v", backup, err)
		}
	}
	if err := writeFileLike(outPath, result.Output, inInfo); err != nil {
		return fail("%v", err)
	}
	printRewriteReport(stdout, outPath, backup, result)
	return 0
}

func printRewriteReport(w io.Writer, out, backup string, result *config.RewriteResult) {
	_, _ = fmt.Fprintf(w, "Wrote %s.\n", out)
	if backup != "" {
		_, _ = fmt.Fprintf(w, "The old file is %s.\n", backup)
	}
	_, _ = io.WriteString(w, result.Report())
}

// copyFileExclusive copies src to a new file dst with src's mode and owner.
func copyFileExclusive(src, dst string, info os.FileInfo) error {
	data, err := os.ReadFile(src) // #nosec G304 -- the operator's configuration file.
	if err != nil {
		return err
	}
	file, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm()) // #nosec G304 G302 -- next to the operator's file, with its mode.
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	keepOwner(dst, info)
	return nil
}

// writeFileLike writes data to path atomically, with the mode and owner of
// like (the old file).
func writeFileLike(path string, data []byte, like os.FileInfo) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".rewrite-*")
	if err != nil {
		return err
	}
	name := temporary.Name()
	cleanup := func(err error) error {
		_ = temporary.Close()
		_ = os.Remove(name)
		return err
	}
	if err := temporary.Chmod(like.Mode().Perm()); err != nil {
		return cleanup(err)
	}
	if _, err := temporary.Write(data); err != nil {
		return cleanup(err)
	}
	if err := temporary.Sync(); err != nil {
		return cleanup(err)
	}
	if err := temporary.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	keepOwner(name, like)
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name)
		return err
	}
	return nil
}

// keepOwner gives path the owner of like when this process may (root); a
// failure leaves the process's own user.
func keepOwner(path string, like os.FileInfo) {
	if stat, ok := like.Sys().(*syscall.Stat_t); ok && os.Geteuid() == 0 {
		_ = os.Chown(path, int(stat.Uid), int(stat.Gid))
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
