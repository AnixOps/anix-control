package config

import (
	"log"
	"strings"

	"gopkg.in/yaml.v3"
)

// removedKeys are the configuration keys an earlier release read and this
// one ignores, each with the reason. A file or an ANIX_CONTROL_* variable
// that still sets one loads: the key changes nothing and is reported once
// (ignoreRemovedKeys). The strict configuration loader
// (docs/design/single-binary/README.md, section 5) takes this table over.
var removedKeys = []struct{ path, reason string }{
	{"plugins.control_execution_enabled", "the built-in packages always run: Control starts their hosts at every start"},
	{"plugins.dispatch_enabled", "Agent plugin dispatch is off until the Agent release manifest brings the Agent plugins (M4)"},
	{"plugins.dispatch_poll_interval", "Agent plugin dispatch is off until the Agent release manifest brings the Agent plugins (M4)"},
	{"plugins.topology_execution_enabled", "topology deployment execution is off until the Agent release manifest brings the Agent plugins (M4)"},
	{"plugins.topology_poll_interval", "topology deployment execution is off until the Agent release manifest brings the Agent plugins (M4)"},
	{"package_routes.default_mode", "route modes come only from the rehearsed list (config/package-route-defaults.json)"},
}

// removedKey is a removed key a configuration sets, with where it is set:
// "the configuration file" or the ANIX_CONTROL_* variable.
type removedKey struct{ path, source, reason string }

// ignoreRemovedKeys logs one warning for each removed key the YAML document
// data or a variable of unknownEnv (ApplyEnv's names that match no key)
// sets, and returns the names of unknownEnv that are not removed keys.
func ignoreRemovedKeys(data []byte, unknownEnv []string) []string {
	found, unknown := findRemovedKeys(data, unknownEnv)
	for _, key := range found {
		log.Printf("Ignoring the removed configuration key %s (set in %s): %s", key.path, key.source, key.reason)
	}
	return unknown
}

// findRemovedKeys returns the removed keys data or unknownEnv set, in the
// table's order, and the names of unknownEnv that are not removed keys. A
// document it cannot read sets none: Load reports the file's own errors.
func findRemovedKeys(data []byte, unknownEnv []string) (found []removedKey, unknown []string) {
	var document any
	_ = yaml.Unmarshal(data, &document)
	env := map[string]bool{}
	for _, name := range unknownEnv {
		env[name] = true
	}
	for _, key := range removedKeys {
		if documentSets(document, key.path) {
			found = append(found, removedKey{key.path, "the configuration file", key.reason})
		}
		name := EnvPrefix + strings.ToUpper(strings.ReplaceAll(key.path, ".", "_"))
		for _, variable := range []string{name, name + EnvFileSuffix} {
			if env[variable] {
				found = append(found, removedKey{key.path, variable, key.reason})
				delete(env, variable)
			}
		}
	}
	for _, name := range unknownEnv {
		if env[name] {
			unknown = append(unknown, name)
		}
	}
	return found, unknown
}

// documentSets reports whether a decoded YAML document has the dotted key
// path, whatever its value.
func documentSets(document any, path string) bool {
	value := document
	for _, part := range strings.Split(path, ".") {
		mapping, ok := value.(map[string]any)
		if !ok {
			return false
		}
		if value, ok = mapping[part]; !ok {
			return false
		}
	}
	return true
}
