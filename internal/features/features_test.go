package features

import (
	"regexp"
	"testing"
)

func withRegistry(t *testing.T, features []Feature, version string) {
	t.Helper()
	saved, savedDev := registry, devBuild.Load()
	t.Cleanup(func() { registry = saved; devBuild.Store(savedDev) })
	registry = features
	SetVersion(version)
}

func TestDevFeaturesRunInDevBuildsOnly(t *testing.T) {
	gated := []Feature{
		{Name: "dev.thing", Description: "in progress", Stage: StageDev},
		{Name: "done.thing", Description: "finished", Stage: StageStable},
	}
	for _, c := range []struct {
		version, channel string
		dev              bool
	}{
		{"5.0.0-rc.15", ChannelStable, false},
		{"v5.0.0", ChannelStable, false},
		{"5.0.0-rc.15.dev.42", ChannelDev, true},
		{"v5.0.1-alpha.0.dev.3", ChannelDev, true},
	} {
		withRegistry(t, gated, c.version)
		if got := Channel(); got != c.channel {
			t.Errorf("%s: Channel() = %s, want %s", c.version, got, c.channel)
		}
		if got := Enabled("dev.thing"); got != c.dev {
			t.Errorf("%s: dev feature enabled = %v, want %v", c.version, got, c.dev)
		}
		if !Enabled("done.thing") {
			t.Errorf("%s: a stable feature is off", c.version)
		}
		if Enabled("unknown") {
			t.Errorf("%s: an unregistered feature is on", c.version)
		}
		list := List()
		if len(list) != 2 || list[0].Enabled != c.dev || !list[1].Enabled {
			t.Errorf("%s: List() = %+v", c.version, list)
		}
	}
}

// Every registered feature has a unique, well-formed name, a description
// and a known stage.
func TestRegistry(t *testing.T) {
	name := regexp.MustCompile(`^[a-z0-9]+([.-][a-z0-9]+)*$`)
	seen := map[string]bool{}
	for _, feature := range registry {
		if !name.MatchString(feature.Name) || seen[feature.Name] {
			t.Errorf("feature name %q is malformed or repeated", feature.Name)
		}
		seen[feature.Name] = true
		if feature.Description == "" {
			t.Errorf("feature %s has no description", feature.Name)
		}
		if feature.Stage != StageDev && feature.Stage != StageStable {
			t.Errorf("feature %s has stage %q", feature.Name, feature.Stage)
		}
	}
}
