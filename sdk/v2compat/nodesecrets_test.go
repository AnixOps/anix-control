package v2compat

import "testing"

func TestRedactNodeSecretsMasksSecretPositionsOnce(t *testing.T) {
	cases := map[string]string{
		"":             "",
		"  ":           "  ",
		"not json":     NodeSecretPlaceholder,
		`{"port":443}`: `{"port":443}`,
		`{"private_key":"k","public_key":"p","short_id":"s"}`: `{"private_key":"********","public_key":"p","short_id":"s"}`,
		`{"private_key":""}`: `{"private_key":""}`,
		`{"peers":[{"preshared_key":"x"}],"keys":["a"],"inner":{"password":"pw"}}`: `{"inner":{"password":"********"},"keys":"********","peers":[{"preshared_key":"********"}]}`,
		`{"z":1, "private_key":"k", "n":1.50}`:                                     `{"n":1.50,"private_key":"********","z":1}`,
	}
	for document, want := range cases {
		got := RedactNodeSecrets(document)
		if got != want {
			t.Fatalf("RedactNodeSecrets(%q) = %q, want %q", document, got, want)
		}
		if again := RedactNodeSecrets(got); again != got {
			t.Fatalf("redacting %q again answered %q", got, again)
		}
	}
}

func TestIsNodeSecretKey(t *testing.T) {
	for _, key := range []string{"private_key", "privateKey", "server_key", "psk", "auth", "obfs-password", "token", "seed", "preshared_key"} {
		if !IsNodeSecretKey(key) {
			t.Fatalf("%s is a secret key", key)
		}
	}
	for _, key := range []string{"public_key", "short_id", "key_file", "cert_path", "port", ""} {
		if IsNodeSecretKey(key) {
			t.Fatalf("%s is not a secret key", key)
		}
	}
}
