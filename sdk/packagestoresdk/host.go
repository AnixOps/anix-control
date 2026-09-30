package packagestoresdk

import (
	"context"
	"io/fs"
	"sync"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
)

// Opener returns the package's open storage.
type Opener func(ctx context.Context) (*Store, error)

// SharedOpener leases and opens storage on first use and reuses it after
// that. A failed attempt is retried on the next call.
func SharedOpener(leaser Leaser) Opener {
	var (
		mu    sync.Mutex
		store *Store
	)
	return func(ctx context.Context) (*Store, error) {
		mu.Lock()
		defer mu.Unlock()
		if store != nil {
			return store, nil
		}
		opened, err := Open(ctx, leaser)
		if err != nil {
			return nil, err
		}
		store = opened
		return store, nil
	}
}

// IndexMigrator returns a pluginhostsdk.RouterConfig.IndexMigration handler
// that applies the migration index embedded in the host binary. The kernel
// sends it when the host starts; a completed run reports StepsDigest, which
// the kernel checks against the verified index of the package artifact.
func IndexMigrator(open Opener, fsys fs.FS, indexPath string) func(context.Context, pluginhostsdk.MigrationRequest) (pluginhostsdk.MigrationResponse, error) {
	return func(ctx context.Context, request pluginhostsdk.MigrationRequest) (pluginhostsdk.MigrationResponse, error) {
		store, err := open(ctx)
		if err != nil {
			return pluginhostsdk.MigrationResponse{}, err
		}
		result, err := RunEmbeddedMigrations(ctx, store, fsys, indexPath)
		if err != nil {
			return pluginhostsdk.MigrationResponse{}, err
		}
		return pluginhostsdk.MigrationResponse{Checkpoint: request.MigrationID, ValidationDigest: result.StepsDigest, Complete: true}, nil
	}
}
