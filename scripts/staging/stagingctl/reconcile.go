package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/sdk/shadowsample"
	"github.com/AnixOps/anix-control/sdk/v2compat"
	"gorm.io/gorm"
)

type reconcileOptions struct {
	LegacyURL, NativeURL string
	DBHost               string
	DBUser, DBPassword   string
	Seed                 int64
}

// WriteCase is one write request replayed on both twins.
type WriteCase struct {
	Label        string   `json:"label"`
	Persona      string   `json:"persona"`
	Method       string   `json:"method"`
	Path         string   `json:"path"`
	LegacyStatus int      `json:"legacy_status"`
	NativeStatus int      `json:"native_status"`
	Equal        bool     `json:"equal"`
	Diff         []string `json:"diff,omitempty"`
	Error        string   `json:"error,omitempty"`
}

// WriteResult is the outcome of one write route.
type WriteResult struct {
	RouteID   string      `json:"route_id"`
	PackageID string      `json:"package_id"`
	Method    string      `json:"method"`
	Path      string      `json:"path"`
	Cases     []WriteCase `json:"cases"`
	Pass      bool        `json:"pass"`
	Skip      string      `json:"skip,omitempty"`
	Reasons   []string    `json:"reasons,omitempty"`
}

// TableResult compares one table of the two twins.
type TableResult struct {
	PackageID   string            `json:"package_id"`
	Table       string            `json:"table"`
	Key         string            `json:"key"`
	Ignore      map[string]string `json:"ignore"`
	LegacyRows  int               `json:"legacy_rows"`
	NativeRows  int               `json:"native_rows"`
	ChangedRows int               `json:"changed_rows_vs_seed"`
	Differences int               `json:"differences"`
	Samples     []string          `json:"samples,omitempty"`
	Equal       bool              `json:"equal"`
	Error       string            `json:"error,omitempty"`
}

// WritesReport is writes.json.
type WritesReport struct {
	Batch      int           `json:"batch"`
	StartedAt  time.Time     `json:"started_at"`
	FinishedAt time.Time     `json:"finished_at"`
	Routes     []WriteResult `json:"routes"`
	Tables     []TableResult `json:"tables"`
	Personas   []string      `json:"persona_notes,omitempty"`
	Pass       bool          `json:"pass"`
}

func methodRank(method string) int {
	switch method {
	case http.MethodPost:
		return 0
	case http.MethodPut, http.MethodPatch:
		return 1
	case http.MethodDelete:
		return 2
	}
	return 3
}

func reconcileWrites(ctx context.Context, common commonFlags, options reconcileOptions) error {
	batch, err := batchByNumber(common.batch)
	if err != nil {
		return err
	}
	routes, err := batchRoutes(batch)
	if err != nil {
		return err
	}
	world, err := loadWorld(manifestPath)
	if err != nil {
		return err
	}
	legacy, native := newClient(options.LegacyURL), newClient(options.NativeURL)
	for _, client := range []*Client{legacy, native} {
		if err := client.WaitReady(ctx, 3*time.Minute); err != nil {
			return err
		}
	}
	if err := prepareNativeTwin(ctx, native, common, batch, routes); err != nil {
		return err
	}
	legacyTokens, unavailable := personaTokens(ctx, legacy, world, common)
	nativeTokens, _ := personaTokens(ctx, native, world, common)
	report := WritesReport{Batch: batch.Number, StartedAt: time.Now().UTC(), Personas: unavailable}

	var writes []CatalogRoute
	for _, route := range routes {
		if !route.Read() {
			writes = append(writes, route)
		}
	}
	sort.SliceStable(writes, func(i, j int) bool { return methodRank(writes[i].Method) < methodRank(writes[j].Method) })
	for _, route := range writes {
		result := WriteResult{RouteID: route.RouteID, PackageID: route.PackageID, Method: route.Method, Path: route.Path}
		spec, ok := routeSpecs[route.RouteID]
		switch {
		case !ok:
			result.Skip = "no request generator"
		case spec.Skip != "":
			result.Skip = spec.Skip
		case spec.Writes == nil:
			result.Skip = "no write generator"
		}
		if result.Skip == "" {
			for _, request := range spec.Writes(world) {
				if _, ok := legacyTokens[request.Persona]; !ok && request.Persona != Anon {
					continue
				}
				result.Cases = append(result.Cases, replayWrite(ctx, legacy, native, world, common, legacyTokens, nativeTokens, route.Method, request))
			}
			if len(result.Cases) == 0 {
				result.Skip = "no usable request"
			}
		}
		if result.Skip != "" {
			result.Reasons = append(result.Reasons, "not rehearsed: "+result.Skip)
		}
		for _, c := range result.Cases {
			if !c.Equal {
				result.Reasons = append(result.Reasons, fmt.Sprintf("%s: legacy %d, native %d %s", c.Label, c.LegacyStatus, c.NativeStatus, strings.Join(c.Diff, "; ")))
			}
		}
		result.Pass = len(result.Reasons) == 0
		report.Routes = append(report.Routes, result)
	}
	// Side effects of a handler (a notification, an order update) may
	// finish after its answer.
	time.Sleep(3 * time.Second)
	report.Tables = compareTables(ctx, batch, options)
	report.Pass = true
	for _, route := range report.Routes {
		report.Pass = report.Pass && route.Pass
	}
	for _, table := range report.Tables {
		report.Pass = report.Pass && table.Equal
	}
	report.FinishedAt = time.Now().UTC()
	return writeJSON(filepath.Join(batchDir(common), "writes.json"), report)
}

