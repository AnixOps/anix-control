package main

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// DumpTable holds a parsed table's column order and rows.
// Each row maps column name -> raw value (nil for SQL NULL).
type DumpTable struct {
	Name    string
	Columns []string
	Rows    []map[string]*string
}

var createTableRe = regexp.MustCompile("^CREATE TABLE `([^`]+)` \\(")
var columnLineRe = regexp.MustCompile("^\\s*`([^`]+)`\\s+\\S")
var insertIntoRe = regexp.MustCompile("^INSERT INTO `([^`]+)` VALUES (.+);$")

// ParseDump reads a mysqldump file (plain .sql or gzip .sql.gz) and returns
// every table's column list and rows.
func ParseDump(path string) (map[string]*DumpTable, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open dump: %w", err)
	}
	defer f.Close()

	var reader *bufio.Reader
	if strings.HasSuffix(path, ".gz") {
		gz, err := gzip.NewReader(f)
		if err != nil {
			return nil, fmt.Errorf("gzip reader: %w", err)
		}
		defer gz.Close()
		reader = bufio.NewReaderSize(gz, 1<<20)
	} else {
		reader = bufio.NewReaderSize(f, 1<<20)
	}

	tables := make(map[string]*DumpTable)

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 1<<20), 64<<20) // INSERT lines can be very long
	var currentTable *DumpTable

	for scanner.Scan() {
		line := scanner.Text()

		if m := createTableRe.FindStringSubmatch(line); m != nil {
			currentTable = &DumpTable{Name: m[1]}
			tables[m[1]] = currentTable
			continue
		}

		if currentTable != nil {
			if strings.HasPrefix(strings.TrimSpace(line), ")") {
				currentTable = nil
				continue
			}
			if m := columnLineRe.FindStringSubmatch(line); m != nil {
				currentTable.Columns = append(currentTable.Columns, m[1])
				continue
			}
			continue
		}

		if m := insertIntoRe.FindStringSubmatch(line); m != nil {
			table := tables[m[1]]
			if table == nil {
				return nil, fmt.Errorf("INSERT INTO %q before its CREATE TABLE", m[1])
			}
			tuples, err := splitTuples(m[2])
			if err != nil {
				return nil, fmt.Errorf("table %s: %w", m[1], err)
			}
			for _, tuple := range tuples {
				values, err := splitTupleValues(tuple)
				if err != nil {
					return nil, fmt.Errorf("table %s: %w", m[1], err)
				}
				if len(values) != len(table.Columns) {
					return nil, fmt.Errorf("table %s: row has %d values, expected %d columns", m[1], len(values), len(table.Columns))
				}
				row := make(map[string]*string, len(table.Columns))
				for i, col := range table.Columns {
					row[col] = values[i]
				}
				table.Rows = append(table.Rows, row)
			}
			continue
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan dump: %w", err)
	}

	return tables, nil
}

// splitTuples splits "(...),(...),(...)" into its individual "(...)" tuples,
// respecting quoted strings so commas/parens inside string values don't
// confuse the split.
func splitTuples(s string) ([]string, error) {
	var tuples []string
	depth := 0
	inQuote := false
	start := -1

	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case inQuote:
			if c == '\\' {
				i++ // skip escaped char
				continue
			}
			if c == '\'' {
				inQuote = false
			}
		case c == '\'':
			inQuote = true
		case c == '(':
			if depth == 0 {
				start = i + 1
			}
			depth++
		case c == ')':
			depth--
			if depth == 0 {
				if start < 0 {
					return nil, fmt.Errorf("unbalanced parens at byte %d", i)
				}
				tuples = append(tuples, s[start:i])
				start = -1
			}
			if depth < 0 {
				return nil, fmt.Errorf("unbalanced parens at byte %d", i)
			}
		}
	}
	if inQuote || depth != 0 {
		return nil, fmt.Errorf("unterminated tuple list")
	}
	return tuples, nil
}

// splitTupleValues splits a single tuple's comma-separated values, honoring
// quoted strings, escapes, and the bare NULL keyword.
func splitTupleValues(s string) ([]*string, error) {
	var values []*string
	var buf strings.Builder
	inQuote := false
	quoted := false // whether the current value was quoted (distinguishes '' from NULL)

	flush := func() {
		if !quoted && buf.String() == "NULL" {
			values = append(values, nil)
		} else {
			v := unescapeSQLString(buf.String())
			values = append(values, &v)
		}
		buf.Reset()
		quoted = false
	}

	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case inQuote:
			if c == '\\' && i+1 < len(s) {
				buf.WriteByte(c)
				i++
				buf.WriteByte(s[i])
				continue
			}
			if c == '\'' {
				inQuote = false
				continue
			}
			buf.WriteByte(c)
		case c == '\'':
			inQuote = true
			quoted = true
		case c == ',':
			flush()
		default:
			buf.WriteByte(c)
		}
	}
	flush()

	if inQuote {
		return nil, fmt.Errorf("unterminated quoted value")
	}
	return values, nil
}

// unescapeSQLString undoes mysqldump's backslash escaping within a quoted
// string value (\\, \', \", \n, \r, \0).
func unescapeSQLString(s string) string {
	if !strings.Contains(s, "\\") {
		return s
	}
	var out strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\\' && i+1 < len(s) {
			i++
			switch s[i] {
			case 'n':
				out.WriteByte('\n')
			case 'r':
				out.WriteByte('\r')
			case '0':
				out.WriteByte(0)
			case 't':
				out.WriteByte('\t')
			default:
				out.WriteByte(s[i])
			}
			continue
		}
		out.WriteByte(c)
	}
	return out.String()
}
