package shadowsample

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const testJWT = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U"

func TestSanitizeString(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"empty", "", ""},
		{"plain text", "plan not found", "plan not found"},
		{"jwt", testJWT, Mask},
		{"jwt in text", "header Bearer " + testJWT + " sent", "header Bearer *** sent"},
		{"basic credentials", "Basic dXNlcjpwYXNz", "Basic ***"},
		{"uuid", "7b3f6c1e-2d4a-4b8f-9c0d-1e2f3a4b5c6d", Mask},
		{"uuid in text", "user 7B3F6C1E-2D4A-4B8F-9C0D-1E2F3A4B5C6D left", "user *** left"},
		{"long hex", "a3f5c9e1b2d4f6a8c0e2b4d6f8a1c3e5", Mask},
		{"long hex in text", "hash=a3f5c9e1b2d4f6a8c0e2b4d6f8a1c3e5a3f5c9e1", "hash=***"},
		{"short hex kept", "deadbeef", "deadbeef"},
		{"base64url secret", "kq3ZQ9v_8xN2mT4pL6rW1yB5cH7jF0dA", Mask},
		{"std base64 secret", "QmFzZTY0U2VjcmV0VmFsdWVGb3JUZXN0aW5nUHVycG9zZXMxMjM0NTY3ODk=", Mask},
		{"long lowercase words kept", "the_quick_brown_fox_jumps_over_the_lazy_dog", "the_quick_brown_fox_jumps_over_the_lazy_dog"},
		{"bcrypt hash", "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy", Mask},
		{"argon2 hash", "$argon2id$v=19$m=65536,t=3,p=4$c29tZXNhbHQ$RdescudvJCsgt3ub+b+dWRWJTmaaJObG", Mask},
		{"private key", "-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXktdjEAAAAA\n-----END OPENSSH PRIVATE KEY-----", Mask},
		{"email", "alice@example.com", "a***@example.com"},
		{"email in text", "sent to bob.smith+x@mail.example.org today", "sent to b***@mail.example.org today"},
		{"ipv4", "10.2.33.44", "10.2.*.*"},
		{"ipv4 with port", "192.168.1.20:443", "192.168.*.*:443"},
		{"ipv4 in text", "client 203.0.113.9 denied", "client 203.0.*.* denied"},
		{"version-like out of range kept", "1.2.3.999", "1.2.3.999"},
		{"ipv6", "2001:db8:85a3::8a2e:370:7334", "2001:db8:*:*:*:*:*:*"},
		{"ipv6 loopback", "::1", "0:0:*:*:*:*:*:*"},
		{"ipv6 in text", "from fe80::1ff:fe23:4567:890a via eth0", "from fe80:0:*:*:*:*:*:* via eth0"},
		{"ipv4-mapped ipv6", "::ffff:10.9.8.7", "10.9.*.*"},
		{"time kept", "12:30:45", "12:30:45"},
		{"mac kept", "aa:bb:cc:dd:ee:ff", "aa:bb:cc:dd:ee:ff"},
		{"subscription url", "https://sub.example.com/api/v1/client/subscribe?token=abcdef", "https://sub.example.com/***"},
		{"url host only", "https://example.com", "https://example.com"},
		{"url ip host", "http://10.1.2.3:8080/path", "http://10.1.*.*:8080/***"},
		{"url ipv6 host", "http://[2001:db8::1]/x", "http://[2001:db8:*:*:*:*:*:*]/***"},
		{"url with userinfo", "https://user:pass@example.com/", "https://***"},
		{"share link", "vless://7b3f6c1e-2d4a-4b8f-9c0d-1e2f3a4b5c6d@1.2.3.4:443?security=tls#node", "vless://***"},
		{"ss link in text", "import ss://YWVzLTI1Ni1nY206cGFzcw@host:8388 now", "import ss://*** now"},
		{"sealed handle", "anix-sealed:v1:Zk3q9v_8xN2mT4pL6rW1yB5cH7jF0dAa1b2c3d4e5f6", "anix-sealed:v1:***"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := SanitizeString(tc.in)
			require.Equal(t, tc.want, got)
			require.Equal(t, got, SanitizeString(got), "sanitizing is idempotent")
		})
	}
}

func TestSanitizeStringTruncates(t *testing.T) {
	got := SanitizeString(strings.Repeat("é ", 400))
	require.LessOrEqual(t, len(got), MaxStringBytes+len("…"))
	require.True(t, strings.HasSuffix(got, "…"))
	require.True(t, json.Valid(mustMarshal(got)))
}

