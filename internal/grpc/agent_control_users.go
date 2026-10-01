package grpc

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	agentcontrol "github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/AnixOps/anix-control/v4/internal/subscriber"
	"gorm.io/gorm"
)

// User deltas on the Agent Control stream (users.v1; PROTOCOL.md, "Data
// plane"). Control follows v4_kernel_subscriber_change, the subscriber change
// log, and sends an Agent the changes of the users its node serves after the
// Agent's cursor. The set is the one the legacy user pulls give the node
// (service.ActiveUsersForNodeQuery), so a node gets the same users on either
// transport. Only proxy nodes serve users; forward nodes are not offered the
// capability.

// Bounds of the user deltas. Variables so tests can lower them.
var (
	// userDeltaPageSize is the most users in one page of a full resync.
	userDeltaPageSize = 500
	// userDeltaBatch is the most change log rows one delta covers.
	userDeltaBatch = 500
	// userDeltaPoll is how often the change log is read for new rows.
	userDeltaPoll = time.Second
)

// usersStart is how a session starts from the Agent's cursor
// (decideUsersStart): with the deltas since it, or with a full resync for one
// of three reasons.
type usersStart int

const (
	// usersResume: the cursor is in the log; the changes after it follow.
	usersResume usersStart = iota
	// usersResyncNoCursor: the cursor is 0; the Agent has no user set.
	usersResyncNoCursor
	// usersResyncStale: the log no longer has every row after the cursor
	// (pruned after subscriber.ChangeRetention).
	usersResyncStale
	// usersResyncUnknown: the cursor is ahead of the log, so it came from
	// another log (a restored database); nothing can be resumed.
	usersResyncUnknown
	// usersResyncPruned: rows after a session's cursor were pruned while it
	// was connected.
	usersResyncPruned
)

// resyncReasons label anixops_agent_user_resyncs_total.
var resyncReasons = map[usersStart]string{
	usersResyncNoCursor: "no_cursor", usersResyncStale: "stale", usersResyncUnknown: "unknown", usersResyncPruned: "pruned",
}

// decideUsersStart chooses how a session starts from the Agent's cursor.
// oldest and latest are the first and last change ids in the log, both 0
// when it is empty. The Agent resumes only when the log still holds every
// change after its cursor: the cursor is at most the latest change, and no
// row between the cursor and the oldest remaining row is missing.
func decideUsersStart(cursor, oldest, latest uint64) usersStart {
	switch {
	case cursor == 0:
		return usersResyncNoCursor
	case cursor > latest:
		return usersResyncUnknown
	case oldest > cursor+1:
		return usersResyncStale
	default:
		return usersResume
	}
}

// changeLogBounds reads the oldest and latest change ids, 0 when the log is
// empty.
func changeLogBounds(db *gorm.DB) (oldest, latest uint64, err error) {
	if err := db.Model(&model.SubscriberChange{}).Select("COALESCE(MIN(id), 0)").Scan(&oldest).Error; err != nil {
		return 0, 0, err
	}
	latest, err = subscriber.Cursor(db)
	return oldest, latest, err
}

// servesUserDeltas reports whether this Control offers users.v1 to node:
// proxy nodes only, since user lists belong to v2_node.
func (s *AgentControlGRPCServer) servesUserDeltas(node agentcontrol.AgentNode) bool {
	return node.Kind == agentcontrol.NodeKindProxy
}

// startUserDeltas serves a session's user deltas in the background from the
// Agent's cursor. The returned function stops the sender and waits for it.
func (s *AgentControlGRPCServer) startUserDeltas(ctx context.Context, connection *AgentControlConnection, node agentcontrol.AgentNode, cursor uint64) (stop func()) {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.serveUserDeltas(ctx, connection, node, cursor)
	}()
	return func() {
		cancel()
		<-done
	}
}