// prepareNativeTwin switches every native-flagged route of the batch to
// native on the throwaway twin and waits until the hosts run them.
func prepareNativeTwin(ctx context.Context, native *Client, common commonFlags, batch Batch, routes []CatalogRoute) error {
	token, err := native.Login(ctx, common.adminEmail, common.adminPassword)
	if err != nil {
		return err
	}
	byPackage := map[string][]string{}
	for _, route := range routes {
		byPackage[route.PackageID] = append(byPackage[route.PackageID], route.RouteID)
	}
	for _, packageID := range batch.Packages {
		if len(byPackage[packageID]) == 0 {
			continue
		}
		if err := native.setModes(ctx, token, packageID, byPackage[packageID], "native",
			fmt.Sprintf("staging reconciliation twin of batch %d (throwaway database copy)", batch.Number), true); err != nil {
			return err
		}
	}
	deadline := time.Now().Add(90 * time.Second)
	for {
		var pending []string
		for _, packageID := range batch.Packages {
			entries, err := native.routeModes(ctx, token, packageID)
			if err != nil {
				return err
			}
			for _, routeID := range byPackage[packageID] {
				entry := entries[routeID]
				if entry.Host == nil || entry.Host.Effective != "native" {
					pending = append(pending, routeID)
				}
			}
		}
		if len(pending) == 0 {
			fmt.Printf("native twin: %d routes run natively\n", len(routes))
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("native twin: routes not native after 90s: %s", strings.Join(pending, ", "))
		}
		time.Sleep(3 * time.Second)
	}
}

func replayWrite(ctx context.Context, legacy, native *Client, world *World, common commonFlags, legacyTokens, nativeTokens map[string]string, method string, request Req) WriteCase {
	c := WriteCase{Label: request.Label, Persona: request.Persona, Method: method, Path: request.Path}
	left, leftErr := sendAs(ctx, legacy, world, common, legacyTokens, request, method)
	right, rightErr := sendAs(ctx, native, world, common, nativeTokens, request, method)
	if leftErr != nil || rightErr != nil {
		c.Error = fmt.Sprintf("legacy: %v; native: %v", leftErr, rightErr)
		return c
	}
	c.LegacyStatus, c.NativeStatus = left.Status, right.Status
	leftBody, rightBody := maskJSON(left.Body, request.Mask), maskJSON(right.Body, request.Mask)
	c.Equal = left.Status == right.Status && v2compat.EqualForCompare(leftBody, rightBody)
	if !c.Equal {
		entries, truncated := shadowsample.Diff(leftBody, rightBody)
		for i, entry := range entries {
			if i == 6 {
				break
			}
			c.Diff = append(c.Diff, fmt.Sprintf("%s %s (legacy %s, native %s)", entry.Kind, entry.Path, clip(entry.Legacy), clip(entry.Native)))
		}
		if truncated || len(entries) > 6 {
			c.Diff = append(c.Diff, fmt.Sprintf("... %d differences", len(entries)))
		}
	}
	return c
}

func clip(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "-"
	}
	value := string(raw)
	if len(value) > 80 {
		value = value[:80] + "..."
	}
	return value
}

// maskJSON replaces the values at the dotted paths with "(masked)".
func maskJSON(body []byte, paths []string) []byte {
	if len(paths) == 0 {
		return body
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return body
	}
	for _, path := range paths {
		maskPath(value, strings.Split(path, "."))
	}
	masked, err := json.Marshal(value)
	if err != nil {
		return body
	}
	return masked
}

func maskPath(value any, segments []string) {
	if len(segments) == 0 {
		return
	}
	head, rest := segments[0], segments[1:]
	switch node := value.(type) {
	case map[string]any:
		for key, child := range node {
			if head != "*" && key != head {
				continue
			}
			if len(rest) == 0 {
				node[key] = "(masked)"
			} else {
				maskPath(child, rest)
			}
		}
	case []any:
		for index, child := range node {
			if head != "*" && head != fmt.Sprint(index) {
				continue
			}
			if len(rest) == 0 {
				node[index] = "(masked)"
			} else {
				maskPath(child, rest)
			}
		}
	}
}

