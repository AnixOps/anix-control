package database

import (
	"strconv"
	"strings"

	"github.com/AnixOps/anix-control/v4/internal/config"
)

// PostgresDSN builds a PostgreSQL keyword/value connection string from cfg.
//
// Every value is single-quoted and escaped the way libpq and pgx expect
// (`\` becomes `\\`, `'` becomes `\'`), so empty values and values containing
// spaces, quotes, `=` or backslashes cannot swallow the next keyword. Without
// quoting, an empty password turned `password= dbname=x` into the password
// "dbname=x" and silently connected to the user's default database.
//
// An empty Host and a Port <= 0 are omitted so the driver defaults apply
// (libpq and pgx: the local Unix socket directory or localhost, port 5432),
// matching libpq, which treats an empty host or port as unset.
//
// The fixed `sslmode=disable TimeZone=Asia/Shanghai` settings come first:
// gorm.io/driver/postgres also scans the raw DSN for the first `TimeZone=`, so a
// password that happens to contain that text cannot override the time zone.
func PostgresDSN(cfg *config.DatabaseConfig) string {
	if cfg == nil {
		cfg = &config.DatabaseConfig{}
	}
	parts := []string{"sslmode=disable", "TimeZone=Asia/Shanghai"}
	if cfg.Host != "" {
		parts = append(parts, "host="+quotePostgresDSNValue(cfg.Host))
	}
	if cfg.Port > 0 {
		parts = append(parts, "port="+quotePostgresDSNValue(strconv.Itoa(cfg.Port)))
	}
	parts = append(parts,
		"user="+quotePostgresDSNValue(cfg.Username),
		"password="+quotePostgresDSNValue(cfg.Password),
		"dbname="+quotePostgresDSNValue(cfg.Database),
	)
	return strings.Join(parts, " ")
}

var postgresDSNValueEscaper = strings.NewReplacer(`\`, `\\`, `'`, `\'`)

func quotePostgresDSNValue(value string) string {
	return "'" + postgresDSNValueEscaper.Replace(value) + "'"
}
