package anixops

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
)

// ProbeReport is what Probe found besides the configuration.
type ProbeReport struct {
	// Missing lists the features the host lacks, with the reason, as
	// "feature: reason".
	Missing []string
	// Warnings are host conditions the driver works with but an operator
	// should know about (UDP socket buffers too small for QUIC).
	Warnings []string
}

// versionOutput matches `anixops-relay -V`: "anixops-relay 4.2.0-rc.2 wire=0".
var versionOutput = regexp.MustCompile(`^anixops-relay (\S+) wire=([0-9]+(?:,[0-9]+)*)$`)

// sysctlDir is where Probe reads the socket buffer limits (tests move it).
var sysctlDir = "/proc/sys/net/core"

// Probe checks the host and answers base with what it offers: Version set to
// "anixops-relay <x>" and the wire versions the binary speaks (or Version
// empty with Unavailable saying why: the relay missing or not running -V, the
// unit missing or someone else's), and the TLS_TCP and QUIC carriers turned off
// when the link certificate files are not there (PLAIN stays). It changes
// nothing on the host. The Agent probes once at start and passes the result to
// New, so Render stays a function of the configuration. Only a done context is
// an error.
func Probe(ctx context.Context, r Runner, sup Supervisor, base Config) (Config, *ProbeReport, error) {
	cfg := base
	cfg.Strategies = slices.Clone(base.Strategies)
	cfg.Carriers = slices.Clone(base.Carriers)
	cfg.ProtocolVersions = slices.Clone(base.ProtocolVersions)
	rep := &ProbeReport{}
	unavailable := func(format string, args ...any) (Config, *ProbeReport, error) {
		cfg.Version = ""
		cfg.Unavailable = clip(fmt.Sprintf(format, args...))
		return cfg, rep, nil
	}
	if err := ctx.Err(); err != nil {
		return base, nil, err
	}
	out, err := r.Run(ctx, CmdRelay, []string{"-V"}, nil)
	if err := ctx.Err(); err != nil {
		return base, nil, err
	}
	if err != nil {
		if missingBinary(err) {
			return unavailable("anixops-relay is not installed (the Agent ships it)")
		}
		return unavailable("anixops-relay -V: %v", err)
	}
	m := versionOutput.FindStringSubmatch(strings.TrimSpace(string(out)))
	if m == nil {
		return unavailable("anixops-relay -V printed no version: %q", firstLine(string(out)))
	}
	var wires []uint32
	for _, w := range strings.Split(m[2], ",") {
		v, err := strconv.ParseUint(w, 10, 32)
		if err != nil {
			return unavailable("anixops-relay -V printed a bad wire version %q", w)
		}
		wires = append(wires, uint32(v))
	}
	if sup != nil {
		if err := sup.Check(ctx); err != nil {
			if err := ctx.Err(); err != nil {
				return base, nil, err
			}
			return unavailable("%s", firstLine(err.Error()))
		}
	}
	cfg.ProtocolVersions = wires

	if cfg.linkFiles() {
		var gone []string
		for _, p := range []string{cfg.LinkCert, cfg.LinkKey, cfg.LinkCA} {
			if _, err := os.Stat(p); err != nil {
				reason := "not readable"
				if errors.Is(err, os.ErrNotExist) {
					reason = "missing"
				}
				gone = append(gone, p+" "+reason)
			}
		}
		if len(gone) > 0 {
			rep.Missing = append(rep.Missing, "link_tls: "+strings.Join(gone, ", ")+"; the PLAIN carrier only")
			cfg.LinkCert, cfg.LinkKey, cfg.LinkCA = "", "", ""
		}
	} else {
		rep.Missing = append(rep.Missing, "link_tls: no link certificate configured; the PLAIN carrier only")
	}
	if !cfg.linkFiles() {
		cfg.Carriers = slices.DeleteFunc(cfg.Carriers, func(c forwardv1.AnixOpsCarrier) bool {
			return c == forwardv1.AnixOpsCarrier_ANIXOPS_CARRIER_TLS_TCP || c == forwardv1.AnixOpsCarrier_ANIXOPS_CARRIER_QUIC
		})
	}
	if cfg.hasCarrier(forwardv1.AnixOpsCarrier_ANIXOPS_CARRIER_QUIC) {
		for _, name := range []string{"rmem_max", "wmem_max"} {
			if v, ok := readSysctl(name); ok && v < QUICSocketBuffer {
				rep.Warnings = append(rep.Warnings, fmt.Sprintf("net.core.%s is %d, below the %d QUIC carriers need; the forward-node sysctl drop-in raises it", name, v, QUICSocketBuffer))
			}
		}
	}
	cfg.Version = "anixops-relay " + m[1]
	cfg.Unavailable = ""
	return cfg, rep, nil
}

func readSysctl(name string) (int, bool) {
	b, err := os.ReadFile(sysctlDir + "/" + name) // #nosec G304 -- a fixed sysctl directory and a constant name
	if err != nil {
		return 0, false
	}
	v, err := strconv.Atoi(strings.TrimSpace(string(b)))
	return v, err == nil
}
