package grpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

func TestGRPCUserContextHelpersAndStreamContext(t *testing.T) {
	base := context.Background()
	_, ok := GetUserIDFromContext(base)
	assert.False(t, ok)
	assert.Equal(t, uint32(0), GetNodeIDFromContext(base))

	_, ok = GetUserEmailFromContext(base)
	assert.False(t, ok)

	_, ok = GetUserAdminFromContext(base)
	assert.False(t, ok)

	ctx := SetUserIDToContext(base, 123)
	ctx = SetUserEmailToContext(ctx, "admin@example.test")
	ctx = SetUserAdminToContext(ctx, true)
	ctx = SetNodeIDToContext(ctx, 77)

	userID, ok := GetUserIDFromContext(ctx)
	assert.True(t, ok)
	assert.Equal(t, uint(123), userID)

	email, ok := GetUserEmailFromContext(ctx)
	assert.True(t, ok)
	assert.Equal(t, "admin@example.test", email)

	isAdmin, ok := GetUserAdminFromContext(ctx)
	assert.True(t, ok)
	assert.True(t, isAdmin)
	assert.Equal(t, uint32(77), GetNodeIDFromContext(ctx))

	wrapped := &streamWithContext{ctx: ctx}
	assert.Equal(t, ctx, wrapped.Context())
}

func TestServerGracefulShutdownContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	srv := &Server{grpcServer: grpc.NewServer()}
	err := srv.GracefulShutdown(ctx)
	assert.ErrorIs(t, err, context.Canceled)
}
