package migrations

import (
	"bufio"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

// The dialects a step is written for. Until PostgreSQL is removed every
// SQL step has one file per dialect.
const (
	DialectSQLite   = "sqlite"
	DialectPostgres = "postgres"
)

// Dialects lists the dialects every SQL step must have a file for.
var Dialects = []string{DialectSQLite, DialectPostgres}

// Step is one numbered migration.
type Step struct {
	// Version is the step's number: 1, 2, 3 ... without gaps.
	Version int
	// Name is the step's name, the file name without the number and the
	// dialect: "baseline" for 0001_baseline.sqlite.sql.
	Name string
	// SQL holds the step's statements by dialect.
	SQL map[string]string
	// Go, when set, runs after the step's SQL in the same transaction. It
	// transforms data the statements cannot (see doc.go).
	Go func(ctx context.Context, tx *gorm.DB, dialect string) error
	// MinProgramVersion, when set, is the oldest program version that may
	// open a database once this step is applied, even if it knows the
	// step: the step changes data in a way older code misreads.
	MinProgramVersion string
}

// ID is the step's number and name, as files and messages name it:
// "0001_baseline".
func (s Step) ID() string { return fmt.Sprintf("%04d_%s", s.Version, s.Name) }

// Checksum is what v5_schema_migration records for the step on dialect:
// the SHA-256 of the dialect's statements (line endings normalized), plus
// a marker when a Go function follows them. A Go function's code is not
// hashed: like a SQL file, a step is never changed once released.
func (s Step) Checksum(dialect string) string {
	sum := sha256.Sum256([]byte(strings.ReplaceAll(s.SQL[dialect], "\r\n", "\n")))
	checksum := "sha256:" + hex.EncodeToString(sum[:])
	if s.Go != nil {
		checksum += "+go"
	}
	return checksum
}

//go:embed sql/*.sql
var sqlFiles embed.FS

// goSteps are the Go functions of steps, by version. A Go-only step has a
// SQL file per dialect with only comments.
var goSteps = map[int]func(ctx context.Context, tx *gorm.DB, dialect string) error{}

// minProgramVersions are the MinProgramVersion of steps, by version.
var minProgramVersions = map[int]string{}

// Steps answers Control's migrations in order.
func Steps() ([]Step, error) {
	return loadSteps(sqlFiles, "sql", goSteps, minProgramVersions)
}

var stepFile = regexp.MustCompile(`^(\d{4})_([a-z0-9_]+)\.([a-z]+)\.sql$`)

func loadSteps(files fs.FS, dir string, goFuncs map[int]func(context.Context, *gorm.DB, string) error, minVersions map[int]string) ([]Step, error) {
	entries, err := fs.ReadDir(files, dir)
	if err != nil {
		return nil, err
	}
	byVersion := map[int]*Step{}
	for _, entry := range entries {
		match := stepFile.FindStringSubmatch(entry.Name())
		if match == nil {
			return nil, fmt.Errorf("migration file %s is not named NNNN_name.dialect.sql", entry.Name())
		}
		version, _ := strconv.Atoi(match[1])
		name, dialect := match[2], match[3]
		if dialect != DialectSQLite && dialect != DialectPostgres {
			return nil, fmt.Errorf("migration file %s: unknown dialect %q", entry.Name(), dialect)
		}
		body, err := fs.ReadFile(files, path.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		step, ok := byVersion[version]
		if !ok {
			step = &Step{Version: version, Name: name, SQL: map[string]string{}}
			byVersion[version] = step
		}
		if step.Name != name {
			return nil, fmt.Errorf("migration %04d has two names: %s and %s", version, step.Name, name)
		}
		step.SQL[dialect] = string(body)
	}
	steps := make([]Step, 0, len(byVersion))
	for _, step := range byVersion {
		for _, dialect := range Dialects {
			if _, ok := step.SQL[dialect]; !ok {
				return nil, fmt.Errorf("migration %s has no %s file", step.ID(), dialect)
			}
		}
		step.Go = goFuncs[step.Version]
		step.MinProgramVersion = minVersions[step.Version]
		steps = append(steps, *step)
	}
	sort.Slice(steps, func(i, j int) bool { return steps[i].Version < steps[j].Version })
	for i, step := range steps {
		if step.Version != i+1 {
			return nil, fmt.Errorf("migrations must be numbered 0001 upwards without gaps; %s follows %d", step.ID(), i)
		}
	}
	for version := range goFuncs {
		if version < 1 || version > len(steps) {
			return nil, fmt.Errorf("migration %04d has a Go function but no SQL files", version)
		}
	}
	return steps, nil
}

// statements splits a step's SQL into statements. A statement ends with a
// semicolon at the end of a line; lines starting with "--" are comments.
func statements(sql string) ([]string, error) {
	var result []string
	var current strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(sql))
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), " \t\r")
		trimmed := strings.TrimSpace(line)
		if current.Len() == 0 && (trimmed == "" || strings.HasPrefix(trimmed, "--")) {
			continue
		}
		if current.Len() > 0 {
			current.WriteByte('\n')
		}
		current.WriteString(line)
		if strings.HasSuffix(trimmed, ";") {
			result = append(result, strings.TrimSuffix(strings.TrimSpace(current.String()), ";"))
			current.Reset()
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(current.String()) != "" {
		return nil, errors.New("the last statement does not end with a semicolon")
	}
	return result, nil
}
