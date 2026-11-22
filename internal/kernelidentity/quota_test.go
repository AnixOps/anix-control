package kernelidentity

import (
	"context"
	"fmt"
	"testing"

	kernelidentityv1 "github.com/AnixOps/anix-control/sdk/api/kernelidentity/v1"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/quota"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"gorm.io/gorm"
)

func TestCreateSubscriberAsksTheQuotaFirst(t *testing.T) {
	f := newFixture(t)
	existing := f.createSubscriber(t, accountA, "kept@example.test")
	var asked []quota.Kind
	quota.Check = func(_ *gorm.DB, kind quota.Kind) error {
		asked = append(asked, kind)
		return fmt.Errorf("%w: user limit reached", quota.ErrExceeded)
	}
	t.Cleanup(func() { quota.Check = nil })

	_, err := f.server.CreateSubscriber(context.Background(), &kernelidentityv1.CreateSubscriberRequest{
		AccountUuid: "1c7e2d4f-9e6b-4b7f-8b1d-7c3e2e0f5b22", Email: "refused@example.test"})
	requireCode(t, err, codes.FailedPrecondition)
	require.Equal(t, []quota.Kind{quota.Users}, asked)
	var users []model.User
	require.NoError(t, f.db.Find(&users).Error)
	require.Len(t, users, 1, "a refusal creates nothing and keeps the existing user")
	require.Equal(t, existing, users[0].ID)
}
