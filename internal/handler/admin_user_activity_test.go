package handler

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseAdminIDs(t *testing.T) {
	ids, err := parseAdminIDs(" 3, 2,2 ,,1 ")
	require.NoError(t, err)
	require.Equal(t, []uint{3, 2, 1}, ids)

	for _, raw := range []string{"", " , ", "x", "1,0", "-1", "1.5", "4294967296", "1;2"} {
		_, err := parseAdminIDs(raw)
		require.Error(t, err, raw)
	}

	many := make([]string, MaxAdminIDs)
	for i := range many {
		many[i] = fmt.Sprint(i + 1)
	}
	ids, err = parseAdminIDs(strings.Join(many, ","))
	require.NoError(t, err)
	require.Len(t, ids, MaxAdminIDs)
	_, err = parseAdminIDs(strings.Join(many, ",") + fmt.Sprintf(",%d", MaxAdminIDs+1))
	require.ErrorContains(t, err, "at most 200 ids")
	// The same id repeated stays within the cap.
	ids, err = parseAdminIDs(strings.Repeat("7,", 500) + "8")
	require.NoError(t, err)
	require.Equal(t, []uint{7, 8}, ids)
}
