package v2compat

import (
	"fmt"
	"sort"
	"strings"
)

// SortColumn is one column an administrator list can be sorted by.
type SortColumn struct {
	// Expr is the ORDER BY expression. The list writes it; a request never
	// supplies any part of it.
	Expr string
	// Nullable marks an expression that can be NULL. NULL then sorts as the
	// largest value on every database (last ascending, first descending),
	// which is PostgreSQL's default and not SQLite's.
	Nullable bool
	// Unique marks an expression no two rows share, so a sort by it needs no
	// tiebreaker.
	Unique bool
}

// SortError is a refused sort or order query value.
type SortError struct {
	// Param is "sort" or "order".
	Param string
	// Value is what the request sent.
	Value string
	// Allowed lists the accepted values, sorted.
	Allowed []string
}

func (e *SortError) Error() string {
	label := "排序字段"
	if e.Param == "order" {
		label = "排序方向"
	}
	value := e.Value
	if runes := []rune(value); len(runes) > 32 {
		value = string(runes[:32]) + "..."
	}
	return fmt.Sprintf("无效的%s: %s (可选: %s)", label, value, strings.Join(e.Allowed, ", "))
}

// ParseListSort validates the sort and order query values of an
// administrator list against the columns the list can sort by, and returns
// the ORDER BY terms. An empty sort returns "" and no error: the list keeps
// its own order and order is not read. Otherwise sort must be a key of
// columns and order "asc" (the default) or "desc", in any case. tiebreaker
// ("id DESC") is appended unless the column is Unique, so a page of equal
// values neither repeats nor skips a row.
//
// The kernel's legacy handlers and the packages' native handlers call it with
// their own columns, so both refuse the same values and answer the same
// order.
func ParseListSort(sortKey, order string, columns map[string]SortColumn, tiebreaker string) (string, error) {
	sortKey = strings.TrimSpace(sortKey)
	if sortKey == "" {
		return "", nil
	}
	column, ok := columns[sortKey]
	if !ok {
		allowed := make([]string, 0, len(columns))
		for key := range columns {
			allowed = append(allowed, key)
		}
		sort.Strings(allowed)
		return "", &SortError{Param: "sort", Value: sortKey, Allowed: allowed}
	}
	direction := "ASC"
	switch strings.ToLower(strings.TrimSpace(order)) {
	case "", "asc":
	case "desc":
		direction = "DESC"
	default:
		return "", &SortError{Param: "order", Value: strings.TrimSpace(order), Allowed: []string{"asc", "desc"}}
	}
	term := column.Expr + " " + direction
	if column.Nullable {
		if direction == "ASC" {
			term += " NULLS LAST"
		} else {
			term += " NULLS FIRST"
		}
	}
	if column.Unique || tiebreaker == "" {
		return term, nil
	}
	return term + ", " + tiebreaker, nil
}
