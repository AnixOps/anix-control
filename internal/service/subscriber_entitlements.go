package service

// SubscriberEntitlements are the v2_user fields an administrator edits
// besides the identity fields. The JSON keys are those of the v2 admin user
// update request; an absent or null field is left unchanged.
type SubscriberEntitlements struct {
	Balance        *int64  `json:"balance"`
	PlanID         *uint   `json:"plan_id"`
	GroupID        *uint   `json:"group_id"`
	ExpiredAt      *int64  `json:"expired_at"`
	TransferEnable *int64  `json:"transfer_enable"`
	SpeedLimit     *int64  `json:"speed_limit"`
	DeviceLimit    *int    `json:"device_limit"`
	FlowResetTime  *int64  `json:"flowResetTime"`
	RemarkContent  *string `json:"remark_content"`
}

// Updates returns the v2_user column updates. A new plan also sets the group
// and, unless given explicitly, the traffic and limits from the plan, as the
// v2 admin API does.
func (e SubscriberEntitlements) Updates(plans *PlanService) map[string]any {
	updates := make(map[string]any)
	if e.Balance != nil {
		updates["balance"] = *e.Balance
	}
	if e.PlanID != nil {
		updates["plan_id"] = *e.PlanID
		if plan, err := plans.Get(*e.PlanID); err == nil {
			updates["group_id"] = plan.GroupID
			if e.TransferEnable == nil {
				updates["transfer_enable"] = plan.TransferEnable * 1073741824
			}
			if e.SpeedLimit == nil && plan.SpeedLimit != nil {
				updates["speed_limit"] = *plan.SpeedLimit
			}
			if e.DeviceLimit == nil && plan.DeviceLimit != nil {
				updates["device_limit"] = *plan.DeviceLimit
			}
		}
	}
	if e.ExpiredAt != nil {
		updates["expired_at"] = *e.ExpiredAt
	}
	if e.TransferEnable != nil {
		updates["transfer_enable"] = *e.TransferEnable
	}
	if e.SpeedLimit != nil {
		updates["speed_limit"] = *e.SpeedLimit
	}
	if e.DeviceLimit != nil {
		updates["device_limit"] = *e.DeviceLimit
	}
	if e.FlowResetTime != nil {
		updates["flow_reset_time"] = *e.FlowResetTime
	}
	if e.RemarkContent != nil {
		updates["remark_content"] = *e.RemarkContent
	}
	if e.GroupID != nil {
		updates["group_id"] = *e.GroupID
	}
	return updates
}
