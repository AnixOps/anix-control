package destinationstats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// The kinds of an administrator's category rule.
const (
	RuleKindDomain   = "domain"
	RuleKindNetwork  = "network"
	RuleKindProtocol = "protocol"
)

// Limits of the administrator's rules and the remote sources.
const (
	MaxRules            = 5000
	MaxSources          = 50
	MinRefreshHours     = 1
	MaxRefreshHours     = 168
	DefaultRefreshHours = 24
	maxNoteLength       = 255
	maxNameLength       = 100
	maxURLLength        = 1024
)

// The rules' audit names.
const (
	auditModule       = "traffic_category"
	auditTargetRule   = "traffic_category_rule"
	auditTargetSource = "traffic_category_source"
)

// The errors of the rule and source writes.
var (
	ErrNotFound    = errors.New("not found")
	ErrRuleInvalid = errors.New("invalid category rule")
	ErrDuplicate   = errors.New("a rule for this value already exists")
)

// Actor is who made an administrator's change, for the audit entry.
type Actor struct {
	UserID uint
	Name   string
	IP     string
}

var protocolPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrRuleInvalid, fmt.Sprintf(format, args...))
}

// checkCategory refuses a category that is not one of Categories or is
// unknown, which no rule can name.
func checkCategory(category string) error {
	if !IsCategory(category) || category == CategoryUnknown {
		return invalid("category must be one of video, music, messaging, social, gaming, ai, download, web")
	}
	return nil
}

// NormalizeDomain answers a rule domain in its stored form: lower case,
// without a leading "*.", "+." or "." and a trailing dot; a name needs
// two labels at least.
func NormalizeDomain(raw string) (string, bool) {
	domain := strings.ToLower(strings.TrimSpace(raw))
	for _, prefix := range []string{"*.", "+.", "."} {
		domain = strings.TrimPrefix(domain, prefix)
	}
	domain = strings.TrimSuffix(domain, ".")
	if domain == "" || len(domain) > 253 || !strings.Contains(domain, ".") {
		return "", false
	}
	if _, err := netip.ParseAddr(domain); err == nil {
		return "", false
	}
	for _, label := range strings.Split(domain, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", false
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
				return "", false
			}
		}
	}
	return domain, true
}

// NormalizeNetwork answers an address or CIDR as its masked network.
func NormalizeNetwork(raw string) (netip.Prefix, bool) {
	value := strings.TrimSpace(raw)
	if prefix, err := netip.ParsePrefix(value); err == nil {
		return prefix.Masked(), true
	}
	if address, err := netip.ParseAddr(value); err == nil {
		address = address.Unmap()
		return netip.PrefixFrom(address, address.BitLen()), true
	}
	return netip.Prefix{}, false
}

// RuleInput is an administrator's rule as the API takes it.
type RuleInput struct {
	Kind     string `json:"kind"`
	Value    string `json:"value"`
	Category string `json:"category"`
	Note     string `json:"note"`
}

func (in RuleInput) normalize() (model.TrafficCategoryRule, error) {
	row := model.TrafficCategoryRule{Kind: strings.TrimSpace(in.Kind), Category: strings.TrimSpace(in.Category), Note: strings.TrimSpace(in.Note)}
	if err := checkCategory(row.Category); err != nil {
		return row, err
	}
	if len(row.Note) > maxNoteLength {
		return row, invalid("note is longer than %d bytes", maxNoteLength)
	}
	switch row.Kind {
	case RuleKindDomain:
		domain, ok := NormalizeDomain(in.Value)
		if !ok {
			return row, invalid("value is not a domain")
		}
		row.Value = domain
	case RuleKindNetwork:
		prefix, ok := NormalizeNetwork(in.Value)
		if !ok {
			return row, invalid("value is not an address or CIDR")
		}
		row.Value = prefix.String()
	case RuleKindProtocol:
		protocol := strings.ToLower(strings.TrimSpace(in.Value))
		if !protocolPattern.MatchString(protocol) {
			return row, invalid("value is not a protocol name")
		}
		row.Value = protocol
	default:
		return row, invalid("kind must be domain, network or protocol")
	}
	return row, nil
}

