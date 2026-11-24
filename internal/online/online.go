// Package online keeps the online users of each proxy node: the set the
// node's latest traffic report carries, user by user with the number of
// distinct IPs. A node's set is replaced as a whole, never merged, only by a
// report whose window ends no earlier than the current one's, so a replayed
// batch never brings back users who have left; and it no longer counts TTL
// after it was stored, so a node that stops reporting drops out.
package online

import (
	"sync"
	"time"
)

// TTL is how long a node's set counts after it was stored. Agents report
// every minute.
const TTL = 5 * time.Minute

// now is the clock; tests in this package replace it.
var now = time.Now

type nodeSet struct {
	window  time.Time // end of the report window the set describes
	expires time.Time
	users   map[uint]int // user id -> distinct IPs
}

var (
	mu    sync.RWMutex
	nodes = map[uint]nodeSet{}
)

// Replace makes users (user id -> IPs) the node's online set as of window,
// the end of the report window (zero: now). It answers false, changing
// nothing, when the node's current set describes a later window.
func Replace(nodeID uint, window time.Time, users map[uint][]string) bool {
	current := now()
	if window.IsZero() {
		window = current
	}
	counts := make(map[uint]int, len(users))
	for user, ips := range users {
		distinct := make(map[string]struct{}, len(ips))
		for _, ip := range ips {
			distinct[ip] = struct{}{}
		}
		if len(distinct) > 0 {
			counts[user] = len(distinct)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if old, ok := nodes[nodeID]; ok && window.Before(old.window) && current.Before(old.expires) {
		return false
	}
	// Expired sets go here, so the map holds only nodes reporting now.
	for id, set := range nodes {
		if !current.Before(set.expires) {
			delete(nodes, id)
		}
	}
	nodes[nodeID] = nodeSet{window: window, expires: current.Add(TTL), users: counts}
	return true
}

// Counts answers each user's online IPs added up over the nodes whose set
// has not expired. Users without any are left out.
func Counts() map[uint64]int {
	current := now()
	mu.RLock()
	defer mu.RUnlock()
	counts := make(map[uint64]int)
	for _, set := range nodes {
		if !current.Before(set.expires) {
			continue
		}
		for user, ips := range set.users {
			counts[uint64(user)] += ips
		}
	}
	return counts
}

// Reset drops every set (tests).
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	nodes = map[uint]nodeSet{}
}
