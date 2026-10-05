package relayd

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/AnixOps/anix-control/sdk/forward/driver/anixops/relayctl"
)

// Version is the relay binary's version; the Agent's build sets it with
// -ldflags "-X github.com/AnixOps/anix-control/sdk/forward/driver/anixops/relayd.Version=...".
var Version = "dev"

// VersionLine is what `anixops-relay -V` prints: the driver's probe reads the
// version and the wire versions the binary speaks from it.
func VersionLine() string {
	return fmt.Sprintf("anixops-relay %s wire=%s", Version, wireVersion)
}

// wireVersion is the wire version this build speaks: the prototype's
// `anixops/0` (docs/architecture/anixops-protocol.md section 6.7); the
// production versions replace it when the wire freezes.
const wireVersion = "0"

// Main is the `anixops-relay` program: it loads the configuration the driver
// last applied (-config), serves the control API on -socket, and runs until
// SIGTERM or SIGINT, then stops cleanly (every carrier gets a GOAWAY). It
// returns the process's exit status.
func Main(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("anixops-relay", flag.ContinueOnError)
	fs.SetOutput(stderr)
	showVersion := fs.Bool("V", false, "print the version and the wire versions, then exit")
	configPath := fs.String("config", "", "the configuration file the driver last applied; loaded at start when it exists")
	socket := fs.String("socket", "", "the control socket to serve (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		_, _ = fmt.Fprintln(stdout, VersionLine())
		return 0
	}
	if *socket == "" {
		_, _ = fmt.Fprintln(stderr, "anixops-relay: -socket is required")
		return 2
	}
	logger := slog.New(slog.NewJSONHandler(stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	r := New(Options{Logger: logger})
	if *configPath != "" {
		if err := r.LoadFile(*configPath); err != nil {
			logger.Error("could not load the configuration; running without hops", "path", *configPath, "err", err)
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	logger.Info("anixops-relay started", "version", Version, "wire", wireVersion, "instance", r.Instance())
	err := r.Serve(ctx, *socket)
	r.Close()
	if err != nil {
		logger.Error("control socket failed", "err", err)
		return 1
	}
	return 0
}

// LoadFile applies the configuration file the driver last wrote, as the unit
// does at start; a missing file is not an error (nothing was applied yet).
func (r *Relay) LoadFile(path string) error {
	b, err := os.ReadFile(path) // #nosec G304 -- the -config path the unit names
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if _, err := relayctl.Parse(b); err != nil {
		return err
	}
	_, _, err = r.Apply(b)
	return err
}