// userDeltaSender builds and sends one session's deltas.
type userDeltaSender struct {
	connection  *AgentControlConnection
	node        agentcontrol.AgentNode
	nodeService *service.NodeService
	db          *gorm.DB
}

// serveUserDeltas sends a session's user deltas until ctx ends: on connect,
// the deltas since the Agent's cursor or a full resync; then a delta for
// each batch of change log rows as the log advances. A database error is
// retried after userDeltaPoll; a send error ends the sender, as the stream
// is gone.
func (s *AgentControlGRPCServer) serveUserDeltas(ctx context.Context, connection *AgentControlConnection, node agentcontrol.AgentNode, cursor uint64) {
	feed := agentUserFeed
	feed.attach(connection, cursor)
	defer feed.detach(connection)
	sender := &userDeltaSender{connection: connection, node: node, nodeService: s.nodeService, db: databaseForAgentChecks()}
	start := usersResume
	started := false
	for {
		if ctx.Err() != nil {
			return
		}
		var err error
		switch {
		case !started:
			start, err = sender.decide(cursor)
			started = err == nil
		case start != usersResume:
			cursor, err = sender.resync(ctx, start)
			if err == nil {
				start = usersResume
				feed.observe(connection, cursor)
			}
		default:
			var advanced, pruned bool
			cursor, advanced, pruned, err = sender.step(ctx, cursor)
			switch {
			case err != nil:
			case pruned:
				start = usersResyncPruned
			case advanced:
				feed.observe(connection, cursor)
			default:
				if err := feed.wait(ctx, cursor); err != nil {
					return
				}
			}
		}
		if err == nil {
			continue
		}
		if ctx.Err() != nil || errors.Is(err, errUserDeltaSend) {
			return
		}
		slog.Warn("agent user deltas: database error, retrying", "component", "agent-control", "node", node.String(), "error", err)
		if err := feed.read(ctx); err != nil {
			return
		}
	}
}

// errUserDeltaSend wraps a failed stream send.
var errUserDeltaSend = errors.New("send user delta")

// decide reads the log's bounds and chooses the session's start.
func (d *userDeltaSender) decide(cursor uint64) (usersStart, error) {
	oldest, latest, err := changeLogBounds(d.db)
	if err != nil {
		return usersResume, err
	}
	return decideUsersStart(cursor, oldest, latest), nil
}

// groupID reads the node's plan group, the one its user list follows.
func (d *userDeltaSender) groupID() (*uint, error) {
	var node model.Node
	if err := d.db.Select("id", "group_id").Where("id = ?", d.node.ID).Take(&node).Error; err != nil {
		return nil, err
	}
	return node.GroupID, nil
}

// resync sends the node's whole user set in pages of userDeltaPageSize, each
// with full and the cursor read before the listing (a change made during it
// is seen again as a delta, never missed), and returns that cursor.
func (d *userDeltaSender) resync(ctx context.Context, reason usersStart) (uint64, error) {
	latest, err := subscriber.Cursor(d.db)
	if err != nil {
		return 0, err
	}
	groupID, err := d.groupID()
	if err != nil {
		return 0, err
	}
	agentUserMetrics.resync(reason)
	afterID := uint(0)
	for {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		var page []*model.User
		if err := service.ActiveUsersForNodeQuery(d.db, groupID, time.Now()).
			Where("id > ?", afterID).Order("id").Limit(userDeltaPageSize).Find(&page).Error; err != nil {
			return 0, err
		}
		upserts, err := d.nodeUsers(page)
		if err != nil {
			return 0, err
		}
		last := len(page) < userDeltaPageSize
		if err := d.send(&agentv1pb.UserDelta{Cursor: latest, Full: true, LastPage: last, Upserts: upserts}); err != nil {
			return 0, err
		}
		agentUserMetrics.fullPages.Add(1)
		if last {
			return latest, nil
		}
		afterID = page[len(page)-1].ID
	}
}

