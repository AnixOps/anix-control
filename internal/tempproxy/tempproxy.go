// Package tempproxy keeps the SOCKS5 proxies a proxy node serves for a
// limited time on an internal address: an administrator opens one (for
// example for 15 minutes) so that devices of the node's internal network,
// such as a mesh's virtual addresses, send traffic out through the node.
//
// A proxy is never public and never permanent. Its listen address must be
// internal (loopback, RFC 1918, 100.64.0.0/10, link-local or IPv6 unique
// local; never a wildcard), Control generates its username and password,
// and it ends at ExpiresAt. The node's desired configuration carries the
// open proxies in the member "temporary_proxies" (NodeMember); the Agent
// runs each as a kernel inbound that the kernel itself closes at the end
// time, so a proxy ends on time even when Control or the Agent is down.
// Opening or closing one calls the OnChange listeners, which push the
// node's configuration at once; an expired proxy leaves the document at the
// next configuration refresh.
package tempproxy

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// Member is the desired configuration member of the open proxies.
const Member = "temporary_proxies"

// Limits of a proxy.
const (
	// MinDuration and MaxDuration bound how long a proxy stays open.
	MinDuration = time.Minute
	MaxDuration = 24 * time.Hour
	// DefaultDuration is the duration of a request that names none.
	DefaultDuration = 15 * time.Minute
	// MaxOpenPerNode bounds the proxies open on one node at once.
	MaxOpenPerNode = 8
	// passwordLength is the length of a generated password (base62, about
	// 143 bits); the kernel requires at least 16 bytes.
	passwordLength = 24
)

var (
	// ErrNotFound: no open proxy with that id on the node.
	ErrNotFound = errors.New("temporary proxy not found")
	// ErrInvalid: a field is invalid; the error says which.
	ErrInvalid = errors.New("invalid temporary proxy")
	// ErrPortTaken: the node already listens on the port.
	ErrPortTaken = errors.New("the node already uses this port")
	// ErrTooMany: the node has MaxOpenPerNode open proxies.
	ErrTooMany = errors.New("too many open temporary proxies on the node")
)

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

// Request opens a proxy.
type Request struct {
	Listen   string
	Port     int
	Duration time.Duration
	// CreatedBy is the administrator; nil for none.
	CreatedBy *uint
}

// Audit records an opened or closed proxy in the transaction that changes
// it; an error rolls the change back. nil records nothing.
type Audit func(tx *gorm.DB, row model.NodeTemporaryProxy) error

// Opened is a proxy just opened, with its password, which no later answer
// carries.
type Opened struct {
	model.NodeTemporaryProxy
	Password string `json:"password"`
}

var internalPrefixes = []netip.Prefix{
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("::1/128"),
}

// IsInternal tells whether addr is an address a proxy may listen on.
func IsInternal(addr netip.Addr) bool {
	addr = addr.Unmap()
	for _, prefix := range internalPrefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

// normalizeListen answers the listen address in its canonical form.
func normalizeListen(listen string) (string, error) {
	addr, err := netip.ParseAddr(listen)
	if err != nil || addr.Zone() != "" {
		return "", invalid("listen: an IP address without a zone, such as the node's mesh address")
	}
	addr = addr.Unmap()
	if !IsInternal(addr) {
		return "", invalid("listen: an internal address (10/8, 172.16/12, 192.168/16, 100.64/10, link-local, loopback or fc00::/7); never a wildcard or public address")
	}
	return addr.String(), nil
}

// Open opens a proxy on nodeID, which must be a proxy node, at now. The
// answer carries the generated password.
func Open(ctx context.Context, db *gorm.DB, nodeID uint, request Request, now time.Time, audit Audit) (Opened, error) {
	listen, err := normalizeListen(request.Listen)
	if err != nil {
		return Opened{}, err
	}
	if request.Port < 1 || request.Port > 65535 {
		return Opened{}, invalid("port: 1 to 65535")
	}
	duration := request.Duration
	if duration == 0 {
		duration = DefaultDuration
	}
	if duration < MinDuration || duration > MaxDuration {
		return Opened{}, invalid("duration: 1 minute to 24 hours")
	}
	password, err := randomText(passwordLength)
	if err != nil {
		return Opened{}, err
	}
	suffix, err := randomText(8)
	if err != nil {
		return Opened{}, err
	}
	row := model.NodeTemporaryProxy{
		NodeID: nodeID, Listen: listen, Port: request.Port, Username: "tmp-" + suffix, Password: password,
		ExpiresAt: now.Add(duration).UTC().Truncate(time.Millisecond), CreatedBy: request.CreatedBy, CreatedAt: now.UTC(),
	}
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		open, err := openRows(tx, nodeID, now)
		if err != nil {
			return err
		}
		if len(open) >= MaxOpenPerNode {
			return fmt.Errorf("%w (%d)", ErrTooMany, MaxOpenPerNode)
		}
		for _, other := range open {
			if other.Port == row.Port {
				return fmt.Errorf("%w: another temporary proxy listens on %d", ErrPortTaken, row.Port)
			}
		}
		var protocols int64
		if err := tx.Model(&model.NodeProtocol{}).Where("node_id = ? AND port = ? AND enable = 1", nodeID, row.Port).
			Count(&protocols).Error; err != nil {
			return err
		}
		if protocols > 0 {
			return fmt.Errorf("%w: a protocol of the node listens on %d", ErrPortTaken, row.Port)
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		if audit != nil {
			return audit(tx, row)
		}
		return nil
	})
	if err != nil {
		return Opened{}, err
	}
	notify(nodeID)
	return Opened{NodeTemporaryProxy: row, Password: password}, nil
}