// Service manages the administrator's category rules and the remote
// sources on one database.
type Service struct {
	DB *gorm.DB
	// Now defaults to time.Now.
	Now func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Rules lists the administrator's rules by kind and value.
func (s *Service) Rules(ctx context.Context) ([]model.TrafficCategoryRule, error) {
	rules := []model.TrafficCategoryRule{}
	err := s.DB.WithContext(ctx).Order("kind, value").Find(&rules).Error
	return rules, err
}

// CreateRule adds a rule, audited.
func (s *Service) CreateRule(ctx context.Context, in RuleInput, actor Actor) (model.TrafficCategoryRule, error) {
	row, err := in.normalize()
	if err != nil {
		return row, err
	}
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&model.TrafficCategoryRule{}).Count(&count).Error; err != nil {
			return err
		}
		if count >= MaxRules {
			return invalid("at most %d rules", MaxRules)
		}
		if err := ruleTaken(tx, row, 0); err != nil {
			return err
		}
		now := s.now()
		row.CreatedAt, row.UpdatedAt = now, now
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		return audit(tx, actor, "create", auditTargetRule, row.ID, row)
	})
	if err == nil {
		Invalidate()
	}
	return row, err
}

// UpdateRule replaces a rule's fields, audited.
func (s *Service) UpdateRule(ctx context.Context, id uint, in RuleInput, actor Actor) (model.TrafficCategoryRule, error) {
	next, err := in.normalize()
	if err != nil {
		return next, err
	}
	var row model.TrafficCategoryRule
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&row, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		if err := ruleTaken(tx, next, id); err != nil {
			return err
		}
		row.Kind, row.Value, row.Category, row.Note, row.UpdatedAt = next.Kind, next.Value, next.Category, next.Note, s.now()
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		return audit(tx, actor, "update", auditTargetRule, row.ID, row)
	})
	if err == nil {
		Invalidate()
	}
	return row, err
}

// DeleteRule removes a rule, audited.
func (s *Service) DeleteRule(ctx context.Context, id uint, actor Actor) error {
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row model.TrafficCategoryRule
		if err := tx.First(&row, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		if err := tx.Delete(&model.TrafficCategoryRule{}, id).Error; err != nil {
			return err
		}
		return audit(tx, actor, "delete", auditTargetRule, row.ID, row)
	})
	if err == nil {
		Invalidate()
	}
	return err
}

func ruleTaken(tx *gorm.DB, row model.TrafficCategoryRule, except uint) error {
	var count int64
	if err := tx.Model(&model.TrafficCategoryRule{}).Where("kind = ? AND value = ? AND id <> ?", row.Kind, row.Value, except).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrDuplicate
	}
	return nil
}

func audit(tx *gorm.DB, actor Actor, action, target string, id uint, content any) error {
	encoded, err := json.Marshal(content)
	if err != nil {
		return err
	}
	var userID *uint
	if actor.UserID != 0 {
		user := actor.UserID
		userID = &user
	}
	targetID := id
	return tx.Create(&model.OperationLog{
		UserID: userID, Username: truncate(actor.Name, 100), Action: action, Module: auditModule,
		TargetType: target, TargetID: &targetID, Content: string(encoded), IP: truncate(actor.IP, 45), Status: 1,
	}).Error
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}

// BuiltinRules are the built-in rules by category, for the administrator
// to read: domains, networks and protocols each sorted.
type BuiltinRules struct {
	Category  string   `json:"category"`
	Domains   []string `json:"domains"`
	Networks  []string `json:"networks"`
	Protocols []string `json:"protocols"`
}

// Builtin answers the built-in rules in the order of Categories.
func Builtin() []BuiltinRules {
	return builtin.Lists()
}

// Lists answers the classifier's rules by category in the order of
// Categories, each list sorted; the categories without a rule are left out.
func (c *Classifier) Lists() []BuiltinRules {
	byCategory := map[string]*BuiltinRules{}
	get := func(category string) *BuiltinRules {
		if byCategory[category] == nil {
			byCategory[category] = &BuiltinRules{Category: category, Domains: []string{}, Networks: []string{}, Protocols: []string{}}
		}
		return byCategory[category]
	}
	for domain, category := range c.domains {
		get(category).Domains = append(get(category).Domains, domain)
	}
	for _, rule := range c.networks {
		get(rule.category).Networks = append(get(rule.category).Networks, rule.prefix.String())
	}
	for protocol, category := range c.protocols {
		get(category).Protocols = append(get(category).Protocols, protocol)
	}
	lists := []BuiltinRules{}
	for _, category := range Categories {
		if rules := byCategory[category]; rules != nil {
			sortStrings(rules.Domains)
			sortStrings(rules.Networks)
			sortStrings(rules.Protocols)
			lists = append(lists, *rules)
		}
	}
	return lists
}