// step reads the next batch of changes after cursor and sends them as one
// delta. It returns the new cursor and whether it advanced; pruned is true
// when the log no longer has every row after cursor, which calls for a
// resync.
func (d *userDeltaSender) step(ctx context.Context, cursor uint64) (next uint64, advanced, pruned bool, err error) {
	changes, resync, err := subscriber.ChangesAfter(d.db, cursor, userDeltaBatch)
	if err != nil {
		return cursor, false, false, err
	}
	if resync {
		return cursor, false, true, nil
	}
	if len(changes) == 0 {
		return cursor, false, false, nil
	}
	groupID, err := d.groupID()
	if err != nil {
		return cursor, false, false, err
	}
	delta, err := d.delta(groupID, changes)
	if err != nil {
		return cursor, false, false, err
	}
	if err := ctx.Err(); err != nil {
		return cursor, false, false, err
	}
	if err := d.send(delta); err != nil {
		return cursor, false, false, err
	}
	agentUserMetrics.deltas.Add(1)
	return delta.Cursor, true, false, nil
}

// delta resolves a batch of changes to the users' current state on this
// node: an upsert for each the node serves now, a removal for the rest (a
// removal may name a user the Agent never had, which it ignores).
func (d *userDeltaSender) delta(groupID *uint, changes []model.SubscriberChange) (*agentv1pb.UserDelta, error) {
	ids := make([]uint, 0, len(changes))
	seen := make(map[uint]struct{}, len(changes))
	for _, change := range changes {
		if _, ok := seen[change.UserID]; ok {
			continue
		}
		seen[change.UserID] = struct{}{}
		ids = append(ids, change.UserID)
	}
	// The users' current state on this node, by the query of the legacy
	// user pulls.
	var active []*model.User
	if err := service.ActiveUsersForNodeQuery(d.db, groupID, time.Now()).
		Where("id IN ?", ids).Order("id").Find(&active).Error; err != nil {
		return nil, err
	}
	upserts, err := d.nodeUsers(active)
	if err != nil {
		return nil, err
	}
	served := make(map[uint64]struct{}, len(upserts))
	for _, user := range upserts {
		served[user.UserId] = struct{}{}
	}
	removed := make([]uint64, 0, len(ids)-len(upserts))
	for _, id := range ids {
		if _, ok := served[uint64(id)]; !ok {
			removed = append(removed, uint64(id))
		}
	}
	return &agentv1pb.UserDelta{
		Cursor: changes[len(changes)-1].ID, LastPage: true, Upserts: upserts, RemovedUserIds: removed,
	}, nil
}

// nodeUsers converts the users the node serves to the wire form, with the
// WireGuard peer fields in extra_json when the node's protocol is WireGuard.
// A WireGuard exit protocol serves no users, as in the legacy pulls.
func (d *userDeltaSender) nodeUsers(users []*model.User) ([]*agentv1pb.NodeUser, error) {
	extras, exit, err := wireGuardUserExtras(d.nodeService, uint(d.node.ID), "", users)
	if err != nil {
		return nil, err
	}
	if exit {
		return nil, nil
	}
	nodeUsers := make([]*agentv1pb.NodeUser, 0, len(users))
	for _, user := range users {
		nodeUser, err := nodeUserFromModel(user, extras[user.ID])
		if err != nil {
			return nil, err
		}
		nodeUsers = append(nodeUsers, nodeUser)
	}
	return nodeUsers, nil
}

func (d *userDeltaSender) send(delta *agentv1pb.UserDelta) error {
	if err := d.connection.send(&agentv1pb.ControlToAgent{
		RequestId:    newAgentControlID("users"),
		NodeId:       d.node.ID,
		SentAtUnixMs: time.Now().UnixMilli(),
		Payload:      &agentv1pb.ControlToAgent_Users{Users: delta},
	}); err != nil {
		return errors.Join(errUserDeltaSend, err)
	}
	return nil
}

