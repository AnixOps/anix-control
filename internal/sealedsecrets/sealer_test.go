package sealedsecrets

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// secretKeys are keys IsNodeSecretKey marks, in the spellings protocols and
// raw configurations use.
var secretKeys = []string{
	"private_key", "privateKey", "server_private_key", "preshared_key", "server_key", "psk", "password",
	"obfs-password", "auth", "auth_str", "seed", "pass", "passwd", "token", "access_token", "secret",
	"client_secret", "credential", "credentials", "api_key", "x25519_private_keys", "PrivateKey", "uuid_secret",
}

func TestTheSecretKeyCorpusIsWhatIsNodeSecretKeyMarks(t *testing.T) {
	for _, key := range secretKeys {
		assert.True(t, service.IsNodeSecretKey(key), key)
	}
	for _, key := range []string{"public_key", "short_id", "key_file", "server_name", "port"} {
		assert.False(t, service.IsNodeSecretKey(key), key)
	}
}

// sampleDocument holds every secret key at the top and nested in objects
// and arrays, and public values beside them.
func sampleDocument(prefix string) (map[string]any, []string) {
	document := map[string]any{"public_key": "pub-" + prefix, "short_id": "ab", "server_port": 443}
	nested := map[string]any{}
	var values []string
	for index, key := range secretKeys {
		top := fmt.Sprintf("%s-top-%d", prefix, index)
		deep := fmt.Sprintf("%s-deep-%d", prefix, index)
		document[key] = top
		nested[key] = deep
		values = append(values, top, deep)
	}
	inArray := fmt.Sprintf("%s-array-element", prefix)
	listed := fmt.Sprintf("%s-listed", prefix)
	document["peers"] = []any{map[string]any{"preshared_key": inArray, "endpoint": "1.2.3.4"}}
	document["reality"] = map[string]any{"inner": nested, "private_keys": []any{listed, listed + "-2"}}
	values = append(values, inArray, listed, listed+"-2")
	return document, values
}

// requestFor builds a body for a listed route: every listed field holds a
// secret (a document field in both its forms, across the route's fields),
// and unlisted members hold secrets too.
func requestFor(t *testing.T, route *Route) ([]byte, []string) {
	t.Helper()
	body := map[string]any{"name": "visible", "port": 443}
	var values []string
	for index, field := range route.Request {
		target := body
		for _, token := range field.tokens[:len(field.tokens)-1] {
			next := map[string]any{}
			target[token] = next
			target = next
		}
		name := field.tokens[len(field.tokens)-1]
		switch field.Kind {
		case FieldValue:
			value := fmt.Sprintf("typed-%s-%d", route.ID, index)
			target[name] = value
			values = append(values, value)
		case FieldDocument:
			document, documentValues := sampleDocument(fmt.Sprintf("%s-%d", route.ID, index))
			values = append(values, documentValues...)
			if index%2 == 0 {
				encoded, err := json.Marshal(document)
				require.NoError(t, err)
				target[name] = string(encoded)
			} else {
				target[name] = document
			}
		}
	}
	body["unlisted"] = map[string]any{"secret": "unlisted-secret", "list": []any{map[string]any{"password": "unlisted-password"}}}
	values = append(values, "unlisted-secret", "unlisted-password")
	encoded, err := json.Marshal(body)
	require.NoError(t, err)
	return encoded, values
}

// Every listed route and field is substituted: no secret of the walk, at
// any listed field, in any document form or depth, or under a secret key
// elsewhere in the body, reaches the sealed body, and every handle resolves
// to its secret.
func TestEveryListedRouteAndFieldIsSubstituted(t *testing.T) {
	f := newFixture(t)
	ids := f.sealer.Table().RouteIDs()
	sort.Strings(ids)
	requestRoutes := 0
	for _, id := range ids {
		route, _ := f.sealer.Table().Route(id)
		if len(route.Request) == 0 {
			continue
		}
		requestRoutes++
		body, values := requestFor(t, route)
		sealed, _ := f.seal(id, "req-"+id, string(body), map[string]string{"id": "7", "protocol_id": "9"})
		for _, value := range values {
			assert.NotContains(t, string(sealed.Body), value, "%s: a secret reached the package", id)
		}
		assert.Contains(t, string(sealed.Body), "visible", id)
		if route.Request[0].Kind == FieldDocument {
			assert.Contains(t, string(sealed.Body), "pub-"+id, "%s: public values stay", id)
			assert.Contains(t, string(sealed.Body), "short_id", id)
		}
		require.True(t, json.Valid(sealed.Body), id)

		// Every handle resolves to one of the secrets, once.
		f.store.mu.Lock()
		request := f.store.requests[sealed.Key]
		resolved := map[string]bool{}
		for _, handle := range request.handles {
			item := f.store.handles[handle]
			value := item.secret.Value
			if item.secret.JSON {
				var list []string
				require.NoError(t, json.Unmarshal([]byte(value), &list))
				for _, element := range list {
					resolved[element] = true
				}
				continue
			}
			resolved[value] = true
		}
		f.store.mu.Unlock()
		for _, value := range values {
			assert.True(t, resolved[value], "%s: %s was not sealed whole", id, value)
		}
		assert.Equal(t, len(handles(sealed.Body)), sealed.Count, id)
	}
	assert.Equal(t, 8, requestRoutes, "the request routes of node-ops-service.md section 6")
}

