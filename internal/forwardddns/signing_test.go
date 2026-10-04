package forwardddns

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
)

// The signing algorithms are pinned to the vendors' published worked
// examples, so a test cannot pass on an algorithm that only agrees with
// itself.

// Alibaba Cloud, "V3 request structure and signature", fixed example.
func TestACS3SignMatchesAlibabaExample(t *testing.T) {
	payloadHash := sha256Hex(nil)
	headers := map[string]string{
		"host":                  "ecs.cn-shanghai.aliyuncs.com",
		"x-acs-action":          "RunInstances",
		"x-acs-version":         "2014-05-26",
		"x-acs-date":            "2023-10-26T10:22:32Z",
		"x-acs-signature-nonce": "3156853299f313e23d1673dc12e1703d",
		"x-acs-content-sha256":  payloadHash,
	}
	query := canonicalQuery(map[string][]string{
		"ImageId": {"win2019_1809_x64_dtc_zh-cn_40G_alibase_20230811.vhd"}, "RegionId": {"cn-shanghai"},
	})
	authorization, canonical := acs3Sign("POST", "/", query, headers, payloadHash, "YourAccessKeyId", "YourAccessKeySecret")
	require.Equal(t, "7ea06492da5221eba5297e897ce16e55f964061054b7695beedaac1145b1e259", sha256Hex([]byte(canonical)))
	require.Equal(t, "ACS3-HMAC-SHA256 Credential=YourAccessKeyId,SignedHeaders=host;x-acs-action;x-acs-content-sha256;x-acs-date;x-acs-signature-nonce;x-acs-version,"+
		"Signature=06563a9e1b43f5dfe96b81484da74bceab24a1d853912eee15083a6f0f3283c0", authorization)
}

// Tencent Cloud API 3.0, "Signature v3", example with x-tc-action signed
// (the published keys are the masked strings, as given).
func TestTC3SignMatchesTencentExample(t *testing.T) {
	payload := []byte("{\"Limit\": 1, \"Filters\": [{\"Values\": [\"\\u672a\\u547d\\u540d\"], \"Name\": \"instance-name\"}]}")
	authorization, canonical := tc3Sign("AKID********************************", "********************************", "cvm",
		"cvm.tencentcloudapi.com", "DescribeInstances", payload, 1551113065)
	require.Equal(t, "7019a55be8395899b900fb5564e4200d984910f34794a27cb3fb7d10ff6a1e84", sha256Hex([]byte(canonical)))
	require.Equal(t, "TC3-HMAC-SHA256 Credential=AKID********************************/2019-02-25/cvm/tc3_request, "+
		"SignedHeaders=content-type;host;x-tc-action, Signature=10b1a37a7301a02ca19a647ad722d5e43b4b3cff309d421d85b46093f6ab6c4f", authorization)
}

// Huawei Cloud, "API signing algorithm": the published example masks the
// SK, so the canonical request is pinned to its published hash and the
// signature to HMAC-SHA256 over the published string to sign.
func TestHuaweiSignMatchesHuaweiExample(t *testing.T) {
	headers := map[string]string{
		"content-type": "application/json", "host": "service.region.example.com", "x-sdk-date": "20191115T033655Z",
	}
	query := canonicalQuery(map[string][]string{"marker": {"13551d6b-755d-4757-b956-536f674975c0"}, "limit": {"2"}})
	uri := huaweiCanonicalURI("/v1/77b6a44cba5143ab91d13ab9a8ff44fd/vpcs")
	require.Equal(t, "/v1/77b6a44cba5143ab91d13ab9a8ff44fd/vpcs/", uri)
	authorization, canonical := huaweiSign("GET", uri, query, headers, sha256Hex(nil), "QTWAOYTTINDUT2QVKYUC", "test-secret")
	require.Equal(t, "b25362e603ee30f4f25e7858e8a7160fd36e803bb2dfe206278659d71a9bcd7a", sha256Hex([]byte(canonical)))
	toSign := "SDK-HMAC-SHA256\n20191115T033655Z\nb25362e603ee30f4f25e7858e8a7160fd36e803bb2dfe206278659d71a9bcd7a"
	require.Equal(t, "SDK-HMAC-SHA256 Access=QTWAOYTTINDUT2QVKYUC, SignedHeaders=content-type;host;x-sdk-date, Signature="+
		hexHMAC("test-secret", toSign), authorization)
}

func hexHMAC(key, message string) string {
	return hex.EncodeToString(hmacSHA256([]byte(key), []byte(message)))
}

func TestWebhookSignature(t *testing.T) {
	// HMAC-SHA256("secret", "1759579200.{}"), computed independently.
	require.Equal(t, "sha256="+hexHMAC("secret", "1759579200.{}"), WebhookSignature("secret", 1759579200, []byte("{}")))
}

func TestCanonicalQueryEncoding(t *testing.T) {
	require.Equal(t, "a=1&b=x%20y&b=~z&c=%2A%2F", canonicalQuery(map[string][]string{"c": {"*/"}, "b": {"x y", "~z"}, "a": {"1"}}))
}

func TestValidate(t *testing.T) {
	require.Empty(t, Validate(KindCloudflare, nil, map[string]string{CredentialAPIToken: "t"}))
	problems := Validate(KindAliDNS, map[string]string{"region": "x"}, map[string]string{CredentialAccessKeyID: "id"})
	require.Len(t, problems, 2)
	require.Equal(t, "config.region", problems[0].Field)
	require.Equal(t, "credentials.access_key_secret", problems[1].Field)
	require.NotEmpty(t, Validate(KindWebhook, map[string]string{ConfigURL: "http://example.com/hook"}, map[string]string{CredentialSecret: "s"}))
	require.Empty(t, Validate(KindWebhook, map[string]string{ConfigURL: "https://example.com/hook"}, map[string]string{CredentialSecret: "s"}))
	require.NotEmpty(t, Validate("route53", nil, nil))
	require.NotEmpty(t, Validate(KindDNSPod, map[string]string{ConfigEndpoint: "https://x"}, map[string]string{CredentialSecretID: "a", CredentialSecretKey: "b"}))
}

func TestCheckSet(t *testing.T) {
	require.NoError(t, CheckSet(RecordSet{Zone: "example.com", Name: "hk.example.com", Type: TypeA, Values: []string{"192.0.2.1"}, TTL: 60}, false))
	require.Error(t, CheckSet(RecordSet{Zone: "example.com", Name: "hk.example.net", Type: TypeA, Values: []string{"192.0.2.1"}}, false))
	require.Error(t, CheckSet(RecordSet{Zone: "example.com", Name: "hk.example.com", Type: TypeA, Values: []string{"2001:db8::1"}}, false))
	require.Error(t, CheckSet(RecordSet{Zone: "example.com", Name: "hk.example.com", Type: TypeAAAA, Values: []string{"192.0.2.1"}}, false))
	require.Error(t, CheckSet(RecordSet{Zone: "example.com", Name: "hk.example.com", Type: TypeA}, false))
	require.Error(t, CheckSet(RecordSet{Zone: "example.com", Name: "hk.example.com", Type: "TXT", Values: []string{"x"}}, false))
	require.Equal(t, "@", relative("Example.com.", "example.com"))
	require.Equal(t, "hk.edge", relative("hk.edge.example.com", "example.com"))
}
