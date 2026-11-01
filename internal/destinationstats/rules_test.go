package destinationstats

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func classifierOf(t *testing.T, db *gorm.DB) *Classifier {
	t.Helper()
	Invalidate()
	classifier, err := CurrentClassifier(db)
	require.NoError(t, err)
	return classifier
}

func TestAdministratorRulesWinOverTheBuiltInOnes(t *testing.T) {
	forEachDialect(t, func(t *testing.T, db *gorm.DB) {
		service := &Service{DB: db}
		ctx := context.Background()
		actor := Actor{UserID: 1, Name: "admin@example.com", IP: "127.0.0.1"}

		// A subdomain rule beats the built-in rule of its parent; a rule to
		// web takes a domain out of its built-in category.
		music, err := service.CreateRule(ctx, RuleInput{Kind: "domain", Value: "*.Music.YouTube.com.", Category: CategoryMusic}, actor)
		require.NoError(t, err)
		assert.Equal(t, "music.youtube.com", music.Value)
		_, err = service.CreateRule(ctx, RuleInput{Kind: "domain", Value: "steamcontent.com", Category: CategoryWeb}, actor)
		require.NoError(t, err)
		_, err = service.CreateRule(ctx, RuleInput{Kind: "network", Value: "203.0.113.7", Category: CategoryGaming}, actor)
		require.NoError(t, err)
		_, err = service.CreateRule(ctx, RuleInput{Kind: "protocol", Value: "BitTorrent", Category: CategoryWeb, Note: "not download"}, actor)
		require.NoError(t, err)

		classifier := classifierOf(t, db)
		assert.Equal(t, CategoryMusic, classifier.Classify("music.youtube.com", "tls"))
		assert.Equal(t, CategoryVideo, classifier.Classify("youtube.com", "tls"))
		assert.Equal(t, CategoryWeb, classifier.Classify("steamcontent.com", "http"))
		assert.Equal(t, CategoryGaming, classifier.Classify("203.0.113.7", ""))
		assert.Equal(t, CategoryUnknown, classifier.Classify("203.0.113.0/24", ""))
		assert.Equal(t, CategoryWeb, classifier.Classify("example.com", "bittorrent"))
		assert.Equal(t, CategoryMessaging, classifier.Classify("149.154.167.0/24", ""))

		// Duplicates and invalid rules are refused.
		_, err = service.CreateRule(ctx, RuleInput{Kind: "domain", Value: "music.youtube.com", Category: CategoryVideo}, actor)
		require.ErrorIs(t, err, ErrDuplicate)
		for _, input := range []RuleInput{
			{Kind: "domain", Value: "bad domain", Category: CategoryVideo},
			{Kind: "domain", Value: "1.2.3.4", Category: CategoryVideo},
			{Kind: "network", Value: "example.com", Category: CategoryVideo},
			{Kind: "protocol", Value: "", Category: CategoryVideo},
			{Kind: "regexp", Value: "x", Category: CategoryVideo},
			{Kind: "domain", Value: "example.org", Category: CategoryUnknown},
			{Kind: "domain", Value: "example.org", Category: "streaming"},
		} {
			_, err := service.CreateRule(ctx, input, actor)
			require.ErrorIs(t, err, ErrRuleInvalid, "%+v", input)
		}

		// Changing and deleting a rule apply at once.
		_, err = service.UpdateRule(ctx, music.ID, RuleInput{Kind: "domain", Value: "music.youtube.com", Category: CategorySocial}, actor)
		require.NoError(t, err)
		assert.Equal(t, CategorySocial, classifierOf(t, db).Classify("music.youtube.com", "tls"))
		require.NoError(t, service.DeleteRule(ctx, music.ID, actor))
		assert.Equal(t, CategoryVideo, classifierOf(t, db).Classify("music.youtube.com", "tls"))
		require.ErrorIs(t, service.DeleteRule(ctx, music.ID, actor), ErrNotFound)

		var audits int64
		require.NoError(t, db.Model(&model.OperationLog{}).Where("module = ?", auditModule).Count(&audits).Error)
		assert.Equal(t, int64(6), audits)
	})
}

func TestSummarizeJudgesByTheCurrentRules(t *testing.T) {
	db := openSQLite(t)
	hour := testNow.Truncate(time.Hour).Unix()
	require.NoError(t, db.Create(&model.DestinationStat{HourAt: hour, NodeID: 1, UserID: 7, Domain: "example.net", Protocol: "tls", Download: 100}).Error)
	query := LastHours(1, testNow)

	summary, err := Summarize(db, query)
	require.NoError(t, err)
	require.Len(t, summary.Domains, 1)
	assert.Equal(t, CategoryWeb, summary.Domains[0].Category)

	_, err = (&Service{DB: db}).CreateRule(context.Background(), RuleInput{Kind: "domain", Value: "example.net", Category: CategoryAI}, Actor{})
	require.NoError(t, err)
	summary, err = Summarize(db, query)
	require.NoError(t, err)
	assert.Equal(t, CategoryAI, summary.Domains[0].Category)
	assert.Equal(t, CategoryAI, summary.Categories[0].Category)
}

