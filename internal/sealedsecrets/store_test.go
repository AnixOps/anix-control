package sealedsecrets

import (
	"strings"
	"sync"
	"testing"
	"time"

	configtables "github.com/AnixOps/anix-control/v4/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// clock is a settable time source.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func newClock() *clock { return &clock{now: time.Unix(1_800_000_000, 0)} }

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// fixtureRoutes are field list rows the tests add to the embedded list: a
// value field of a forward node, by path parameter and new. The routes that
// listed it (/api/v2/admin/forward/nodes*) were removed in v4.2 (F5d), and
// the sealer keeps the target kind.
const (
	fixtureNodeUpdate = "fixture.forward.nodes.id.put"
	fixtureNodeCreate = "fixture.forward.nodes.post"
)

const fixtureRoutes = `,
    {"route_id": "` + fixtureNodeUpdate + `", "target": {"kind": "forward", "path_param": "id"}, "request": [{"pointer": "/api_token", "kind": "value"}]},
    {"route_id": "` + fixtureNodeCreate + `", "target": {"kind": "forward", "new": true}, "request": [{"pointer": "/api_token", "kind": "value"}], "answer": [{"pointer": "/data/api_token", "name": "api_token"}]}
  ]
}`

// fixtureTable is the embedded field list with fixtureRoutes.
func fixtureTable(t *testing.T) *Table {
	t.Helper()
	list := strings.TrimRight(string(configtables.NodeSecretFields), " \n")
	end := strings.LastIndex(list, "]")
	require.Positive(t, end)
	table, err := ParseTable([]byte(strings.TrimRight(list[:end], " \n") + fixtureRoutes))
	require.NoError(t, err)
	return table
}

// fixture is a sealer on the embedded field list (with fixtureRoutes) and
// its own store.
type fixture struct {
	t      *testing.T
	clock  *clock
	store  *Store
	sealer *Sealer
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	table := fixtureTable(t)
	c := newClock()
	store := NewStore(c.Now)
	return &fixture{t: t, clock: c, store: store, sealer: NewSealer(table, nil, store)}
}

// seal seals one request of a route for package "pkg" generation 3.
func (f *fixture) seal(routeID, requestID, body string, params map[string]string) (Sealed, Binding) {
	f.t.Helper()
	sealed, err := f.sealer.SealRequest(RequestInput{
		PackageID: "pkg", Generation: 3, RequestID: requestID, RouteID: routeID, Deadline: f.clock.Now().Add(time.Minute),
		Body: []byte(body), PathParams: params,
	})
	require.NoError(f.t, err)
	f.t.Cleanup(func() { f.sealer.Release(sealed.Key) })
	return sealed, Binding{Key: sealed.Key, PackageID: "pkg", Generation: 3, RequestID: requestID, RouteID: routeID}
}

// handles returns the handles in a sealed body, in order.
func handles(body []byte) []string {
	var found []string
	text := string(body)
	for {
		index := strings.Index(text, Prefix)
		if index < 0 {
			return found
		}
		found = append(found, text[index:index+len(Prefix)+43])
		text = text[index+len(Prefix):]
	}
}

func forward(id uint64) Target { return Target{Kind: TargetForward, ID: id} }

// A handle resolves only for the request, route, target and field it was
// sealed for, once, and all of a resolution or nothing.
func TestResolveIsBoundToRequestRouteTargetAndField(t *testing.T) {
	f := newFixture(t)
	sealed, binding := f.seal(fixtureNodeUpdate, "req-1", `{"name":"edge","api_token":"tok-1"}`, map[string]string{"id": "7"})
	require.Equal(t, 1, sealed.Count)
	handle := handles(sealed.Body)[0]
	assert.NotContains(t, string(sealed.Body), "tok-1")
	use := Use{Handle: handle, Target: forward(7), Field: "/api_token"}

	other, otherBinding := f.seal(fixtureNodeUpdate, "req-2", `{"api_token":"tok-2"}`, map[string]string{"id": "7"})
	otherHandle := handles(other.Body)[0]

	refusals := map[string]struct {
		binding Binding
		use     Use
	}{
		"another request's handle":          {binding, Use{Handle: otherHandle, Target: forward(7), Field: "/api_token"}},
		"this handle in another request":    {otherBinding, use},
		"another request id":                {Binding{Key: binding.Key, PackageID: "pkg", Generation: 3, RequestID: "req-x", RouteID: binding.RouteID}, use},
		"another route":                     {Binding{Key: binding.Key, PackageID: "pkg", Generation: 3, RequestID: "req-1", RouteID: fixtureNodeCreate}, use},
		"another package":                   {Binding{Key: binding.Key, PackageID: "other", Generation: 3, RequestID: "req-1", RouteID: binding.RouteID}, use},
		"another generation":                {Binding{Key: binding.Key, PackageID: "pkg", Generation: 4, RequestID: "req-1", RouteID: binding.RouteID}, use},
		"another target":                    {binding, Use{Handle: handle, Target: forward(8), Field: "/api_token"}},
		"another target kind":               {binding, Use{Handle: handle, Target: Target{Kind: TargetProxy, ID: 7}, Field: "/api_token"}},
		"another field":                     {binding, Use{Handle: handle, Target: forward(7), Field: "/token"}},
		"no target":                         {binding, Use{Handle: handle, Field: "/api_token"}},
		"an unknown handle":                 {binding, Use{Handle: Prefix + strings.Repeat("A", 43), Target: forward(7), Field: "/api_token"}},
		"the same handle twice in one call": {binding, use},
	}
	for name, refusal := range refusals {
		uses := []Use{refusal.use}
		if name == "the same handle twice in one call" {
			uses = append(uses, use)
		}
		_, err := f.store.Resolve(refusal.binding, uses...)
		require.Error(t, err, name)
		assert.NotContains(t, err.Error(), handle, "errors never name a handle")
		assert.NotContains(t, err.Error(), "tok-1")
	}

	secrets, err := f.store.Resolve(binding, use)
	require.NoError(t, err, "every refusal above used nothing")
	assert.Equal(t, []Secret{{Value: "tok-1"}}, secrets)
	_, err = f.store.Resolve(binding, use)
	assert.ErrorIs(t, err, ErrRefused, "a handle is single-use")
}

func TestResolveIsAllOrNothing(t *testing.T) {
	f := newFixture(t)
	sealed, binding := f.seal("protocol.admin.nodes.id.protocols.protocol_id.put", "req-1",
		`{"settings":"{\"password\":\"p1\"}","reality_settings":{"private_key":"k1","public_key":"pub"}}`, map[string]string{"id": "1", "protocol_id": "5"})
	require.Equal(t, 2, sealed.Count)
	found := handles(sealed.Body)
	target := Target{Kind: TargetProtocol, ID: 5}
	password := Use{Handle: found[0], Target: target, Field: "/settings/password"}
	privateKey := Use{Handle: found[1], Target: target, Field: "/reality_settings/private_key"}
	_, err := f.store.Resolve(binding, password, Use{Handle: privateKey.Handle, Target: target, Field: "/reality_settings/public_key"})
	require.Error(t, err)
	secrets, err := f.store.Resolve(binding, password, privateKey)
	require.NoError(t, err, "the refused call used neither handle")
	assert.Equal(t, []Secret{{Value: "p1"}, {Value: "k1"}}, secrets)
}

// A request that creates its target binds it at the first resolution.
func TestANewTargetIsBoundByItsFirstResolution(t *testing.T) {
	f := newFixture(t)
	sealed, binding := f.seal(fixtureNodeCreate, "req-1", `{"api_token":"tok","name":"n","nested":{"secret":"s"}}`, nil)
	found := handles(sealed.Body)
	require.Len(t, found, 2)
	_, err := f.store.Resolve(binding, Use{Handle: found[0], Target: forward(0), Field: "/api_token"})
	require.Error(t, err, "a resolution names the created resource")
	_, err = f.store.Resolve(binding, Use{Handle: found[0], Target: forward(12), Field: "/api_token"})
	require.NoError(t, err)
	_, err = f.store.Resolve(binding, Use{Handle: found[1], Target: forward(13), Field: "/nested/secret"})
	require.Error(t, err, "the request created forward node 12")
	_, err = f.store.Resolve(binding, Use{Handle: found[1], Target: forward(12), Field: "/nested/secret"})
	require.NoError(t, err)
}

// A validation stores nothing: its handles never resolve.
func TestHandlesOfAValidationNeverResolve(t *testing.T) {
	f := newFixture(t)
	sealed, binding := f.seal("proxy.admin.nodes.validate_config.post", "req-1", `{"raw_config":{"private_key":"k"}}`, nil)
	for _, target := range []Target{{Kind: TargetProxy, ID: 1}, {Kind: TargetNone, ID: 1}} {
		_, err := f.store.Resolve(binding, Use{Handle: handles(sealed.Body)[0], Target: target, Field: "/raw_config/private_key"})
		require.Error(t, err, target.Kind)
	}
}

// Handles die with their request: released, or past its deadline.
func TestHandlesExpireWithTheirRequest(t *testing.T) {
	f := newFixture(t)
	sealed, binding := f.seal(fixtureNodeUpdate, "req-1", `{"api_token":"tok"}`, map[string]string{"id": "7"})
	use := Use{Handle: handles(sealed.Body)[0], Target: forward(7), Field: "/api_token"}
	f.clock.advance(time.Minute)
	_, err := f.store.Resolve(binding, use)
	require.ErrorIs(t, err, ErrRequestEnded)
	assert.True(t, f.store.Live(use.Handle), "still held until it is released or purged")

	f.seal(fixtureNodeUpdate, "req-2", `{}`, map[string]string{"id": "7"})
	assert.False(t, f.store.Live(use.Handle), "a later request purges the expired ones")

	sealed, binding = f.seal(fixtureNodeUpdate, "req-3", `{"api_token":"tok"}`, map[string]string{"id": "7"})
	use = Use{Handle: handles(sealed.Body)[0], Target: forward(7), Field: "/api_token"}
	f.sealer.Release(sealed.Key)
	assert.False(t, f.store.Live(use.Handle))
	_, err = f.store.Resolve(binding, use)
	require.ErrorIs(t, err, ErrRequestEnded)
	f.sealer.Release(sealed.Key)
}

// The kernel mints answer handles only for a name the route's answer shows,
// and a minted handle never resolves as a request secret.
func TestMintIsBoundToTheRoutesAnswer(t *testing.T) {
	f := newFixture(t)
	_, binding := f.seal("proxy.admin.nodes.post", "req-1", `{"name":"edge"}`, nil)
	handle, expires, err := f.store.Mint(binding, "api_key", "generated")
	require.NoError(t, err)
	require.True(t, IsHandle(handle))
	assert.Equal(t, f.clock.Now().Add(time.Minute), expires)
	_, _, err = f.store.Mint(binding, "token", "generated")
	require.Error(t, err, "the route shows no token")
	_, _, err = f.store.Mint(binding, "api_key", "")
	require.Error(t, err)
	_, _, err = f.store.Mint(Binding{Key: binding.Key, PackageID: "pkg", Generation: 3, RequestID: "other", RouteID: binding.RouteID}, "api_key", "x")
	require.Error(t, err)
	for _, field := range []string{"", "/data/api_key", "api_key"} {
		_, err = f.store.Resolve(binding, Use{Handle: handle, Target: Target{Kind: TargetProxy, ID: 9}, Field: field})
		require.Error(t, err, "an answer handle is not a request secret")
	}

	_, unlisted := f.seal("knowledge.article.list", "req-2", `{}`, nil)
	_, _, err = f.store.Mint(unlisted, "api_key", "x")
	require.Error(t, err, "an unlisted route has no sealed request")
}
