package systemdreport

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"strconv"
)

// The services table is off on every node until an administrator enables it
// for that node (owner decision H24). The switch and the node's unit globs
// live in the machine-telemetry Agent installation's configuration, under
// ConfigKey:
//
//	{
//	  "interval_seconds": 30,
//	  "systemd_services": {
//	    "nodes": {
//	      "12": {"enabled": true, "include": ["nginx*.service"], "exclude": ["*-debug.service"]}
//	    }
//	  }
//	}
//
// Control pushes that whole document to every node the package is assigned
// to (the plugin.configure operation's config); each Agent reads only the
// entry of its own node id, with ParseConfig and Config.Node. A node without
// an entry, or with enabled false, collects nothing.
const (
	// ConfigKey is the key of the services settings in the package
	// configuration document.
	ConfigKey = "systemd_services"
	// MaxConfigNodes caps the nodes listed in the settings.
	MaxConfigNodes = 4096
	// MaxGlobs caps the include and the exclude list of one node.
	MaxGlobs = 32
	// MaxGlobLength caps one glob, in bytes.
	MaxGlobLength = MaxNameLength
)

// ErrInvalidConfig wraps every reason ParseConfig refuses the settings.
var ErrInvalidConfig = errors.New("invalid systemd_services configuration")

var (
	// globPattern is the unit name alphabet plus path.Match's * ? [ ] ^.
	globPattern   = regexp.MustCompile(`^[A-Za-z0-9:_.\\@*?\[\]^-]+$`)
	nodeIDPattern = regexp.MustCompile(`^[1-9][0-9]{0,9}$`)
)

// Config is the services settings of every node.
type Config struct {
	// Nodes maps a proxy node id, in decimal, to its settings.
	Nodes map[string]NodeConfig `json:"nodes"`
}

// NodeConfig is one node's services settings.
type NodeConfig struct {
	// Enabled turns collection on for the node. It is false by default.
	Enabled bool `json:"enabled"`
	// Include, when not empty, limits the table to the units matching one
	// of its globs; Exclude drops the units matching one of its globs.
	// Both use path.Match syntax, as Selected applies them.
	Include []string `json:"include,omitempty"`
	Exclude []string `json:"exclude,omitempty"`
}

// Node returns the settings of a node: off when it has no entry.
func (c Config) Node(nodeID uint64) NodeConfig {
	return c.Nodes[strconv.FormatUint(nodeID, 10)]
}

// Selected applies the node's globs to a unit name, as Selected does.
func (n NodeConfig) Selected(name string) bool {
	return Selected(name, n.Include, n.Exclude)
}

// ValidGlob reports whether glob is a well-formed path.Match pattern of at
// most MaxGlobLength bytes over the unit name alphabet.
func ValidGlob(glob string) bool {
	if glob == "" || len(glob) > MaxGlobLength || !globPattern.MatchString(glob) {
		return false
	}
	// path.Match checks the whole pattern, even when the name does not match.
	_, err := path.Match(glob, "")
	return err == nil
}

// ParseConfig reads the services settings from a machine-telemetry package
// configuration document. A document without ConfigKey, or with it null,
// enables no node. It refuses settings with an unknown field, a node id
// that is not a decimal proxy node id, more than MaxConfigNodes nodes, more
// than MaxGlobs globs in a list, or a malformed glob.
func ParseConfig(document []byte) (Config, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(document, &fields); err != nil || fields == nil {
		return Config{}, fmt.Errorf("%w: the configuration is not a JSON object", ErrInvalidConfig)
	}
	raw, ok := fields[ConfigKey]
	if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return Config{Nodes: map[string]NodeConfig{}}, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var config Config
	if err := decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Config{}, fmt.Errorf("%w: trailing data", ErrInvalidConfig)
	}
	if config.Nodes == nil {
		config.Nodes = map[string]NodeConfig{}
	}
	if len(config.Nodes) > MaxConfigNodes {
		return Config{}, fmt.Errorf("%w: more than %d nodes", ErrInvalidConfig, MaxConfigNodes)
	}
	for nodeID, node := range config.Nodes {
		if !nodeIDPattern.MatchString(nodeID) {
			return Config{}, fmt.Errorf("%w: node %q is not a node id", ErrInvalidConfig, nodeID)
		}
		if parsed, err := strconv.ParseUint(nodeID, 10, 32); err != nil || parsed == 0 {
			return Config{}, fmt.Errorf("%w: node %q is not a node id", ErrInvalidConfig, nodeID)
		}
		for list, globs := range map[string][]string{"include": node.Include, "exclude": node.Exclude} {
			if len(globs) > MaxGlobs {
				return Config{}, fmt.Errorf("%w: node %s: more than %d %s globs", ErrInvalidConfig, nodeID, MaxGlobs, list)
			}
			for _, glob := range globs {
				if !ValidGlob(glob) {
					return Config{}, fmt.Errorf("%w: node %s: %s glob %q is malformed", ErrInvalidConfig, nodeID, list, glob)
				}
			}
		}
	}
	return config, nil
}
