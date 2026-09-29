package database

import (
	"math/rand"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// isolatePostgresEnv keeps PG* environment variables and ~/.pgpass from
// leaking into pgconn.ParseConfig results.
func isolatePostgresEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"PGHOST", "PGPORT", "PGUSER", "PGPASSWORD", "PGDATABASE", "PGSERVICE", "PGSERVICEFILE", "PGSSLMODE", "PGTZ", "PGOPTIONS"} {
		t.Setenv(key, "")
	}
	t.Setenv("PGPASSFILE", filepath.Join(t.TempDir(), "missing.pgpass"))
}

func parsePostgresDSN(t *testing.T, cfg *config.DatabaseConfig) *pgconn.Config {
	t.Helper()
	dsn := PostgresDSN(cfg)
	parsed, err := pgconn.ParseConfig(dsn)
	require.NoError(t, err, "dsn: %s", dsn)
	return parsed
}

func TestPostgresDSNKeepsFixedSettings(t *testing.T) {
	assert.Equal(t,
		`sslmode=disable TimeZone=Asia/Shanghai host='db.internal' port='5432' user='anix' password='secret' dbname='anix_control'`,
		PostgresDSN(&config.DatabaseConfig{Host: "db.internal", Port: 5432, Username: "anix", Password: "secret", Database: "anix_control"}),
	)

	isolatePostgresEnv(t)
	parsed := parsePostgresDSN(t, &config.DatabaseConfig{Host: "db.internal", Port: 5432, Username: "anix", Password: "secret", Database: "anix_control"})
	assert.Nil(t, parsed.TLSConfig, "sslmode=disable must be preserved")
	assert.Equal(t, "Asia/Shanghai", parsed.RuntimeParams["TimeZone"])

	// gorm.io/driver/postgres takes the first `TimeZone=` it finds in the raw
	// DSN; the fixed setting must precede any user-controlled value.
	dsn := PostgresDSN(&config.DatabaseConfig{Host: "h", Username: "u TimeZone=UTC", Password: "p TimeZone=UTC", Database: "d"})
	assert.True(t, strings.HasPrefix(dsn, "sslmode=disable TimeZone=Asia/Shanghai "), dsn)
}

func TestPostgresDSNParsesWithPgconn(t *testing.T) {
	isolatePostgresEnv(t)
	defaults, err := pgconn.ParseConfig("")
	require.NoError(t, err)

	tests := []struct {
		name string
		cfg  config.DatabaseConfig
		host string
		port uint16
	}{
		{
			name: "empty password keeps dbname",
			cfg:  config.DatabaseConfig{Host: "127.0.0.1", Port: 5432, Username: "anix", Password: "", Database: "anix_control"},
			host: "127.0.0.1", port: 5432,
		},
		{
			name: "empty host falls back to driver default",
			cfg:  config.DatabaseConfig{Host: "", Port: 6543, Username: "anix", Password: "secret", Database: "anix_control"},
			host: defaults.Host, port: 6543,
		},
		{
			name: "zero port falls back to driver default",
			cfg:  config.DatabaseConfig{Host: "db.internal", Port: 0, Username: "anix", Password: "secret", Database: "anix_control"},
			host: "db.internal", port: defaults.Port,
		},
		{
			name: "values with spaces and equals signs",
			cfg:  config.DatabaseConfig{Host: "db.internal", Port: 5432, Username: "anix user", Password: "pass word dbname=other", Database: "anix control"},
			host: "db.internal", port: 5432,
		},
		{
			name: "single quotes",
			cfg:  config.DatabaseConfig{Host: "db.internal", Port: 5432, Username: "o'brien", Password: "it's'", Database: "'quoted'"},
			host: "db.internal", port: 5432,
		},
		{
			name: "backslashes",
			cfg:  config.DatabaseConfig{Host: "db.internal", Port: 5432, Username: `dom\user`, Password: `a\b\\c\'d\`, Database: `db\`},
			host: "db.internal", port: 5432,
		},
		{
			name: "unix socket host",
			cfg:  config.DatabaseConfig{Host: "/var/run/postgresql", Port: 5432, Username: "anix", Password: "", Database: "anix_control"},
			host: "/var/run/postgresql", port: 5432,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parsed := parsePostgresDSN(t, &tc.cfg)
			assert.Equal(t, tc.host, parsed.Host)
			assert.Equal(t, tc.port, parsed.Port)
			assert.Equal(t, tc.cfg.Username, parsed.User)
			assert.Equal(t, tc.cfg.Password, parsed.Password)
			assert.Equal(t, tc.cfg.Database, parsed.Database)
			assert.Equal(t, "Asia/Shanghai", parsed.RuntimeParams["TimeZone"])
		})
	}
}

func TestPostgresDSNRoundTripsArbitraryValues(t *testing.T) {
	isolatePostgresEnv(t)
	alphabet := []rune{'a', ' ', '\'', '\\', '=', '"', '\t', 'é'}
	rng := rand.New(rand.NewSource(1)) // #nosec G404 -- deterministic test input, not security sensitive.
	randomValue := func() string {
		value := make([]rune, 1+rng.Intn(8))
		for i := range value {
			value[i] = alphabet[rng.Intn(len(alphabet))]
		}
		return string(value)
	}
	for i := 0; i < 500; i++ {
		cfg := config.DatabaseConfig{Host: "db.internal", Port: 5432, Username: randomValue(), Password: randomValue(), Database: randomValue()}
		parsed := parsePostgresDSN(t, &cfg)
		require.Equal(t, cfg.Username, parsed.User, "dsn: %s", PostgresDSN(&cfg))
		require.Equal(t, cfg.Password, parsed.Password, "dsn: %s", PostgresDSN(&cfg))
		require.Equal(t, cfg.Database, parsed.Database, "dsn: %s", PostgresDSN(&cfg))
	}
}