// The placeholder keeps the stored secret, and empty values carry none:
// both pass unchanged.
func TestThePlaceholderAndEmptyValuesPassUnchanged(t *testing.T) {
	f := newFixture(t)
	bodies := map[string]string{
		"forward.admin.forward.nodes.id.put":                `{"name":"n","api_token":"********"}`,
		"forward.admin.forward.nodes.post":                  `{"name":"n","api_token":"","x":null}`,
		"proxy.admin.nodes.id.raw_config.put":               `{"raw_config":{"private_key":"********","preshared_key":"","peers":[],"secret":null}}`,
		"proxy.admin.nodes.id.put":                          `{"raw_config":"********","name":"n"}`,
		"proxy.admin.nodes.post":                            `{"raw_config":"","name":"n"}`,
		"protocol.admin.nodes.id.protocols.protocol_id.put": `{"settings":"{\"password\":\"********\",\"psk\":\"\"}","reality_settings":null,"tls_settings":"  "}`,
	}
	for id, body := range bodies {
		sealed, _ := f.seal(id, "req-"+id, body, map[string]string{"id": "7", "protocol_id": "9"})
		assert.Equal(t, body, string(sealed.Body), id)
		assert.Zero(t, sealed.Count, id)
	}
}

// Substitution rewrites only the secrets: every other byte stays as sent.
func TestSubstitutionKeepsEveryOtherByte(t *testing.T) {
	f := newFixture(t)
	body := "{ \"z\" : 1,\n  \"api_token\":\"t\\u00e9st\", \"a\":[ 1 , 2 ],\"b\":\"<&>\" }"
	sealed, binding := f.seal("forward.admin.forward.nodes.id.put", "req-1", body, map[string]string{"id": "7"})
	handle := handles(sealed.Body)[0]
	assert.Equal(t, strings.Replace(body, `"t\u00e9st"`, `"`+handle+`"`, 1), string(sealed.Body))
	secrets, err := f.store.Resolve(binding, Use{Handle: handle, Target: forward(7), Field: "/api_token"})
	require.NoError(t, err)
	assert.Equal(t, "tést", secrets[0].Value, "the secret is the decoded string")

	document := `{"raw_config":"{ \"b\": 1, \"private_key\" : \"k\", \"a\": [\"x\"] }"}`
	sealed, _ = f.seal("proxy.admin.nodes.id.raw_config.put", "req-2", document, map[string]string{"id": "7"})
	var decoded struct {
		RawConfig string `json:"raw_config"`
	}
	require.NoError(t, json.Unmarshal(sealed.Body, &decoded))
	assert.Equal(t, `{ "b": 1, "private_key" : "`+handles(sealed.Body)[0]+`", "a": ["x"] }`, decoded.RawConfig,
		"a document in a string keeps its own text too")
}

// A listed field matches in every spelling the legacy handlers bind.
func TestListedFieldsMatchEverySpelling(t *testing.T) {
	f := newFixture(t)
	for _, body := range []string{`{"API_TOKEN":"tok"}`, `{"ApiToken":"tok"}`, `{"apitoken":"tok","api_token":"tok2"}`} {
		sealed, _ := f.seal("forward.admin.forward.nodes.id.put", "req-"+body, body, map[string]string{"id": "7"})
		assert.NotContains(t, string(sealed.Body), `"tok`, body)
	}
	sealed, _ := f.seal("proxy.admin.nodes.id.put", "req-raw", `{"RawConfig":"{\"private_key\":\"k\"}"}`, map[string]string{"id": "7"})
	assert.NotContains(t, string(sealed.Body), `\"k\"`)
	_, err := f.store.Resolve(Binding{Key: sealed.Key, PackageID: "pkg", Generation: 3, RequestID: "req-raw", RouteID: "proxy.admin.nodes.id.put"},
		Use{Handle: handles(sealed.Body)[0], Target: Target{Kind: TargetProxy, ID: 7}, Field: "/raw_config/private_key"})
	require.NoError(t, err, "the field is named as the list spells it")
}

