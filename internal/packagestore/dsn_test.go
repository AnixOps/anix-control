package packagestore

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPackageDSNKeepsOnlyServerSettings(t *testing.T) {
	kernel := `sslmode=verify-full TimeZone=UTC host='db.internal' port='5432' user='kernel' password='s3cr\'et x' dbname='v2board' sslcert=/etc/kernel.crt sslkey=/etc/kernel.key passfile=/root/.pgpass options='-c role=postgres'`
	dsn, err := packageDSN(kernel, "anix_pkg_knowledge", "p'w", "anix-pkg-knowledge")
	require.NoError(t, err)
	require.Equal(t, `TimeZone=UTC dbname='v2board' host='db.internal' port='5432' sslmode='verify-full' user='anix_pkg_knowledge' password='p\'w' application_name='anix-pkg-knowledge'`, dsn)
	for _, leaked := range []string{"kernel", "s3cr", "sslcert", "sslkey", "passfile", "options"} {
		require.NotContains(t, strings.ReplaceAll(dsn, "anix-pkg-knowledge", ""), leaked)
	}

	settings, err := parseDSN(dsn)
	require.NoError(t, err)
	require.Equal(t, "p'w", settings["password"])
	require.Equal(t, "UTC", settings["TimeZone"])

	dsn, err = packageDSN(`TimeZone='Bad Zone' host=db`, "u", "p", "a")
	require.NoError(t, err)
	require.Equal(t, `host='db' user='u' password='p' application_name='a'`, dsn)
	require.Equal(t, "anix_pkg_knowledge", settings["user"])
}

func TestPackageDSNFromURL(t *testing.T) {
	dsn, err := packageDSN("postgresql://kernel:secret@db1:5433,[::1]/v2board?sslmode=require&sslcert=/k.crt&connect_timeout=5", "anix_pkg_ticket", "pw", "anix-pkg-ticket")
	require.NoError(t, err)
	require.Equal(t, `connect_timeout='5' dbname='v2board' host='db1,::1' port='5433,' sslmode='require' user='anix_pkg_ticket' password='pw' application_name='anix-pkg-ticket'`, dsn)
	require.NotContains(t, dsn, "secret")
}

func TestParseKeywordDSN(t *testing.T) {
	settings, err := parseDSN(`host = localhost  dbname=a\ b password='' port=5432`)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"host": "localhost", "dbname": "a b", "password": "", "port": "5432"}, settings)

	for _, invalid := range []string{"host", "host=localhost dbname", "password='open", "=x"} {
		_, err := parseDSN(invalid)
		require.Error(t, err, invalid)
	}
}

func TestScramVerifierFormat(t *testing.T) {
	verifier, err := scramVerifier("pencil", []byte("0123456789abcdef"), 4096)
	require.NoError(t, err)
	require.Regexp(t, `^SCRAM-SHA-256\$4096:MDEyMzQ1Njc4OWFiY2RlZg==\$[A-Za-z0-9+/]{43}=:[A-Za-z0-9+/]{43}=$`, verifier)

	password, verifier, err := newRolePassword()
	require.NoError(t, err)
	require.Len(t, password, 32)
	require.NotContains(t, verifier, password)
}
