package main

import (
	"fmt"

	"github.com/AnixOps/anix-control/v4/internal/model"
)

// buildPlans converts old v2_plan rows into model.Plan. Old IDs are
// preserved verbatim so v2_user.plan_id references stay valid without
// remapping.
func buildPlans(t *DumpTable) ([]*model.Plan, error) {
	plans := make([]*model.Plan, 0, len(t.Rows))
	for _, r := range t.Rows {
		id, err := mustUint(r, "id")
		if err != nil {
			return nil, fmt.Errorf("v2_plan: %w", err)
		}
		groupID, err := mustUint(r, "group_id")
		if err != nil {
			return nil, fmt.Errorf("v2_plan id=%d: %w", id, err)
		}
		transferEnable, err := mustInt64(r, "transfer_enable")
		if err != nil {
			return nil, fmt.Errorf("v2_plan id=%d: %w", id, err)
		}
		show, err := mustInt(r, "show")
		if err != nil {
			return nil, fmt.Errorf("v2_plan id=%d: %w", id, err)
		}
		renew, err := mustInt(r, "renew")
		if err != nil {
			return nil, fmt.Errorf("v2_plan id=%d: %w", id, err)
		}
		createdAt, err := unixTime(r, "created_at")
		if err != nil {
			return nil, fmt.Errorf("v2_plan id=%d: %w", id, err)
		}
		updatedAt, err := unixTime(r, "updated_at")
		if err != nil {
			return nil, fmt.Errorf("v2_plan id=%d: %w", id, err)
		}

		plan := &model.Plan{
			ID:                 id,
			GroupID:            groupID,
			TransferEnable:     transferEnable,
			SpeedLimit:         int64Ptr(r, "speed_limit"),
			Name:               str(r, "name"),
			Content:            strPtr(r, "content"),
			Show:               show,
			Sort:               intPtr(r, "sort"),
			Renew:              renew,
			ResetPrice:         int64Ptr(r, "reset_price"),
			ResetTrafficMethod: intPtr(r, "reset_traffic_method"),
			CapacityLimit:      intPtr(r, "capacity_limit"),
			MonthPrice:         int64Ptr(r, "month_price"),
			QuarterPrice:       int64Ptr(r, "quarter_price"),
			HalfYearPrice:      int64Ptr(r, "half_year_price"),
			YearPrice:          int64Ptr(r, "year_price"),
			TwoYearPrice:       int64Ptr(r, "two_year_price"),
			ThreeYearPrice:     int64Ptr(r, "three_year_price"),
			OnetimePrice:       int64Ptr(r, "onetime_price"),
			CreatedAt:          createdAt,
			UpdatedAt:          updatedAt,
		}
		plans = append(plans, plan)
	}
	return plans, nil
}

// buildSubscriptionGroups converts old v2_server_group rows into
// model.SubscriptionGroup. Old IDs are preserved verbatim so they can be
// referenced directly by node/protocol group associations and by
// UserSubscriptionGroup rows built from the old user.group_id column.
func buildSubscriptionGroups(t *DumpTable) ([]*model.SubscriptionGroup, error) {
	groups := make([]*model.SubscriptionGroup, 0, len(t.Rows))
	for _, r := range t.Rows {
		id, err := mustUint(r, "id")
		if err != nil {
			return nil, fmt.Errorf("v2_server_group: %w", err)
		}
		createdAt, err := unixTime(r, "created_at")
		if err != nil {
			return nil, fmt.Errorf("v2_server_group id=%d: %w", id, err)
		}
		updatedAt, err := unixTime(r, "updated_at")
		if err != nil {
			return nil, fmt.Errorf("v2_server_group id=%d: %w", id, err)
		}

		groups = append(groups, &model.SubscriptionGroup{
			ID:        id,
			Name:      str(r, "name"),
			Priority:  0,
			Enable:    1,
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
		})
	}
	return groups, nil
}
