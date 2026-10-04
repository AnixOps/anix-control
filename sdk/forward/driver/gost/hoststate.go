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
	// Loads counts the starts and reloads Apply and ReloadCredentials made
	// gost do: each one re-creates every service, so it is part of the
	// counter epoch.
	Loads uint64 `json:"loads,omitempty"`
	// Served is the metrics path of the configuration gost loaded at the
	// start or reload Apply made last. Changes through the web API keep
	// it (no API call moves the metrics path), so it differs from the
	// configuration file's once Apply changed services that way.
	Served string `json:"served,omitempty"`
	// Seq counts the applies and credential reloads that deleted or
	// created services through the web API, and Created holds, per hop
	// name, the Seq of the last one that deleted or created one of the
	// hop's services: part of the hop's counter epoch, which those end.
	// gost's own creation times have a resolution of a second, too coarse
	// to tell two re-creations apart.
	Seq     uint64            `json:"seq,omitempty"`
	Created map[string]uint64 `json:"created,omitempty"`
}

// carry copies what outlives an apply from the recorded state: the
// loads, the served metrics path and the hops' creations, the latter for
// the hops of next only.
func (s *hostState) carry(from *hostState) {
	if from == nil {
		return
	}
	s.Loads, s.Served, s.Seq = from.Loads, from.Served, from.Seq
	s.Created = nil
	for _, h := range s.Hops {
		if v, ok := from.Created[hopName(h.key())]; ok {
			if s.Created == nil {
				s.Created = map[string]uint64{}
			}
			s.Created[hopName(h.key())] = v
		}
	}
}

// recreated records that an apply or a credential reload deleted or
// created services of the named hops: the counter epochs of those the
// state has end.
func (s *hostState) recreated(hops map[string]bool) {
	if len(hops) == 0 {
		return
	}
	s.Seq++
	for _, h := range s.Hops {
		if name := hopName(h.key()); hops[name] {
			if s.Created == nil {
				s.Created = map[string]uint64{}
			}
			s.Created[name] = s.Seq
		}
	}
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