// nodeUserFromModel carries only what the node needs: the id, the proxy
// uuid, the limits (0 for none, as the legacy user pulls send them) and the
// protocol-specific extras. Never the e-mail, the password hash or the
// subscription token.
func nodeUserFromModel(user *model.User, extra map[string]string) (*agentv1pb.NodeUser, error) {
	if user == nil {
		return nil, errors.New("user is nil")
	}
	deviceLimit, err := intToInt32("device limit", user.GetDeviceLimit())
	if err != nil {
		return nil, err
	}
	nodeUser := &agentv1pb.NodeUser{
		UserId: uint64(user.ID), Uuid: user.UUID, SpeedLimitMbps: user.GetSpeedLimit(), DeviceLimit: deviceLimit,
	}
	if len(extra) > 0 {
		encoded, err := json.Marshal(extra)
		if err != nil {
			return nil, err
		}
		nodeUser.ExtraJson = encoded
	}
	return nodeUser, nil
}

// userChangeFeed tells sessions when the change log advances. The waiting
// sessions take turns reading the log's cursor, one read per userDeltaPoll
// however many wait (a token elects the reader), so the cost does not grow
// with the sessions, and no goroutine outlives a stream. A session is woken
// only by a read that shows a change after its cursor, never by a cached
// value: the log can be behind a cached cursor after a database restore, and
// a session must not spin on it. The feed also keeps each session's cursor
// for the lag gauge.
type userChangeFeed struct {
	mu   sync.Mutex
	poll time.Duration
	// cursor is the latest change the last read saw.
	cursor uint64
	// waiters maps each waiting session's channel to the cursor it waits
	// past.
	waiters  map[chan struct{}]uint64
	sessions map[*AgentControlConnection]uint64
	// token is held by the session reading the log; it has one slot.
	token chan struct{}
}

// agentUserFeed is shared by every server in the process, like the managers.
var agentUserFeed = newUserChangeFeed()

func newUserChangeFeed() *userChangeFeed {
	feed := &userChangeFeed{
		poll: userDeltaPoll, waiters: make(map[chan struct{}]uint64), sessions: make(map[*AgentControlConnection]uint64),
		token: make(chan struct{}, 1),
	}
	feed.token <- struct{}{}
	return feed
}

// setPoll sets how often the log is read, for tests.
func (f *userChangeFeed) setPoll(poll time.Duration) {
	f.mu.Lock()
	f.poll = poll
	f.mu.Unlock()
}

func (f *userChangeFeed) attach(connection *AgentControlConnection, cursor uint64) {
	f.mu.Lock()
	f.sessions[connection] = cursor
	f.mu.Unlock()
}

func (f *userChangeFeed) observe(connection *AgentControlConnection, cursor uint64) {
	f.mu.Lock()
	if _, ok := f.sessions[connection]; ok {
		f.sessions[connection] = cursor
	}
	f.mu.Unlock()
}

func (f *userChangeFeed) detach(connection *AgentControlConnection) {
	f.mu.Lock()
	delete(f.sessions, connection)
	f.mu.Unlock()
}

// wait returns when a read of the log shows a change after cursor, or ctx
// ends.
func (f *userChangeFeed) wait(ctx context.Context, cursor uint64) error {
	waiter := make(chan struct{})
	f.mu.Lock()
	f.waiters[waiter] = cursor
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		delete(f.waiters, waiter)
		f.mu.Unlock()
	}()
	for {
		select {
		case <-waiter:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		case <-f.token:
			err := f.read(ctx)
			f.token <- struct{}{}
			if err != nil {
				return err
			}
			select {
			case <-waiter:
				return nil
			default:
			}
		}
	}
}

