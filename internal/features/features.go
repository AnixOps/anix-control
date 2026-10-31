// Package features gates unfinished features behind the dev channel
// (docs/RELEASING.md, "Dev builds"). A dev build (a version with ".dev.",
// such as 5.0.0-rc.15.dev.42) is built from main after every merge and runs
// on test installations only; a feature still being built is registered
// with StageDev and runs only there, so main can take it piece by piece
// while releases keep it off.
//
// To gate a feature, add it to registry with StageDev and check
// Enabled(name) where it starts (a route, a worker, a field the API sends).
// When it is finished, move it to StageStable: the next release turns it
// on. A release later, remove the entry and the checks.
package features

import (
	"strings"
	"sync/atomic"
)

// Stage is where a feature runs.
type Stage string

// Feature stages.
const (
	// StageDev runs in dev builds only.
	StageDev Stage = "dev"
	// StageStable runs in every build.
	StageStable Stage = "stable"
)

// Channels a build belongs to.
const (
	ChannelDev    = "dev"
	ChannelStable = "stable"
)

// Feature is a gated feature.
type Feature struct {
	// Name is the stable identifier the code checks, lowercase with dots
	// or dashes ("nodes.bulk-edit").
	Name string `json:"name"`
	// Description says what the feature does, for the administrator.
	Description string `json:"description"`
	Stage       Stage  `json:"stage"`
}

// Status is a feature and whether this build runs it.
type Status struct {
	Feature
	Enabled bool `json:"enabled"`
}

// registry lists the gated features. Empty while nothing is gated.
var registry = []Feature{}

var devBuild atomic.Bool

// IsDevVersion reports whether version (with or without "v") is a dev
// build's.
func IsDevVersion(version string) bool {
	return strings.Contains(version, ".dev.")
}

// SetVersion records the running build's version: a dev build's turns the
// StageDev features on. Control calls it once at startup.
func SetVersion(version string) {
	devBuild.Store(IsDevVersion(version))
}

// Channel is the running build's channel, ChannelDev or ChannelStable.
func Channel() string {
	if devBuild.Load() {
		return ChannelDev
	}
	return ChannelStable
}

// Enabled reports whether the running build runs the feature. A name that
// is not registered is off.
func Enabled(name string) bool {
	for _, feature := range registry {
		if feature.Name == name {
			return enabled(feature)
		}
	}
	return false
}

// List returns every registered feature with whether it runs here.
func List() []Status {
	list := make([]Status, 0, len(registry))
	for _, feature := range registry {
		list = append(list, Status{Feature: feature, Enabled: enabled(feature)})
	}
	return list
}

func enabled(feature Feature) bool {
	switch feature.Stage {
	case StageStable:
		return true
	case StageDev:
		return devBuild.Load()
	default:
		return false
	}
}
