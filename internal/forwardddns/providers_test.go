package forwardddns

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Each provider runs against an httptest TLS server that plays the
// provider's API on an in-memory zone. The server verifies every request's
// signature from the request as it arrived on the wire (so what is sent is
// what is signed), and the test pins the calls each change makes: new
// values are added before old ones are deleted.

var fixedNow = time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)

func fixedOptions(server *httptest.Server) Options {
	return Options{Client: server.Client(), Now: func() time.Time { return fixedNow }, Nonce: func() string { return "nonce-0001" }}
}

func endpointOf(server *httptest.Server) string {
	parsed, _ := url.Parse(server.URL)
	return parsed.Host
}

// zoneStore is the fake provider's records of hk.example.com.
type zoneStore struct {
	mu      sync.Mutex
	nextID  int
	records map[string]fakeRecord // id -> record
	calls   []string
}

type fakeRecord struct {
	Name, Type, Value string
	TTL               uint32
}

func newZoneStore(initial ...fakeRecord) *zoneStore {
	z := &zoneStore{records: map[string]fakeRecord{}, nextID: 100}
	for _, r := range initial {
		z.add(r)
	}
	return z
}

func (z *zoneStore) add(r fakeRecord) string {
	z.nextID++
	id := strconv.Itoa(z.nextID)
	z.records[id] = r
	return id
}