// read waits one poll interval, reads the log's cursor and wakes the
// waiters it is past. A read that fails is tried again by the next turn.
func (f *userChangeFeed) read(ctx context.Context) error {
	f.mu.Lock()
	poll := f.poll
	f.mu.Unlock()
	timer := time.NewTimer(poll)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
	}
	latest, err := subscriber.Cursor(databaseForAgentChecks())
	if err != nil {
		return nil
	}
	f.mu.Lock()
	f.cursor = latest
	for waiter, cursor := range f.waiters {
		if latest > cursor {
			close(waiter)
			delete(f.waiters, waiter)
		}
	}
	f.mu.Unlock()
	return nil
}

// lag returns the sessions served and the most changes any of them has not
// received, by latest.
func (f *userChangeFeed) lag(latest uint64) (sessions int, lag uint64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, cursor := range f.sessions {
		if latest > cursor && latest-cursor > lag {
			lag = latest - cursor
		}
	}
	return len(f.sessions), lag
}

// userDeltaMetrics counts what the user deltas sent.
type userDeltaMetrics struct {
	deltas    atomic.Uint64
	fullPages atomic.Uint64
	resyncs   map[usersStart]*atomic.Uint64
}

var agentUserMetrics = newUserDeltaMetrics()

func newUserDeltaMetrics() *userDeltaMetrics {
	metrics := &userDeltaMetrics{resyncs: make(map[usersStart]*atomic.Uint64, len(resyncReasons))}
	for reason := range resyncReasons {
		metrics.resyncs[reason] = &atomic.Uint64{}
	}
	return metrics
}

func (m *userDeltaMetrics) resync(reason usersStart) {
	if counter := m.resyncs[reason]; counter != nil {
		counter.Add(1)
	}
}

// WriteAgentUsersPrometheus renders the user delta counters and gauges of
// the Agent Control stream in the Prometheus text format.
func WriteAgentUsersPrometheus(body *strings.Builder) {
	body.WriteString("# HELP anixops_agent_user_deltas_sent_total UserDelta messages sent on the Agent Control stream: changes (delta) and pages of a full resync (full).\n")
	body.WriteString("# TYPE anixops_agent_user_deltas_sent_total counter\n")
	body.WriteString("anixops_agent_user_deltas_sent_total{kind=\"delta\"} " + strconv.FormatUint(agentUserMetrics.deltas.Load(), 10) + "\n")
	body.WriteString("anixops_agent_user_deltas_sent_total{kind=\"full\"} " + strconv.FormatUint(agentUserMetrics.fullPages.Load(), 10) + "\n")
	body.WriteString("# HELP anixops_agent_user_resyncs_total Full user resyncs sent, by reason: no_cursor, stale, unknown or pruned.\n")
	body.WriteString("# TYPE anixops_agent_user_resyncs_total counter\n")
	for _, reason := range []usersStart{usersResyncNoCursor, usersResyncStale, usersResyncUnknown, usersResyncPruned} {
		body.WriteString("anixops_agent_user_resyncs_total{reason=\"" + resyncReasons[reason] + "\"} " + strconv.FormatUint(agentUserMetrics.resyncs[reason].Load(), 10) + "\n")
	}
	// The lag is by the log as it is now; the poller's last read stands in
	// when the database cannot be read.
	latest, err := subscriber.Cursor(databaseForAgentChecks())
	if err != nil {
		agentUserFeed.mu.Lock()
		latest = agentUserFeed.cursor
		agentUserFeed.mu.Unlock()
	}
	sessions, lag := agentUserFeed.lag(latest)
	body.WriteString("# HELP anixops_agent_users_sessions Agent sessions receiving user deltas.\n")
	body.WriteString("# TYPE anixops_agent_users_sessions gauge\n")
	body.WriteString("anixops_agent_users_sessions " + strconv.Itoa(sessions) + "\n")
	body.WriteString("# HELP anixops_agent_users_cursor_lag Changes of the subscriber change log the slowest Agent session has not received.\n")
	body.WriteString("# TYPE anixops_agent_users_cursor_lag gauge\n")
	body.WriteString("anixops_agent_users_cursor_lag " + strconv.FormatUint(lag, 10) + "\n")
}
