package main

import (
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDumpPlainSQL(t *testing.T) {
	dumpPath := filepath.Join(t.TempDir(), "dump.sql")
	require.NoError(t, os.WriteFile(dumpPath, []byte("CREATE TABLE `v2_plan` (\n  `id` int,\n  `name` varchar(255)\n);\nINSERT INTO `v2_plan` VALUES (1,'Basic'),(2,NULL);\n"), 0o600))

	tables, err := ParseDump(dumpPath)
	require.NoError(t, err)

	plan := tables["v2_plan"]
	require.NotNil(t, plan)
	assert.Equal(t, []string{"id", "name"}, plan.Columns)
	require.Len(t, plan.Rows, 2)
	require.NotNil(t, plan.Rows[0]["name"])
	assert.Equal(t, "Basic", *plan.Rows[0]["name"])
	assert.Nil(t, plan.Rows[1]["name"])
}

func TestParseDumpGzipSQL(t *testing.T) {
	dumpPath := filepath.Join(t.TempDir(), "dump.sql.gz")
	file, err := os.OpenFile(dumpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	require.NoError(t, err)
	gz := gzip.NewWriter(file)
	_, err = gz.Write([]byte("CREATE TABLE `v2_user` (\n  `id` int,\n  `email` varchar(255)\n);\nINSERT INTO `v2_user` VALUES (1,'user@example.com');\n"))
	require.NoError(t, err)
	require.NoError(t, gz.Close())
	require.NoError(t, file.Close())

	tables, err := ParseDump(dumpPath)
	require.NoError(t, err)

	user := tables["v2_user"]
	require.NotNil(t, user)
	require.Len(t, user.Rows, 1)
	require.NotNil(t, user.Rows[0]["email"])
	assert.Equal(t, "user@example.com", *user.Rows[0]["email"])
}

func TestOpenDumpFileRejectsEmptyPath(t *testing.T) {
	file, closeFn, err := openDumpFile(" ")
	require.Error(t, err)
	assert.Nil(t, file)
	assert.Nil(t, closeFn)
}
