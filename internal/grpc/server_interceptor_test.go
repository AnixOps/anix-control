package grpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGRPCUserContextHelpersAndStreamContext(t *testing.T) {
	base := context.Background()
	_, ok := GetUserIDFromContext(base)
	assert.False(t, ok)

	_, ok = GetUserEmailFromContext(base)
	assert.False(t, ok)

	_, ok = GetUserAdminFromContext(base)
	assert.False(t, ok)

	ctx := SetUserIDToContext(base, 123)
	ctx = SetUserEmailToContext(ctx, "admin@example.test")
	ctx = SetUserAdminToContext(ctx, true)

	userID, ok := GetUserIDFromContext(ctx)
	assert.True(t, ok)
	assert.Equal(t, uint(123), userID)

	email, ok := GetUserEmailFromContext(ctx)
	assert.True(t, ok)
	assert.Equal(t, "admin@example.test", email)

	isAdmin, ok := GetUserAdminFromContext(ctx)
	assert.True(t, ok)
	assert.True(t, isAdmin)

	wrapped := &streamWithContext{ctx: ctx}
	assert.Equal(t, ctx, wrapped.Context())
}
