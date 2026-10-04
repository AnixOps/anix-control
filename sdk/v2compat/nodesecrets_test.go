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

func TestKeepNodeSecretsRestoresPlaceholders(t *testing.T) {
	stored := `{"private_key":"k","peers":[{"preshared_key":"p"}],"name":"n"}`
	cases := []struct{ incoming, stored, want string }{
		{NodeSecretPlaceholder, stored, stored},
		{`{"name":"m"}`, stored, `{"name":"m"}`},
		{`{"private_key":"********","name":"m"}`, stored, `{"name":"m","private_key":"k"}`},
		{`{"peers":[{"preshared_key":"********"}]}`, stored, `{"peers":[{"preshared_key":"p"}]}`},
		{`{"private_key":"********"}`, "", `{"private_key":""}`},
		{`{"token":"********"}`, stored, `{"token":""}`},
		{`not json ********`, stored, `not json ********`},
	}
	for _, c := range cases {
		if got := KeepNodeSecrets(c.incoming, c.stored); got != c.want {
			t.Fatalf("KeepNodeSecrets(%q, %q) = %q, want %q", c.incoming, c.stored, got, c.want)
		}
	}
}