// List answers the node's open proxies at now, the soonest to end first.
func List(ctx context.Context, db *gorm.DB, nodeID uint, now time.Time) ([]model.NodeTemporaryProxy, error) {
	return openRows(db.WithContext(ctx), nodeID, now)
}

// Close closes the node's open proxy id at now.
func Close(ctx context.Context, db *gorm.DB, nodeID, id uint, now time.Time, audit Audit) (model.NodeTemporaryProxy, error) {
	var row model.NodeTemporaryProxy
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []model.NodeTemporaryProxy
		if err := tx.Where("id = ? AND node_id = ? AND closed_at IS NULL AND expires_at > ?", id, nodeID, now.UTC()).
			Limit(1).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return ErrNotFound
		}
		row = rows[0]
		closed := now.UTC()
		row.ClosedAt = &closed
		if err := tx.Model(&model.NodeTemporaryProxy{}).Where("id = ?", row.ID).Update("closed_at", closed).Error; err != nil {
			return err
		}
		if audit != nil {
			return audit(tx, row)
		}
		return nil
	})
	if err != nil {
		return model.NodeTemporaryProxy{}, err
	}
	notify(nodeID)
	return row, nil
}

// openRows are the node's proxies neither closed nor expired at now.
func openRows(db *gorm.DB, nodeID uint, now time.Time) ([]model.NodeTemporaryProxy, error) {
	rows := []model.NodeTemporaryProxy{}
	err := db.Where("node_id = ? AND closed_at IS NULL AND expires_at > ?", nodeID, now.UTC()).
		Order("expires_at ASC, id ASC").Find(&rows).Error
	return rows, err
}

// NodeMember is the member "temporary_proxies" of the node's desired
// configuration at now: its open proxies by id, nil when it has none (so
// other nodes keep their configuration hash) or when the database has no
// table for them yet.
func NodeMember(db *gorm.DB, nodeID uint, now time.Time) ([]map[string]any, error) {
	if !db.Migrator().HasTable(&model.NodeTemporaryProxy{}) {
		return nil, nil
	}
	rows, err := openRows(db, nodeID, now)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	member := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		member = append(member, map[string]any{
			"id": row.ID, "listen": row.Listen, "port": row.Port, "username": row.Username,
			"password": row.Password, "expires_at_unix_ms": row.ExpiresAt.UnixMilli(),
		})
	}
	return member, nil
}

const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

// randomText is n characters of alphabet from the system's random source,
// without modulo bias.
func randomText(n int) (string, error) {
	out := make([]byte, 0, n)
	buf := make([]byte, 64)
	for len(out) < n {
		if _, err := rand.Read(buf); err != nil {
			return "", fmt.Errorf("generate credentials: %w", err)
		}
		for _, b := range buf {
			// 248 = 4 * 62: larger values would favour the first letters.
			if b < 248 && len(out) < n {
				out = append(out, alphabet[int(b)%len(alphabet)])
			}
		}
	}
	return string(out), nil
}

// Listener is told the node whose open proxies changed.
type Listener func(nodeID uint)

var (
	listenersMu sync.Mutex
	listeners   = map[int]Listener{}
	nextID      int
)

// OnChange registers listener for every opened or closed proxy; cancel
// removes it.
func OnChange(listener Listener) (cancel func()) {
	listenersMu.Lock()
	defer listenersMu.Unlock()
	nextID++
	id := nextID
	listeners[id] = listener
	return func() {
		listenersMu.Lock()
		delete(listeners, id)
		listenersMu.Unlock()
	}
}

func notify(nodeID uint) {
	listenersMu.Lock()
	ids := make([]int, 0, len(listeners))
	for id := range listeners {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	called := make([]Listener, 0, len(ids))
	for _, id := range ids {
		called = append(called, listeners[id])
	}
	listenersMu.Unlock()
	for _, listener := range called {
		listener(nodeID)
	}
}
