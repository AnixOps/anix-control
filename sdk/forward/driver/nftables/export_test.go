package nftables

import "github.com/AnixOps/anix-control/sdk/forward/driver"

// CheckManifest parses the manifest of an artifact as Apply does.
func CheckManifest(a driver.Artifact) error {
	_, err := parseManifest(a)
	return err
}