// The cache of the classifier of each database: rebuilt when the rules or
// the sources changed, checked at most every cacheCheckInterval.
const cacheCheckInterval = 15 * time.Second

type cachedClassifier struct {
	classifier *Classifier
	stamp      string
	checkedAt  time.Time
}

var classifierCache = struct {
	sync.Mutex
	entries map[*gorm.Config]*cachedClassifier
}{entries: map[*gorm.Config]*cachedClassifier{}}

// Invalidate makes the next CurrentClassifier of every database check its
// rules again.
func Invalidate() {
	classifierCache.Lock()
	defer classifierCache.Unlock()
	for _, entry := range classifierCache.entries {
		entry.checkedAt = time.Time{}
	}
}

// CurrentClassifier answers the classifier of db's rules: the built-in
// ones, the enabled sources' and the administrator's. Without the tables it
// answers the built-in classifier.
func CurrentClassifier(db *gorm.DB) (*Classifier, error) {
	classifierCache.Lock()
	entry := classifierCache.entries[db.Config]
	if entry != nil && time.Since(entry.checkedAt) < cacheCheckInterval {
		classifier := entry.classifier
		classifierCache.Unlock()
		return classifier, nil
	}
	classifierCache.Unlock()

	if !db.Migrator().HasTable(&model.TrafficCategoryRule{}) || !db.Migrator().HasTable(&model.TrafficCategorySource{}) {
		return builtin, nil
	}
	stamp, err := rulesStamp(db)
	if err != nil {
		return nil, err
	}
	if entry != nil && entry.stamp == stamp {
		classifierCache.Lock()
		entry.checkedAt = time.Now()
		classifierCache.Unlock()
		return entry.classifier, nil
	}
	classifier, err := loadClassifier(db)
	if err != nil {
		return nil, err
	}
	classifierCache.Lock()
	classifierCache.entries[db.Config] = &cachedClassifier{classifier: classifier, stamp: stamp, checkedAt: time.Now()}
	classifierCache.Unlock()
	return classifier, nil
}

// rulesStamp changes whenever a rule or a source is added, changed or
// removed.
func rulesStamp(db *gorm.DB) (string, error) {
	type stampRow struct {
		Count   int64
		Updated string
	}
	var rules, sources stampRow
	if err := db.Model(&model.TrafficCategoryRule{}).Select("COUNT(*) AS count, COALESCE(CAST(MAX(updated_at) AS TEXT), '') AS updated").Scan(&rules).Error; err != nil {
		return "", err
	}
	if err := db.Model(&model.TrafficCategorySource{}).Select("COUNT(*) AS count, COALESCE(CAST(MAX(updated_at) AS TEXT), '') AS updated").Scan(&sources).Error; err != nil {
		return "", err
	}
	return fmt.Sprintf("%d/%s/%d/%s", rules.Count, rules.Updated, sources.Count, sources.Updated), nil
}

func loadClassifier(db *gorm.DB) (*Classifier, error) {
	builder := newClassifierBuilder().addBuiltin()
	var sources []model.TrafficCategorySource
	if err := db.Where("enabled = ?", true).Order("id").Find(&sources).Error; err != nil {
		return nil, err
	}
	for _, source := range sources {
		if !IsCategory(source.Category) || source.Category == CategoryUnknown {
			continue
		}
		entries, err := decodeEntries(source.EntriesJSON)
		if err != nil {
			continue
		}
		for _, domain := range entries.Domains {
			builder.domains[domain] = source.Category
		}
		for _, network := range entries.Networks {
			if prefix, ok := NormalizeNetwork(network); ok {
				builder.networks[prefix] = source.Category
			}
		}
	}
	var rules []model.TrafficCategoryRule
	if err := db.Find(&rules).Error; err != nil {
		return nil, err
	}
	for _, rule := range rules {
		switch rule.Kind {
		case RuleKindDomain:
			builder.domains[rule.Value] = rule.Category
		case RuleKindNetwork:
			if prefix, ok := NormalizeNetwork(rule.Value); ok {
				builder.networks[prefix] = rule.Category
			}
		case RuleKindProtocol:
			builder.protocols[rule.Value] = rule.Category
		}
	}
	return builder.build(), nil
}
