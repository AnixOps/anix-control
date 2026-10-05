package v2compat

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var testSortColumns = map[string]SortColumn{
	"id":         {Expr: "id", Unique: true},
	"email":      {Expr: "email"},
	"expired_at": {Expr: "expired_at", Nullable: true},
}

func TestParseListSort(t *testing.T) {
	cases := []struct {
		name, sort, order, want string
	}{
		{"no sort keeps the default order", "", "", ""},
		{"order alone is not read", "", "sideways", ""},
		{"blank sort keeps the default order", "  ", "desc", ""},
		{"ascending by default", "email", "", "email ASC, id DESC"},
		{"descending", "email", "desc", "email DESC, id DESC"},
		{"order in any case", "email", " DESC ", "email DESC, id DESC"},
		{"a unique column needs no tiebreaker", "id", "asc", "id ASC"},
		{"null last ascending", "expired_at", "asc", "expired_at ASC NULLS LAST, id DESC"},
		{"null first descending", "expired_at", "desc", "expired_at DESC NULLS FIRST, id DESC"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseListSort(c.sort, c.order, testSortColumns, "id DESC")
			require.NoError(t, err)
			require.Equal(t, c.want, got)
		})
	}
}

func TestParseListSortWithoutTiebreaker(t *testing.T) {
	got, err := ParseListSort("email", "desc", testSortColumns, "")
	require.NoError(t, err)
	require.Equal(t, "email DESC", got)
}

func TestParseListSortRefusesWhatIsNotListed(t *testing.T) {
	for _, value := range []string{"password", "email; DROP TABLE v2_user", "EMAIL", "(select 1)", "id desc", "u + d"} {
		_, err := ParseListSort(value, "", testSortColumns, "id DESC")
		var refused *SortError
		require.True(t, errors.As(err, &refused), value)
		require.Equal(t, "sort", refused.Param)
		require.Equal(t, []string{"email", "expired_at", "id"}, refused.Allowed)
		require.Contains(t, err.Error(), "无效的排序字段")
	}
	_, err := ParseListSort("email", "up", testSortColumns, "id DESC")
	var refused *SortError
	require.True(t, errors.As(err, &refused))
	require.Equal(t, "order", refused.Param)
	require.Contains(t, err.Error(), "无效的排序方向")
}

func TestSortErrorTruncatesTheValue(t *testing.T) {
	err := &SortError{Param: "sort", Value: strings.Repeat("字", 100), Allowed: []string{"id"}}
	require.Less(t, len([]rune(err.Error())), 80)
	require.Contains(t, err.Error(), "...")
}
