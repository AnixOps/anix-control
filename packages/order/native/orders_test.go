package native

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSnapshotOfAPlan(t *testing.T) {
	speed, devices := int64(10), 1<<40
	got := snapshot(CatalogPlan{ID: 3, GroupID: 2, TransferEnable: 5, SpeedLimit: &speed, DeviceLimit: &devices},
		[]PlanGroup{{PlanID: 3, GroupID: 9}, {PlanID: 3, GroupID: 7}})
	require.Equal(t, uint64(5<<30), got.GetTransferBytes())
	require.Equal(t, int64(10), got.GetSpeedLimitMbps())
	require.Equal(t, int32(1<<31-1), got.GetDeviceLimit())
	require.Equal(t, []uint64{9, 7}, got.GetSubscriptionGroupIds(), "the plan's groups, in the order read")

	bare := snapshot(CatalogPlan{ID: 3}, nil)
	require.Nil(t, bare.SpeedLimitMbps, "absent limits are left unchanged")
	require.Nil(t, bare.DeviceLimit)
	require.Empty(t, bare.GetSubscriptionGroupIds(), "a plan without groups clears the subscriber's")
	require.Zero(t, snapshot(CatalogPlan{TransferEnable: -1}, nil).GetTransferBytes())
	require.Equal(t, uint64(1<<62), snapshot(CatalogPlan{TransferEnable: 1 << 40}, nil).GetTransferBytes())
}

func TestPeriodPrice(t *testing.T) {
	month := int64(1000)
	plan := CatalogPlan{MonthPrice: &month}
	price, err := periodPrice(plan, "month")
	require.NoError(t, err)
	require.Equal(t, int64(1000), price)
	_, err = periodPrice(plan, "half_year")
	require.EqualError(t, err, "该套餐不支持半年付")
	_, err = periodPrice(plan, "weekly")
	require.EqualError(t, err, "无效的付费周期")
	for period := range periodMonths {
		_, err := periodPrice(CatalogPlan{}, period)
		require.ErrorContains(t, err, "该套餐不支持", "every period a completion knows is sold by period")
	}
}

func TestTradeNumberFormat(t *testing.T) {
	at := time.Date(2026, 9, 30, 12, 34, 56, 0, time.Local)
	service := &Service{Now: func() time.Time { return at }}
	first, err := service.tradeNo()
	require.NoError(t, err)
	require.Regexp(t, `^20260930123456[A-Z0-9]{8}$`, first)
	second, err := service.tradeNo()
	require.NoError(t, err)
	require.NotEqual(t, first, second)
	require.Equal(t, "order:42", CompleteRequestID(42))
}
