package destinationstats

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// Limits of a remote source's list.
const (
	MaxSourceBytes   = 16 << 20
	MaxSourceEntries = 200_000
	fetchTimeout     = time.Minute
	// sourceCheckInterval is how often the worker looks for due sources.
	sourceCheckInterval = 10 * time.Minute
)

// SourceEntries are a source's parsed list as EntriesJSON keeps it.
type SourceEntries struct {
	Domains  []string `json:"domains"`
	Networks []string `json:"networks"`
}

func decodeEntries(raw string) (SourceEntries, error) {
	var entries SourceEntries
	if strings.TrimSpace(raw) == "" {
		return entries, nil
	}
	err := json.Unmarshal([]byte(raw), &entries)
	return entries, err
}

func sortStrings(values []string) { sort.Strings(values) }

// ParseSourceList reads a list of domains and networks in any of the
// common formats, line by line: plain domains, addresses and CIDRs;
// v2fly domain-list-community (domain:, full:, with @attributes); Clash
// classical rules (DOMAIN-SUFFIX, DOMAIN, IP-CIDR, IP-CIDR6); Clash rule
// providers (payload: with "- '+.example.com'"). Comments (#, !, //) and
// the entries that cannot be matched by domain or network (keyword,
// regexp, include, DOMAIN-KEYWORD, process rules, ...) are skipped.
func ParseSourceList(r io.Reader) (SourceEntries, int, error) {
	entries := SourceEntries{Domains: []string{}, Networks: []string{}}
	seenDomains, seenNetworks := map[string]bool{}, map[string]bool{}
	skipped := 0
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1<<20)
	for scanner.Scan() {
		kind, value, ok := parseSourceLine(scanner.Text())
		if !ok {
			continue
		}
		switch kind {
		case RuleKindDomain:
			domain, valid := NormalizeDomain(value)
			if !valid {
				skipped++
				continue
			}
			if !seenDomains[domain] {
				seenDomains[domain] = true
				entries.Domains = append(entries.Domains, domain)
			}
		case RuleKindNetwork:
			prefix, valid := NormalizeNetwork(value)
			if !valid {
				skipped++
				continue
			}
			if network := prefix.String(); !seenNetworks[network] {
				seenNetworks[network] = true
				entries.Networks = append(entries.Networks, network)
			}
		default:
			skipped++
			continue
		}
		if len(entries.Domains)+len(entries.Networks) > MaxSourceEntries {
			return entries, skipped, fmt.Errorf("the list has more than %d entries", MaxSourceEntries)
		}
	}
	if err := scanner.Err(); err != nil {
		return entries, skipped, err
	}
	sort.Strings(entries.Domains)
	sort.Strings(entries.Networks)
	return entries, skipped, nil
}

// parseSourceLine answers the kind and value of one list line; ok is false
// for a blank line, a comment or a header, and kind is empty for an entry
// of a kind no rule can hold.
func parseSourceLine(line string) (kind, value string, ok bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "[") {
		return "", "", false
	}
	if index := strings.Index(line, " #"); index >= 0 {
		line = strings.TrimSpace(line[:index])
	}
	if strings.HasSuffix(line, ":") && !strings.Contains(line, ",") {
		// A YAML key such as "payload:".
		return "", "", false
	}
	// A Clash rule provider's list item.
	if strings.HasPrefix(line, "- ") || line == "-" {
		line = strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "-")), `'"`)
		if line == "" {
			return "", "", false
		}
	}
	// v2fly attributes: "domain:example.com @ads"; any other line with
	// spaces is not an entry.
	if index := strings.IndexAny(line, " \t"); index >= 0 && !strings.Contains(line, ",") {
		if !strings.Contains(line[:index], ":") {
			return "", "", true
		}
		line = line[:index]
	}
	if comma := strings.IndexByte(line, ','); comma >= 0 {
		parts := strings.Split(line, ",")
		rule, target := strings.ToUpper(strings.TrimSpace(parts[0])), strings.TrimSpace(parts[1])
		switch rule {
		case "DOMAIN-SUFFIX", "DOMAIN", "HOST-SUFFIX", "HOST":
			return RuleKindDomain, target, true
		case "IP-CIDR", "IP-CIDR6", "IP6-CIDR":
			return RuleKindNetwork, target, true
		default:
			return "", "", true
		}
	}
	if prefix, rest, found := strings.Cut(line, ":"); found && !strings.Contains(prefix, ".") {
		switch strings.ToLower(prefix) {
		case "domain", "full":
			return RuleKindDomain, rest, true
		case "keyword", "regexp", "include":
			return "", "", true
		}
		// An IPv6 address or network contains colons too.
		if _, valid := NormalizeNetwork(line); valid {
			return RuleKindNetwork, line, true
		}
		return "", "", true
	}
	if _, valid := NormalizeNetwork(line); valid {
		return RuleKindNetwork, line, true
	}
	return RuleKindDomain, line, true
}

