package handler

import (
	"sort"

)

// orderColumns are the keys of an order's own columns in an answer.
var orderColumns = []string{
	"balance", "callback_no", "commission_balance", "commission_status", "coupon_id", "created_at", "discount_amount", "id",
	"invite_user_id", "paid_at", "payment_id", "period", "plan_id", "refund_amount", "status", "surplus_amount",
	"surplus_order", "total_amount", "trade_no", "type", "updated_at", "user_id",
}

// Secrets and row fields of the buyer and the plan that no answer may carry.
const (
	buyerToken = "buyer-subscription-token"
	buyerUUID  = "8f0e5b3c-buyer-proxy-uuid"
	otherToken = "other-subscription-token"
	otherUUID  = "2a9d7c4e-other-proxy-uuid"
	remark     = "buyer remark only admins wrote"
	planText   = "plan description content"
)

func keys(object map[string]any) []string {
	out := make([]string, 0, len(object))
	for key := range object {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}
