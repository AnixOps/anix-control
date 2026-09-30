package main

import (
	"log"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
)

// newGenericService returns the host's router. The generic host has no
// native routes, so every route passes through the package bridge to its
// legacy handler whatever the installation configuration says.
func newGenericService(id string, bridge pluginhostsdk.RouterBridge, leaseID string) (*pluginhostsdk.Router, error) {
	return pluginhostsdk.NewRouter(pluginhostsdk.RouterConfig{PackageID: id, LeaseID: leaseID, Bridge: bridge, Logf: log.Printf})
}
