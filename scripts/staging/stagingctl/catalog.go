package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	configtables "github.com/AnixOps/anix-control/v4/config"
)

// Batch is one cutover batch of the v4.1.0 staging rehearsal (owner
// decision H7). Identity group A is not part of any batch: it moves only with
// the identity cutover.
type Batch struct {
	Number   int
	Name     string
	Packages []string
	// Parity lists the packagecompat suites (internal/tests/<name>) that
	// prove the batch's write routes.
	Parity []string
}

// Batches assigns every package with native-flagged routes, except
// identity-platform, to a batch. The four packages the roadmap did not
// name go where their routes belong: machine-telemetry (admin dashboard and
// traffic reports) and protocol-runtime (agent task and protocol template
// reads) next to platform in batch 2; gost-mesh (forward NodeX status and
// connection tests) and wireguard (key pair generation) with forward and
// proxy-node in batch 4.
var Batches = []Batch{
	{Number: 1, Name: "knowledge + ticket", Packages: []string{"knowledge", "ticket"},
		Parity: []string{"knowledgecompat", "ticketcompat"}},
	{Number: 2, Name: "notification + platform (+ machine-telemetry, protocol-runtime)",
		Packages: []string{"notification", "platform", "machine-telemetry", "protocol-runtime"},
		Parity:   []string{"notificationcompat", "platformcompat", "machinetelemetrycompat", "protocolruntimecompat"}},
	{Number: 3, Name: "plan + order + payment + affiliate", Packages: []string{"plan", "order", "payment", "affiliate"},
		Parity: []string{"plancompat", "ordercompat", "paymentcompat", "affiliatecompat"}},
	{Number: 4, Name: "subscription + forward + proxy-node (+ gost-mesh, wireguard)",
		Packages: []string{"subscription", "forward", "proxy-node", "gost-mesh", "wireguard"},
		Parity:   []string{"subscriptioncompat", "proxynodecompat", "gostmeshcompat", "wireguardcompat"}},
}

// CatalogRoute is one row of config/package-extraction.json.
type CatalogRoute struct {
	Method    string `json:"method"`
	Path      string `json:"path"`
	PackageID string `json:"package_id"`
	RouteID   string `json:"route_id"`
	Mode      string `json:"mode"`
}

// Read reports whether the route can run in shadow: only GET routes do.
func (r CatalogRoute) Read() bool { return r.Method == "GET" }

func batchByNumber(number int) (Batch, error) {
	for _, batch := range Batches {
		if batch.Number == number {
			return batch, nil
		}
	}
	return Batch{}, fmt.Errorf("unknown batch %d (1-%d)", number, len(Batches))
}

func loadCatalog() ([]CatalogRoute, error) {
	var document struct {
		Routes []CatalogRoute `json:"routes"`
	}
	if err := json.Unmarshal(configtables.PackageExtraction, &document); err != nil {
		return nil, fmt.Errorf("parse config/package-extraction.json: %w", err)
	}
	return document.Routes, nil
}

// batchRoutes returns the native-flagged routes of a batch, sorted by
// package and route id.
func batchRoutes(batch Batch) ([]CatalogRoute, error) {
	routes, err := loadCatalog()
	if err != nil {
		return nil, err
	}
	packages := map[string]bool{}
	for _, id := range batch.Packages {
		packages[id] = true
	}
	var selected []CatalogRoute
	for _, route := range routes {
		if route.Mode == "native-flagged" && packages[route.PackageID] {
			selected = append(selected, route)
		}
	}
	sort.Slice(selected, func(i, j int) bool {
		if selected[i].PackageID != selected[j].PackageID {
			return selected[i].PackageID < selected[j].PackageID
		}
		return selected[i].RouteID < selected[j].RouteID
	})
	return selected, nil
}

// checkBatchCoverage fails when a package with native-flagged routes is in
// no batch (identity-platform excepted) or in two.
func checkBatchCoverage() error {
	routes, err := loadCatalog()
	if err != nil {
		return err
	}
	owner := map[string]int{}
	for _, batch := range Batches {
		for _, id := range batch.Packages {
			if previous, ok := owner[id]; ok {
				return fmt.Errorf("package %s is in batches %d and %d", id, previous, batch.Number)
			}
			owner[id] = batch.Number
		}
	}
	var missing []string
	seen := map[string]bool{}
	for _, route := range routes {
		if route.Mode != "native-flagged" || route.PackageID == "identity-platform" || seen[route.PackageID] {
			continue
		}
		seen[route.PackageID] = true
		if _, ok := owner[route.PackageID]; !ok {
			missing = append(missing, route.PackageID)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("packages with native-flagged routes in no batch: %s", strings.Join(missing, ", "))
	}
	return nil
}
