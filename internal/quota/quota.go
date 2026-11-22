// Package quota is the check before a new user or node is created.
//
// The community edition sets no check: every creation is allowed and there is
// no limit (LICENSING.md, commitment 1; TestNoPublicCodeSetsTheCheck). The
// commercial edition sets Check to its license's limits. A check can only
// refuse a creation; it never touches existing users or nodes (commitment 4).
package quota

import (
	"errors"

	"gorm.io/gorm"
)

// Kind is what a check limits.
type Kind string

const (
	// Users are subscriber and administrator accounts.
	Users Kind = "users"
	// Nodes are proxy nodes and forward nodes.
	Nodes Kind = "nodes"
)

// ErrExceeded is wrapped by every refusal of a check.
var ErrExceeded = errors.New("quota exceeded")

// Check, when set, runs inside the creating transaction before the new row is
// written; a refusal wraps ErrExceeded. Set it once at startup.
var Check func(tx *gorm.DB, kind Kind) error

// Allow reports whether one more object of kind may be created.
func Allow(tx *gorm.DB, kind Kind) error {
	if Check == nil {
		return nil
	}
	return Check(tx, kind)
}