// Substitution fails closed: the caller serves the request without the
// package host, or refuses it.
func TestSubstitutionFailsClosed(t *testing.T) {
	f := newFixture(t)
	input := func(routeID, body string, params map[string]string) RequestInput {
		return RequestInput{PackageID: "pkg", Generation: 3, RequestID: "req", RouteID: routeID, Deadline: f.clock.Now().Add(time.Minute), Body: []byte(body), PathParams: params}
	}
	id := map[string]string{"id": "7"}
	cases := map[string]struct {
		input RequestInput
		err   error
	}{
		"a body that is not JSON":         {input("forward.admin.forward.nodes.id.put", `api_token=tok`, id), ErrNotJSON},
		"a truncated body":                {input("forward.admin.forward.nodes.id.put", `{"api_token":"tok"`, id), ErrNotJSON},
		"a document that is not JSON":     {input("proxy.admin.nodes.id.raw_config.put", `{"raw_config":"private_key=k"}`, id), ErrNotJSON},
		"a value that is not a string":    {input("forward.admin.forward.nodes.id.put", `{"api_token":12345}`, id), ErrValueNotString},
		"an object at a value field":      {input("forward.admin.forward.nodes.id.put", `{"api_token":{"v":"tok"}}`, id), ErrValueNotString},
		"a target that is not an id":      {input("forward.admin.forward.nodes.id.put", `{"api_token":"tok"}`, map[string]string{"id": "7 OR 1=1"}), ErrTargetInvalid},
		"a zero target":                   {input("forward.admin.forward.nodes.id.put", `{"api_token":"tok"}`, map[string]string{"id": "0"}), ErrTargetInvalid},
		"a missing target":                {input("forward.admin.forward.nodes.id.put", `{"api_token":"tok"}`, nil), ErrTargetInvalid},
		"a nesting deeper than the limit": {input("forward.admin.forward.nodes.id.put", strings.Repeat("[", maxDepth+2)+strings.Repeat("]", maxDepth+2), id), ErrNotJSON},
	}
	for name, test := range cases {
		_, err := f.sealer.SealRequest(test.input)
		require.ErrorIs(t, err, test.err, name)
		assert.NotContains(t, err.Error(), "tok", name)
	}
	tooLarge := input("forward.admin.forward.nodes.id.put", `{"api_token":"t"}`, id)
	tooLarge.BodyLimit = int64(len(tooLarge.Body))
	_, err := f.sealer.SealRequest(tooLarge)
	require.ErrorIs(t, err, ErrTooLarge, "a handle is longer than the secret it stands for")
	assert.Zero(t, f.store.Pending(), "a failed substitution keeps nothing")

	unavailable := NewSealer(nil, ErrFieldsUnavailable, NewStore(nil))
	assert.True(t, unavailable.Listed("knowledge.article.list"), "without the list no route can be told apart")
	_, err = unavailable.SealRequest(input("knowledge.article.list", `{"q":"x"}`, nil))
	require.ErrorIs(t, err, ErrFieldsUnavailable)
	sealed, err := unavailable.SealRequest(input("knowledge.article.list", ``, nil))
	require.NoError(t, err, "a request without a body carries no secret")
	unavailable.Release(sealed.Key)

	sealed, err = f.sealer.SealRequest(input("knowledge.article.list", `not json`, nil))
	require.NoError(t, err, "an unlisted route is not sealed")
	assert.Equal(t, "not json", string(sealed.Body))
	assert.Empty(t, sealed.Key)
}

// expandFixture mints handles for one request of proxy.admin.nodes.post.
func expandFixture(t *testing.T) (*fixture, Sealed, Binding, string, string) {
	f := newFixture(t)
	sealed, binding := f.seal("proxy.admin.nodes.post", "req-1", `{"name":"edge","raw_config":{"private_key":"typed"}}`, nil)
	key, _, err := f.store.Mint(binding, "api_key", "generated-key")
	require.NoError(t, err)
	secret, _, err := f.store.Mint(binding, "secret", "generated-secret")
	require.NoError(t, err)
	return f, sealed, binding, key, secret
}

