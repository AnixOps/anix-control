package gost

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// ProbeReport is what Probe found besides the configuration.
type ProbeReport struct {
	// Missing lists the features the host lacks, with the reason, as
	// "feature: reason".
	Missing []string
	// Warnings are host conditions the driver works with but an operator
	// should know about (a gost release other than the pinned one).
	Warnings []string
}

// gostVersion matches `gost -V`: "gost v3.2.6 (go1.25.4 linux/amd64)".
var gostVersion = regexp.MustCompile(`gost v([0-9]+)\.([0-9]+)\.([0-9]+)`)

// Probe checks the host and answers base with what it offers: Version set
// to "gost <x.y.z>" (or empty with Unavailable saying why: gost or ss
// missing, a gost that is not v3, the unit missing or someone else's),
// and the link securities turned off when the link certificate files are
// not there. It changes nothing on the host. The Agent probes once at
// start and passes the result to New, so Render stays a function of the
// configuration. Only a done context is an error.
func Probe(ctx context.Context, r Runner, sup Supervisor, base Config) (Config, *ProbeReport, error) {
	cfg := base
	cfg.Strategies = slices.Clone(base.Strategies)
	rep := &ProbeReport{}
	unavailable := func(format string, args ...any) (Config, *ProbeReport, error) {
		cfg.Version = ""
		cfg.Unavailable = clip(fmt.Sprintf(format, args...))
		return cfg, rep, nil
	}
	if err := ctx.Err(); err != nil {
		return base, nil, err
	}
	out, err := r.Run(ctx, "gost", []string{"-V"}, nil)
	if err := ctx.Err(); err != nil {
		return base, nil, err
	}
	if err != nil {
		if missingBinary(err) {
			return unavailable("gost is not installed (the Agent ships gost %s)", PinnedVersion)
		}
		return unavailable("gost -V: %v", err)
	}
	m := gostVersion.FindStringSubmatch(string(out))
	if m == nil {
		return unavailable("gost -V printed no version: %q", firstLine(string(out)))
	}
	version := m[1] + "." + m[2] + "." + m[3]
	if m[1] != "3" {
		return unavailable("gost %s is not gost v3 (the Agent ships gost %s)", version, PinnedVersion)
	}
	if version != PinnedVersion {
		rep.Warnings = append(rep.Warnings, fmt.Sprintf("gost %s is not the pinned release %s; the driver is tested with %s only", version, PinnedVersion, PinnedVersion))
	}
	if minor, _ := strconv.Atoi(m[2]); minor < 2 {
		return unavailable("gost %s is older than 3.2, whose configuration the driver writes", version)
	}

	if _, err := r.Run(ctx, "ss", []string{"-V"}, nil); err != nil {
		if err := ctx.Err(); err != nil {
			return base, nil, err
		}
		return unavailable("ss (iproute2), which the driver reads listening ports with, does not run: %s", firstLine(err.Error()))
	}
	if sup != nil {
		if err := sup.Check(ctx); err != nil {
			if err := ctx.Err(); err != nil {
				return base, nil, err
			}
			return unavailable("%s", firstLine(err.Error()))
		}
	}

	if cfg.linkTLS() {
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
			rep.Missing = append(rep.Missing, "link_tls: "+strings.Join(gone, ", "))
			cfg.LinkCert, cfg.LinkKey, cfg.LinkCA = "", "", ""
		}
	} else {
		rep.Missing = append(rep.Missing, "link_tls: no link certificate configured; RAW links only")
	}
	cfg.Version = "gost " + version
	cfg.Unavailable = ""
	return cfg, rep, nil
}
