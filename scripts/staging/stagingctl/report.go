package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ParitySuite is one packagecompat suite run by rehearse.sh (parity.json).
type ParitySuite struct {
	Name     string  `json:"name"`
	Pass     bool    `json:"pass"`
	Seconds  float64 `json:"seconds"`
	Postgres bool    `json:"postgres"`
	Tail     string  `json:"tail,omitempty"`
}

// BatchReport is report.json.
type BatchReport struct {
	Batch          int           `json:"batch"`
	Name           string        `json:"name"`
	Packages       []string      `json:"packages"`
	GeneratedAt    time.Time     `json:"generated_at"`
	Seed           int64         `json:"seed"`
	Result         string        `json:"result"`
	Reasons        []string      `json:"reasons,omitempty"`
	Reads          *ReadsReport  `json:"reads"`
	Writes         *WritesReport `json:"writes"`
	Parity         []ParitySuite `json:"parity"`
	MissingSpecs   []string      `json:"missing_specs,omitempty"`
	NativeCommands []string      `json:"native_commands,omitempty"`
	Rollback       string        `json:"rollback_command"`
}

func readJSON(path string, out any) error {
	data, err := os.ReadFile(path) // #nosec G304 -- report files of the batch directory
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

func writeReport(common commonFlags) error {
	batch, err := batchByNumber(common.batch)
	if err != nil {
		return err
	}
	routes, err := batchRoutes(batch)
	if err != nil {
		return err
	}
	dir := batchDir(common)
	report := BatchReport{Batch: batch.Number, Name: batch.Name, Packages: batch.Packages, GeneratedAt: time.Now().UTC(),
		MissingSpecs: specCoverage(routes),
		Rollback:     fmt.Sprintf("scripts/staging/rehearse.sh --batch %d --rollback", batch.Number)}
	if world, err := loadWorld(manifestPath); err == nil {
		report.Seed = world.Seed
	}
	var reads ReadsReport
	if err := readJSON(filepath.Join(dir, "reads.json"), &reads); err == nil {
		report.Reads = &reads
	} else {
		report.Reasons = append(report.Reasons, "no read replay result: "+err.Error())
	}
	var writes WritesReport
	if err := readJSON(filepath.Join(dir, "writes.json"), &writes); err == nil {
		report.Writes = &writes
	} else {
		report.Reasons = append(report.Reasons, "no write reconciliation result: "+err.Error())
	}
	if err := readJSON(filepath.Join(dir, "parity.json"), &report.Parity); err != nil {
		report.Reasons = append(report.Reasons, "no parity test result: "+err.Error())
	}
	if len(report.MissingSpecs) > 0 {
		report.Reasons = append(report.Reasons, "routes without a request generator: "+strings.Join(report.MissingSpecs, ", "))
	}
	if report.Reads != nil {
		for _, route := range report.Reads.Routes {
			if !route.Pass {
				report.Reasons = append(report.Reasons, fmt.Sprintf("read %s: %s", route.RouteID, strings.Join(route.Reasons, "; ")))
			}
		}
	}
	if report.Writes != nil {
		for _, route := range report.Writes.Routes {
			if !route.Pass {
				report.Reasons = append(report.Reasons, fmt.Sprintf("write %s: %d of %d requests differ", route.RouteID, countUnequal(route), len(route.Cases)))
			}
		}
		for _, table := range report.Writes.Tables {
			if !table.Equal {
				report.Reasons = append(report.Reasons, fmt.Sprintf("table %s: %d rows differ %s", table.Table, table.Differences, table.Error))
			}
		}
	}
	for _, suite := range report.Parity {
		if !suite.Pass {
			report.Reasons = append(report.Reasons, "parity suite failed: "+suite.Name)
		}
	}
	report.Result = "PASS"
	if len(report.Reasons) > 0 {
		report.Result = "FAIL"
	} else {
		report.NativeCommands = nativeCommands(batch)
	}
	if err := writeJSON(filepath.Join(dir, "report.json"), report); err != nil {
		return err
	}
	markdown := renderMarkdown(report)
	if err := os.WriteFile(filepath.Join(dir, "report.md"), []byte(markdown), 0o600); err != nil {
		return err
	}
	fmt.Printf("wrote %s\n\n", filepath.Join(dir, "report.md"))
	fmt.Print(summaryText(report))
	if report.Result != "PASS" {
		return errors.New("batch " + fmt.Sprint(batch.Number) + ": FAIL")
	}
	return nil
}

func countUnequal(route WriteResult) int {
	n := 0
	for _, c := range route.Cases {
		if !c.Equal {
			n++
		}
	}
	return n
}

func summaryText(report BatchReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "batch %d (%s): %s\n", report.Batch, report.Name, report.Result)
	for _, reason := range report.Reasons {
		fmt.Fprintf(&b, "  - %s\n", reason)
	}
	if len(report.NativeCommands) > 0 {
		fmt.Fprintln(&b, "After the owner signs this batch off (H7), switch it to native with:")
		for _, command := range report.NativeCommands {
			fmt.Fprintf(&b, "  %s\n", command)
		}
	}
	fmt.Fprintf(&b, "Roll back to legacy: %s\n", report.Rollback)
	return b.String()
}