// compareTables compares the batch's tables row by row on the two twins,
// without the ignored columns, and counts how many rows the replay changed
// against the seeded template.
func compareTables(ctx context.Context, batch Batch, options reconcileOptions) []TableResult {
	open := func(database string) (*gorm.DB, error) {
		return openDatabase(options.DBHost, options.DBUser, options.DBPassword, database)
	}
	legacyDB, legacyErr := open("recon_legacy")
	nativeDB, nativeErr := open("recon_native")
	seedDB, seedErr := open("staging_seed")
	var results []TableResult
	for _, packageID := range batch.Packages {
		for _, table := range reconTables[packageID] {
			result := TableResult{PackageID: packageID, Table: table.Table, Key: table.Key, Ignore: table.Ignore}
			if result.Key == "" {
				result.Key = "id"
			}
			if legacyErr != nil || nativeErr != nil || seedErr != nil {
				result.Error = fmt.Sprintf("open databases: %v %v %v", legacyErr, nativeErr, seedErr)
				results = append(results, result)
				continue
			}
			left, err := tableRows(ctx, legacyDB, table, result.Key)
			if err == nil {
				var right, seeded map[string]string
				if right, err = tableRows(ctx, nativeDB, table, result.Key); err == nil {
					if seeded, err = tableRows(ctx, seedDB, table, result.Key); err == nil {
						diffRows(&result, left, right, seeded)
					}
				}
			}
			if err != nil {
				result.Error = err.Error()
			}
			result.Equal = result.Error == "" && result.Differences == 0
			results = append(results, result)
		}
	}
	return results
}

// tableRows returns key -> JSON row without the ignored columns.
func tableRows(ctx context.Context, db *gorm.DB, table TableSpec, key string) (map[string]string, error) {
	ignore := make([]string, 0, len(table.Ignore))
	for column := range table.Ignore {
		ignore = append(ignore, "'"+strings.ReplaceAll(column, "'", "''")+"'")
	}
	sort.Strings(ignore)
	removal := ""
	if len(ignore) > 0 {
		removal = " - ARRAY[" + strings.Join(ignore, ",") + "]::text[]"
	}
	keyColumns := strings.Split(key, ",")
	keyExpr := make([]string, len(keyColumns))
	for i, column := range keyColumns {
		keyExpr[i] = fmt.Sprintf("COALESCE(t.%q::text, '')", strings.TrimSpace(column))
	}
	where := ""
	if table.Where != "" {
		where = " WHERE " + table.Where
	}
	query := fmt.Sprintf(`SELECT concat_ws('/', %s) AS k, (to_jsonb(t)%s)::text AS v FROM %q t%s`,
		strings.Join(keyExpr, ", "), removal, table.Table, where)
	var rows []struct {
		K string
		V string
	}
	if err := db.WithContext(ctx).Raw(query).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("%s: %w", table.Table, err)
	}
	out := make(map[string]string, len(rows))
	for _, row := range rows {
		out[row.K] = row.V
	}
	return out, nil
}

func diffRows(result *TableResult, left, right, seeded map[string]string) {
	result.LegacyRows, result.NativeRows = len(left), len(right)
	keys := map[string]bool{}
	for key := range left {
		keys[key] = true
	}
	for key := range right {
		keys[key] = true
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	for _, key := range ordered {
		l, lok := left[key]
		r, rok := right[key]
		if s, sok := seeded[key]; !sok || (lok && s != l) || (rok && s != r) {
			result.ChangedRows++
		}
		if lok && rok && l == r {
			continue
		}
		result.Differences++
		if len(result.Samples) >= 8 {
			continue
		}
		switch {
		case !lok:
			result.Samples = append(result.Samples, fmt.Sprintf("%s=%s only on native: %s", result.Key, key, clipString(r)))
		case !rok:
			result.Samples = append(result.Samples, fmt.Sprintf("%s=%s only on legacy: %s", result.Key, key, clipString(l)))
		default:
			result.Samples = append(result.Samples, fmt.Sprintf("%s=%s columns differ: %s", result.Key, key, columnDiff(l, r)))
		}
	}
}

func columnDiff(left, right string) string {
	var l, r map[string]any
	if json.Unmarshal([]byte(left), &l) != nil || json.Unmarshal([]byte(right), &r) != nil {
		return "unparsable rows"
	}
	var parts []string
	for column, value := range l {
		if fmt.Sprint(value) != fmt.Sprint(r[column]) {
			parts = append(parts, fmt.Sprintf("%s: legacy %s, native %s", column, clipString(fmt.Sprint(value)), clipString(fmt.Sprint(r[column]))))
		}
	}
	for column, value := range r {
		if _, ok := l[column]; !ok {
			parts = append(parts, fmt.Sprintf("%s: native only %s", column, clipString(fmt.Sprint(value))))
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, "; ")
}

func clipString(value string) string {
	if len(value) > 120 {
		return value[:120] + "..."
	}
	return value
}
