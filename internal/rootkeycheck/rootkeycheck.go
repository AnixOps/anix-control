// Package rootkeycheck checks that a root key opens what a database holds
// sealed with it: every current and next module CA and forward link CA,
// and, in a database without a CA, its sealed ACME cache entry and node
// group join token. Control runs it at every start before anything uses
// the key (cmd/server), and the backup verification runs it against the
// snapshot of a bundle with the bundle's own root.key (internal/backup), so
// a bundle whose key and database are not a pair fails its verification.
//
// A check whose table the database does not have is skipped: a snapshot
// taken before a migration (a pre-migration backup) may lack the tables of
// later migrations. A started Control always has them.
package rootkeycheck

import (
	"context"
	"fmt"

	"github.com/AnixOps/anix-control/v5/internal/agentpki"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/modulepki"
	"github.com/AnixOps/anix-control/v5/internal/nodegroup"
	"github.com/AnixOps/anix-control/v5/internal/panelcert"
	"gorm.io/gorm"
)

// Mismatch is a stored key the root key does not open. Its message never
// contains key material.
type Mismatch struct {
	// What names the stored key: "module CA key", "forward link CA key",
	// "ACME account or certificate key", "node group join token".
	What string
	Err  error
}

func (m *Mismatch) Error() string {
	return fmt.Sprintf("the root key does not open the %s stored in this database (%v)", m.What, m.Err)
}

func (m *Mismatch) Unwrap() error { return m.Err }

// Warning is called for a stored key the root key does not open while it
// opens the CA keys: resetting a join token or requesting a certificate
// again heals it, so it does not refuse the key.
type Warning func(what string, err error)

// Check verifies kek (the root key's 32 bytes) against db. It answers a
// *Mismatch for a stored key the root key does not open (a CA row that
// cannot be read counts as one, as it always did at start), and nil
// otherwise. warn may be nil.
func Check(ctx context.Context, db *gorm.DB, kek []byte, warn Warning) error {
	if db == nil || len(kek) == 0 {
		return nil
	}
	if warn == nil {
		warn = func(string, error) {}
	}
	moduleCAs, err := ifTable(db, &model.ServiceCA{}, func() (int, error) { return modulepki.VerifyKEK(ctx, db, kek) })
	if err != nil {
		return &Mismatch{What: "module CA key", Err: err}
	}
	linkCAs, err := ifTable(db, &model.ForwardLinkCA{}, func() (int, error) { return agentpki.VerifyLinkKEK(ctx, db, kek) })
	if err != nil {
		return &Mismatch{What: "forward link CA key", Err: err}
	}
	authoritative := moduleCAs+linkCAs == 0
	others := []struct {
		what  string
		table any
		check func() (bool, error)
	}{
		{"ACME account or certificate key", &model.ACMECacheEntry{}, func() (bool, error) { return panelcert.VerifyKEK(ctx, db, kek) }},
		{"node group join token", &model.NodeGroup{}, func() (bool, error) { return nodegroup.VerifyKEK(db, kek) }},
	}
	for _, other := range others {
		if !db.WithContext(ctx).Migrator().HasTable(other.table) {
			continue
		}
		_, err := other.check()
		switch {
		case err == nil:
		case authoritative:
			return &Mismatch{What: other.what, Err: err}
		default:
			warn(other.what, err)
		}
	}
	return nil
}

// ifTable runs check when db has table's table, else answers 0.
func ifTable(db *gorm.DB, table any, check func() (int, error)) (int, error) {
	if !db.Migrator().HasTable(table) {
		return 0, nil
	}
	return check()
}