func (z *zoneStore) find(name, rtype string) []string {
	var ids []string
	for id, r := range z.records {
		if r.Name == name && r.Type == rtype {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func (z *zoneStore) values(name, rtype string) []string {
	z.mu.Lock()
	defer z.mu.Unlock()
	var out []string
	for _, id := range z.find(name, rtype) {
		r := z.records[id]
		out = append(out, fmt.Sprintf("%s/%d", r.Value, r.TTL))
	}
	sort.Strings(out)
	return out
}

func (z *zoneStore) log(call string) {
	z.calls = append(z.calls, call)
}

func initialRecords() []fakeRecord {
	return []fakeRecord{
		{Name: "hk.example.com", Type: TypeA, Value: "192.0.2.1", TTL: 60},
		{Name: "hk.example.com", Type: TypeA, Value: "192.0.2.9", TTL: 60},
	}
}

var change = RecordSet{Zone: "example.com", Name: "hk.example.com", Type: TypeA, Values: []string{"192.0.2.2", "192.0.2.1"}, TTL: 120}

func runScenario(t *testing.T, provider Provider, store *zoneStore) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, provider.SetRecords(ctx, change))
	require.Equal(t, []string{"192.0.2.1/120", "192.0.2.2/120"}, store.values("hk.example.com", TypeA))
	// The same set again changes nothing.
	before := len(store.calls)
	require.NoError(t, provider.SetRecords(ctx, change))
	for _, call := range store.calls[before:] {
		require.True(t, strings.HasPrefix(call, "read "), "an unchanged set wrote %s", call)
	}
	require.NoError(t, provider.DeleteRecords(ctx, "example.com", "hk.example.com", TypeA))
	require.Empty(t, store.values("hk.example.com", TypeA))
}

func writes(calls []string) []string {
	var out []string
	for _, call := range calls {
		if !strings.HasPrefix(call, "read ") {
			out = append(out, call)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Cloudflare

func TestCloudflareSetRecords(t *testing.T) {
	store := newZoneStore(initialRecords()...)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		store.mu.Lock()
		defer store.mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer cf-token" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"success":false,"errors":[{"code":9109,"message":"Invalid access token"}]}`)
			return
		}
		body, _ := io.ReadAll(r.Body)
		ok := func(result any) {
			encoded, _ := json.Marshal(map[string]any{"success": true, "errors": []any{}, "result": result})
			_, _ = w.Write(encoded)
		}
		path := strings.TrimPrefix(r.URL.Path, "/client/v4")
		switch {
		case r.Method == http.MethodGet && path == "/zones":
			store.log("read zones " + r.URL.RawQuery)
			ok([]map[string]string{{"id": "zone-1", "name": r.URL.Query().Get("name")}})
		case r.Method == http.MethodGet && path == "/zones/zone-1/dns_records":
			store.log("read records " + r.URL.RawQuery)
			var out []map[string]any
			for _, id := range store.find(r.URL.Query().Get("name"), r.URL.Query().Get("type")) {
				rec := store.records[id]
				out = append(out, map[string]any{"id": id, "type": rec.Type, "name": rec.Name, "content": rec.Value, "ttl": rec.TTL})
			}
			ok(out)
		case r.Method == http.MethodPost && path == "/zones/zone-1/dns_records":
			store.log("POST " + string(body))
			var rec cloudflareRecord
			_ = json.Unmarshal(body, &rec)
			ok(map[string]string{"id": store.add(fakeRecord{Name: rec.Name, Type: rec.Type, Value: rec.Content, TTL: rec.TTL})})
		case r.Method == http.MethodPatch && strings.HasPrefix(path, "/zones/zone-1/dns_records/"):
			id := strings.TrimPrefix(path, "/zones/zone-1/dns_records/")
			store.log("PATCH " + id + " " + string(body))
			var patch struct{ TTL uint32 }
			_ = json.Unmarshal(body, &patch)
			rec := store.records[id]
			rec.TTL = patch.TTL
			store.records[id] = rec
			ok(map[string]string{"id": id})
		case r.Method == http.MethodDelete && strings.HasPrefix(path, "/zones/zone-1/dns_records/"):
			id := strings.TrimPrefix(path, "/zones/zone-1/dns_records/")
			store.log("DELETE " + id)
			delete(store.records, id)
			ok(map[string]string{"id": id})
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"success":false,"errors":[{"code":7003,"message":"no route"}]}`)
		}
	}))
	defer server.Close()

	provider, err := New(KindCloudflare, map[string]string{ConfigEndpoint: endpointOf(server)}, map[string]string{CredentialAPIToken: "cf-token"}, fixedOptions(server))
	require.NoError(t, err)
	runScenario(t, provider, store)
	require.Equal(t, []string{
		"read zones name=example.com",
		"read records name=hk.example.com&per_page=100&type=A",
		`POST {"type":"A","name":"hk.example.com","content":"192.0.2.2","ttl":120,"proxied":false}`,
		`PATCH 101 {"ttl":120}`,
		"DELETE 102",
	}, store.calls[:5])
	require.Equal(t, []string{`POST {"type":"A","name":"hk.example.com","content":"192.0.2.2","ttl":120,"proxied":false}`,
		`PATCH 101 {"ttl":120}`, "DELETE 102", "DELETE 101", "DELETE 103"}, writes(store.calls))

	bad, err := New(KindCloudflare, map[string]string{ConfigEndpoint: endpointOf(server)}, map[string]string{CredentialAPIToken: "wrong"}, fixedOptions(server))
	require.NoError(t, err)
	err = bad.SetRecords(context.Background(), change)
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, http.StatusForbidden, apiErr.Status)
	require.Equal(t, "9109", apiErr.Code)
	require.NotContains(t, err.Error(), "wrong")
}

// ---------------------------------------------------------------------------
// Alibaba Cloud DNS

// verifyACS3 checks a request's V3 signature as Alibaba Cloud does.
func verifyACS3(r *http.Request, keyID, secret string) bool {
	auth := r.Header.Get("Authorization")
	signedPart := strings.SplitN(strings.TrimPrefix(auth, acs3Algorithm+" "), ",", 3)
	if len(signedPart) != 3 || signedPart[0] != "Credential="+keyID {
		return false
	}
	headers := map[string]string{}
	for _, name := range strings.Split(strings.TrimPrefix(signedPart[1], "SignedHeaders="), ";") {
		if name == "host" {
			headers[name] = r.Host
		} else {
			headers[name] = r.Header.Get(name)
		}
	}
	if r.Header.Get("x-acs-content-sha256") != sha256Hex(nil) {
		return false
	}
	want, _ := acs3Sign(r.Method, r.URL.Path, canonicalQuery(r.URL.Query()), headers, sha256Hex(nil), keyID, secret)
	return want == auth
}

func TestAliDNSSetRecords(t *testing.T) {
	store := newZoneStore(initialRecords()...)
	var firstAuth []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		store.mu.Lock()
		defer store.mu.Unlock()
		if !verifyACS3(r, "ak-id", "ak-secret") {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"Code":"SignatureDoesNotMatch","Message":"signature mismatch","RequestId":"x"}`)
			return
		}
		if firstAuth == nil {
			firstAuth = []string{r.Header.Get("x-acs-action"), r.Header.Get("x-acs-version"), r.Header.Get("x-acs-date"), r.Header.Get("x-acs-signature-nonce")}
		}
		q := r.URL.Query()
		action := r.Header.Get("x-acs-action")
		name := func(rr string) string {
			if rr == "@" {
				return "example.com"
			}
			return rr + ".example.com"
		}
		write := func(value any) { encoded, _ := json.Marshal(value); _, _ = w.Write(encoded) }
		switch action {
		case "DescribeSubDomainRecords":
			store.log("read " + canonicalQuery(q))
			var out []map[string]any
			for _, id := range store.find(q.Get("SubDomain"), q.Get("Type")) {
				rec := store.records[id]
				out = append(out, map[string]any{"RecordId": id, "RR": strings.TrimSuffix(rec.Name, ".example.com"), "Type": rec.Type, "Value": rec.Value, "TTL": rec.TTL})
			}
			write(map[string]any{"DomainRecords": map[string]any{"Record": out}, "TotalCount": len(out)})
		case "AddDomainRecord":
			store.log(action + " " + canonicalQuery(q))
			ttl, _ := strconv.Atoi(q.Get("TTL"))
			write(map[string]string{"RecordId": store.add(fakeRecord{Name: name(q.Get("RR")), Type: q.Get("Type"), Value: q.Get("Value"), TTL: uint32(ttl)})})
		case "UpdateDomainRecord":
			store.log(action + " " + canonicalQuery(q))
			ttl, _ := strconv.Atoi(q.Get("TTL"))
			store.records[q.Get("RecordId")] = fakeRecord{Name: name(q.Get("RR")), Type: q.Get("Type"), Value: q.Get("Value"), TTL: uint32(ttl)}
			write(map[string]string{"RecordId": q.Get("RecordId")})
		case "DeleteDomainRecord":
			store.log(action + " " + canonicalQuery(q))
			delete(store.records, q.Get("RecordId"))
			write(map[string]string{"RecordId": q.Get("RecordId")})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	provider, err := New(KindAliDNS, map[string]string{ConfigEndpoint: endpointOf(server)},
		map[string]string{CredentialAccessKeyID: "ak-id", CredentialAccessKeySecret: "ak-secret"}, fixedOptions(server))
	require.NoError(t, err)
	runScenario(t, provider, store)
	require.Equal(t, []string{"DescribeSubDomainRecords", "2015-01-09", "2026-10-04T12:00:00Z", "nonce-0001"}, firstAuth)
	require.Equal(t, []string{
		"read DomainName=example.com&PageSize=500&SubDomain=hk.example.com&Type=A",
		"AddDomainRecord DomainName=example.com&RR=hk&TTL=120&Type=A&Value=192.0.2.2",
		"UpdateDomainRecord RR=hk&RecordId=101&TTL=120&Type=A&Value=192.0.2.1",
		"DeleteDomainRecord RecordId=102",
	}, store.calls[:4])

	bad, err := New(KindAliDNS, map[string]string{ConfigEndpoint: endpointOf(server)},
		map[string]string{CredentialAccessKeyID: "ak-id", CredentialAccessKeySecret: "wrong"}, fixedOptions(server))
	require.NoError(t, err)
	var apiErr *APIError
	require.ErrorAs(t, bad.SetRecords(context.Background(), change), &apiErr)
	require.Equal(t, "SignatureDoesNotMatch", apiErr.Code)
}

// ---------------------------------------------------------------------------
// DNSPod

func verifyTC3(r *http.Request, body []byte, secretID, secretKey string) bool {
	timestamp, err := strconv.ParseInt(r.Header.Get("X-TC-Timestamp"), 10, 64)
	if err != nil || r.Header.Get("Content-Type") != tc3ContentType || r.Header.Get("X-TC-Version") != dnspodVersion {
		return false
	}
	want, _ := tc3Sign(secretID, secretKey, dnspodService, r.Host, r.Header.Get("X-TC-Action"), body, timestamp)
	return want == r.Header.Get("Authorization")
}

func TestDNSPodSetRecords(t *testing.T) {
	store := newZoneStore(initialRecords()...)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		store.mu.Lock()
		defer store.mu.Unlock()
		body, _ := io.ReadAll(r.Body)
		respond := func(value map[string]any) {
			value["RequestId"] = "req-1"
			encoded, _ := json.Marshal(map[string]any{"Response": value})
			_, _ = w.Write(encoded)
		}
		if !verifyTC3(r, body, "sid", "skey") {
			respond(map[string]any{"Error": map[string]string{"Code": "AuthFailure.SignatureFailure", "Message": "signature mismatch"}})
			return
		}
		var params struct {
			Domain, Subdomain, SubDomain, RecordType, RecordLine, Value string
			RecordID                                                    uint64 `json:"RecordId"`
			TTL                                                         uint32
		}
		_ = json.Unmarshal(body, &params)
		action := r.Header.Get("X-TC-Action")
		switch action {
		case "DescribeRecordList":
			store.log("read " + string(body))
			var out []map[string]any
			for _, id := range store.find(params.Subdomain+".example.com", params.RecordType) {
				rec := store.records[id]
				n, _ := strconv.ParseUint(id, 10, 64)
				out = append(out, map[string]any{"RecordId": n, "Name": params.Subdomain, "Type": rec.Type, "Value": rec.Value, "TTL": rec.TTL, "Line": dnspodDefaultLine})
			}
			if len(out) == 0 {
				respond(map[string]any{"Error": map[string]string{"Code": dnspodNoRecords, "Message": "no records"}})
				return
			}
			respond(map[string]any{"RecordList": out})
		case "CreateRecord":
			store.log(action + " " + string(body))
			id := store.add(fakeRecord{Name: params.SubDomain + ".example.com", Type: params.RecordType, Value: params.Value, TTL: params.TTL})
			n, _ := strconv.ParseUint(id, 10, 64)
			respond(map[string]any{"RecordId": n})
		case "ModifyRecord":
			store.log(action + " " + string(body))
			store.records[strconv.FormatUint(params.RecordID, 10)] = fakeRecord{Name: params.SubDomain + ".example.com", Type: params.RecordType, Value: params.Value, TTL: params.TTL}
			respond(map[string]any{"RecordId": params.RecordID})
		case "DeleteRecord":
			store.log(action + " " + string(body))
			delete(store.records, strconv.FormatUint(params.RecordID, 10))
			respond(map[string]any{})
		}
	}))
	defer server.Close()

	provider, err := New(KindDNSPod, map[string]string{ConfigEndpoint: endpointOf(server)},
		map[string]string{CredentialSecretID: "sid", CredentialSecretKey: "skey"}, fixedOptions(server))
	require.NoError(t, err)
	runScenario(t, provider, store)
	require.Equal(t, []string{
		`read {"Domain":"example.com","Limit":3000,"RecordType":"A","Subdomain":"hk"}`,
		`CreateRecord {"Domain":"example.com","RecordLine":"默认","RecordType":"A","SubDomain":"hk","TTL":120,"Value":"192.0.2.2"}`,
		`ModifyRecord {"Domain":"example.com","RecordId":101,"RecordLine":"默认","RecordType":"A","SubDomain":"hk","TTL":120,"Value":"192.0.2.1"}`,
		`DeleteRecord {"Domain":"example.com","RecordId":102}`,
	}, store.calls[:4])

	bad, err := New(KindDNSPod, map[string]string{ConfigEndpoint: endpointOf(server)},
		map[string]string{CredentialSecretID: "sid", CredentialSecretKey: "wrong"}, fixedOptions(server))
	require.NoError(t, err)
	var apiErr *APIError
	require.ErrorAs(t, bad.SetRecords(context.Background(), change), &apiErr)
	require.Equal(t, "AuthFailure.SignatureFailure", apiErr.Code)
}

