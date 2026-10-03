package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type pluginRelease struct {
	ID       uint   `json:"id"`
	PluginID string `json:"plugin_id"`
	Version  string `json:"version"`
}

type pluginInstallation struct {
	ID              uint   `json:"id"`
	PluginID        string `json:"plugin_id"`
	Target          string `json:"target"`
	DesiredVersion  string `json:"desired_version"`
	ObservedVersion string `json:"observed_version"`
	State           string `json:"state"`
	Enabled         bool   `json:"enabled"`
	LastError       string `json:"last_error"`
}

// installPackages registers, uploads and enables every signed package in
// dir on the control target, as UPGRADE.md "Package Install Window" step 3
// does by hand. identity-platform is imported by Control's bootstrap and
// only enabled here when needed.
func installPackages(ctx context.Context, client *Client, token, dir string, timeout time.Duration) error {
	manifests, err := filepath.Glob(filepath.Join(dir, "*.manifest.json"))
	if err != nil {
		return err
	}
	if len(manifests) == 0 {
		return fmt.Errorf("no *.manifest.json in %s", dir)
	}
	sort.Strings(manifests)
	var existing []pluginRelease
	if err := client.kernel(ctx, http.MethodGet, "/api/v3/plugin-releases", nil, nil, &existing, token); err != nil {
		return err
	}
	want := map[string]string{}
	for _, manifestPath := range manifests {
		base := strings.TrimSuffix(manifestPath, ".manifest.json")
		manifest, err := os.ReadFile(manifestPath) // #nosec G304 G703 -- files of the operator's own package directory
		if err != nil {
			return err
		}
		var header struct {
			PluginID string `json:"id"`
			Version  string `json:"version"`
		}
		if err := json.Unmarshal(manifest, &header); err != nil || header.PluginID == "" || header.Version == "" {
			return fmt.Errorf("%s: manifest has no plugin_id/version", manifestPath)
		}
		want[header.PluginID] = header.Version
		releaseID := uint(0)
		for _, release := range existing {
			if release.PluginID == header.PluginID && release.Version == header.Version {
				releaseID = release.ID
			}
		}
		if releaseID == 0 {
			signature, err := os.ReadFile(base + ".manifest.sig") // #nosec G304 G703 -- files of the operator's own package directory
			if err != nil {
				return err
			}
			var release pluginRelease
			if err := client.kernel(ctx, http.MethodPost, "/api/v3/plugin-releases", nil,
				map[string]string{"manifest": string(manifest), "signature": strings.TrimSpace(string(signature))}, &release, token); err != nil {
				return err
			}
			releaseID = release.ID
			artifact, err := os.ReadFile(base + ".anxp") // #nosec G304 G703 -- files of the operator's own package directory
			if err != nil {
				return err
			}
			if err := client.kernel(ctx, http.MethodPost, fmt.Sprintf("/api/v3/plugin-releases/%d/artifact", releaseID), nil,
				map[string]string{"artifact_base64": base64.StdEncoding.EncodeToString(artifact)}, nil, token); err != nil {
				return err
			}
			fmt.Printf("registered %s %s (release %d)\n", header.PluginID, header.Version, releaseID)
		}
	}
	installations, err := listInstallations(ctx, client, token)
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(want))
	for id := range want {
		ids = append(ids, id)
	}
	// identity-platform first: every other package's routes need a login.
	sort.Slice(ids, func(i, j int) bool {
		if (ids[i] == "identity-platform") != (ids[j] == "identity-platform") {
			return ids[i] == "identity-platform"
		}
		return ids[i] < ids[j]
	})
	for _, id := range ids {
		if current, ok := installations[id]; ok && current.Enabled && current.DesiredVersion == want[id] {
			continue
		}
		body := map[string]any{"plugin_id": id, "target": "control", "desired_version": want[id], "enabled": true}
		if err := client.kernel(ctx, http.MethodPut, "/api/v3/plugin-installations", nil, body, nil, token); err != nil {
			return err
		}
		fmt.Printf("installing %s %s\n", id, want[id])
	}
	deadline := time.Now().Add(timeout)
	for {
		installations, err = listInstallations(ctx, client, token)
		if err != nil {
			return err
		}
		var pending []string
		for _, id := range ids {
			installation := installations[id]
			if installation.ObservedVersion != want[id] || !strings.EqualFold(installation.State, "running") && !strings.EqualFold(installation.State, "healthy") {
				pending = append(pending, fmt.Sprintf("%s(%s %s)", id, installation.State, installation.LastError))
			}
		}
		if len(pending) == 0 {
			fmt.Printf("%d packages installed and running\n", len(ids))
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("packages not running after %s: %s", timeout, strings.Join(pending, ", "))
		}
		time.Sleep(3 * time.Second)
	}
}

func listInstallations(ctx context.Context, client *Client, token string) (map[string]pluginInstallation, error) {
	var rows []pluginInstallation
	if err := client.kernel(ctx, http.MethodGet, "/api/v3/plugin-installations", nil, nil, &rows, token); err != nil {
		return nil, err
	}
	installations := map[string]pluginInstallation{}
	for _, row := range rows {
		if row.Target == "control" {
			installations[row.PluginID] = row
		}
	}
	return installations, nil
}
