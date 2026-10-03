package gost

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/AnixOps/anix-control/sdk/forward/driver"
)

// hostState is the driver's state file, Dir/state.json: the ownership
// mark and what was applied last. It is written before the first
// configuration (Digest empty: nothing applied yet), so a crash between
// the two never leaves the driver's own directory looking foreign, and
// after every apply that succeeded.
type hostState struct {
	Owner      string        `json:"owner"`
	Node       string        `json:"node,omitempty"`
	Generation uint64        `json:"generation,omitempty"`
	StateHash  string        `json:"state_hash,omitempty"`
	Digest     string        `json:"digest,omitempty"`
	Hops       []manifestHop `json:"hops,omitempty"`
}

func (s *hostState) applied() bool { return s != nil && s.Digest != "" }

func (s *hostState) hop(k driver.HopKey) *manifestHop {
	for i := range s.Hops {
		if s.Hops[i].key() == k {
			return &s.Hops[i]
		}
	}
	return nil
}

// host is the driver's directory as Apply, Observe and Remove find it.
type host struct {
	present bool       // a configuration or a state file exists
	owned   bool       // the state file carries OwnerMark
	state   *hostState // when owned
	config  []byte     // the configuration file, nil when missing
}

// readHost reads the driver's directory. A configuration without a state
// file, or a state file without OwnerMark, is someone else's.
func (d *Driver) readHost() (*host, error) {
	h := &host{}
	cfg, err := os.ReadFile(d.cfg.configPath())
	switch {
	case err == nil:
		h.present, h.config = true, cfg
	case !errors.Is(err, fs.ErrNotExist):
		return nil, fmt.Errorf("gost driver: read configuration: %w", err)
	}
	raw, err := os.ReadFile(d.cfg.statePath())
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return h, nil
	case err != nil:
		return nil, fmt.Errorf("gost driver: read state: %w", err)
	}
	h.present = true
	var st hostState
	if json.Unmarshal(raw, &st) == nil && st.Owner == OwnerMark {
		h.owned, h.state = true, &st
	}
	return h, nil
}

// writeState records the state file.
func (d *Driver) writeState(st *hostState) error {
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	if err := writeFileAtomic(d.cfg.statePath(), append(b, '\n'), 0o600); err != nil {
		return fmt.Errorf("gost driver: write state: %w", err)
	}
	return nil
}

// removeFiles deletes the configuration and the state file.
func (d *Driver) removeFiles() error {
	for _, p := range []string{d.cfg.configPath(), d.cfg.statePath()} {
		if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("gost driver: remove %s: %w", p, err)
		}
	}
	return nil
}

// notOwned is Apply's error for a directory someone else's files occupy.
func (d *Driver) notOwned() error {
	return fmt.Errorf("%w: %s holds a gost configuration without the driver's state file (%s)", driver.ErrNotOwned, d.cfg.Dir, OwnerMark)
}