// Expansion happens only in the bound answer, at the listed field the
// handle was minted for, once.
func TestExpansionHappensOnlyInTheBoundAnswer(t *testing.T) {
	f, sealed, _, key, secret := expandFixture(t)
	answer := `{"code":0,"data":{"node_id":7,"api_key":"` + key + `","secret":"` + secret + `"},"msg":"ok"}`
	expanded, count, err := f.sealer.ExpandAnswer(sealed.Key, "proxy.admin.nodes.post", []byte(answer), nil)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
	assert.Equal(t, `{"code":0,"data":{"node_id":7,"api_key":"generated-key","secret":"generated-secret"},"msg":"ok"}`, string(expanded))
	_, _, err = f.sealer.ExpandAnswer(sealed.Key, "proxy.admin.nodes.post", []byte(answer), nil)
	require.ErrorIs(t, err, ErrAnswerHandle, "a handle expands once")

	f, sealed, binding, key, secret := expandFixture(t)
	other, _ := f.seal("proxy.admin.nodes.post", "req-2", `{}`, nil)
	inbound := handles(sealed.Body)[0]
	refusals := map[string]struct {
		key, route, body string
		headers          []Header
	}{
		"in another request's answer":         {other.Key, "proxy.admin.nodes.post", `{"data":{"api_key":"` + key + `"}}`, nil},
		"under another name":                  {sealed.Key, "proxy.admin.nodes.post", `{"data":{"api_key":"` + secret + `"}}`, nil},
		"at an unlisted position":             {sealed.Key, "proxy.admin.nodes.post", `{"data":{"note":"` + key + `"}}`, nil},
		"inside a longer text":                {sealed.Key, "proxy.admin.nodes.post", `{"data":{"note":"key ` + key + ` here"}}`, nil},
		"as a member name":                    {sealed.Key, "proxy.admin.nodes.post", `{"data":{"` + key + `":1}}`, nil},
		"a request's own handle echoed":       {sealed.Key, "proxy.admin.nodes.post", `{"data":{"api_key":"` + inbound + `"}}`, nil},
		"a request's handle elsewhere":        {sealed.Key, "proxy.admin.nodes.post", `{"data":{"raw_config":{"private_key":"` + inbound + `"}}}`, nil},
		"an unknown handle at a listed field": {sealed.Key, "proxy.admin.nodes.post", `{"data":{"api_key":"` + Prefix + strings.Repeat("B", 43) + `"}}`, nil},
		"in an unlisted route's answer":       {"", "knowledge.article.list", `{"data":"` + key + `"}`, nil},
		"in a header":                         {sealed.Key, "proxy.admin.nodes.post", `{}`, []Header{{Name: "X-Key", Value: key}}},
		"in a body that is not JSON":          {sealed.Key, "proxy.admin.nodes.post", `key=` + key, nil},
		"written with escapes":                {sealed.Key, "proxy.admin.nodes.post", `{"data":{"note":"` + `\u0061` + key[1:] + `"}}`, nil},
		"at a listed field without a request": {"", "proxy.admin.nodes.post", `{"data":{"api_key":"` + key + `"}}`, nil},
	}
	for name, refusal := range refusals {
		_, _, err := f.sealer.ExpandAnswer(refusal.key, refusal.route, []byte(refusal.body), refusal.headers)
		require.ErrorIs(t, err, ErrAnswerHandle, name)
		assert.NotContains(t, err.Error(), "anix-sealed", name)
	}
	expanded, _, err = f.sealer.ExpandAnswer(sealed.Key, "proxy.admin.nodes.post", []byte(`{"data":{"api_key":"`+key+`"}}`), nil)
	require.NoError(t, err, "no refusal used the handle")
	assert.Equal(t, `{"data":{"api_key":"generated-key"}}`, string(expanded))
	_ = binding

	// A string with the shape of a handle that is none is data.
	shaped := Prefix + strings.Repeat("C", 43)
	answer = `{"data":{"title":"` + shaped + `"}}`
	expanded, count, err = f.sealer.ExpandAnswer("", "knowledge.article.list", []byte(answer), nil)
	require.NoError(t, err)
	assert.Zero(t, count)
	assert.Equal(t, answer, string(expanded))
	plain := `{"data":{"title":"caf\u00e9 \u003cb\u003e"}}`
	expanded, _, err = f.sealer.ExpandAnswer("", "knowledge.article.list", []byte(plain), nil)
	require.NoError(t, err)
	assert.Equal(t, plain, string(expanded))
}

// After its request ends a minted handle expands nowhere.
func TestAnswerHandlesDieWithTheirRequest(t *testing.T) {
	f, sealed, _, key, _ := expandFixture(t)
	f.clock.advance(time.Minute)
	_, _, err := f.sealer.ExpandAnswer(sealed.Key, "proxy.admin.nodes.post", []byte(`{"data":{"api_key":"`+key+`"}}`), nil)
	require.ErrorIs(t, err, ErrAnswerHandle)
	f.sealer.Release(sealed.Key)
	assert.False(t, f.store.Live(key))
}

func TestMayHoldHandle(t *testing.T) {
	assert.False(t, mayHoldHandle([]byte(`{"a":"\u003c\u0026\u2028"}`)), "the escapes encoders write by themselves")
	assert.True(t, mayHoldHandle([]byte(`{"a":"\u0061nix"}`)))
	assert.True(t, mayHoldHandle([]byte(`{"a":"x`+Prefix+`"}`)))
	assert.False(t, mayHoldHandle([]byte(`{"a":"\u00`)))
}