func renderMarkdown(report BatchReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Staging rehearsal: batch %d (%s)\n\n", report.Batch, report.Name)
	fmt.Fprintf(&b, "**Result: %s**\n\n", report.Result)
	fmt.Fprintf(&b, "- Packages: %s\n- Generated: %s\n- Seed: %d (synthetic data only)\n",
		strings.Join(report.Packages, ", "), report.GeneratedAt.Format(time.RFC3339), report.Seed)
	if report.Reads != nil {
		fmt.Fprintf(&b, "- Read thresholds: %d compared requests and %s in shadow per route, %.2f req/s\n",
			report.Reads.MinRequests, report.Reads.MinDuration, report.Reads.Rate)
	}
	b.WriteString("\n")
	if len(report.Reasons) > 0 {
		b.WriteString("## Why it fails\n\n")
		for _, reason := range report.Reasons {
			fmt.Fprintf(&b, "- %s\n", reason)
		}
		b.WriteString("\n")
	}
	if report.Reads != nil {
		b.WriteString("## Read routes (shadow on the rehearsal instance)\n\n")
		b.WriteString("| Route | Sent | Compared | Mismatches | Native errors | Skipped | Shadow for | Result | Samples |\n|---|---:|---:|---:|---:|---:|---|---|---|\n")
		for _, route := range report.Reads.Routes {
			result := "pass"
			if !route.Pass {
				result = "FAIL: " + strings.Join(route.Reasons, "; ")
			}
			fmt.Fprintf(&b, "| `%s` | %d | %d | %d | %d | %d | %s | %s | [API](%s) |\n", route.RouteID, route.Sent, route.Compared,
				route.Mismatches, route.Errors, route.Skipped, orNone(route.ShadowFor), escapePipes(result), route.SamplesAPI)
		}
		b.WriteString("\nStatus codes the replayer received (legacy answers):\n\n")
		for _, route := range report.Reads.Routes {
			fmt.Fprintf(&b, "- `%s`: %s\n", route.RouteID, statusList(route.Statuses))
		}
		for _, route := range report.Reads.Routes {
			if len(route.Samples) == 0 {
				continue
			}
			fmt.Fprintf(&b, "\n### Mismatch samples: `%s`\n\n", route.RouteID)
			for _, sample := range route.Samples {
				fmt.Fprintf(&b, "- %s `%s` legacy %d native %d, request %s: %s\n", sample.ObservedAt.Format(time.RFC3339), sample.Path,
					sample.LegacyStatus, sample.NativeStatus, sample.RequestID, strings.Join(sample.DiffPaths, ", "))
			}
		}
		if len(report.Reads.Personas) > 0 {
			fmt.Fprintf(&b, "\nPersonas: %s\n", strings.Join(report.Reads.Personas, "; "))
		}
		b.WriteString("\n")
	}
	if report.Writes != nil {
		b.WriteString("## Write routes (reconciliation twins: legacy vs native on identical database copies)\n\n")
		b.WriteString("| Route | Requests | Differing answers | Result |\n|---|---:|---:|---|\n")
		for _, route := range report.Writes.Routes {
			result := "pass"
			if !route.Pass {
				result = "FAIL"
				if route.Skip != "" {
					result = "not rehearsed: " + route.Skip
				}
			}
			fmt.Fprintf(&b, "| `%s %s` (`%s`) | %d | %d | %s |\n", route.Method, route.Path, route.RouteID, len(route.Cases), countUnequal(route), escapePipes(result))
		}
		for _, route := range report.Writes.Routes {
			for _, c := range route.Cases {
				if c.Equal {
					continue
				}
				fmt.Fprintf(&b, "\n- `%s` %s as %s (`%s %s`): legacy %d, native %d", route.RouteID, c.Label, c.Persona, c.Method, c.Path, c.LegacyStatus, c.NativeStatus)
				if c.Error != "" {
					fmt.Fprintf(&b, ", error %s", c.Error)
				}
				for _, diff := range c.Diff {
					fmt.Fprintf(&b, "\n  - %s", diff)
				}
			}
		}
		b.WriteString("\n\n### Table reconciliation\n\n")
		b.WriteString("| Package | Table | Rows (legacy/native) | Rows changed by the replay | Differences | Ignored columns |\n|---|---|---|---:|---:|---|\n")
		for _, table := range report.Writes.Tables {
			ignored := make([]string, 0, len(table.Ignore))
			for column, why := range table.Ignore {
				ignored = append(ignored, fmt.Sprintf("`%s` (%s)", column, why))
			}
			sort.Strings(ignored)
			differences := fmt.Sprint(table.Differences)
			if table.Error != "" {
				differences = "error: " + table.Error
			}
			fmt.Fprintf(&b, "| %s | `%s` | %d/%d | %d | %s | %s |\n", table.PackageID, table.Table, table.LegacyRows, table.NativeRows,
				table.ChangedRows, escapePipes(differences), strings.Join(ignored, ", "))
		}
		for _, table := range report.Writes.Tables {
			for _, sample := range table.Samples {
				fmt.Fprintf(&b, "\n- `%s`: %s", table.Table, escapePipes(sample))
			}
		}
		b.WriteString("\n\n")
	}
	b.WriteString("## Parity suites\n\n")
	for _, suite := range report.Parity {
		result := "pass"
		if !suite.Pass {
			result = "FAIL"
		}
		database := "SQLite"
		if suite.Postgres {
			database = "SQLite and PostgreSQL"
		}
		fmt.Fprintf(&b, "- `internal/tests/%s` (%s): %s in %.0fs\n", suite.Name, database, result, suite.Seconds)
	}
	b.WriteString("\n## Next step\n\n")
	if len(report.NativeCommands) > 0 {
		b.WriteString("Nothing was switched to native. After the owner signs this batch off (H7), run:\n\n```bash\n")
		for _, command := range report.NativeCommands {
			b.WriteString(command + "\n")
		}
		b.WriteString("```\n\n")
	} else {
		b.WriteString("The batch is not eligible for native until every check passes.\n\n")
	}
	fmt.Fprintf(&b, "Roll the batch back to legacy: `%s`\n", report.Rollback)
	return b.String()
}

func statusList(statuses map[string]int) string {
	keys := make([]string, 0, len(statuses))
	for key := range statuses {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, key := range keys {
		parts[i] = fmt.Sprintf("%s x%d", key, statuses[key])
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ", ")
}

func escapePipes(value string) string { return strings.ReplaceAll(value, "|", `\|`) }