// SourceInput is a remote source as the API takes it.
type SourceInput struct {
	Name         string `json:"name"`
	URL          string `json:"url"`
	Category     string `json:"category"`
	Enabled      *bool  `json:"enabled"`
	RefreshHours int    `json:"refresh_hours"`
}

func (in SourceInput) apply(row *model.TrafficCategorySource) error {
	row.Name, row.URL, row.Category = strings.TrimSpace(in.Name), strings.TrimSpace(in.URL), strings.TrimSpace(in.Category)
	if row.Name == "" || len(row.Name) > maxNameLength {
		return invalid("name is required, at most %d bytes", maxNameLength)
	}
	parsed, err := url.Parse(row.URL)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" || len(row.URL) > maxURLLength {
		return invalid("url must be an http or https URL of at most %d bytes", maxURLLength)
	}
	if err := checkCategory(row.Category); err != nil {
		return err
	}
	row.RefreshHours = in.RefreshHours
	if row.RefreshHours == 0 {
		row.RefreshHours = DefaultRefreshHours
	}
	if row.RefreshHours < MinRefreshHours || row.RefreshHours > MaxRefreshHours {
		return invalid("refresh_hours must be between %d and %d", MinRefreshHours, MaxRefreshHours)
	}
	row.Enabled = in.Enabled == nil || *in.Enabled
	return nil
}

// Sources lists the remote sources by name.
func (s *Service) Sources(ctx context.Context) ([]model.TrafficCategorySource, error) {
	sources := []model.TrafficCategorySource{}
	err := s.DB.WithContext(ctx).Omit("entries_json").Order("name").Find(&sources).Error
	return sources, err
}

// CreateSource adds a remote source, audited; the worker fetches it soon,
// or RefreshSource at once.
func (s *Service) CreateSource(ctx context.Context, in SourceInput, actor Actor) (model.TrafficCategorySource, error) {
	var row model.TrafficCategorySource
	if err := in.apply(&row); err != nil {
		return row, err
	}
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&model.TrafficCategorySource{}).Count(&count).Error; err != nil {
			return err
		}
		if count >= MaxSources {
			return invalid("at most %d sources", MaxSources)
		}
		if err := sourceNameTaken(tx, row.Name, 0); err != nil {
			return err
		}
		now := s.now()
		row.CreatedAt, row.UpdatedAt = now, now
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		return audit(tx, actor, "create", auditTargetSource, row.ID, sourceAudit(row))
	})
	if err == nil {
		Invalidate()
	}
	return row, err
}

// UpdateSource replaces a source's settings, audited; a changed URL is
// fetched again at the worker's next check.
func (s *Service) UpdateSource(ctx context.Context, id uint, in SourceInput, actor Actor) (model.TrafficCategorySource, error) {
	var row model.TrafficCategorySource
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&row, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		previousURL := row.URL
		if err := in.apply(&row); err != nil {
			return err
		}
		if err := sourceNameTaken(tx, row.Name, id); err != nil {
			return err
		}
		if row.URL != previousURL {
			row.FetchedAt = nil
		}
		row.UpdatedAt = s.now()
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		return audit(tx, actor, "update", auditTargetSource, row.ID, sourceAudit(row))
	})
	if err == nil {
		Invalidate()
	}
	row.EntriesJSON = ""
	return row, err
}

// DeleteSource removes a source and its list, audited.
func (s *Service) DeleteSource(ctx context.Context, id uint, actor Actor) error {
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row model.TrafficCategorySource
		if err := tx.Omit("entries_json").First(&row, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		if err := tx.Delete(&model.TrafficCategorySource{}, id).Error; err != nil {
			return err
		}
		return audit(tx, actor, "delete", auditTargetSource, row.ID, sourceAudit(row))
	})
	if err == nil {
		Invalidate()
	}
	return err
}

