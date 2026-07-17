package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// row is a single parsed dump row: column name -> raw value (nil = SQL NULL).
type row = map[string]*string

func str(r row, col string) string {
	if v := r[col]; v != nil {
		return *v
	}
	return ""
}

func strPtr(r row, col string) *string {
	v := r[col]
	if v == nil || *v == "" {
		return nil
	}
	cp := *v
	return &cp
}

func mustInt64(r row, col string) (int64, error) {
	v := r[col]
	if v == nil || *v == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(*v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("column %s: %w", col, err)
	}
	return n, nil
}

func int64Ptr(r row, col string) *int64 {
	v := r[col]
	if v == nil || *v == "" {
		return nil
	}
	n, err := strconv.ParseInt(*v, 10, 64)
	if err != nil {
		return nil
	}
	return &n
}

func intPtr(r row, col string) *int {
	v := r[col]
	if v == nil || *v == "" {
		return nil
	}
	n, err := strconv.Atoi(*v)
	if err != nil {
		return nil
	}
	return &n
}

func uintPtr(r row, col string) *uint {
	v := r[col]
	if v == nil || *v == "" {
		return nil
	}
	n, err := strconv.ParseUint(*v, 10, 64)
	if err != nil {
		return nil
	}
	u := uint(n)
	return &u
}

func mustInt(r row, col string) (int, error) {
	v := r[col]
	if v == nil || *v == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(*v)
	if err != nil {
		return 0, fmt.Errorf("column %s: %w", col, err)
	}
	return n, nil
}

func mustUint(r row, col string) (uint, error) {
	v := r[col]
	if v == nil || *v == "" {
		return 0, nil
	}
	n, err := strconv.ParseUint(*v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("column %s: %w", col, err)
	}
	return uint(n), nil
}

func mustFloat64(r row, col string) (float64, error) {
	v := r[col]
	if v == nil || *v == "" {
		return 0, nil
	}
	n, err := strconv.ParseFloat(*v, 64)
	if err != nil {
		return 0, fmt.Errorf("column %s: %w", col, err)
	}
	return n, nil
}

// unixTime converts a dump column holding a unix-seconds integer (the old
// schema's created_at/updated_at convention) into a time.Time. A missing or
// empty value maps to the zero time.
func unixTime(r row, col string) (time.Time, error) {
	v := r[col]
	if v == nil || *v == "" {
		return time.Time{}, nil
	}
	n, err := strconv.ParseInt(*v, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("column %s: %w", col, err)
	}
	return time.Unix(n, 0), nil
}

// parseJSONIntArray parses a dump column holding a JSON array of numeric
// strings (e.g. `["1","2"]`, the old schema's convention for group_id/
// route_id columns) into a slice of uints. A missing or empty value returns
// nil with no error.
func parseJSONIntArray(r row, col string) ([]uint, error) {
	v := r[col]
	if v == nil || *v == "" {
		return nil, nil
	}
	var raw []string
	if err := json.Unmarshal([]byte(*v), &raw); err != nil {
		return nil, fmt.Errorf("column %s: %w", col, err)
	}
	ids := make([]uint, 0, len(raw))
	for _, s := range raw {
		n, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("column %s: %w", col, err)
		}
		ids = append(ids, uint(n))
	}
	return ids, nil
}