// ---------------------------------------------------------------------------
// Huawei Cloud DNS

func verifyHuawei(r *http.Request, body []byte, accessKey, secretKey string) bool {
	headers := map[string]string{"content-type": r.Header.Get("Content-Type"), "host": r.Host, "x-sdk-date": r.Header.Get("X-Sdk-Date")}
	want, _ := huaweiSign(r.Method, huaweiCanonicalURI(r.URL.Path), canonicalQuery(r.URL.Query()), headers, sha256Hex(body), accessKey, secretKey)
	return want == r.Header.Get("Authorization") && headers["x-sdk-date"] == "20261004T120000Z"
}

func TestHuaweiSetRecords(t *testing.T) {
	type rrset struct {
		Name, Type string
		TTL        uint32
		Records    []string
	}
	var mu sync.Mutex
	sets := map[string]*rrset{"rs-1": {Name: "hk.example.com.", Type: TypeA, TTL: 60, Records: []string{"192.0.2.1", "192.0.2.9"}}}
	var calls []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		body, _ := io.ReadAll(r.Body)
		if !verifyHuawei(r, body, "hw-ak", "hw-sk") {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"error_code":"APIGW.0301","error_msg":"Incorrect IAM authentication information"}`)
			return
		}
		write := func(value any) { encoded, _ := json.Marshal(value); _, _ = w.Write(encoded) }
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v2/zones":
			calls = append(calls, "read zones "+r.URL.RawQuery)
			write(map[string]any{"zones": []map[string]string{{"id": "zone-1", "name": "example.com."}}})
		case r.Method == http.MethodGet && r.URL.Path == "/v2/zones/zone-1/recordsets":
			calls = append(calls, "read recordsets "+r.URL.RawQuery)
			var out []map[string]any
			for id, set := range sets {
				if set.Name == r.URL.Query().Get("name") && set.Type == r.URL.Query().Get("type") {
					out = append(out, map[string]any{"id": id, "name": set.Name, "type": set.Type, "ttl": set.TTL, "records": set.Records})
				}
			}
			write(map[string]any{"recordsets": out})
		case r.Method == http.MethodPost && r.URL.Path == "/v2/zones/zone-1/recordsets":
			calls = append(calls, "POST "+string(body))
			var set rrset
			_ = json.Unmarshal(body, &set)
			sets["rs-2"] = &set
			write(map[string]string{"id": "rs-2"})
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/v2/zones/zone-1/recordsets/"):
			id := strings.TrimPrefix(r.URL.Path, "/v2/zones/zone-1/recordsets/")
			calls = append(calls, "PUT "+id+" "+string(body))
			var update rrset
			_ = json.Unmarshal(body, &update)
			sets[id].TTL, sets[id].Records = update.TTL, update.Records
			write(map[string]string{"id": id})
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/v2/zones/zone-1/recordsets/"):
			id := strings.TrimPrefix(r.URL.Path, "/v2/zones/zone-1/recordsets/")
			calls = append(calls, "DELETE "+id)
			delete(sets, id)
			write(map[string]string{"id": id})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	provider, err := New(KindHuaweiCloud, map[string]string{ConfigEndpoint: endpointOf(server)},
		map[string]string{CredentialAccessKey: "hw-ak", CredentialSecretKey: "hw-sk"}, fixedOptions(server))
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, provider.SetRecords(ctx, change))
	require.Equal(t, []string{"192.0.2.1", "192.0.2.2"}, sets["rs-1"].Records)
	require.EqualValues(t, 120, sets["rs-1"].TTL)
	require.NoError(t, provider.SetRecords(ctx, change))
	require.NoError(t, provider.DeleteRecords(ctx, "example.com", "hk.example.com", TypeA))
	require.Empty(t, sets)
	require.NoError(t, provider.SetRecords(ctx, RecordSet{Zone: "example.com", Name: "hk.example.com", Type: TypeA, Values: []string{"192.0.2.3"}, TTL: 60}))
	require.Equal(t, []string{
		"read zones name=example.com.&type=public",
		"read recordsets name=hk.example.com.&type=A",
		`PUT rs-1 {"ttl":120,"records":["192.0.2.1","192.0.2.2"]}`,
		"read recordsets name=hk.example.com.&type=A",
		"read recordsets name=hk.example.com.&type=A",
		"DELETE rs-1",
		"read recordsets name=hk.example.com.&type=A",
		`POST {"name":"hk.example.com.","type":"A","ttl":60,"records":["192.0.2.3"]}`,
	}, calls)

	bad, err := New(KindHuaweiCloud, map[string]string{ConfigEndpoint: endpointOf(server)},
		map[string]string{CredentialAccessKey: "hw-ak", CredentialSecretKey: "wrong"}, fixedOptions(server))
	require.NoError(t, err)
	var apiErr *APIError
	require.ErrorAs(t, bad.SetRecords(ctx, change), &apiErr)
	require.Equal(t, "APIGW.0301", apiErr.Code)
}

// ---------------------------------------------------------------------------
// Webhook

func TestWebhookSetRecords(t *testing.T) {
	var got []WebhookPayload
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		timestamp, _ := strconv.ParseInt(r.Header.Get(WebhookHeaderTimestamp), 10, 64)
		if r.Header.Get(WebhookHeaderSignature) != WebhookSignature("hook-secret", timestamp, body) || r.Header.Get(WebhookHeaderEvent) != WebhookEvent {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var payload WebhookPayload
		require.NoError(t, json.Unmarshal(body, &payload))
		got = append(got, payload)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	provider, err := New(KindWebhook, map[string]string{ConfigURL: server.URL + "/dns"}, map[string]string{CredentialSecret: "hook-secret"}, fixedOptions(server))
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, provider.SetRecords(ctx, change))
	require.NoError(t, provider.DeleteRecords(ctx, "example.com", "hk.example.com", TypeAAAA))
	require.Equal(t, []WebhookPayload{
		{Version: 1, Action: "set", Zone: "example.com", Name: "hk.example.com", Type: TypeA, Values: []string{"192.0.2.1", "192.0.2.2"}, TTL: 120, Timestamp: fixedNow.Unix()},
		{Version: 1, Action: "delete", Zone: "example.com", Name: "hk.example.com", Type: TypeAAAA, Timestamp: fixedNow.Unix()},
	}, got)

	bad, err := New(KindWebhook, map[string]string{ConfigURL: server.URL + "/dns"}, map[string]string{CredentialSecret: "wrong"}, fixedOptions(server))
	require.NoError(t, err)
	var apiErr *APIError
	require.ErrorAs(t, bad.SetRecords(ctx, change), &apiErr)
	require.Equal(t, http.StatusUnauthorized, apiErr.Status)
}