func sourceAudit(row model.TrafficCategorySource) map[string]any {
	return map[string]any{"name": row.Name, "url": row.URL, "category": row.Category, "enabled": row.Enabled, "refresh_hours": row.RefreshHours}
}

func sourceNameTaken(tx *gorm.DB, name string, except uint) error {
	var count int64
	if err := tx.Model(&model.TrafficCategorySource{}).Where("name = ? AND id <> ?", name, except).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return invalid("a source named %q already exists", name)
	}
	return nil
}

// Fetcher downloads a source's list.
type Fetcher func(ctx context.Context, rawURL string) (io.ReadCloser, error)

// HTTPFetcher fetches over HTTP(S) with the environment's proxy.
func HTTPFetcher(ctx context.Context, rawURL string) (io.ReadCloser, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "AnixOps-Control traffic-categories")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		_ = response.Body.Close()
		return nil, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	return response.Body, nil
}

// RefreshSource fetches a source's list now and stores it; a failed fetch
// keeps the previous list and records the error. It answers the source.
func (s *Service) RefreshSource(ctx context.Context, id uint, fetch Fetcher) (model.TrafficCategorySource, error) {
	var row model.TrafficCategorySource
	if err := s.DB.WithContext(ctx).Omit("entries_json").First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return row, ErrNotFound
		}
		return row, err
	}
	if fetch == nil {
		fetch = HTTPFetcher
	}
	fetchCtx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	entries, fetchErr := fetchEntries(fetchCtx, fetch, row.URL)
	now := s.now()
	updates := map[string]any{"fetched_at": now, "updated_at": now}
	if fetchErr != nil {
		updates["fetch_error"] = truncate(fetchErr.Error(), 1000)
	} else {
		encoded, err := json.Marshal(entries)
		if err != nil {
			return row, err
		}
		updates["entries_json"] = string(encoded)
		updates["entry_count"] = len(entries.Domains) + len(entries.Networks)
		updates["fetch_error"] = ""
	}
	if err := s.DB.WithContext(ctx).Model(&model.TrafficCategorySource{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return row, err
	}
	Invalidate()
	err := s.DB.WithContext(ctx).Omit("entries_json").First(&row, id).Error
	return row, err
}

func fetchEntries(ctx context.Context, fetch Fetcher, rawURL string) (SourceEntries, error) {
	body, err := fetch(ctx, rawURL)
	if err != nil {
		return SourceEntries{}, err
	}
	defer func() { _ = body.Close() }()
	limited := &io.LimitedReader{R: body, N: MaxSourceBytes + 1}
	entries, _, err := ParseSourceList(limited)
	if err != nil {
		return entries, err
	}
	if limited.N <= 0 {
		return entries, fmt.Errorf("the list is larger than %d bytes", MaxSourceBytes)
	}
	if len(entries.Domains)+len(entries.Networks) == 0 {
		return entries, errors.New("the list has no domain or network entries")
	}
	return entries, nil
}

// SourceWorker fetches the enabled sources whose list is older than their
// refresh_hours (never-fetched sources at once). It runs in the singleton
// worker process.
type SourceWorker struct {
	Service *Service
	Fetch   Fetcher
}

// Run checks the sources every sourceCheckInterval until ctx is done.
func (w *SourceWorker) Run(ctx context.Context) {
	ticker := time.NewTicker(sourceCheckInterval)
	defer ticker.Stop()
	for {
		w.RunOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// RunOnce fetches the sources due now.
func (w *SourceWorker) RunOnce(ctx context.Context) {
	db := w.Service.DB.WithContext(ctx)
	if !db.Migrator().HasTable(&model.TrafficCategorySource{}) {
		return
	}
	var due []model.TrafficCategorySource
	if err := db.Omit("entries_json").Where("enabled = ?", true).Order("id").Find(&due).Error; err != nil {
		log.Printf("traffic category sources: %v", err)
		return
	}
	now := w.Service.now()
	for _, source := range due {
		if ctx.Err() != nil {
			return
		}
		if source.FetchedAt != nil && now.Sub(*source.FetchedAt) < time.Duration(source.RefreshHours)*time.Hour {
			continue
		}
		if row, err := w.Service.RefreshSource(ctx, source.ID, w.Fetch); err != nil {
			log.Printf("traffic category source %d: %v", source.ID, err)
		} else if row.FetchError != "" {
			log.Printf("traffic category source %q: %s", row.Name, row.FetchError)
		}
	}
}
