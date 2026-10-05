package anixops

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/AnixOps/anix-control/sdk/forward/driver"
)

// hostState is the driver's state file, Dir/state.json: the ownership mark
// and what was applied last. It is written before the first configuration
// (Digest empty: nothing applied yet), so a crash between the two never leaves
// the driver's own directory looking foreign, and after every apply that
// succeeded.
type hostState struct {
	Owner      string `json:"owner"`
	Node       string `json:"node,omitempty"`
	Generation uint64 `json:"generation,omitempty"`
	StateHash  string `json:"state_hash,omitempty"`
	Digest     string `json:"digest,omitempty"`
}

func (s *hostState) applied() bool { return s != nil && s.Digest != "" }

// host is the driver's directory as Apply, Observe and Remove find it.
type host struct {
	present bool       // a configuration or a state file exists
	owned   bool       // the state file carries OwnerMark
	state   *hostState // when owned
	config  []byte     // the configuration file, nil when missing
}

// readHost reads the driver's directory. A configuration without a state file,
// or a state file without OwnerMark, is someone else's.
func (d *Driver) readHost() (*host, error) {
	h := &host{}
	cfg, err := os.ReadFile(d.cfg.configPath())
	switch {
	case err == nil:
		h.present, h.config = true, cfg
	case !errors.Is(err, fs.ErrNotExist):
		return nil, fmt.Errorf("anixops driver: read configuration: %w", err)
	}
	raw, err := os.ReadFile(d.cfg.statePath())
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return h, nil
	case err != nil:
		return nil, fmt.Errorf("anixops driver: read state: %w", err)
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
		return fmt.Errorf("anixops driver: write state: %w", err)
	}
	return nil
}

// removeFiles deletes the configuration, the state file and the stateless
// reset key.
func (d *Driver) removeFiles() error {
	for _, p := range []string{d.cfg.configPath(), d.cfg.statePath(), d.cfg.resetPath()} {
		if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("anixops driver: remove %s: %w", p, err)
		}
	}
	return nil
}

// ensureResetKey makes the 32 bytes a QUIC listener answers the packets of
// connections it forgot with: the relay cannot write its own directory, so the
// driver does, once; the relay reads it (group readable).
func (d *Driver) ensureResetKey() error {
	if b, err := os.ReadFile(d.cfg.resetPath()); err == nil && len(b) == 32 {
		return nil
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return err
	}
	if err := writeFileAtomic(d.cfg.resetPath(), key, 0o640); err != nil {
		return fmt.Errorf("anixops driver: write the stateless reset key: %w", err)
	}
	return nil
}

// notOwned is Apply's error for a directory someone else's files occupy.
func (d *Driver) notOwned() error {
	return fmt.Errorf("%w: %s holds a relay configuration without the driver's state file (%s)", driver.ErrNotOwned, d.cfg.Dir, OwnerMark)
}
