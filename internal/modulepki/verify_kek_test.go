package modulepki

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerifyKEKOpensEveryCurrentAndNextCA(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	kek := bytes.Repeat([]byte{3}, 32)

	opened, err := VerifyKEK(ctx, db, kek)
	require.NoError(t, err)
	assert.Zero(t, opened, "an empty database has nothing to open")

	clock := &testClock{now: time.Now().UTC()}
	for _, cluster := range []string{"default", "prod"} {
		authority, err := New(Options{DB: db, Cluster: cluster, KEK: kek, Now: clock.Now})
		require.NoError(t, err)
		require.NoError(t, authority.Ensure(ctx))
		_, err = authority.Rotate(ctx)
		require.NoError(t, err)
	}
	opened, err = VerifyKEK(ctx, db, kek)
	require.NoError(t, err)
	assert.Equal(t, 4, opened, "the current and next CA of both clusters")

	_, err = VerifyKEK(ctx, db, bytes.Repeat([]byte{4}, 32))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "wrong key-encryption key")
}