func TestSensitiveKeys(t *testing.T) {
	for _, key := range []string{
		"token", "subscribe_token", "accessToken", "SECRET", "client_secret", "password", "Passwd", "pwd", "password_hash",
		"hash", "salt", "api_key", "apiKey", "private_key", "public_key", "uuid", "user_uuid", "sign", "signature",
		"cookie", "Set-Cookie", "authorization", "Authorization", "auth_data", "session_id", "subscribe_url",
		"subscriptionUrl", "sub_url", "otp", "nonce", "credentials",
	} {
		require.True(t, IsSensitiveKey(key), key)
	}
	for _, key := range []string{"id", "email", "name", "plan_id", "status", "transfer_enable", "created_at", "data", "code", "msg"} {
		require.False(t, IsSensitiveKey(key), key)
	}
}

func TestSanitizeValue(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{
			"sensitive keys mask any type",
			`{"token":"abc","password":12345,"uuid":{"nested":"x"},"auth_data":["a","b"],"is_secret":true,"name":"plan"}`,
			`{"auth_data":"***","is_secret":"***","name":"plan","password":"***","token":"***","uuid":"***"}`,
		},
		{
			"nested objects and arrays",
			`{"data":{"users":[{"email":"alice@example.com","last_login_ip":"10.2.3.4","subscribe_url":"https://x/s/abc","n":3}]}}`,
			`{"data":{"users":[{"email":"a***@example.com","last_login_ip":"10.2.*.*","n":3,"subscribe_url":"***"}]}}`,
		},
		{
			"jwt under an unexpected key",
			`{"note":"` + testJWT + `","list":["` + testJWT + `"]}`,
			`{"list":["***"],"note":"***"}`,
		},
		{
			"secret-looking keys",
			`{"7b3f6c1e-2d4a-4b8f-9c0d-1e2f3a4b5c6d":1,"alice@example.com":2}`,
			`{"***":1,"a***@example.com":2}`,
		},
		{
			"key collisions stay distinct",
			`{"7b3f6c1e-2d4a-4b8f-9c0d-1e2f3a4b5c6d":1,"8b3f6c1e-2d4a-4b8f-9c0d-1e2f3a4b5c6d":2}`,
			`{"***":1,"***~2":2}`,
		},
		{
			"numbers keep their spelling",
			`{"amount":1.50,"big":12345678901234567890,"ok":false,"none":null}`,
			`{"amount":1.50,"big":12345678901234567890,"none":null,"ok":false}`,
		},
		{
			"ipv6 in arrays",
			`{"ips":["2001:db8::1","10.0.0.1"]}`,
			`{"ips":["2001:db8:*:*:*:*:*:*","10.0.*.*"]}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := SanitizeJSON([]byte(tc.in))
			require.JSONEq(t, tc.want, string(got))
			if strings.Contains(tc.want, "1.50") {
				require.Contains(t, string(got), "1.50")
			}
		})
	}
}

func TestSanitizeJSONNeverKeepsNonJSON(t *testing.T) {
	got := SanitizeJSON([]byte("password=hunter2&token=abc"))
	require.JSONEq(t, `"<non-JSON body, 26 bytes>"`, string(got))
}

func TestSanitizePath(t *testing.T) {
	cases := []struct {
		name   string
		path   string
		params map[string]string
		query  map[string][]string
		want   string
	}{
		{"plain", "/api/v2/user/plan/fetch", nil, nil, "/api/v2/user/plan/fetch"},
		{"query values masked", "/api/v2/client/subscribe?token=abc&flag=1", nil, nil, "/api/v2/client/subscribe?flag=***&token=***"},
		{"metadata query", "/api/v2/client/subscribe", nil, map[string][]string{"token": {"abc"}}, "/api/v2/client/subscribe?token=***"},
		{"path params become names", "/s/abcdef123/info", map[string]string{"token": "abcdef123"}, nil, "/s/{token}/info"},
		{"secret segments masked", "/api/v2/user/7b3f6c1e-2d4a-4b8f-9c0d-1e2f3a4b5c6d/a@b.example.com", nil, nil, "/api/v2/user/***/a***@b.example.com"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, SanitizePath(tc.path, tc.params, tc.query))
		})
	}
}

func TestSanitizeIP(t *testing.T) {
	require.Equal(t, "10.2.*.*", SanitizeIP("10.2.3.4"))
	require.Equal(t, "2001:db8:*:*:*:*:*:*", SanitizeIP("2001:0db8:0000::1"))
	require.Equal(t, "not-an-ip", SanitizeIP("not-an-ip"))
}
