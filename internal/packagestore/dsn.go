package packagestore

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"
)

// packageDSNKeys are the kernel connection settings a package connection
// reuses: where the server is and how to verify it. User, password, passfile,
// client certificates, service files and everything else are dropped, so no
// kernel credential reaches a package.
var packageDSNKeys = map[string]bool{
	"host": true, "hostaddr": true, "port": true, "dbname": true,
	"sslmode": true, "sslrootcert": true, "sslnegotiation": true, "sslsni": true,
	"connect_timeout": true, "target_session_attrs": true, "TimeZone": true,
}

// packageDSN derives the package role's connection string from the kernel's.
// Both keyword/value and postgres:// URL forms are accepted; the result is
// always keyword/value.
func packageDSN(kernelDSN, user, password, applicationName string) (string, error) {
	return packageDSNWithHost(kernelDSN, "", user, password, applicationName)
}

// packageDSNWithHost is packageDSN with the server address replaced by
// hostPort ("host" or "host:port") when it is not empty.
func packageDSNWithHost(kernelDSN, hostPort, user, password, applicationName string) (string, error) {
	settings, err := parseDSN(kernelDSN)
	if err != nil {
		return "", err
	}
	if hostPort = strings.TrimSpace(hostPort); hostPort != "" {
		host, port, err := net.SplitHostPort(hostPort)
		if err != nil {
			host, port = hostPort, ""
		}
		delete(settings, "hostaddr")
		settings["host"] = strings.Trim(host, "[]")
		if port != "" {
			settings["port"] = port
		}
	}
	keys := make([]string, 0, len(settings))
	for key := range settings {
		if packageDSNKeys[key] && key != "TimeZone" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys)+4)
	// gorm.io/driver/postgres reads TimeZone from the raw string, so it goes
	// first and unquoted, and is dropped if it would need quoting.
	if timeZone := settings["TimeZone"]; timeZone != "" && !strings.ContainsAny(timeZone, " \t\r\n'\\=") {
		parts = append(parts, "TimeZone="+timeZone)
	}
	for _, key := range keys {
		parts = append(parts, key+"="+quoteDSNValue(settings[key]))
	}
	parts = append(parts,
		"user="+quoteDSNValue(user),
		"password="+quoteDSNValue(password),
		"application_name="+quoteDSNValue(applicationName),
	)
	return strings.Join(parts, " "), nil
}

var dsnValueEscaper = strings.NewReplacer(`\`, `\\`, `'`, `\'`)

func quoteDSNValue(value string) string {
	return "'" + dsnValueEscaper.Replace(value) + "'"
}

func parseDSN(dsn string) (map[string]string, error) {
	dsn = strings.TrimSpace(dsn)
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		return parseURLDSN(dsn)
	}
	return parseKeywordDSN(dsn)
}

func parseURLDSN(dsn string) (map[string]string, error) {
	// net/url rejects libpq's multi-host authority, so split it off first.
	scheme, rest, _ := strings.Cut(dsn, "://")
	end := strings.IndexAny(rest, "/?")
	if end < 0 {
		end = len(rest)
	}
	authority := rest[:end]
	if at := strings.LastIndex(authority, "@"); at >= 0 {
		authority = authority[at+1:]
	}
	parsed, err := url.Parse(scheme + "://placeholder" + rest[end:])
	if err != nil {
		return nil, errors.New("invalid PostgreSQL connection URL")
	}
	settings := map[string]string{}
	var hosts, ports []string
	for _, hostPort := range strings.Split(authority, ",") {
		if hostPort == "" {
			continue
		}
		host, port := hostPort, ""
		if index := strings.LastIndex(hostPort, ":"); index >= 0 && !strings.HasSuffix(hostPort, "]") {
			host, port = hostPort[:index], hostPort[index+1:]
		}
		hosts = append(hosts, strings.Trim(host, "[]"))
		ports = append(ports, port)
	}
	if len(hosts) > 0 {
		settings["host"] = strings.Join(hosts, ",")
		if strings.Join(ports, "") != "" {
			settings["port"] = strings.Join(ports, ",")
		}
	}
	if database := strings.TrimPrefix(parsed.Path, "/"); database != "" {
		settings["dbname"] = database
	}
	for key, values := range parsed.Query() {
		if len(values) > 0 {
			settings[key] = values[len(values)-1]
		}
	}
	return settings, nil
}

// parseKeywordDSN parses the libpq keyword/value form: key = value pairs
// separated by whitespace, values optionally single-quoted with backslash
// escapes. A later key overrides an earlier one, as in libpq.
func parseKeywordDSN(dsn string) (map[string]string, error) {
	settings := map[string]string{}
	input := []rune(dsn)
	position := 0
	skipSpace := func() {
		for position < len(input) && (input[position] == ' ' || input[position] == '\t' || input[position] == '\n' || input[position] == '\r') {
			position++
		}
	}
	for {
		skipSpace()
		if position >= len(input) {
			return settings, nil
		}
		start := position
		for position < len(input) && input[position] != '=' && input[position] != ' ' && input[position] != '\t' {
			position++
		}
		key := string(input[start:position])
		skipSpace()
		if key == "" || position >= len(input) || input[position] != '=' {
			return nil, fmt.Errorf("invalid PostgreSQL connection string near %q", key)
		}
		position++
		skipSpace()
		var value strings.Builder
		if position < len(input) && input[position] == '\'' {
			position++
			closed := false
			for position < len(input) {
				char := input[position]
				position++
				if char == '\\' && position < len(input) {
					value.WriteRune(input[position])
					position++
					continue
				}
				if char == '\'' {
					closed = true
					break
				}
				value.WriteRune(char)
			}
			if !closed {
				return nil, fmt.Errorf("unterminated quoted value for %q in PostgreSQL connection string", key)
			}
		} else {
			for position < len(input) && input[position] != ' ' && input[position] != '\t' && input[position] != '\n' && input[position] != '\r' {
				char := input[position]
				position++
				if char == '\\' && position < len(input) {
					char = input[position]
					position++
				}
				value.WriteRune(char)
			}
		}
		settings[key] = value.String()
	}
}
