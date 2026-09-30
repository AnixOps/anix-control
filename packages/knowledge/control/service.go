package main

import (
	"context"
	"log"

	"github.com/AnixOps/anix-control/sdk/packagestoresdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/packages/knowledge/native"
	"gorm.io/gorm"
)

// knowledgeBridge is what the knowledge host needs from the package bridge.
type knowledgeBridge interface {
	pluginhostsdk.RouterBridge
	packagestoresdk.Leaser
}

// newKnowledgeService returns the knowledge host's router. Every route has a
// native handler on the adopted v2_knowledge table; a route serves natively
// once the kernel sets its mode, and falls back to the legacy handler
// otherwise.
func newKnowledgeService(bridge knowledgeBridge, leaseID string) (*pluginhostsdk.Router, error) {
	storage := packagestoresdk.SharedOpener(bridge)
	service := &native.Service{Open: func(ctx context.Context) (*gorm.DB, error) {
		store, err := storage(ctx)
		if err != nil {
			return nil, err
		}
		return store.DB.WithContext(ctx), nil
	}}
	return pluginhostsdk.NewRouter(pluginhostsdk.RouterConfig{
		PackageID: "knowledge", LeaseID: leaseID, Bridge: bridge, Logf: log.Printf,
		AllowRoute: func(routeID string) bool {
			_, allowed := knowledgeRoutes[routeID]
			return allowed
		},
		Native: service.Handlers(),
	})
}

// knowledgeRoutes are the package's compatibility routes; each has a native
// handler.
var knowledgeRoutes = map[string]struct{}{
	"knowledge.admin.knowledge.get":       {},
	"knowledge.admin.knowledge.post":      {},
	"knowledge.admin.knowledge.id.put":    {},
	"knowledge.admin.knowledge.id.delete": {},
	"knowledge.article.list":              {},
	"knowledge.user.knowledge.id.get":     {},
}
