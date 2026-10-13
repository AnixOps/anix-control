package migrations

import (
	"fmt"
	"regexp"
	"strings"
)

// baselineObjects is the baseline step parsed into the tables and indexes
// it creates, for adoption: a missing table is created with its own
// statement, a missing column is added with its definition from the
// table's statement, a missing index with its statement.
type baselineObjects struct {
	tables  []baselineTable
	indexes []baselineIndex
}

type baselineTable struct {
	name      string
	statement string
	columns   []baselineColumn
}

type baselineColumn struct {
	name       string
	definition string
}

type baselineIndex struct {
	name      string
	table     string
	statement string
}

var (
	createTable = regexp.MustCompile("(?s)^CREATE TABLE [`\"]?([A-Za-z0-9_]+)[`\"]? \\((.*)\\)$")
	createIndex = regexp.MustCompile("^CREATE (?:UNIQUE )?INDEX (?:IF NOT EXISTS )?[`\"]?([A-Za-z0-9_]+)[`\"]? ON [`\"]?([A-Za-z0-9_]+)[`\"]?")
)

func parseBaseline(sql string) (*baselineObjects, error) {
	list, err := statements(sql)
	if err != nil {
		return nil, err
	}
	objects := &baselineObjects{}
	for _, statement := range list {
		flat := strings.TrimSpace(statement)
		if match := createTable.FindStringSubmatch(flat); match != nil {
			columns, err := columnDefinitions(match[2])
			if err != nil {
				return nil, fmt.Errorf("table %s: %w", match[1], err)
			}
			objects.tables = append(objects.tables, baselineTable{name: match[1], statement: statement, columns: columns})
			continue
		}
		if match := createIndex.FindStringSubmatch(flat); match != nil {
			objects.indexes = append(objects.indexes, baselineIndex{name: match[1], table: match[2], statement: statement})
			continue
		}
		return nil, fmt.Errorf("the baseline has a statement adoption does not know: %s", abbreviate(statement))
	}
	return objects, nil
}

// columnDefinitions splits the body of a CREATE TABLE statement at its
// top-level commas and keeps the column definitions (the parts that start
// with a quoted name), not the table constraints.
func columnDefinitions(body string) ([]baselineColumn, error) {
	parts, err := splitTopLevel(body)
	if err != nil {
		return nil, err
	}
	var columns []baselineColumn
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || (part[0] != '`' && part[0] != '"') {
			continue
		}
		end := strings.IndexByte(part[1:], part[0])
		if end < 0 {
			return nil, fmt.Errorf("unterminated column name in %q", part)
		}
		columns = append(columns, baselineColumn{name: part[1 : end+1], definition: part})
	}
	return columns, nil
}

// splitTopLevel splits a parenthesized list at the commas outside quotes
// and parentheses.
func splitTopLevel(body string) ([]string, error) {
	var parts []string
	depth, start := 0, 0
	var quote byte
	for i := 0; i < len(body); i++ {
		c := body[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"' || c == '`':
			quote = c
		case c == '(':
			depth++
		case c == ')':
			depth--
		case c == ',' && depth == 0:
			parts = append(parts, body[start:i])
			start = i + 1
		}
	}
	if quote != 0 || depth != 0 {
		return nil, fmt.Errorf("unbalanced quotes or parentheses")
	}
	return append(parts, body[start:]), nil
}