func TestParseSourceListReadsTheCommonFormats(t *testing.T) {
	list := `
# v2fly domain-list-community
domain:Example.com @ads
full:cdn.example.com
keyword:example
regexp:^ex.*\.com$
include:other
plain.example.org
.dot.example.org

! Clash classical
DOMAIN-SUFFIX,suffix.example.net,Proxy
DOMAIN,exact.example.net
DOMAIN-KEYWORD,kw
IP-CIDR,198.51.100.0/24,no-resolve
IP-CIDR6,2001:db8::/32
PROCESS-NAME,foo.exe
payload:
  - '+.provider.example'
  - "203.0.113.9"
2001:db8:1::1
not a domain!
192.0.2.0/24 # comment
`
	entries, skipped, err := ParseSourceList(strings.NewReader(list))
	require.NoError(t, err)
	assert.Equal(t, []string{
		"cdn.example.com", "dot.example.org", "exact.example.net", "example.com",
		"plain.example.org", "provider.example", "suffix.example.net",
	}, entries.Domains)
	assert.Equal(t, []string{"192.0.2.0/24", "198.51.100.0/24", "2001:db8:1::1/128", "2001:db8::/32", "203.0.113.9/32"}, entries.Networks)
	assert.Equal(t, 6, skipped)
}

func TestSourcesAreFetchedStoredAndKeptOnFailure(t *testing.T) {
	db := openSQLite(t)
	now := testNow
	service := &Service{DB: db, Now: func() time.Time { return now }}
	ctx := context.Background()
	body, fetchErr := "domain:stream.example\n203.0.113.0/24\n", error(nil)
	fetch := func(context.Context, string) (io.ReadCloser, error) {
		if fetchErr != nil {
			return nil, fetchErr
		}
		return io.NopCloser(strings.NewReader(body)), nil
	}

	_, err := service.CreateSource(ctx, SourceInput{Name: "streams", URL: "ftp://example.com/list", Category: CategoryVideo}, Actor{})
	require.ErrorIs(t, err, ErrRuleInvalid)
	_, err = service.CreateSource(ctx, SourceInput{Name: "streams", URL: "https://example.com/list", Category: CategoryVideo, RefreshHours: 500}, Actor{})
	require.ErrorIs(t, err, ErrRuleInvalid)
	source, err := service.CreateSource(ctx, SourceInput{Name: "streams", URL: "https://example.com/list", Category: CategoryVideo}, Actor{})
	require.NoError(t, err)
	assert.Equal(t, DefaultRefreshHours, source.RefreshHours)
	assert.True(t, source.Enabled)
	_, err = service.CreateSource(ctx, SourceInput{Name: "streams", URL: "https://example.com/other", Category: CategoryVideo}, Actor{})
	require.ErrorIs(t, err, ErrRuleInvalid)

	worker := &SourceWorker{Service: service, Fetch: fetch}
	worker.RunOnce(ctx)
	classifier := classifierOf(t, db)
	assert.Equal(t, CategoryVideo, classifier.Classify("stream.example", "tls"))
	assert.Equal(t, CategoryVideo, classifier.Classify("203.0.113.0/24", ""))
	sources, err := service.Sources(ctx)
	require.NoError(t, err)
	require.Len(t, sources, 1)
	assert.Equal(t, 2, sources[0].EntryCount)
	assert.Empty(t, sources[0].FetchError)

	// Not due again before refresh_hours; a failure keeps the list.
	body = "domain:other.example\n"
	now = now.Add(time.Hour)
	worker.RunOnce(ctx)
	assert.Equal(t, CategoryVideo, classifierOf(t, db).Classify("stream.example", "tls"))
	fetchErr = errors.New("connection refused")
	now = now.Add(DefaultRefreshHours * time.Hour)
	worker.RunOnce(ctx)
	sources, err = service.Sources(ctx)
	require.NoError(t, err)
	assert.Equal(t, "connection refused", sources[0].FetchError)
	assert.Equal(t, CategoryVideo, classifierOf(t, db).Classify("stream.example", "tls"))

	// A disabled source no longer counts; an administrator's rule wins over
	// a source's.
	disabled := false
	_, err = service.UpdateSource(ctx, source.ID, SourceInput{Name: "streams", URL: "https://example.com/list", Category: CategoryVideo, Enabled: &disabled}, Actor{})
	require.NoError(t, err)
	assert.Equal(t, CategoryWeb, classifierOf(t, db).Classify("stream.example", "tls"))
	enabled := true
	_, err = service.UpdateSource(ctx, source.ID, SourceInput{Name: "streams", URL: "https://example.com/list", Category: CategoryVideo, Enabled: &enabled}, Actor{})
	require.NoError(t, err)
	_, err = service.CreateRule(ctx, RuleInput{Kind: "domain", Value: "stream.example", Category: CategoryGaming}, Actor{})
	require.NoError(t, err)
	assert.Equal(t, CategoryGaming, classifierOf(t, db).Classify("stream.example", "tls"))

	// An empty list is an error, not a cleared source.
	fetchErr, body = nil, "# nothing\n"
	refreshed, err := service.RefreshSource(ctx, source.ID, fetch)
	require.NoError(t, err)
	assert.Contains(t, refreshed.FetchError, "no domain or network")
	assert.Equal(t, 2, refreshed.EntryCount)

	require.NoError(t, service.DeleteSource(ctx, source.ID, Actor{}))
	_, err = service.RefreshSource(ctx, source.ID, fetch)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestBuiltinListsEveryBuiltInRule(t *testing.T) {
	lists := Builtin()
	total := 0
	for _, list := range lists {
		total += len(list.Domains) + len(list.Networks) + len(list.Protocols)
	}
	assert.Equal(t, len(domainRules)+len(networkRules)+len(protocolRules), total)
	assert.Equal(t, CategoryVideo, lists[0].Category)
}
