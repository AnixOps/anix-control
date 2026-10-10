package main

import (
	"crypto/hmac"
	"crypto/md5" // #nosec G501 -- EPay's callback protocol signs with MD5; the key is a fake.
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
)

// Batch 3: plan + order + payment + affiliate (commercial edition).
//
// Every secret below is a fake that protects nothing: the gateways point at
// example.com and the staging network has no route out. The EPay key and the
// x402 webhook secret are known here so that a few callbacks can be signed:
// they exercise the paid path (record paid, order completed through the
// kernel) without any provider. Stripe and PayPal deliveries need request
// headers (Stripe-Signature, PayPal-Transmission-*), which Req cannot carry,
// so they only exercise the refusals.
const (
	b3EPayKey      = "staging-epay-merchant-key-not-secret"
	b3X402Secret   = "staging-x402-webhook-secret-not-secret" // #nosec G101 -- synthetic staging secret, not a credential
	b3StripeSecret = "whsec_staging_not_secret"               // #nosec G101 -- synthetic staging secret, not a credential
	// b3MissingTrade is a trade number no seeded record has.
	b3MissingTrade = "STGNOSUCHTRADE0000"
)

// b3PayableTotals are the totals (cents) of the user persona's pending
// orders "order.user.payable", by index: the payment creation routes send
// exactly these amounts (yuan).
var b3PayableTotals = []int64{1990, 990, 4990, 2790, 13990}

// b3RenewalTotal is the total of the renewal orders the signed callbacks and
// the "mark paid" route complete.
const b3RenewalTotal = 1990

func b3Yuan(cents int64) float64 { return float64(cents) / 100 }

func init() {
	registerSeed(30, "batch 3: plans and coupons", seedB3Catalog)
	registerSeed(31, "batch 3: orders", seedB3Orders)
	registerSeed(32, "batch 3: payment gateways and methods", seedB3Gateways)
	registerSeed(33, "batch 3: payment records", seedB3Records)
	registerSeed(34, "batch 3: invite configuration, codes, commissions and withdrawals", seedB3Affiliate)

	// v2_user: plan assignments, order completions (plan, group, traffic,
	// limits, expiry, counters) and commission debits and refunds.
	userIgnore := map[string]string{
		"updated_at":    "set from the handler's clock",
		"last_login_at": "every persona login stamps it",
	}
	registerTables("plan",
		TableSpec{Table: "v2_plan", Ignore: timestamps()},
		TableSpec{Table: "v2_event", Where: "type LIKE 'plan.%'", Ignore: map[string]string{
			"created_at":   "set from the handler's clock",
			"processed_at": "set by the event worker when it gets to the row",
			"status":       "the event worker moves it on its own schedule",
			"payload":      "the JSON of the plan, with its clock created_at and updated_at",
		}},
		TableSpec{Table: "v2_user", Ignore: userIgnore},
	)
	registerTables("order",
		TableSpec{Table: "v2_order", Ignore: timestamps(
			"trade_no", "a created order's trade number is the clock plus random characters",
			"paid_at", "marking an order paid stamps it from the handler's clock",
		)},
		// A coupon created without a window starts at the handler's clock:
		// the row is left out (its answer is masked instead).
		TableSpec{Table: "v2_coupon", Where: "code <> 'STAGE-NOW'", Ignore: timestamps()},
		TableSpec{Table: "v2_user_subscription_group", Key: "user_id,group_id", Ignore: map[string]string{
			"id":         "rows deleted and recreated by a completion take new ids",
			"created_at": "set from the handler's clock",
		}},
		// The kernel's subscriber request ledger. Plan assignments are
		// named from the request's X-Request-ID or a random token, which
		// the replayer does not send: they are left out.
		TableSpec{Table: "v4_kernel_subscriber_request", Key: "request_id", Where: "request_id NOT LIKE 'plan.assign:%'",
			Ignore: map[string]string{"created_at": "set from the handler's clock"}},
	)
	registerTables("payment",
		TableSpec{Table: "v2_payment_gateway", Ignore: timestamps()},
		TableSpec{Table: "v2_payment_record", Ignore: timestamps(
			"trade_no", "a created payment's trade number is the clock plus random hex",
			"wallet_address", "an x402 payment's address is random",
			"paid_at", "a paid callback stamps it from the handler's clock",
			"cancelled_at", "a failed or expired payment is stamped from the handler's clock",
		)},
	)
	registerTables("affiliate",
		TableSpec{Table: "v2_commission_withdraw", Ignore: timestamps(
			"processed_at", "a decision is stamped from the handler's clock",
		)},
		TableSpec{Table: "v2_invite_code", Ignore: timestamps(
			"code", "a generated code is random",
			"expired_at", "a generated code expires code_expire_days after the handler's clock",
		)},
		TableSpec{Table: "v2_invite_config", Ignore: timestamps()},
		TableSpec{Table: "v2_system_config", Key: "key", Where: "key = 'invite.frontend.config'", Ignore: timestamps()},
	)

	registerSpecs(b3PlanSpecs()...)
	registerSpecs(b3OrderSpecs()...)
	registerSpecs(b3PaymentSpecs()...)
	registerSpecs(b3AffiliateSpecs()...)
}

// ---------------------------------------------------------------- plan

func b3PlanSpecs() []RouteSpec {
	created := []string{"data.created_at", "data.updated_at"}
	return []RouteSpec{
		{RouteID: "plan.admin.plans.get", Reads: func(w *World) []Req {
			path := "/api/v2/admin/plans"
			return append([]Req{
				{Persona: Admin, Path: path, Label: "every plan by sort then id"},
				{Persona: Admin2, Path: path, Query: q("page", "2", "page_size", "3", "show", "1"), Label: "ignored pagination and filter"},
				{Persona: Staff, Path: path, Label: "staff"},
			}, forbidden(path)...)
		}},
		{RouteID: "plan.admin.plans.id.get", Reads: func(w *World) []Req {
			var reqs []Req
			for i := 0; i < len(w.IDs["plan"]); i++ {
				reqs = append(reqs, Req{Persona: Admin, Path: fill("/api/v2/admin/plans/:id", w.ID("plan", i)), Label: "plan"})
			}
			return append(reqs,
				Req{Persona: Staff, Path: fill("/api/v2/admin/plans/:id", w.ID("plan.hidden", 0)), Label: "hidden plan"},
				Req{Persona: Admin, Path: fill("/api/v2/admin/plans/:id", w.ID("plan.b3.update", 1)), Label: "plan without month price"},
				Req{Persona: Admin, Path: fill("/api/v2/admin/plans/:id", Missing), Label: "not found"},
				Req{Persona: Admin, Path: "/api/v2/admin/plans/0", Label: "plan zero"},
				Req{Persona: Admin, Path: "/api/v2/admin/plans/x", Label: "invalid id"},
				Req{Persona: Admin, Path: "/api/v2/admin/plans/4294967296", Label: "id beyond 32 bits"},
				Req{Persona: Admin, Path: "/api/v2/admin/plans/0%20OR%201=1", Label: "id that is a condition"},
				Req{Persona: User, Path: fill("/api/v2/admin/plans/:id", w.ID("plan", 0)), Label: "member on admin route"},
				Req{Persona: Anon, Path: fill("/api/v2/admin/plans/:id", w.ID("plan", 0)), Label: "anonymous"},
			)
		}},
		{RouteID: "plan.user.plan.get", Reads: func(w *World) []Req {
			path := "/api/v2/user/plan"
			return []Req{
				{Persona: User, Path: path, Label: "plans on sale"},
				{Persona: Fresh, Path: path, Label: "member without data"},
				{Persona: Expired, Path: path, Label: "expired member"},
				{Persona: User2, Path: path, Query: q("show", "0"), Label: "ignored filter"},
				{Persona: Anon, Path: path, Label: "anonymous"},
			}
		}},
		{RouteID: "plan.admin.plans.post", Writes: func(w *World) []Req {
			path := "/api/v2/admin/plans"
			group := w.ID("node_group", 1)
			return []Req{
				{Persona: Admin, Path: path, Mask: created, Label: "create with every field", Body: map[string]any{
					"group_id": group, "transfer_enable": 80, "speed_limit": 300, "device_limit": 4, "name": "B3 新套餐 created",
					"content": "<p>created by the staging rehearsal</p>", "show": 1, "sort": 30, "renew": 1, "reset_price": 600,
					"reset_traffic_method": 1, "capacity_limit": 50, "month_price": 1290, "quarter_price": 3690, "half_year_price": 6990,
					"year_price": 12900, "two_year_price": 23900, "three_year_price": 32900,
				}},
				{Persona: Admin, Path: path, Mask: created, Body: map[string]any{"name": "B3 bare"}, Label: "create with defaults"},
				{Persona: Staff, Path: path, Mask: created, Label: "a subscription template without prices",
					Body: map[string]any{"name": "B3 template", "group_id": group, "transfer_enable": 100, "speed_limit": 50, "device_limit": 3}},
				{Persona: Admin, Path: path, Mask: created, Body: map[string]any{"name": "B3 no renew", "renew": 0}, Label: "renew zero keeps the column default"},
				{Persona: Admin, Path: path, Body: map[string]any{"id": w.ID("plan", 0), "name": "Taken"}, Label: "create with a taken id"},
				{Persona: Admin, Path: path, Body: map[string]any{"transfer_enable": "a lot"}, Label: "wrong field type"},
				{Persona: Admin, Path: path, Body: `{"name":`, Label: "malformed body"},
				{Persona: Admin, Path: path, Label: "no body"},
				{Persona: User, Path: path, Body: map[string]any{"name": "x"}, Label: "member on admin route"},
				{Persona: Anon, Path: path, Body: map[string]any{"name": "x"}, Label: "anonymous"},
			}
		}},
		{RouteID: "plan.admin.plans.id.put", Writes: func(w *World) []Req {
			mask := []string{"data.updated_at", "data.created_at"}
			at := func(id any) string { return fill("/api/v2/admin/plans/:id", id) }
			return []Req{
				{Persona: Admin, Path: at(w.ID("plan.b3.update", 0)), Mask: mask, Label: "update keeps the creation time", Body: map[string]any{
					"name": "B3 改名 renamed", "group_id": w.ID("node_group", 2), "transfer_enable": 220, "show": 0, "sort": 9,
					"month_price": 3500, "created_at": "2020-01-01T00:00:00Z",
				}},
				{Persona: Admin, Path: at(w.ID("plan.b3.update", 1)), Mask: mask, Body: map[string]any{}, Label: "an empty body clears every field"},
				{Persona: Admin, Path: at(w.ID("plan.b3.update", 2)), Mask: mask, Body: map[string]any{"id": w.ID("plan", 0), "name": "Moved"},
					Label: "the body cannot move another plan"},
				{Persona: Admin, Path: at(Missing), Body: map[string]any{"name": "x"}, Label: "not found"},
				{Persona: Admin, Path: at(0), Body: map[string]any{"name": "x"}, Label: "plan zero"},
				{Persona: Admin, Path: "/api/v2/admin/plans/x", Body: map[string]any{"name": "x"}, Label: "invalid id"},
				{Persona: Admin, Path: at(w.ID("plan.b3.update", 0)), Body: map[string]any{"sort": "first"}, Label: "wrong field type"},
				{Persona: Admin, Path: at(w.ID("plan.b3.update", 0)), Label: "no body"},
				{Persona: User, Path: at(w.ID("plan.b3.update", 0)), Body: map[string]any{"name": "x"}, Label: "member on admin route"},
			}
		}},
		{RouteID: "plan.admin.plans.id.delete", Writes: func(w *World) []Req {
			at := func(id any) string { return fill("/api/v2/admin/plans/:id", id) }
			return []Req{
				{Persona: Admin, Path: at(w.ID("plan.b3.delete", 0)), Label: "delete"},
				{Persona: Admin, Path: at(w.ID("plan.b3.delete", 0)), Label: "delete again"},
				{Persona: Staff, Path: at(w.ID("plan.b3.delete", 1)), Label: "staff delete"},
				{Persona: Admin, Path: at(w.ID("plan", 0)), Label: "a plan members hold (foreign key)"},
				{Persona: Admin, Path: at(Missing), Label: "not found"},
				{Persona: Admin, Path: at(0), Label: "plan zero"},
				{Persona: Admin, Path: "/api/v2/admin/plans/x", Label: "invalid id"},
				{Persona: Admin, Path: "/api/v2/admin/plans/0%20OR%201=1", Label: "id that is a condition"},
				{Persona: User, Path: at(w.ID("plan.b3.delete", 1)), Label: "member on admin route"},
			}
		}},
		{RouteID: "plan.admin.plans.id.assign.post", Writes: func(w *World) []Req {
			at := func(id any) string { return fill("/api/v2/admin/plans/:id/assign", id) }
			target := func(i int) uint { return w.ID("b3.assign.target", i) }
			return []Req{
				{Persona: Admin, Path: at(w.ID("plan", 1)), Body: map[string]any{"user_id": target(0), "expire_at": 2000000000}, Label: "assign with an expiry"},
				{Persona: Admin, Path: at(w.ID("plan", 0)), Body: map[string]any{"user_id": target(1)}, Label: "assign keeps the expiry"},
				{Persona: Admin, Path: at(w.ID("plan", 2)), Body: map[string]any{"user_id": target(2), "expire_at": nil}, Label: "a null expiry is no expiry"},
				{Persona: Admin, Path: at(w.ID("plan", 3)), Body: map[string]any{"user_id": target(3), "expire_at": 0}, Label: "expiry zero"},
				{Persona: Admin, Path: at(w.ID("plan", 1)), Body: map[string]any{"user_id": w.ID("user.banned", 1), "expire_at": 2000000000}, Label: "a banned subscriber"},
				{Persona: Staff, Path: at(w.ID("plan.hidden", 0)), Body: map[string]any{"user_id": target(4)}, Label: "a hidden plan"},
				{Persona: Admin, Path: at(w.ID("plan", 1)), Body: map[string]any{"user_id": target(0), "expire_at": 2000000000}, Label: "the same grant again"},
				{Persona: Admin, Path: at(w.ID("plan", 1)), Body: map[string]any{"user_id": Missing}, Label: "unknown subscriber"},
				{Persona: Admin, Path: at(w.ID("plan", 1)), Body: map[string]any{"user_id": 4294967296}, Label: "subscriber id beyond the contract"},
				{Persona: Admin, Path: at(Missing), Body: map[string]any{"user_id": target(5)}, Label: "unknown plan"},
				{Persona: Admin, Path: at(Missing), Body: map[string]any{"user_id": Missing}, Label: "unknown plan and subscriber"},
				{Persona: Admin, Path: at(0), Body: map[string]any{"user_id": target(5)}, Label: "plan zero"},
				{Persona: Admin, Path: "/api/v2/admin/plans/x/assign", Body: map[string]any{"user_id": target(5)}, Label: "invalid plan id"},
				{Persona: Admin, Path: "/api/v2/admin/plans/0%20OR%201=1/assign", Body: map[string]any{"user_id": target(5)}, Label: "plan id that is a condition"},
				{Persona: Admin, Path: at(w.ID("plan", 1)), Body: map[string]any{}, Label: "no subscriber"},
				{Persona: Admin, Path: at(w.ID("plan", 1)), Body: map[string]any{"user_id": 0}, Label: "subscriber zero"},
				{Persona: Admin, Path: at(w.ID("plan", 1)), Body: map[string]any{"user_id": -1}, Label: "negative subscriber"},
				{Persona: Admin, Path: at(w.ID("plan", 1)), Body: map[string]any{"user_id": target(5), "expire_at": "soon"}, Label: "expiry that is not a number"},
				{Persona: Admin, Path: at(w.ID("plan", 1)), Label: "no body"},
				{Persona: User, Path: at(w.ID("plan", 1)), Body: map[string]any{"user_id": w.PersonaID(User)}, Label: "member on admin route"},
			}
		}},
	}
}

// ---------------------------------------------------------------- order

func b3OrderSpecs() []RouteSpec {
	return []RouteSpec{
		{RouteID: "order.admin.coupon.get", Reads: func(w *World) []Req {
			path := "/api/v2/admin/coupon"
			return append([]Req{
				{Persona: Admin, Path: path, Label: "every coupon newest first"},
				{Persona: Staff, Path: path, Label: "staff"},
				{Persona: Admin2, Path: path, Query: q("page", "2", "code", "STAGE"), Label: "ignored pagination and filter"},
			}, forbidden(path)...)
		}},
		{RouteID: "order.admin.coupon.post", Writes: func(w *World) []Req {
			path := "/api/v2/admin/coupon"
			created := []string{"data.data.created_at", "data.data.updated_at"}
			start, end := w.Base.Add(Days(-1)).Unix(), w.Base.Add(Days(3650)).Unix()
			return []Req{
				{Persona: Admin, Path: path, Mask: created, Label: "create a percentage coupon",
					Body: map[string]any{"code": "STAGE-NEW-PCT", "name": "新券 25%", "type": 1, "value": 25, "limit_use": 10, "started_at": start, "ended_at": end}},
				{Persona: Staff, Path: path, Mask: created, Label: "create a fixed coupon",
					Body: map[string]any{"code": "STAGE-NEW-FIX", "name": "Fixed 3.00", "type": 2, "value": 300, "started_at": start, "ended_at": end}},
				{Persona: Admin, Path: path, Mask: append([]string{"data.data.started_at", "data.data.ended_at"}, created...), Label: "create with the default window",
					Body: map[string]any{"code": "STAGE-NOW", "name": "Now", "type": 2, "value": 300}},
				{Persona: Admin, Path: path, Body: map[string]any{"code": w.Str("coupon.code.valid.percent", 0), "name": "Again", "type": 1, "value": 5}, Label: "a taken code"},
				{Persona: Admin, Path: path, Body: map[string]any{"code": "STAGE-ZERO", "name": "Zero", "type": 1, "value": 0}, Label: "a value of zero"},
				{Persona: Admin, Path: path, Body: map[string]any{"code": "STAGE-T3", "name": "Type", "type": 3, "value": 5}, Label: "an unknown type"},
				{Persona: Admin, Path: path, Body: map[string]any{"code": "STAGE-NEG", "name": "Limit", "type": 1, "value": 5, "limit_use": -1}, Label: "a negative limit"},
				{Persona: Admin, Path: path, Body: map[string]any{"code": strings.Repeat("L", 65), "name": "Long", "type": 1, "value": 5}, Label: "a code too long"},
				{Persona: Admin, Path: path, Body: map[string]any{"code": "STAGE-W", "name": "Wrong", "type": "one", "value": 5}, Label: "wrong field type"},
				{Persona: Admin, Path: path, Label: "no body"},
				{Persona: User, Path: path, Body: map[string]any{"code": "STAGE-MEMBER", "name": "m", "type": 1, "value": 5}, Label: "member on admin route"},
			}
		}},
		{RouteID: "order.admin.coupon.id.delete", Writes: func(w *World) []Req {
			at := func(id any) string { return fill("/api/v2/admin/coupon/:id", id) }
			return []Req{
				{Persona: Admin, Path: at(w.ID("coupon.deletable", 0)), Label: "delete"},
				{Persona: Admin, Path: at(w.ID("coupon.deletable", 0)), Label: "delete again"},
				{Persona: Staff, Path: at(w.ID("coupon.deletable", 1)), Label: "staff delete"},
				{Persona: Admin, Path: at(Missing), Label: "unknown coupon"},
				{Persona: Admin, Path: at(0), Label: "coupon zero"},
				{Persona: Admin, Path: "/api/v2/admin/coupon/x", Label: "invalid id"},
				{Persona: Admin, Path: "/api/v2/admin/coupon/0%20OR%201=1", Label: "id that is a condition"},
				{Persona: User, Path: at(w.ID("coupon.deletable", 2)), Label: "member on admin route"},
			}
		}},
		{RouteID: "order.user.coupon.check.post", Writes: func(w *World) []Req {
			path := "/api/v2/user/coupon/check"
			plan := w.ID("plan.visible", 0)
			check := func(key string) map[string]any {
				return map[string]any{"code": w.Str("coupon.code."+key, 0), "plan_id": plan}
			}
			return []Req{
				{Persona: User, Path: path, Body: check("valid.percent"), Label: "a percentage coupon"},
				{Persona: User2, Path: path, Body: check("valid.fixed"), Label: "a fixed coupon"},
				{Persona: User, Path: path, Body: check("expired"), Label: "an expired coupon"},
				{Persona: User, Path: path, Body: check("future"), Label: "a coupon not started"},
				{Persona: User, Path: path, Body: check("exhausted"), Label: "a used up coupon"},
				{Persona: User, Path: path, Body: check("limited"), Label: "a coupon with uses left"},
				{Persona: User, Path: path, Body: check("plan_limited"), Label: "a plan-limited coupon for another plan"},
				{Persona: User, Path: path, Body: check("negative_limit"), Label: "a negative limit is no limit here"},
				{Persona: Fresh, Path: path, Body: check("zero_limit"), Label: "a zero limit is unlimited"},
				{Persona: User, Path: path, Body: map[string]any{"code": "STAGE-NOPE", "plan_id": plan}, Label: "an unknown code"},
				{Persona: User, Path: path, Body: map[string]any{"code": w.Str("coupon.code.valid.percent", 0)}, Label: "no plan"},
				{Persona: User, Path: path, Body: map[string]any{"plan_id": plan}, Label: "no code"},
				{Persona: User, Path: path, Label: "no body"},
				{Persona: Anon, Path: path, Body: check("valid.percent"), Label: "anonymous"},
			}
		}},
		{RouteID: "order.admin.orders.get", Reads: func(w *World) []Req {
			path := "/api/v2/admin/orders"
			user := fmt.Sprint(w.PersonaID(User))
			reqs := []Req{
				{Persona: Admin, Path: path, Label: "every order newest first"},
				{Persona: Staff, Path: path, Label: "staff"},
				{Persona: Admin, Path: path, Query: q("page", "2", "page_size", "5"), Label: "a page"},
				{Persona: Admin, Path: path, Query: q("page", "9999"), Label: "a page beyond the last"},
				{Persona: Admin, Path: path, Query: q("page", "0", "page_size", "2"), Label: "page zero"},
				{Persona: Admin, Path: path, Query: q("page_size", "1000"), Label: "a page size beyond the maximum"},
				{Persona: Admin, Path: path, Query: q("page_size", "x"), Label: "a page size that is not a number"},
				{Persona: Admin, Path: path, Query: q("page", "", "page_size", ""), Label: "empty page parameters"},
				{Persona: Admin, Path: path, Query: q("user_id", user), Label: "by buyer"},
				{Persona: Admin, Path: path, Query: q("user_id", fmt.Sprint(w.PersonaID(Fresh))), Label: "a buyer without orders"},
				{Persona: Admin, Path: path, Query: q("user_id", "x"), Label: "a buyer id that does not parse"},
				{Persona: Admin, Path: path, Query: q("status", "paid"), Label: "a status that does not parse"},
				{Persona: Admin, Path: path, Query: q("trade_no", w.Str("order.trade_no.user", 3)), Label: "by trade number"},
				{Persona: Admin, Path: path, Query: q("trade_no", b3MissingTrade), Label: "an unknown trade number"},
				{Persona: Admin, Path: path, Query: q("email", "member00"), Label: "by the start of the e-mail"},
				{Persona: Admin, Path: path, Query: q("email", "example.org"), Label: "by the domain"},
				{Persona: Admin, Path: path, Query: q("email", "%example"), Label: "an e-mail with a wildcard"},
				{Persona: Admin, Path: path, Query: q("email", "nobody"), Label: "an e-mail no buyer has"},
				{Persona: Admin2, Path: path, Query: q("user_id", user, "status", "0", "type", "2", "email", "member", "page_size", "3"), Label: "every filter"},
			}
			for status := 0; status <= 4; status++ {
				reqs = append(reqs, Req{Persona: Admin, Path: path, Query: q("status", strconv.Itoa(status)), Label: "by status " + strconv.Itoa(status)})
			}
			for kind := 1; kind <= 4; kind++ {
				reqs = append(reqs, Req{Persona: Admin, Path: path, Query: q("type", strconv.Itoa(kind), "page", "2"), Label: "by type " + strconv.Itoa(kind)})
			}
			return append(reqs, forbidden(path)...)
		}},
		{RouteID: "order.admin.orders.id.get", Reads: func(w *World) []Req {
			at := func(id any) string { return fill("/api/v2/admin/orders/:id", id) }
			var reqs []Req
			for status := 0; status <= 4; status++ {
				reqs = append(reqs, Req{Persona: Admin, Path: at(w.ID(fmt.Sprintf("order.user.status.%d", status), status)), Label: "order in status " + strconv.Itoa(status)})
			}
			for i := 0; i < 6; i++ {
				reqs = append(reqs, Req{Persona: Admin, Path: at(w.ID("order.other", i*17)), Label: "another member's order"})
			}
			return append(reqs,
				Req{Persona: Staff, Path: at(w.ID("order.invited", 0)), Label: "an invited buyer's order"},
				Req{Persona: Admin, Path: at(Missing), Label: "unknown order"},
				Req{Persona: Admin, Path: at(0), Label: "order zero"},
				Req{Persona: Admin, Path: "/api/v2/admin/orders/x", Label: "invalid id"},
				Req{Persona: Admin, Path: "/api/v2/admin/orders/0%20OR%201=1", Label: "id that is a condition"},
				Req{Persona: Admin, Path: "/api/v2/admin/orders/4294967296", Label: "id beyond 32 bits"},
				Req{Persona: User, Path: at(w.ID("order.user.status.0", 0)), Label: "member on admin route"},
				Req{Persona: Anon, Path: at(w.ID("order.user.status.0", 0)), Label: "anonymous"},
			)
		}},
		{RouteID: "order.admin.orders.stats.get", Reads: func(w *World) []Req {
			path := "/api/v2/admin/orders/stats"
			return append([]Req{
				{Persona: Admin, Path: path, Label: "counts and revenue"},
				{Persona: Staff, Path: path, Query: q("start", "2026-01-01"), Label: "ignored filter"},
			}, forbidden(path)...)
		}},
		{RouteID: "order.user.order.get", Reads: func(w *World) []Req {
			path := "/api/v2/user/order"
			return []Req{
				{Persona: User, Path: path, Label: "own orders"},
				{Persona: User2, Path: path, Label: "own orders"},
				{Persona: Fresh, Path: path, Label: "no orders"},
				{Persona: Expired, Path: path, Label: "expired member"},
				{Persona: Admin, Path: path, Label: "an administrator without orders"},
				{Persona: User, Path: path, Query: q("page", "2", "page_size", "5"), Label: "a page"},
				{Persona: User, Path: path, Query: q("page", "99"), Label: "a page beyond the last"},
				{Persona: User, Path: path, Query: q("page_size", "1000"), Label: "a page size beyond the maximum"},
				{Persona: User, Path: path, Query: q("page_size", "-1"), Label: "a negative page size"},
				{Persona: User, Path: path, Query: q("page", "-2", "page_size", "1"), Label: "a negative page"},
				{Persona: User2, Path: path, Query: q("user_id", fmt.Sprint(w.PersonaID(User)), "status", "0", "email", "member"), Label: "query parameters cannot widen the list"},
				{Persona: Anon, Path: path, Label: "anonymous"},
			}
		}},
		{RouteID: "order.user.order.id.get", Reads: func(w *World) []Req {
			at := func(id any) string { return fill("/api/v2/user/order/:id", id) }
			var reqs []Req
			for status := 0; status <= 4; status++ {
				reqs = append(reqs, Req{Persona: User, Path: at(w.ID(fmt.Sprintf("order.user.status.%d", status), 1)), Label: "own order in status " + strconv.Itoa(status)})
			}
			return append(reqs,
				Req{Persona: User2, Path: at(w.ID("order.user2", 0)), Label: "own order"},
				Req{Persona: User, Path: at(w.ID("order.user2", 0)), Label: "another member's order"},
				Req{Persona: Admin, Path: at(w.ID("order.user.status.0", 0)), Label: "an administrator sees only their own"},
				Req{Persona: User, Path: at(Missing), Label: "unknown order"},
				Req{Persona: User, Path: at(0), Label: "order zero"},
				Req{Persona: User, Path: "/api/v2/user/order/x", Label: "invalid id"},
				Req{Persona: User, Path: fill("/api/v2/user/order/:id", w.ID("order.user.status.0", 0)) + "%20OR%201=1", Label: "id that is a condition"},
				Req{Persona: User, Path: "/api/v2/user/order/4294967297", Label: "id beyond 32 bits"},
				Req{Persona: Anon, Path: at(w.ID("order.user.status.0", 0)), Label: "anonymous"},
			)
		}},
		{RouteID: "order.user.order.save.post", Writes: func(w *World) []Req {
			path := "/api/v2/user/order/save"
			created := []string{"data.trade_no", "data.created_at", "data.updated_at"}
			plan := func(i int) uint { return w.ID("plan", i) }
			coupon := func(key string) uint { return w.ID("coupon."+key, 0) }
			return []Req{
				{Persona: User, Path: path, Mask: created, Label: "a renewal",
					Body: map[string]any{"plan_id": w.ID("order.user.plan", 0), "period": w.Str("order.user.renew_period", 0)}},
				{Persona: User, Path: path, Mask: created, Label: "an upgrade", Body: map[string]any{"plan_id": w.ID("order.user.upgrade_plan", 0), "period": "month"}},
				{Persona: Fresh, Path: path, Mask: created, Label: "a new purchase", Body: map[string]any{"plan_id": plan(0), "period": "quarter"}},
				{Persona: Expired, Path: path, Mask: created, Label: "an expired member", Body: map[string]any{"plan_id": plan(2), "period": "year"}},
				{Persona: Fresh, Path: path, Mask: created, Label: "a hidden plan", Body: map[string]any{"plan_id": plan(4), "period": "month"}},
				{Persona: Fresh, Path: path, Mask: created, Label: "a sold out plan", Body: map[string]any{"plan_id": plan(6), "period": "month"}},
				{Persona: Fresh, Path: path, Mask: created, Label: "a free plan", Body: map[string]any{"plan_id": plan(7), "period": "month"}},
				{Persona: Fresh, Path: path, Mask: created, Label: "a onetime plan", Body: map[string]any{"plan_id": plan(3), "period": "onetime"}},
				{Persona: Fresh, Path: path, Mask: created, Label: "a percentage coupon", Body: map[string]any{"plan_id": plan(2), "period": "year", "coupon_id": coupon("valid.percent")}},
				{Persona: User2, Path: path, Mask: created, Label: "a fixed coupon", Body: map[string]any{"plan_id": plan(0), "period": "month", "coupon_id": coupon("valid.fixed")}},
				{Persona: Fresh, Path: path, Mask: created, Label: "a coupon above the price", Body: map[string]any{"plan_id": plan(0), "period": "month", "coupon_id": coupon("valid.big")}},
				{Persona: Fresh, Path: path, Mask: created, Label: "an expired coupon", Body: map[string]any{"plan_id": plan(0), "period": "month", "coupon_id": coupon("expired")}},
				{Persona: Fresh, Path: path, Mask: created, Label: "a coupon not started", Body: map[string]any{"plan_id": plan(0), "period": "month", "coupon_id": coupon("future")}},
				{Persona: Fresh, Path: path, Mask: created, Label: "a used up coupon", Body: map[string]any{"plan_id": plan(0), "period": "month", "coupon_id": coupon("exhausted")}},
				{Persona: Fresh, Path: path, Mask: created, Label: "a plan-limited coupon", Body: map[string]any{"plan_id": plan(0), "period": "month", "coupon_id": coupon("plan_limited")}},
				{Persona: Fresh, Path: path, Mask: created, Label: "a negative limit applies no coupon", Body: map[string]any{"plan_id": plan(0), "period": "month", "coupon_id": coupon("negative_limit")}},
				{Persona: Fresh, Path: path, Mask: created, Label: "a zero limit is unlimited", Body: map[string]any{"plan_id": plan(0), "period": "month", "coupon_id": coupon("zero_limit")}},
				{Persona: Fresh, Path: path, Mask: created, Label: "an unknown coupon is kept without a discount", Body: map[string]any{"plan_id": plan(0), "period": "month", "coupon_id": Missing}},
				{Persona: Fresh, Path: path, Mask: created, Label: "coupon zero", Body: map[string]any{"plan_id": plan(0), "period": "month", "coupon_id": 0}},
				{Persona: Fresh, Path: path, Body: map[string]any{"plan_id": plan(0), "period": "half_year"}, Label: "a period the plan does not sell"},
				{Persona: Fresh, Path: path, Body: map[string]any{"plan_id": plan(3), "period": "month"}, Label: "a onetime plan for a month"},
				{Persona: Fresh, Path: path, Body: map[string]any{"plan_id": plan(1), "period": "reset_price"}, Label: "the reset period is not sold here"},
				{Persona: Fresh, Path: path, Body: map[string]any{"plan_id": plan(0), "period": "weekly"}, Label: "an unknown period"},
				{Persona: Fresh, Path: path, Body: map[string]any{"plan_id": Missing, "period": "month"}, Label: "an unknown plan"},
				{Persona: Fresh, Path: path, Body: map[string]any{"period": "month"}, Label: "no plan"},
				{Persona: Fresh, Path: path, Body: map[string]any{"plan_id": plan(0)}, Label: "no period"},
				{Persona: Fresh, Path: path, Body: `{"plan_id":`, Label: "invalid JSON"},
				{Persona: Fresh, Path: path, Label: "no body"},
				{Persona: Anon, Path: path, Body: map[string]any{"plan_id": plan(0), "period": "month"}, Label: "anonymous"},
			}
		}},
		{RouteID: "order.admin.orders.id.cancel.post", Writes: func(w *World) []Req {
			at := func(id any) string { return fill("/api/v2/admin/orders/:id/cancel", id) }
			return []Req{
				{Persona: Admin, Path: at(w.ID("order.admin.pending", 0)), Label: "cancel a pending order"},
				{Persona: Staff, Path: at(w.ID("order.admin.pending", 1)), Label: "staff cancel"},
				{Persona: Admin, Path: at(w.ID("order.admin.pending", 0)), Label: "cancel twice"},
				{Persona: Admin, Path: at(w.ID("order.admin.completed", 0)), Label: "cancel a completed order"},
				{Persona: Admin, Path: at(w.ID("order.admin.paid", 0)), Label: "cancel a paid order"},
				{Persona: Admin, Path: at(Missing), Label: "unknown order"},
				{Persona: Admin, Path: at(0), Label: "order zero"},
				{Persona: Admin, Path: "/api/v2/admin/orders/x/cancel", Label: "invalid id"},
				{Persona: Admin, Path: "/api/v2/admin/orders/4294967296/cancel", Label: "id beyond 32 bits"},
				{Persona: User, Path: at(w.ID("order.user.status.0", 4)), Label: "member on admin route"},
			}
		}},
		{RouteID: "order.admin.orders.id.paid.post", Writes: func(w *World) []Req {
			at := func(id any) string { return fill("/api/v2/admin/orders/:id/paid", id) }
			return []Req{
				{Persona: Admin, Path: at(w.ID("order.renewal.markpaid", 0)), Label: "a renewal extends the current expiry"},
				{Persona: Admin, Path: at(w.ID("order.renewal.markpaid", 0)), Label: "marking twice grants once"},
				{Persona: Staff, Path: at(w.ID("order.renewal.markpaid", 1)), Label: "staff marks a renewal paid"},
				{Persona: Admin, Path: at(w.ID("order.renewal.markpaid.paid", 0)), Label: "an order already paid"},
				{Persona: Admin, Path: at(w.ID("order.admin.cancelled", 0)), Label: "a cancelled order"},
				{Persona: Admin, Path: at(Missing), Label: "unknown order"},
				{Persona: Admin, Path: at(0), Label: "order zero"},
				{Persona: Admin, Path: "/api/v2/admin/orders/x/paid", Label: "invalid id"},
				{Persona: Admin, Path: "/api/v2/admin/orders/0%20OR%201=1/paid", Label: "id that is a condition"},
				{Persona: User, Path: at(w.ID("order.user.status.0", 4)), Label: "member on admin route"},
			}
		}},
		{RouteID: "order.admin.orders.id.status.put", Writes: func(w *World) []Req {
			at := func(id any) string { return fill("/api/v2/admin/orders/:id/status", id) }
			return []Req{
				{Persona: Admin, Path: at(w.ID("order.admin.pending", 2)), Body: map[string]any{"status": 1}, Label: "paid sets the payment time"},
				{Persona: Admin, Path: at(w.ID("order.admin.pending", 2)), Body: map[string]any{"status": 3}, Label: "completed grants nothing"},
				{Persona: Staff, Path: at(w.ID("order.admin.pending", 3)), Body: map[string]any{"status": 2}, Label: "cancelled"},
				{Persona: Admin, Path: at(w.ID("order.admin.cancelled", 1)), Body: map[string]any{"status": 1}, Label: "a cancelled order paid"},
				{Persona: Admin, Path: at(w.ID("order.admin.pending", 4)), Body: map[string]any{"status": 0}, Label: "pending fails the required binding"},
				{Persona: Admin, Path: at(w.ID("order.admin.pending", 4)), Body: map[string]any{"status": 4}, Label: "an unknown status"},
				{Persona: Admin, Path: at(w.ID("order.admin.pending", 4)), Body: map[string]any{"status": "2"}, Label: "a status that is a string"},
				{Persona: Admin, Path: at(Missing), Body: map[string]any{"status": 2}, Label: "unknown order"},
				{Persona: Admin, Path: at(0), Body: map[string]any{"status": 2}, Label: "order zero"},
				{Persona: Admin, Path: "/api/v2/admin/orders/x/status", Body: map[string]any{"status": 2}, Label: "invalid id"},
				{Persona: Admin, Path: at(w.ID("order.admin.pending", 5)), Label: "no body"},
				{Persona: User, Path: at(w.ID("order.admin.pending", 5)), Body: map[string]any{"status": 2}, Label: "member on admin route"},
			}
		}},
	}
}

// ---------------------------------------------------------------- payment

func b3PaymentSpecs() []RouteSpec {
	return []RouteSpec{
		{RouteID: "payment.admin.payment.gateways.get", Reads: func(w *World) []Req {
			path := "/api/v2/admin/payment/gateways"
			return append([]Req{
				{Persona: Admin, Path: path, Label: "every gateway, secrets redacted"},
				{Persona: Staff, Path: path, Label: "staff"},
				{Persona: Admin2, Path: path, Query: q("type", "epay", "enabled", "1"), Label: "ignored filters"},
			}, forbidden(path)...)
		}},
		{RouteID: "payment.admin.payment.records.get", Reads: func(w *World) []Req {
			path := "/api/v2/admin/payment/records"
			reqs := []Req{
				{Persona: Admin, Path: path, Label: "every record newest first"},
				{Persona: Staff, Path: path, Query: q("page", "2", "page_size", "7"), Label: "a page"},
				{Persona: Admin, Path: path, Query: q("page", "0", "page_size", "500"), Label: "page bounds"},
				{Persona: Admin, Path: path, Query: q("page", "x"), Label: "a page that is not a number"},
				{Persona: Admin, Path: path, Query: q("page", "999"), Label: "a page beyond the last"},
				{Persona: Admin, Path: path, Query: q("status", "bogus"), Label: "an unknown status filters nothing"},
				{Persona: Admin, Path: path, Query: q("gateway_type", "epay"), Label: "by gateway type"},
				{Persona: Admin, Path: path, Query: q("gateway_type", "crypto", "status", "paid"), Label: "crypto and paid"},
				{Persona: Admin, Path: path, Query: q("gateway_type", "fiat"), Label: "fiat"},
				{Persona: Admin, Path: path, Query: q("gateway_type", "nothing"), Label: "an unknown gateway type"},
			}
			for _, status := range []string{"0", "1", "2", "3", "4", "pending", "PAID", " failed ", "cancelled", "refunded"} {
				reqs = append(reqs, Req{Persona: Admin, Path: path, Query: q("status", status), Label: "by status " + strings.TrimSpace(status)})
			}
			return append(reqs, forbidden(path)...)
		}},
		{RouteID: "payment.admin.payment.stats.get", Reads: func(w *World) []Req {
			path := "/api/v2/admin/payment/stats"
			return append([]Req{
				{Persona: Admin, Path: path, Label: "the last 30 days"},
				{Persona: Admin, Path: path, Query: q("start", "2026-07-01", "end", "2026-09-30"), Label: "the seeded quarter"},
				{Persona: Staff, Path: path, Query: q("start", "2026-08-25", "end", "2026-09-05"), Label: "around the seed base"},
				{Persona: Admin, Path: path, Query: q("start", "2020-01-01", "end", "2020-01-31"), Label: "no records"},
				{Persona: Admin, Path: path, Query: q("start", "2026-09-30", "end", "2026-07-01"), Label: "an inverted range"},
				{Persona: Admin, Path: path, Query: q("start", "yesterday"), Label: "invalid start date"},
				{Persona: Admin, Path: path, Query: q("start", "2026-07-01", "end", "2026/09/30"), Label: "invalid end date"},
			}, forbidden(path)...)
		}},
		{RouteID: "payment.payment.methods.get", Reads: func(w *World) []Req {
			path := "/api/v2/payment/methods"
			return []Req{
				{Persona: Anon, Path: path, Label: "anonymous"},
				{Persona: User, Path: path, Label: "member"},
				{Persona: Fresh, Path: path, Query: q("provider", "x402"), Label: "ignored filter"},
			}
		}},
		{RouteID: "payment.payment.status.trade_no.get", Reads: func(w *World) []Req {
			at := func(tradeNo string) string { return fill("/api/v2/payment/status/:trade_no", tradeNo) }
			var reqs []Req
			for _, key := range []string{"pending", "paid", "cancelled", "refunded", "expired", "x402.pending", "x402.paid", "fiat"} {
				reqs = append(reqs, Req{Persona: Anon, Path: at(w.Str("payment.trade_no."+key, 0)), Label: "a " + key + " payment"})
			}
			return append(reqs,
				Req{Persona: User, Path: at(w.Str("payment.trade_no.user2", 0)), Label: "another member's payment (public)"},
				Req{Persona: Anon, Path: at(w.Str("payment.trade_no.other", 3)), Label: "a bulk payment"},
				Req{Persona: Anon, Path: at(b3MissingTrade), Label: "unknown trade number"},
				Req{Persona: Anon, Path: at("x' OR '1'='1"), Label: "a trade number that is a condition"},
			)
		}},
		{RouteID: "payment.payment.x402.check.id.get", Reads: func(w *World) []Req {
			at := func(id any) string { return fill("/api/v2/payment/x402/check/:id", id) }
			return []Req{
				{Persona: User, Path: at(w.Str("payment.trade_no.x402.pending", 0)), Label: "own pending payment by trade number"},
				{Persona: User, Path: at(w.Str("payment.trade_no.x402.paid", 0)), Label: "own confirmed payment"},
				{Persona: User, Path: at(w.Str("payment.trade_no.cancelled", 0)), Label: "own failed payment"},
				{Persona: User, Path: at(w.Str("payment.trade_no.refunded", 0)), Label: "own refunded payment"},
				{Persona: User, Path: at(w.Str("payment.trade_no.expired", 0)), Label: "own expired payment"},
				{Persona: User, Path: at(w.ID("payment.record.user", 0)), Label: "own payment by id"},
				{Persona: User, Path: at(w.ID("payment.record.user2", 0)), Label: "another member's payment by id"},
				{Persona: User2, Path: at(w.Str("payment.trade_no.x402.pending", 0)), Label: "another member's payment by trade number"},
				{Persona: User, Path: at(Missing), Label: "unknown id"},
				{Persona: User, Path: at(b3MissingTrade), Label: "unknown trade number"},
				{Persona: User, Path: at("18446744073709551617"), Label: "an id beyond 64 bits wraps"},
				{Persona: Fresh, Path: at(w.ID("payment.record.user", 0)), Label: "member without payments"},
				{Persona: Anon, Path: at(w.Str("payment.trade_no.x402.pending", 0)), Label: "anonymous"},
			}
		}},
		{RouteID: "payment.user.payment.channels.get", Reads: func(w *World) []Req {
			path := "/api/v2/user/payment/channels"
			return []Req{
				{Persona: User, Path: path, Label: "enabled gateways"},
				{Persona: Fresh, Path: path, Label: "member without data"},
				{Persona: Expired, Path: path, Query: q("type", "epay"), Label: "ignored filter"},
				{Persona: Anon, Path: path, Label: "anonymous"},
			}
		}},
		{RouteID: "payment.user.payment.records.get", Reads: func(w *World) []Req {
			path := "/api/v2/user/payment/records"
			return []Req{
				{Persona: User, Path: path, Label: "own records"},
				{Persona: User2, Path: path, Label: "own records"},
				{Persona: Fresh, Path: path, Label: "no records"},
				{Persona: User, Path: path, Query: q("page", "2", "page_size", "3"), Label: "a page"},
				{Persona: User, Path: path, Query: q("page", "0"), Label: "page zero is not clamped"},
				{Persona: User, Path: path, Query: q("page_size", "0"), Label: "page size zero is not clamped"},
				{Persona: User, Path: path, Query: q("page", "x", "page_size", "y"), Label: "pagination that does not parse"},
				{Persona: User, Path: path, Query: q("status", "1"), Label: "ignored filter"},
				{Persona: Anon, Path: path, Label: "anonymous"},
			}
		}},
		{RouteID: "payment.user.payment.status.trade_no.get", Reads: func(w *World) []Req {
			at := func(tradeNo string) string { return fill("/api/v2/user/payment/status/:trade_no", tradeNo) }
			var reqs []Req
			for _, key := range []string{"pending", "paid", "cancelled", "refunded", "expired", "x402.paid"} {
				reqs = append(reqs, Req{Persona: User, Path: at(w.Str("payment.trade_no."+key, 0)), Label: "own " + key + " payment"})
			}
			return append(reqs,
				Req{Persona: User2, Path: at(w.Str("payment.trade_no.user2", 1)), Label: "own payment"},
				Req{Persona: User, Path: at(w.Str("payment.trade_no.user2", 0)), Label: "another member's payment"},
				Req{Persona: User, Path: at(b3MissingTrade), Label: "unknown trade number"},
				Req{Persona: Anon, Path: at(w.Str("payment.trade_no.paid", 0)), Label: "anonymous"},
			)
		}},
		{RouteID: "payment.admin.payment.gateways.post", Writes: func(w *World) []Req {
			path := "/api/v2/admin/payment/gateways"
			created := []string{"data.created_at", "data.updated_at"}
			return []Req{
				{Persona: Admin, Path: path, Mask: created, Label: "create an EPay gateway (stored disabled)", Body: map[string]any{
					"name": "B3 新 EPay", "type": "epay", "icon": "https://cdn.example.com/epay.png", "fee_rate": 0.006, "fee_fixed": 0.1,
					"min_amount": 1, "max_amount": 5000, "sort": 20, "description": "created by the staging rehearsal",
					"config": map[string]any{"api_url": "https://pay.example.net", "pid": "2002", "key": "fake-new-epay-key",
						"notify_url": "https://panel.example.com/api/v2/payment/callback/epay", "return_url": "https://panel.example.com/#/order"},
				}},
				{Persona: Staff, Path: path, Mask: created, Label: "a configuration sent as a string", Body: map[string]any{
					"name": "B3 Stripe", "type": "stripe",
					"config": `{"publishable_key":"pk_test_b3","secret_key":"sk_test_b3_fake","webhook_secret":"whsec_b3_fake","currency":"USD"}`,
				}},
				{Persona: Admin, Path: path, Mask: created, Label: "a secret sent as the placeholder is stored empty", Body: map[string]any{
					"name": "B3 USDT", "type": "usdt", "config": map[string]any{"network": "TRC20", "wallet_address": "T-staging-wallet", "api_key": "********"},
				}},
				{Persona: Admin, Path: path, Mask: created, Label: "no configuration", Body: map[string]any{"name": "B3 WeChat", "type": "wechat"}},
				{Persona: Admin, Path: path, Body: map[string]any{"name": "B3 x402", "type": "x402"}, Label: "a type the route does not take"},
				{Persona: Admin, Path: path, Body: map[string]any{"type": "epay"}, Label: "no name"},
				{Persona: Admin, Path: path, Body: map[string]any{"name": "fee", "type": "epay", "fee_rate": "high"}, Label: "wrong field type"},
				{Persona: Admin, Path: path, Body: `{"name":`, Label: "malformed body"},
				{Persona: Admin, Path: path, Label: "no body"},
				{Persona: User, Path: path, Body: map[string]any{"name": "m", "type": "epay"}, Label: "member on admin route"},
				{Persona: Anon, Path: path, Body: map[string]any{"name": "a", "type": "epay"}, Label: "anonymous"},
			}
		}},
		{RouteID: "payment.admin.payment.gateways.id.toggle.post", Writes: func(w *World) []Req {
			at := func(id any) string { return fill("/api/v2/admin/payment/gateways/:id/toggle", id) }
			toggle := w.ID("gateway.toggle", 0)
			return []Req{
				{Persona: Admin, Path: at(toggle), Label: "enable a disabled EPay gateway"},
				{Persona: Admin, Path: at(toggle), Label: "toggle it back"},
				{Persona: Staff, Path: at(toggle), Body: map[string]any{"enabled": true}, Label: "enable explicitly"},
				{Persona: Admin, Path: at(toggle), Body: map[string]any{"enabled": false}, Label: "disable explicitly"},
				{Persona: Admin, Path: at(w.ID("gateway.toggle", 1)), Body: map[string]any{"enabled": true}, Label: "enable a disabled Stripe gateway"},
				{Persona: Admin, Path: at(toggle), Body: map[string]any{"enabled": "yes"}, Label: "enabled that is not boolean"},
				{Persona: Admin, Path: at(w.ID("gateway.disabled", 0)), Body: map[string]any{"enabled": true}, Label: "a type that cannot be enabled"},
				{Persona: Admin, Path: at(w.ID("gateway.disabled", 1)), Label: "toggle a type that cannot be enabled"},
				{Persona: Admin, Path: at(Missing), Body: map[string]any{"enabled": false}, Label: "disable an unknown gateway"},
				{Persona: Admin, Path: at(Missing), Label: "toggle an unknown gateway"},
				{Persona: Admin, Path: "/api/v2/admin/payment/gateways/x/toggle", Label: "invalid id"},
				{Persona: Admin, Path: at(toggle), Body: `{"enabled":`, Label: "malformed body"},
				{Persona: User, Path: at(toggle), Label: "member on admin route"},
			}
		}},
		{RouteID: "payment.admin.payment.gateways.id.put", Writes: func(w *World) []Req {
			at := func(id any) string { return fill("/api/v2/admin/payment/gateways/:id", id) }
			mask := []string{"data.created_at", "data.updated_at"}
			target := w.ID("gateway.update", 0)
			return []Req{
				{Persona: Admin, Path: at(target), Mask: mask, Label: "rename, fees and a configuration whose secret is the placeholder", Body: map[string]any{
					"name": "B3 EPay 改名", "fee_rate": 0.01, "fee_fixed": 0.2, "min_amount": 2, "max_amount": 8000, "sort": 7,
					"description": "updated by the staging rehearsal", "icon": "https://cdn.example.com/epay2.png",
					"config": map[string]any{"api_url": "https://pay2.example.net", "pid": "3003", "key": "********"},
				}},
				{Persona: Staff, Path: at(target), Mask: mask, Body: map[string]any{"type": "wechat"}, Label: "another type while disabled"},
				{Persona: Admin, Path: at(w.ID("gateway.update", 1)), Mask: mask, Body: map[string]any{}, Label: "an empty update saves as it is"},
				{Persona: Admin, Path: at(w.ID("gateway.update", 1)), Mask: mask, Body: map[string]any{"config": `{"key":"fake-replaced-key"}`, "sort": 0}, Label: "a configuration string and sort zero"},
				{Persona: Admin, Path: at(w.ID("gateway.epay", 0)), Body: map[string]any{"type": "alipay"}, Label: "an enabled gateway to a type that cannot be enabled"},
				{Persona: Admin, Path: at(target), Body: map[string]any{"type": "bogus"}, Label: "an unknown type"},
				{Persona: Admin, Path: at(target), Body: map[string]any{"fee_rate": "x"}, Label: "wrong field type"},
				{Persona: Admin, Path: at(Missing), Body: map[string]any{"name": "x"}, Label: "unknown gateway"},
				{Persona: Admin, Path: "/api/v2/admin/payment/gateways/x", Body: map[string]any{"name": "x"}, Label: "invalid id"},
				{Persona: Admin, Path: at(target), Label: "no body"},
				{Persona: User, Path: at(target), Body: map[string]any{"name": "x"}, Label: "member on admin route"},
			}
		}},
		{RouteID: "payment.admin.payment.gateways.id.delete", Writes: func(w *World) []Req {
			at := func(id any) string { return fill("/api/v2/admin/payment/gateways/:id", id) }
			return []Req{
				{Persona: Admin, Path: at(w.ID("gateway.delete", 0)), Label: "delete"},
				{Persona: Admin, Path: at(w.ID("gateway.delete", 0)), Label: "delete again succeeds"},
				{Persona: Staff, Path: at(w.ID("gateway.delete", 1)), Label: "staff delete"},
				{Persona: Admin, Path: at(Missing), Label: "unknown gateway succeeds"},
				{Persona: Admin, Path: "/api/v2/admin/payment/gateways/x", Label: "invalid id"},
				{Persona: User, Path: at(w.ID("gateway.update", 1)), Label: "member on admin route"},
			}
		}},
		{RouteID: "payment.callback", Writes: func(w *World) []Req {
			path := "/api/v2/payment/callback/epay"
			paid := func(tradeNo, money string) map[string]string {
				return map[string]string{"pid": "1001", "type": "alipay", "out_trade_no": tradeNo, "trade_no": "EP" + tradeNo,
					"name": "staging order", "money": money, "trade_status": "TRADE_SUCCESS"}
			}
			cb1, cb2 := w.Str("payment.trade_no.cb.epay", 0), w.Str("payment.trade_no.cb.epay", 1)
			total := fmt.Sprintf("%.2f", b3Yuan(b3RenewalTotal))
			forged := paid(cb2, total)
			forged["sign"], forged["sign_type"] = "deadbeefdeadbeefdeadbeefdeadbeef", "MD5"
			forgedQuery := url.Values{}
			for k, v := range forged {
				forgedQuery.Set(k, v)
			}
			return []Req{
				{Persona: Anon, Path: path, Query: b3EPayQuery(paid(cb1, total), b3EPayKey), Label: "a signed payment renews the buyer's plan"},
				{Persona: Anon, Path: path, Query: b3EPayQuery(paid(cb1, total), b3EPayKey), Label: "a repeat answers fail and grants once"},
				{Persona: Anon, Path: path, Query: b3EPayQuery(paid(cb2, "0.01"), b3EPayKey), Label: "a signed amount that is not the payment's"},
				{Persona: Anon, Path: path, Query: forgedQuery, Label: "a forged signature"},
				{Persona: Anon, Path: path, Query: b3EPayQuery(paid(cb2, total), "guessed-key"), Label: "signed with another key"},
				{Persona: Anon, Path: path, Query: b3EPayQuery(map[string]string{"pid": "1001", "out_trade_no": cb2, "trade_no": "EPW", "trade_status": "WAIT_BUYER_PAY"}, b3EPayKey),
					Label: "a pending notification changes nothing"},
				{Persona: Anon, Path: path, Query: b3EPayQuery(paid(b3MissingTrade, total), b3EPayKey), Label: "an unknown trade number"},
				{Persona: Anon, Path: path, Query: b3EPayQuery(paid(w.Str("payment.trade_no.paid", 0), "10.00"), b3EPayKey), Label: "a payment already paid"},
				{Persona: Anon, Path: path, Query: b3EPayQuery(map[string]string{"pid": "1001", "out_trade_no": cb2, "trade_status": "TRADE_SUCCESS", "money": total}, b3EPayKey),
					Label: "a signed payment without the gateway's trade number"},
				{Persona: Anon, Path: path, Query: b3EPayQuery(paid(cb2, "abc"), b3EPayKey), Label: "a signed payment with money that is not a number"},
				{Persona: Anon, Path: "/api/v2/payment/callback/alipay", Query: b3EPayQuery(paid(cb2, total), b3EPayKey), Label: "a gateway type without a callback"},
				{Persona: Anon, Path: "/api/v2/payment/callback/stripe", Body: map[string]any{"type": "checkout.session.completed"}, Label: "stripe on the generic callback"},
				{Persona: Anon, Path: path, Label: "no parameters"},
				{Persona: User, Path: path, Query: b3EPayQuery(paid(cb2, "0.02"), b3EPayKey), Label: "a member token changes nothing"},
			}
		}},
		{RouteID: "payment.payment.x402.callback.post", Writes: func(w *World) []Req {
			path := "/api/v2/payment/x402/callback"
			x1, x2, x3 := w.Str("payment.trade_no.cb.x402", 0), w.Str("payment.trade_no.cb.x402", 1), w.Str("payment.trade_no.cb.x402", 2)
			amount := fmt.Sprintf("%.8f", float64(b3RenewalTotal)/100_000_000)
			confirmed := func(tradeNo, amount, token string) b3X402Callback {
				return b3X402Callback{TradeNo: tradeNo, TxHash: fmt.Sprintf("0x%064x", len(tradeNo)*7919), BlockNumber: 6100000, Confirmations: 6,
					Status: "confirmed", Amount: amount, Token: token}
			}
			failed := confirmed(x2, amount, "ETH")
			failed.Status = "failed"
			unconfirmed := confirmed(x3, amount, "ETH")
			unconfirmed.Confirmations = 0
			confirming := confirmed(x3, amount, "ETH")
			confirming.Status = "confirming"
			success := confirmed(x3, "0.5", "eth")
			success.Status = "success"
			return []Req{
				{Persona: Anon, Path: path, Body: confirmed(x1, amount, "eth").signed(b3X402Secret), Label: "the payment's token and amount"},
				{Persona: Anon, Path: path, Body: confirmed(x1, amount, "ETH").signed(b3X402Secret), Label: "a repeat is already processed"},
				{Persona: Anon, Path: path, Body: confirmed(x3, amount, "USDC").signed(b3X402Secret), Label: "another token"},
				{Persona: Anon, Path: path, Body: confirmed(x3, amount, "USDT").signed(b3X402Secret), Label: "a token the gateway does not accept"},
				{Persona: Anon, Path: path, Body: confirmed(x3, "0.00001989", "ETH").signed(b3X402Secret), Label: "less than asked"},
				{Persona: Anon, Path: path, Body: confirmed(x3, "2e-5", "ETH").signed(b3X402Secret), Label: "an amount that is not a decimal"},
				{Persona: Anon, Path: path, Body: unconfirmed.signed(b3X402Secret), Label: "not yet confirmed"},
				{Persona: Anon, Path: path, Body: confirming.signed(b3X402Secret), Label: "a transfer still confirming"},
				{Persona: Anon, Path: path, Body: failed.signed(b3X402Secret), Label: "a failed transfer cancels the payment"},
				{Persona: Anon, Path: path, Body: confirmed(x3, amount, "ETH").withSignature("00ff"), Label: "a forged signature"},
				{Persona: Anon, Path: path, Body: confirmed(x3, amount, "ETH").signed("guessed-secret"), Label: "signed with another secret"},
				{Persona: Anon, Path: path, Body: confirmed(x3, amount, "ETH").withSignature(""), Label: "no signature"},
				{Persona: Anon, Path: path, Body: confirmed(b3MissingTrade, amount, "ETH").signed(b3X402Secret), Label: "an unknown trade number"},
				{Persona: Anon, Path: path, Body: success.signed(b3X402Secret), Label: "more than asked, status success"},
				{Persona: Anon, Path: path, Body: `{"trade_no":`, Label: "a body that is not JSON"},
				{Persona: Anon, Path: path, Body: map[string]any{"trade_no": x3, "block_number": "x"}, Label: "a field of the wrong type"},
				{Persona: Anon, Path: path, Label: "no body"},
			}
		}},
		{RouteID: "payment.payment.stripe.webhook.post", Writes: func(w *World) []Req {
			path := "/api/v2/payment/stripe/webhook"
			event := func(kind, tradeNo string) map[string]any {
				return map[string]any{"id": "evt_staging_" + kind, "type": kind, "data": map[string]any{"object": map[string]any{
					"id": "cs_test_staging", "status": "complete", "amount_total": b3RenewalTotal, "currency": "usd",
					"client_reference_id": tradeNo, "metadata": map[string]any{"trade_no": tradeNo, "order_id": "1"},
				}}}
			}
			fiat := w.Str("payment.trade_no.fiat", 0)
			// Stripe-Signature cannot be sent (Req has no headers): every
			// delivery is unsigned and refused before it is read.
			return []Req{
				{Persona: Anon, Path: path, Body: event("checkout.session.completed", fiat), Label: "an unsigned completed checkout"},
				{Persona: Anon, Path: path, Body: event("checkout.session.expired", fiat), Label: "an unsigned expired checkout"},
				{Persona: Anon, Path: path, Body: event("checkout.session.async_payment_failed", fiat), Label: "an unsigned failed checkout"},
				{Persona: Anon, Path: path, Body: event("customer.created", ""), Label: "an unsigned event of another type"},
				{Persona: Anon, Path: path, Body: `{"type":`, Label: "a body that is not JSON"},
				{Persona: Anon, Path: path, Label: "no body"},
				{Persona: User, Path: path, Body: event("checkout.session.completed", fiat), Label: "a member token does not sign"},
			}
		}},
		{RouteID: "payment.payment.paypal.webhook.post", Writes: func(w *World) []Req {
			path := "/api/v2/payment/paypal/webhook"
			event := func(kind, tradeNo string) map[string]any {
				return map[string]any{"id": "WH-STAGING-" + kind, "event_type": kind, "resource": map[string]any{
					"id": "CAPTURE-STAGING", "status": "COMPLETED", "custom_id": tradeNo,
					"amount": map[string]any{"value": "19.90", "currency_code": "USD"},
				}}
			}
			fiat := w.Str("payment.trade_no.fiat", 0)
			// Without PayPal-Transmission-* headers the delivery is refused
			// before PayPal's API would be asked, so nothing leaves the network.
			return []Req{
				{Persona: Anon, Path: path, Body: event("PAYMENT.CAPTURE.COMPLETED", fiat), Label: "an unverified capture"},
				{Persona: Anon, Path: path, Body: event("PAYMENT.CAPTURE.DENIED", fiat), Label: "an unverified denial"},
				{Persona: Anon, Path: path, Body: event("CHECKOUT.ORDER.COMPLETED", fiat), Label: "an unverified order completion"},
				{Persona: Anon, Path: path, Body: `{"event_type":`, Label: "a body that is not JSON"},
				{Persona: Anon, Path: path, Label: "no body"},
			}
		}},
		{RouteID: "payment.payment.x402.create.post", Writes: func(w *World) []Req {
			path := "/api/v2/payment/x402/create"
			mask := []string{"data.trade_no", "data.wallet_address", "data.expires_at", "data.qr_code"}
			payable := w.ID("order.user.payable", 1)
			return []Req{
				{Persona: User, Path: path, Mask: mask, Body: map[string]any{"order_id": payable, "token": "ETH", "network": "sepolia"}, Label: "a crypto payment of a pending order"},
				{Persona: User, Path: path, Mask: mask, Body: map[string]any{"order_id": payable, "token": "USDC", "network": "base-sepolia"}, Label: "a second payment of the same order"},
				{Persona: User, Path: path, Mask: mask, Body: map[string]any{"order_id": payable}, Label: "no token or network"},
				{Persona: User, Path: path, Body: map[string]any{"order_id": w.ID("order.user2.status.0", 0), "token": "ETH"}, Label: "another member's order"},
				{Persona: User, Path: path, Body: map[string]any{"order_id": w.ID("order.user.status.3", 0), "token": "ETH"}, Label: "a completed order"},
				{Persona: User, Path: path, Body: map[string]any{"order_id": w.ID("order.user.status.2", 0), "token": "ETH"}, Label: "a cancelled order"},
				{Persona: User, Path: path, Body: map[string]any{"order_id": Missing, "token": "ETH"}, Label: "unknown order"},
				{Persona: User, Path: path, Body: map[string]any{"token": "ETH"}, Label: "no order"},
				{Persona: User, Path: path, Body: `{"order_id":`, Label: "invalid JSON"},
				{Persona: Fresh, Path: path, Body: map[string]any{"order_id": payable, "token": "ETH"}, Label: "member without orders"},
				{Persona: Anon, Path: path, Body: map[string]any{"order_id": payable, "token": "ETH"}, Label: "anonymous"},
			}
		}},
		{RouteID: "payment.payment.fiat.create.post", Writes: func(w *World) []Req {
			path := "/api/v2/payment/fiat/create"
			payable := w.ID("order.user.payable", 3)
			return []Req{
				{Persona: User, Path: path, Body: map[string]any{"order_id": payable, "provider": "stripe"}, Label: "Stripe checkout (not implemented: refused, no payment record)"},
				{Persona: User, Path: path, Body: map[string]any{"order_id": payable, "provider": "paypal"}, Label: "PayPal order (not implemented: refused, no payment record)"},
				{Persona: User, Path: path, Body: map[string]any{"order_id": payable, "provider": "alipay"}, Label: "an unsupported provider"},
				{Persona: User, Path: path, Body: map[string]any{"order_id": w.ID("order.user2.status.0", 0), "provider": "stripe"}, Label: "another member's order"},
				{Persona: User, Path: path, Body: map[string]any{"order_id": w.ID("order.user.status.1", 0), "provider": "stripe"}, Label: "a paid order"},
				{Persona: User, Path: path, Body: map[string]any{"order_id": Missing, "provider": "stripe"}, Label: "unknown order"},
				{Persona: User, Path: path, Body: map[string]any{"order_id": payable}, Label: "no provider"},
				{Persona: User, Path: path, Label: "no body"},
				{Persona: Anon, Path: path, Body: map[string]any{"order_id": payable, "provider": "stripe"}, Label: "anonymous"},
			}
		}},
		{RouteID: "payment.user.payment.create.post", Writes: func(w *World) []Req {
			path := "/api/v2/user/payment/create"
			mask := []string{"data.trade_no"}
			epay, stripe := w.ID("gateway.epay", 0), w.ID("gateway.stripe", 0)
			payable := func(i int) uint { return w.ID("order.user.payable", i) }
			return []Req{
				{Persona: User, Path: path, Mask: mask, Body: map[string]any{"gateway_id": epay, "amount": b3Yuan(b3PayableTotals[0]), "order_id": payable(0)}, Label: "pay a pending order in full"},
				{Persona: User, Path: path, Mask: mask, Body: map[string]any{"gateway_id": epay, "amount": 50}, Label: "a top-up"},
				{Persona: User, Path: path, Mask: mask, Body: map[string]any{"gateway_id": stripe, "amount": b3Yuan(b3PayableTotals[2]), "order_id": payable(2)}, Label: "a gateway with fees"},
				{Persona: Fresh, Path: path, Mask: mask, Body: map[string]any{"gateway_id": epay, "amount": 20}, Label: "a member's first top-up"},
				{Persona: User, Path: path, Body: map[string]any{"gateway_id": stripe, "amount": 2}, Label: "below the gateway's minimum"},
				{Persona: User, Path: path, Body: map[string]any{"gateway_id": stripe, "amount": 3000}, Label: "above the gateway's maximum"},
				{Persona: User, Path: path, Body: map[string]any{"gateway_id": epay, "amount": 10, "order_id": payable(1)}, Label: "an amount that is not the order's"},
				{Persona: User, Path: path, Body: map[string]any{"gateway_id": epay, "amount": 9.9, "order_id": w.ID("order.user2.status.0", 0)}, Label: "another member's order"},
				{Persona: User, Path: path, Body: map[string]any{"gateway_id": epay, "amount": 9.9, "order_id": w.ID("order.user.status.3", 2)}, Label: "a completed order"},
				{Persona: User, Path: path, Body: map[string]any{"gateway_id": epay, "amount": 9.9, "order_id": Missing}, Label: "unknown order"},
				{Persona: User, Path: path, Body: map[string]any{"gateway_id": w.ID("gateway.disabled", 0), "amount": 10}, Label: "a disabled gateway"},
				{Persona: User, Path: path, Body: map[string]any{"gateway_id": Missing, "amount": 10}, Label: "unknown gateway"},
				{Persona: User, Path: path, Body: map[string]any{"amount": 10}, Label: "no gateway"},
				{Persona: User, Path: path, Body: map[string]any{"gateway_id": epay, "amount": 0.5}, Label: "an amount below one"},
				{Persona: User, Path: path, Body: `{"gateway_id":`, Label: "invalid JSON"},
				{Persona: Anon, Path: path, Body: map[string]any{"gateway_id": epay, "amount": 10}, Label: "anonymous"},
			}
		}},
	}
}

// b3EPayQuery is an EPay notification signed as EPay signs it: the
// non-empty parameters but sign and sign_type in key order, joined, the
// merchant key appended, MD5.
func b3EPayQuery(params map[string]string, key string) url.Values {
	keys := make([]string, 0, len(params))
	values := url.Values{}
	for k, v := range params {
		values.Set(k, v)
		if v != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString(k + "=" + params[k])
	}
	b.WriteString(key)
	sum := md5.Sum([]byte(b.String())) // #nosec G401 -- EPay's protocol.
	values.Set("sign", hex.EncodeToString(sum[:]))
	values.Set("sign_type", "MD5")
	return values
}

// b3X402Callback is an x402 confirmation service report.
type b3X402Callback struct {
	TradeNo       string `json:"trade_no"`
	TxHash        string `json:"tx_hash"`
	BlockNumber   int64  `json:"block_number"`
	Confirmations int    `json:"confirmations"`
	Status        string `json:"status"`
	Amount        string `json:"amount"`
	Token         string `json:"token"`
	Signature     string `json:"signature"`
}

// signed signs the report as the x402 service does: HMAC-SHA256 of the
// fields in key order, "k=v" joined with "&".
func (c b3X402Callback) signed(secret string) b3X402Callback {
	canonical := fmt.Sprintf("amount=%s&block_number=%d&confirmations=%d&status=%s&token=%s&trade_no=%s&tx_hash=%s",
		c.Amount, c.BlockNumber, c.Confirmations, c.Status, c.Token, c.TradeNo, c.TxHash)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(canonical))
	c.Signature = hex.EncodeToString(mac.Sum(nil))
	return c
}

func (c b3X402Callback) withSignature(signature string) b3X402Callback {
	c.Signature = signature
	return c
}

// ---------------------------------------------------------------- affiliate

func b3AffiliateSpecs() []RouteSpec {
	return []RouteSpec{
		{RouteID: "affiliate.admin.invite.config.get", Reads: func(w *World) []Req {
			path := "/api/v2/admin/invite/config"
			return append([]Req{
				{Persona: Admin, Path: path, Label: "stored configuration and settings"},
				{Persona: Staff, Path: path, Label: "staff"},
			}, forbidden(path)...)
		}},
		{RouteID: "affiliate.admin.invite.stats.get", Reads: func(w *World) []Req {
			path := "/api/v2/admin/invite/stats"
			return append([]Req{
				{Persona: Admin, Path: path, Label: "inviters, commissions and withdrawals"},
				{Persona: Admin2, Path: path, Query: q("days", "7"), Label: "ignored filter"},
			}, forbidden(path)...)
		}},
		{RouteID: "affiliate.admin.invite.withdrawals.get", Reads: func(w *World) []Req {
			path := "/api/v2/admin/invite/withdrawals"
			reqs := []Req{
				{Persona: Admin, Path: path, Label: "every withdrawal"},
				{Persona: Staff, Path: path, Label: "staff"},
				{Persona: Admin, Path: path, Query: q("status", "bogus"), Label: "an unknown status filters nothing"},
				{Persona: Admin, Path: path, Query: q("page", "0", "page_size", "500"), Label: "page bounds"},
				{Persona: Admin, Path: path, Query: q("page", "2", "page_size", "3"), Label: "a second page"},
				{Persona: Admin, Path: path, Query: q("page", "x", "page_size", "-4"), Label: "pagination that does not parse"},
				{Persona: Admin, Path: path, Query: q("page", "99"), Label: "a page beyond the last"},
			}
			for _, status := range []string{"pending", "Approved", "rejected", "0", "1", "2", "-1"} {
				reqs = append(reqs, Req{Persona: Admin, Path: path, Query: q("status", status), Label: "status " + status})
			}
			return append(reqs, forbidden(path)...)
		}},
		{RouteID: "affiliate.user.invite.get", Reads: func(w *World) []Req {
			path := "/api/v2/user/invite"
			return []Req{
				{Persona: User, Path: path, Label: "codes, balance and statistics"},
				{Persona: User2, Path: path, Label: "another inviter"},
				{Persona: Fresh, Path: path, Label: "no codes and no invitees"},
				{Persona: Expired, Path: path, Label: "expired member"},
				{Persona: Admin, Path: path, Label: "the administrator"},
				{Persona: Anon, Path: path, Label: "anonymous"},
			}
		}},
		{RouteID: "affiliate.user.invite.commissions.get", Reads: func(w *World) []Req {
			path := "/api/v2/user/invite/commissions"
			return []Req{
				{Persona: User, Path: path, Label: "own commissions newest first"},
				{Persona: User2, Path: path, Label: "another inviter's"},
				{Persona: Fresh, Path: path, Label: "none"},
				{Persona: User, Path: path, Query: q("page", "2", "page_size", "4"), Label: "a second page"},
				{Persona: User, Path: path, Query: q("page", "0"), Label: "page zero"},
				{Persona: User, Path: path, Query: q("page_size", "0"), Label: "page size zero"},
				{Persona: User, Path: path, Query: q("page_size", "-1"), Label: "a negative page size"},
				{Persona: User, Path: path, Query: q("page", "x", "page_size", ""), Label: "a page that is not a number"},
				{Persona: Anon, Path: path, Label: "anonymous"},
			}
		}},
		{RouteID: "affiliate.user.invite.withdrawals.get", Reads: func(w *World) []Req {
			path := "/api/v2/user/invite/withdrawals"
			return []Req{
				{Persona: User, Path: path, Label: "own withdrawals with a reservation"},
				{Persona: User2, Path: path, Label: "another member's"},
				{Persona: Fresh, Path: path, Label: "none"},
				{Persona: User, Path: path, Query: q("page", "2", "page_size", "3"), Label: "a second page"},
				{Persona: User, Path: path, Query: q("page", "-1"), Label: "a negative page"},
				{Persona: User, Path: path, Query: q("status", "1"), Label: "ignored filter"},
				{Persona: Anon, Path: path, Label: "anonymous"},
			}
		}},
		{RouteID: "affiliate.user.invite.generate.post", Writes: func(w *World) []Req {
			path := "/api/v2/user/invite/generate"
			generated := []string{"data.code", "data.created_at", "data.updated_at", "data.expired_at"}
			return []Req{
				{Persona: Fresh, Path: path, Mask: generated, Label: "a member without codes"},
				{Persona: User, Path: path, Mask: generated, Label: "a code under the limit (expired and used codes do not count)"},
				{Persona: User, Path: path, Mask: generated, Label: "the last code under the limit"},
				{Persona: User, Path: path, Mask: generated, Label: "at the limit"},
				{Persona: User2, Path: path, Mask: generated, Label: "a member at the limit"},
				{Persona: Expired, Path: path, Mask: generated, Body: map[string]any{"count": 9}, Label: "a body is ignored"},
				{Persona: Anon, Path: path, Label: "anonymous"},
			}
		}},
		{RouteID: "affiliate.user.invite.withdraw.post", Writes: func(w *World) []Req {
			path := "/api/v2/user/invite/withdraw"
			created := []string{"data.created_at", "data.updated_at"}
			body := func(amount any) map[string]any {
				return map[string]any{"amount": amount, "method": "bank", "account": "6222 0000 0000 0000", "name": "Staging 测试"}
			}
			return []Req{
				{Persona: User, Path: path, Mask: created, Body: body(100), Label: "a withdrawal"},
				{Persona: User, Path: path, Mask: created, Body: map[string]any{"amount": 250, "method": "alipay", "account": "alipay@example.com", "name": "Alice"}, Label: "an alipay withdrawal"},
				{Persona: User, Path: path, Mask: created, Body: `{"amount":50.0,"method":"wechat","account":"wx-staging","name":"Alice"}`, Label: "a whole amount written with a fraction"},
				// user2 holds 1500 plus the 200 refunded when the process
				// route (replayed first) rejects its pending withdrawal.
				{Persona: User2, Path: path, Mask: created, Body: body(1700), Label: "the whole balance"},
				{Persona: User2, Path: path, Body: body(30), Label: "overdraws after the whole balance"},
				{Persona: User, Path: path, Body: body(999999), Label: "more than the balance"},
				{Persona: User, Path: path, Body: `{"amount":1e300,"method":"bank","account":"a","name":"n"}`, Label: "a huge amount"},
				{Persona: User, Path: path, Body: body(9), Label: "below the minimum"},
				{Persona: User, Path: path, Body: body(10.5), Label: "a fractional amount"},
				{Persona: User, Path: path, Body: body(-5), Label: "a negative amount"},
				{Persona: User, Path: path, Body: body(0), Label: "a zero amount"},
				{Persona: User, Path: path, Body: body("100"), Label: "an amount that is a string"},
				{Persona: User, Path: path, Body: map[string]any{"amount": 100, "method": "paypal", "account": "a", "name": "n"}, Label: "an unknown method"},
				{Persona: User, Path: path, Body: map[string]any{"amount": 100, "method": "bank", "name": "n"}, Label: "no account"},
				{Persona: User, Path: path, Label: "no body"},
				{Persona: User, Path: path, Body: `{"amount":`, Label: "a body that does not parse"},
				{Persona: Fresh, Path: path, Body: body(10), Label: "no commission"},
				{Persona: Anon, Path: path, Body: body(10), Label: "anonymous"},
			}
		}},
		{RouteID: "affiliate.admin.invite.withdrawals.id.process.post", Writes: func(w *World) []Req {
			at := func(id any) string { return fill("/api/v2/admin/invite/withdrawals/:id/process", id) }
			decided := []string{"data.processed_at", "data.updated_at"}
			pending := func(i int) string { return at(w.ID("withdraw.pending", i)) }
			return []Req{
				{Persona: Admin, Path: pending(0), Mask: decided, Body: map[string]any{"approve": true, "remark": "paid out 已打款"}, Label: "approve"},
				{Persona: Admin, Path: pending(1), Mask: decided, Body: map[string]any{"approve": false, "remark": "wrong account"}, Label: "reject refunds"},
				{Persona: Staff, Path: pending(2), Mask: decided, Body: map[string]any{"approved": "yes"}, Label: "approved alias"},
				{Persona: Admin, Path: pending(3), Mask: decided, Body: map[string]any{"status": "rejected"}, Label: "status text"},
				{Persona: Admin, Path: pending(4), Mask: decided, Body: map[string]any{"status": 1}, Label: "status number"},
				{Persona: Admin, Path: pending(5), Mask: decided, Body: map[string]any{"approve": 0, "status": 1}, Label: "approve wins over status"},
				{Persona: Admin, Path: pending(6), Body: map[string]any{"approve": "maybe"}, Label: "approve that is not boolean"},
				{Persona: Admin, Path: pending(6), Body: map[string]any{"approved": 2}, Label: "approved that is not boolean"},
				{Persona: Admin, Path: pending(6), Body: map[string]any{"status": 3}, Label: "an unknown status"},
				{Persona: Admin, Path: pending(6), Body: map[string]any{"remark": "x"}, Label: "no decision"},
				{Persona: Admin, Path: pending(6), Mask: decided, Body: map[string]any{"status": "2"}, Label: "status digit text"},
				{Persona: Admin, Path: pending(6), Body: map[string]any{"approve": false}, Label: "rejecting twice refunds once"},
				{Persona: Admin, Path: at(w.ID("withdraw.fractional", 0)), Mask: decided, Body: map[string]any{"approve": false}, Label: "a fractional withdrawal is refunded its whole part"},
				{Persona: Admin, Path: at(w.ID("withdraw.orphan", 0)), Mask: decided, Body: map[string]any{"approve": false}, Label: "a rejection without its user"},
				{Persona: Admin, Path: at(w.ID("withdraw.user2.pending", 0)), Mask: decided, Body: map[string]any{"approve": false}, Label: "reject another member's"},
				{Persona: Admin, Path: at(w.ID("withdraw.approved", 0)), Body: map[string]any{"approve": false}, Label: "already approved"},
				{Persona: Admin, Path: at(w.ID("withdraw.rejected", 0)), Body: map[string]any{"approve": true}, Label: "already rejected"},
				{Persona: Admin, Path: at(w.ID("withdraw.reserving", 0)), Body: map[string]any{"approve": true}, Label: "a reservation"},
				{Persona: Admin, Path: at(Missing), Body: map[string]any{"approve": true}, Label: "unknown withdrawal"},
				{Persona: Admin, Path: at(0), Body: map[string]any{"approve": true}, Label: "withdrawal zero"},
				{Persona: Admin, Path: "/api/v2/admin/invite/withdrawals/x/process", Body: map[string]any{"approve": true}, Label: "invalid id"},
				{Persona: Admin, Path: "/api/v2/admin/invite/withdrawals/0%20OR%201=1/process", Body: map[string]any{"approve": false}, Label: "id that is a condition"},
				{Persona: Admin, Path: "/api/v2/admin/invite/withdrawals/4294967297/process", Body: map[string]any{"approve": true}, Label: "id beyond 32 bits"},
				{Persona: Admin, Path: "/api/v2/admin/invite/withdrawals/x/process", Body: `{"approve":`, Label: "an invalid body before the id"},
				{Persona: Admin, Path: pending(7), Label: "no body"},
				{Persona: Admin, Path: pending(7), Body: `[1]`, Label: "a body that is not an object"},
				{Persona: User, Path: pending(7), Body: map[string]any{"approve": true}, Label: "member on admin route"},
			}
		}},
		{RouteID: "affiliate.admin.invite.config.put", Writes: func(w *World) []Req {
			path := "/api/v2/admin/invite/config"
			clock := []string{"data.updated_at"}
			return []Req{
				{Persona: Admin, Path: path, Mask: clock, Label: "every field, loosely typed", Body: `{"enabled":"no","commission_enabled":0,"auto_generate":"on",` +
					`"code_count":"7","code_expire_days":14.9,"commission_type":"percentage","commission_rate":25,"commission_fixed":"1.5",` +
					`"commission_min":3,"first_order_bonus":0.5,"first_traffic_bonus":"1024","code_prefix":"  VIP ","code_length":"10",` +
					`"withdraw_fee":"0.75","withdraw_methods":"bank, alipay ,"}`},
				{Persona: Admin, Path: path, Mask: clock, Body: map[string]any{"commission_rate_ratio": 0.35, "commission_rate": 80}, Label: "commission_rate_ratio wins over commission_rate"},
				{Persona: Staff, Path: path, Mask: clock, Body: map[string]any{"commission_min": 5, "min_withdraw": 8}, Label: "min_withdraw wins over commission_min"},
				{Persona: Admin, Path: path, Mask: clock, Body: map[string]any{"commission_type": " ", "commission_type_code": 0}, Label: "zero and blank commission types are unspecified"},
				{Persona: Admin, Path: path, Mask: clock, Body: map[string]any{"commission_type": "fixed", "commission_type_code": 1}, Label: "commission_type_code wins"},
				{Persona: Admin, Path: path, Mask: clock, Body: map[string]any{"code_prefix": "", "withdraw_methods": []string{" bank ", ""}}, Label: "an empty prefix answers empty and stores the default"},
				{Persona: Admin, Path: path, Mask: clock, Body: map[string]any{"withdraw_methods": `["usdt","bank"]`}, Label: "withdraw methods as a JSON array in a string"},
				{Persona: Admin, Path: path, Mask: clock, Body: map[string]any{}, Label: "an empty update saves as it is"},
				{Persona: Admin, Path: path, Mask: clock, Body: `null`, Label: "a null body"},
				{Persona: Admin, Path: path, Body: map[string]any{"enabled": "maybe"}, Label: "not a boolean"},
				{Persona: Admin, Path: path, Body: map[string]any{"code_count": -1}, Label: "a negative code count"},
				{Persona: Admin, Path: path, Body: map[string]any{"code_expire_days": "soon"}, Label: "an expiry that is not a number"},
				{Persona: Admin, Path: path, Body: map[string]any{"commission_type": "weekly"}, Label: "an unknown commission type"},
				{Persona: Admin, Path: path, Body: map[string]any{"commission_type": 3}, Label: "a commission type out of range"},
				{Persona: Admin, Path: path, Body: map[string]any{"commission_type": true}, Label: "a commission type of another type"},
				{Persona: Admin, Path: path, Body: map[string]any{"commission_type_code": "9"}, Label: "a commission type code out of range"},
				{Persona: Admin, Path: path, Body: map[string]any{"commission_rate_ratio": -0.1}, Label: "a negative ratio"},
				{Persona: Admin, Path: path, Body: map[string]any{"commission_rate": "-1"}, Label: "a negative rate"},
				{Persona: Admin, Path: path, Body: map[string]any{"min_withdraw": "lots"}, Label: "a minimum that is not a number"},
				{Persona: Admin, Path: path, Body: map[string]any{"first_traffic_bonus": -2}, Label: "a negative traffic bonus"},
				{Persona: Admin, Path: path, Body: map[string]any{"code_prefix": 5}, Label: "a prefix that is not a string"},
				{Persona: Admin, Path: path, Body: map[string]any{"code_length": 0}, Label: "a code length of zero"},
				{Persona: Admin, Path: path, Body: map[string]any{"withdraw_fee": -0.5}, Label: "a negative fee"},
				{Persona: Admin, Path: path, Body: map[string]any{"withdraw_methods": []string{}}, Label: "no withdraw methods"},
				{Persona: Admin, Path: path, Body: map[string]any{"withdraw_methods": []int{1}}, Label: "withdraw methods that are not strings"},
				{Persona: Admin, Path: path, Body: map[string]any{"withdraw_fee": -1, "code_count": -1}, Label: "the first refused field answers"},
				{Persona: Admin, Path: path, Body: `[1]`, Label: "a body that is not an object"},
				{Persona: Admin, Path: path, Body: `{"enabled":`, Label: "a body that does not parse"},
				{Persona: Admin, Path: path, Mask: clock, Body: map[string]any{"code_count": 5, "code_expire_days": 30, "commission_min": 10, "code_prefix": "STG", "code_length": 8,
					"withdraw_fee": 1.5, "withdraw_methods": []string{"alipay", "bank"}, "enabled": true, "commission_enabled": true}, Label: "restore"},
				{Persona: User, Path: path, Body: map[string]any{"code_count": 1}, Label: "member on admin route"},
				{Persona: Anon, Path: path, Body: map[string]any{"code_count": 1}, Label: "anonymous"},
			}
		}},
	}
}

// ---------------------------------------------------------------- seed

// b3User reads the plan and expiry of a seeded user.
func b3User(s *Seeder, id uint) model.User {
	var user model.User
	if err := s.DB.Select("id", "plan_id", "group_id", "expired_at", "invite_user_id").Where("id = ?", id).Take(&user).Error; err != nil {
		panic(fmt.Errorf("seed: user %d: %w", id, err))
	}
	return user
}

// b3SoldPeriod is a period a plan sells.
func b3SoldPeriod(s *Seeder, planID uint) string {
	var plan model.Plan
	if err := s.DB.Where("id = ?", planID).Take(&plan).Error; err != nil {
		panic(fmt.Errorf("seed: plan %d: %w", planID, err))
	}
	switch {
	case plan.MonthPrice != nil:
		return "month"
	case plan.QuarterPrice != nil:
		return "quarter"
	case plan.YearPrice != nil:
		return "year"
	default:
		return "onetime"
	}
}

// seedB3Catalog writes the plans the plan write routes change or delete
// (so that the shared plans of the other batches stay as they are) and
// coupons of every kind: percentage and fixed, valid, expired, not started,
// exhausted, with uses left, limited to a plan and a period, zero and
// negative limits, and coupons to delete.
func seedB3Catalog(s *Seeder) {
	group := s.World.ID("node_group", 0)
	type planShape struct {
		key   string
		name  string
		show  int
		month *int64
		year  *int64
	}
	shapes := []planShape{
		{"plan.b3.update", "B3 可编辑 editable A", 1, ptr[int64](1590), ptr[int64](15900)},
		{"plan.b3.update", "B3 editable B", 0, nil, ptr[int64](9900)},
		{"plan.b3.update", "B3 editable C", 1, ptr[int64](790), nil},
		{"plan.b3.delete", "B3 可删除 deletable A", 0, ptr[int64](100), nil},
		{"plan.b3.delete", "B3 deletable B", 0, ptr[int64](200), nil},
	}
	for i, shape := range shapes {
		content := fmt.Sprintf("<p>%s, synthetic batch 3 plan</p>", shape.name)
		plan := model.Plan{
			GroupID: group, TransferEnable: int64(60 + 10*i), Name: shape.name, Content: &content, Show: shape.show, Sort: ptr(40 + i),
			Renew: 1, MonthPrice: shape.month, YearPrice: shape.year, DeviceLimit: ptr(2), SpeedLimit: ptr[int64](200),
			CreatedAt: s.At(Days(-90 + i)), UpdatedAt: s.At(Days(-90 + i)),
		}
		s.Create(&plan)
		s.World.Add(shape.key, plan.ID)
	}

	plans := s.World.IDs["plan"]
	month := "month"
	type couponShape struct {
		key, code, name string
		kind, value     int
		limit           *int
		used            int
		with            *uint
		period          *string
		start, end      time.Duration
	}
	valid := [2]time.Duration{Days(-60), Days(3650)}
	shapesC := []couponShape{
		{"valid.percent", "STAGE-PCT20", "八折 20% off", 1, 20, nil, 12, nil, nil, valid[0], valid[1]},
		{"valid.fixed", "STAGE-FIX500", "立减 5.00", 2, 500, nil, 3, nil, nil, valid[0], valid[1]},
		{"valid.big", "STAGE-FIX99999", "Bigger than any price", 2, 99999, ptr(1000), 0, nil, nil, valid[0], valid[1]},
		{"expired", "STAGE-EXPIRED", "Expired 过期", 1, 50, nil, 40, nil, nil, Days(-200), Days(-10)},
		{"future", "STAGE-FUTURE", "Not started 未开始", 1, 10, nil, 0, nil, nil, Days(3000), Days(3650)},
		{"exhausted", "STAGE-USEDUP", "Used up 已用完", 2, 300, ptr(3), 3, nil, nil, valid[0], valid[1]},
		{"limited", "STAGE-LIMIT100", "100 uses", 1, 15, ptr(100), 5, nil, nil, valid[0], valid[1]},
		{"plan_limited", "STAGE-PLAN-MONTH", "Plan and period limited", 1, 30, ptr(50), 1, &plans[2], &month, valid[0], valid[1]},
		{"zero_limit", "STAGE-ZERO-LIMIT", "Zero limit is unlimited", 2, 100, ptr(0), 7, nil, nil, valid[0], valid[1]},
		{"negative_limit", "STAGE-MINUS", "Negative limit", 1, 5, ptr(-1), 2, nil, nil, valid[0], valid[1]},
		{"deletable", "STAGE-DEL-1", "To delete 1", 1, 5, nil, 0, nil, nil, valid[0], valid[1]},
		{"deletable", "STAGE-DEL-2", "To delete 2", 2, 50, nil, 0, nil, nil, valid[0], valid[1]},
		{"deletable", "STAGE-DEL-3", "To delete 3", 2, 60, ptr(1), 0, nil, nil, valid[0], valid[1]},
	}
	n := 0
	createCoupon := func(key string, coupon model.Coupon) {
		n++
		// Distinct creation times: the coupon list orders by created_at
		// only.
		coupon.CreatedAt = s.At(Days(-100) + time.Duration(n)*61*time.Minute)
		coupon.UpdatedAt = coupon.CreatedAt
		s.Create(&coupon)
		s.World.Add("coupon."+key, coupon.ID)
		s.World.AddString("coupon.code."+key, coupon.Code)
	}
	for _, shape := range shapesC {
		createCoupon(shape.key, model.Coupon{
			Code: shape.code, Name: shape.name, Type: shape.kind, Value: shape.value, LimitUse: shape.limit, UseCount: shape.used,
			LimitUseWith: shape.with, LimitPeriod: shape.period, StartedAt: s.At(shape.start).Unix(), EndedAt: s.At(shape.end).Unix(),
		})
	}
	for i := 0; i < 24*s.Scale; i++ {
		kind := 1 + i%2
		value := 5 + s.Rand.Intn(40)
		if kind == 2 {
			value = 100 * (1 + s.Rand.Intn(20))
		}
		end := valid[1]
		if i%4 == 3 {
			end = Days(-1 - i)
		}
		var limit *int
		if i%3 == 0 {
			limit = ptr(5 + s.Rand.Intn(50))
		}
		createCoupon("bulk", model.Coupon{
			Code: fmt.Sprintf("STG-BULK-%03d", i+1), Name: fmt.Sprintf("批量券 bulk %d", i+1), Type: kind, Value: value,
			LimitUse: limit, UseCount: s.Rand.Intn(5), StartedAt: s.At(Days(-90)).Unix(), EndedAt: s.At(end).Unix(),
		})
	}
}

// seedB3Orders writes orders in every status (0 pending, 1 paid,
// 2 cancelled, 3 completed, 4 discounted) and of every type (1 new,
// 2 renewal, 3 upgrade, 4 traffic reset) for the user persona, orders of
// user2, the targets of the administrator's order routes, renewal orders
// the "mark paid" route and the signed callbacks complete (a renewal of an
// unexpired plan extends the stored expiry, so the result does not depend
// on the clock), orders of invited members and a bulk of other members'
// orders.
func seedB3Orders(s *Seeder) {
	userID, user2ID := s.World.PersonaID(User), s.World.PersonaID(User2)
	members := s.World.IDs["user.member"]
	plans := s.World.IDs["plan"]
	if len(members) < 50 {
		panic("seed: batch 3 needs at least 50 members")
	}
	user := b3User(s, userID)
	userPlan := plans[0]
	if user.PlanID != nil {
		userPlan = *user.PlanID
	}
	upgrade := plans[2]
	if upgrade == userPlan {
		upgrade = plans[1]
	}
	s.World.Add("order.user.plan", userPlan)
	s.World.Add("order.user.upgrade_plan", upgrade)
	s.World.AddString("order.user.renew_period", b3SoldPeriod(s, userPlan))
	percent := s.World.ID("coupon.valid.percent", 0)

	n := 0
	create := func(keys []string, order model.Order) model.Order {
		n++
		order.TradeNo = fmt.Sprintf("STG3%014d", n)
		order.CreatedAt = s.At(-time.Duration(n) * 53 * time.Minute)
		order.UpdatedAt = order.CreatedAt.Add(time.Minute)
		if order.Status == 1 || order.Status == 3 || order.Status == 4 {
			paid := order.CreatedAt.Add(5 * time.Minute).Unix()
			callback := "CB" + order.TradeNo
			order.PaidAt, order.CallbackNo = &paid, &callback
		}
		if order.DiscountAmount == nil {
			order.DiscountAmount = ptr[int64](0)
		}
		s.Create(&order)
		for _, key := range keys {
			s.World.Add(key, order.ID)
		}
		return order
	}

	// The user persona: every status of every type.
	periods := []string{"month", "quarter", "year", "onetime"}
	for status := 0; status <= 4; status++ {
		for kind := 1; kind <= 4; kind++ {
			order := model.Order{UserID: userID, Type: kind, Status: status, Period: periods[(status+kind)%len(periods)],
				TotalAmount: int64(990 + 100*kind + status)}
			switch kind {
			case 1:
				order.PlanID = plans[0]
			case 2:
				order.PlanID = userPlan
			case 3:
				order.PlanID = upgrade
				order.SurplusAmount = ptr[int64](350)
				order.SurplusOrder = ptr(`[1]`)
			case 4:
				order.PlanID, order.Period = plans[1], "reset_price"
			}
			if status == 4 || kind == 3 {
				order.CouponID, order.DiscountAmount = &percent, ptr[int64](200)
			}
			if status == 2 && kind == 1 {
				order.RefundAmount = ptr[int64](0)
				order.Balance = ptr[int64](100)
			}
			created := create([]string{fmt.Sprintf("order.user.status.%d", status), "order.user"}, order)
			s.World.AddString("order.trade_no.user", created.TradeNo)
		}
	}
	// Pending orders the payment creation routes pay.
	for _, total := range b3PayableTotals {
		create([]string{"order.user.payable", "order.user"}, model.Order{UserID: userID, PlanID: userPlan, Type: 2, Period: "month", TotalAmount: total})
	}
	// Renewals the signed EPay and x402 callbacks complete.
	for i := 0; i < 2; i++ {
		create([]string{"order.renewal.epay", "order.user"}, model.Order{UserID: userID, PlanID: userPlan, Type: 2, Period: "month", TotalAmount: b3RenewalTotal})
	}
	for i := 0; i < 3; i++ {
		create([]string{"order.renewal.x402", "order.user"}, model.Order{UserID: userID, PlanID: userPlan, Type: 2, Period: "month", TotalAmount: b3RenewalTotal})
	}

	// user2: one order in each status.
	for status := 0; status <= 4; status++ {
		create([]string{fmt.Sprintf("order.user2.status.%d", status), "order.user2"},
			model.Order{UserID: user2ID, PlanID: plans[status%4], Type: 1 + status%3, Period: "month", TotalAmount: int64(1990 + status)})
	}

	// Targets of the administrator's status, cancel and "mark paid" routes.
	targets := []struct {
		key    string
		status int
		count  int
	}{{"order.admin.pending", 0, 6}, {"order.admin.completed", 3, 2}, {"order.admin.cancelled", 2, 2}, {"order.admin.paid", 1, 2}}
	next := 20
	for _, target := range targets {
		for i := 0; i < target.count; i++ {
			create([]string{target.key}, model.Order{UserID: members[next], PlanID: plans[next%4], Type: 1, Period: "quarter",
				Status: target.status, TotalAmount: int64(2790 + next)})
			next++
		}
	}
	// Renewals of the members' own, unexpired plans.
	for i, key := range []string{"order.renewal.markpaid", "order.renewal.markpaid", "order.renewal.markpaid.paid"} {
		member := b3User(s, members[next+i])
		if member.PlanID == nil {
			panic(fmt.Errorf("seed: member %d has no plan", member.ID))
		}
		status := 0
		if key == "order.renewal.markpaid.paid" {
			status = 1
		}
		create([]string{key}, model.Order{UserID: member.ID, PlanID: *member.PlanID, Type: 2, Period: "month", Status: status, TotalAmount: b3RenewalTotal})
	}
	// Members the plan assignment route grants plans to.
	for i := 40; i < 46; i++ {
		s.World.Add("b3.assign.target", members[i])
	}

	// Invited members' orders, with the inviter and its commission.
	var invited []model.User
	if err := s.DB.Select("id", "invite_user_id").Where("id IN ?", s.World.IDs["user.invited"]).Order("id").Find(&invited).Error; err != nil {
		panic(err)
	}
	for i, buyer := range invited {
		for j, status := range []int{3, 1, 0} {
			if j == 2 && i%2 == 1 {
				continue
			}
			total := int64(1990 + 1000*(i%3))
			order := model.Order{UserID: buyer.ID, InviteUserID: buyer.InviteUserID, PlanID: plans[i%4], Type: 1, Period: "month",
				Status: status, TotalAmount: total}
			if status != 0 {
				order.CommissionStatus, order.CommissionBalance = 1+j%2, total/10
			}
			create([]string{"order.invited"}, order)
		}
	}

	// Other members' orders.
	others := members[30:]
	kinds := []string{"month", "quarter", "half_year", "year", "two_year", "three_year", "onetime"}
	for i := 0; i < 140*s.Scale; i++ {
		status, kind := s.Rand.Intn(5), 1+s.Rand.Intn(4)
		order := model.Order{UserID: others[s.Rand.Intn(len(others))], PlanID: plans[s.Rand.Intn(6)], Type: kind,
			Period: kinds[s.Rand.Intn(len(kinds))], Status: status, TotalAmount: int64(500 + 10*s.Rand.Intn(5000))}
		if kind == 4 {
			order.Period = "reset_price"
		}
		if i%9 == 0 {
			order.CouponID, order.DiscountAmount = &percent, ptr(order.TotalAmount/5)
		}
		created := create([]string{"order.other"}, order)
		if i < 8 {
			s.World.AddString("order.trade_no.other", created.TradeNo)
		}
	}
}

// seedB3Gateways writes payment gateways of several types, enabled and
// disabled, with fake credentials and example.com URLs, and the older
// v2_payment method rows the method list and x402 creation read.
func seedB3Gateways(s *Seeder) {
	notify := func(kind string) string { return "https://panel.example.com/api/v2/payment/callback/" + kind }
	type gatewayShape struct {
		keys        []string
		name, kind  string
		enabled     bool
		config      string
		rate, fixed float64
		min, max    float64
	}
	shapes := []gatewayShape{
		// The enabled gateways come first: a callback uses the first
		// enabled gateway of its type.
		{[]string{"gateway.epay"}, "EPay 易支付", "epay", true,
			fmt.Sprintf(`{"api_url":"https://pay.example.com","pid":"1001","key":%q,"notify_url":%q,"return_url":"https://panel.example.com/#/order"}`, b3EPayKey, notify("epay")),
			0, 0, 1, 10000},
		{[]string{"gateway.stripe"}, "Stripe 信用卡", "stripe", true,
			fmt.Sprintf(`{"publishable_key":"pk_test_staging","secret_key":"sk_test_staging_fake","webhook_secret":%q,"currency":"USD"}`, b3StripeSecret),
			0.03, 0.3, 5, 2000},
		{[]string{"gateway.paypal"}, "PayPal", "paypal", true,
			`{"client_id":"staging-client-id","client_secret":"staging-client-secret-fake","webhook_id":"WH-STAGING","sandbox_mode":true,"currency":"USD"}`,
			0.034, 0.35, 1, 5000},
		{[]string{"gateway.x402"}, "x402 测试币", "x402", true,
			fmt.Sprintf(`{"wallet_address":"0x00000000000000000000000000000000000c0de0","network":"sepolia","accept_tokens":["ETH ","usdc"],"test_mode":true,"webhook_secret":%q,"confirm_blocks":1}`, b3X402Secret),
			0, 0, 1, 10000},
		{[]string{"gateway.toggle"}, "EPay backup 备用", "epay", false,
			fmt.Sprintf(`{"api_url":"https://pay-backup.example.com","pid":"1002","key":"fake-backup-key","notify_url":%q}`, notify("epay")), 0.01, 0, 1, 10000},
		{[]string{"gateway.toggle"}, "Stripe EU", "stripe", false,
			`{"publishable_key":"pk_test_eu","secret_key":"sk_test_eu_fake","webhook_secret":"whsec_eu_fake","currency":"EUR"}`, 0.029, 0.25, 1, 1000},
		{[]string{"gateway.disabled"}, "支付宝 Alipay", "alipay", false,
			`{"app_id":"2021000000000000","private_key":"fake-private-key","public_key":"fake-public-key","notify_url":"https://panel.example.com/api/v2/payment/callback/alipay","sandbox":true}`,
			0.006, 0, 0.01, 50000},
		{[]string{"gateway.disabled"}, "微信支付 WeChat Pay", "wechat", false,
			`{"app_id":"wx0000000000","mch_id":"1900000000","api_key":"fake-api-key","api_v3_key":"fake-v3-key","notify_url":"https://panel.example.com/api/v2/payment/callback/wechat"}`,
			0.006, 0, 0.01, 50000},
		{[]string{"gateway.disabled"}, "USDT TRC20", "usdt", false,
			`{"network":"TRC20","wallet_address":"T-staging-wallet-0001","api_key":"fake-tron-key","confirmations":19}`, 0, 1, 10, 100000},
		{[]string{"gateway.update"}, "EPay to edit", "epay", false,
			`{"api_url":"https://pay-edit.example.org","pid":"1003","key":"fake-edit-key"}`, 0, 0, 1, 10000},
		{[]string{"gateway.update"}, "USDT to edit", "usdt", false, `{"network":"ERC20","wallet_address":"0xedit"}`, 0, 0, 1, 10000},
		{[]string{"gateway.delete"}, "WeChat to delete", "wechat", false, ``, 0, 0, 1, 10000},
		{[]string{"gateway.delete"}, "EPay to delete", "epay", false, `not json`, 0, 0, 1, 10000},
	}
	for i, shape := range shapes {
		gateway := model.PaymentGateway{
			Name: shape.name, Type: shape.kind, Enabled: shape.enabled, Icon: "https://cdn.example.com/" + shape.kind + ".png", Config: shape.config,
			FeeRate: shape.rate, FeeFixed: shape.fixed, MinAmount: shape.min, MaxAmount: shape.max, Sort: 10 - i%5,
			Description: "synthetic staging gateway " + shape.kind, TotalOrders: int64(3 * i), TotalAmount: float64(150 * i),
			CreatedAt: s.At(Days(-150 + i)), UpdatedAt: s.At(Days(-150 + i)),
		}
		s.Create(&gateway)
		for _, key := range shape.keys {
			s.World.Add(key, gateway.ID)
		}
	}

	methods := []struct {
		name, method, provider string
		enable                 int
	}{
		{"虚拟货币 x402", model.PaymentMethodCrypto, model.PaymentProviderX402, 1},
		{"Stripe", model.PaymentMethodFiat, model.PaymentProviderStripe, 1},
		{"微信支付", model.PaymentMethodFiat, model.PaymentProviderWechatPay, 1},
		{"支付宝", model.PaymentMethodFiat, model.PaymentProviderAlipay, 0},
		{"PayPal", model.PaymentMethodFiat, model.PaymentProviderPayPal, 0},
	}
	for i, m := range methods {
		icon, notifyURL := "https://cdn.example.com/"+m.provider+".svg", notify(m.provider)
		payment := model.Payment{Name: m.name, Icon: &icon, Method: m.method, Provider: m.provider,
			Config: `{"note":"synthetic staging configuration","secret":"fake"}`, Notify: &notifyURL, Sort: i, Enable: m.enable,
			CreatedAt: s.At(Days(-160 + i)), UpdatedAt: s.At(Days(-160 + i))}
		s.Create(&payment)
		if m.enable == 0 {
			// enable has a column default of 1, which GORM writes for a
			// zero value; UpdateColumn leaves updated_at alone.
			if err := s.DB.Model(&payment).UpdateColumn("enable", 0).Error; err != nil {
				panic(err)
			}
		}
		s.World.Add("payment.method", payment.ID)
	}
}

// seedB3Records writes payment records in every status (0 pending, 1 paid,
// 2 cancelled, 3 refunded, 4 expired) for the user persona and user2, the
// pending records the signed callbacks pay, and a bulk of top-ups. A paid
// record names a completed order or none: a paid record of a pending order
// would be completed by the kernel's order payment reconciler on its own.
func seedB3Records(s *Seeder) {
	userID, user2ID := s.World.PersonaID(User), s.World.PersonaID(User2)
	epay, stripe := s.World.ID("gateway.epay", 0), s.World.ID("gateway.stripe", 0)
	n := 0
	create := func(keys []string, tradeKey string, record model.PaymentRecord, offset time.Duration) {
		n++
		if record.TradeNo == "" {
			record.TradeNo = fmt.Sprintf("PAYSTG3%011d", n)
		}
		if record.Currency == "" {
			record.Currency = "CNY"
		}
		if record.ActualAmount == 0 {
			record.ActualAmount = record.Amount + record.FeeAmount
		}
		record.ClientIP = fmt.Sprintf("198.51.100.%d", 1+n%250)
		record.CreatedAt = s.At(offset)
		record.UpdatedAt = record.CreatedAt.Add(time.Minute)
		at := record.CreatedAt.Add(3 * time.Minute)
		switch record.Status {
		case model.PaymentStatusPaid:
			record.PaidAt = &at
			if record.GatewayTradeNo == "" {
				record.GatewayTradeNo = "GW" + record.TradeNo
			}
			record.NotifyData = fmt.Sprintf(`{"trade_no":%q,"status":"paid"}`, record.TradeNo)
		case model.PaymentStatusCancelled, model.PaymentStatusExpired:
			record.CancelledAt = &at
		case model.PaymentStatusRefunded:
			paid := record.CreatedAt.Add(2 * time.Minute)
			record.PaidAt, record.RefundedAt = &paid, &at
		}
		s.Create(&record)
		for _, key := range keys {
			s.World.Add(key, record.ID)
		}
		if tradeKey != "" {
			s.World.AddString(tradeKey, record.TradeNo)
		}
	}
	order := func(key string, i int) *uint { id := s.World.ID(key, i); return &id }
	wallet := func(i int) *string { return ptr(fmt.Sprintf("0x%040x", 0xc0ffee+i)) }
	network := ptr("sepolia")

	user := []string{"payment.record.user"}
	create(user, "payment.trade_no.pending", model.PaymentRecord{GatewayID: epay, GatewayType: "epay", Provider: "epay", UserID: userID,
		Amount: 10.9, Status: model.PaymentStatusPending, OrderID: order("order.user.status.0", 0)}, Days(2))
	create(user, "payment.trade_no.paid", model.PaymentRecord{GatewayID: epay, GatewayType: "epay", Provider: "epay", UserID: userID,
		Amount: 11.93, Status: model.PaymentStatusPaid, OrderID: order("order.user.status.3", 0)}, Days(3))
	create(user, "payment.trade_no.cancelled", model.PaymentRecord{GatewayID: stripe, GatewayType: "stripe", Provider: "stripe", UserID: userID,
		Amount: 12.0, FeeAmount: 0.66, Status: model.PaymentStatusCancelled, Currency: "USD"}, Days(4))
	create(user, "payment.trade_no.refunded", model.PaymentRecord{GatewayID: epay, GatewayType: "epay", Provider: "epay", UserID: userID,
		Amount: 30, Status: model.PaymentStatusRefunded}, Days(5))
	create(user, "payment.trade_no.expired", model.PaymentRecord{GatewayID: stripe, GatewayType: "stripe", Provider: "stripe", UserID: userID,
		Amount: 15, FeeAmount: 0.75, Status: model.PaymentStatusExpired, Currency: "USD"}, Days(6))
	create(user, "payment.trade_no.x402.pending", model.PaymentRecord{TradeNo: "X402STG3PENDING01", GatewayType: "crypto", Provider: "x402",
		UserID: userID, Amount: 9.9, ActualAmount: 0.000099, Currency: "ETH", Status: model.PaymentStatusPending,
		WalletAddress: wallet(1), Network: network, OrderID: order("order.user.status.0", 1)}, Days(7))
	create(user, "payment.trade_no.x402.paid", model.PaymentRecord{TradeNo: "X402STG3PAID0001", GatewayType: "crypto", Provider: "x402",
		UserID: userID, Amount: 12.93, ActualAmount: 0.0001293, Currency: "ETH", Status: model.PaymentStatusPaid,
		TxHash: ptr(fmt.Sprintf("0x%064x", 0xbeef)), WalletAddress: wallet(2), Network: network, OrderID: order("order.user.status.3", 1)}, Days(8))
	create(user, "payment.trade_no.fiat", model.PaymentRecord{TradeNo: "FIATSTG3PENDING01", GatewayType: "fiat", Provider: "stripe",
		UserID: userID, Amount: 19.9, Currency: "USD", Status: model.PaymentStatusPending}, Days(9))
	// The signed callbacks' records.
	for i := 0; i < 2; i++ {
		create(append(user, "payment.record.cb.epay"), "payment.trade_no.cb.epay", model.PaymentRecord{TradeNo: fmt.Sprintf("PAYSTG3CBEPAY%d", i+1),
			GatewayID: epay, GatewayType: "epay", Provider: "epay", UserID: userID, Amount: b3Yuan(b3RenewalTotal),
			Status: model.PaymentStatusPending, OrderID: order("order.renewal.epay", i)}, Days(10+i))
	}
	for i := 0; i < 3; i++ {
		create(append(user, "payment.record.cb.x402"), "payment.trade_no.cb.x402", model.PaymentRecord{TradeNo: fmt.Sprintf("X402STG3CB%d", i+1),
			GatewayType: "crypto", Provider: "x402", UserID: userID, Amount: b3Yuan(b3RenewalTotal), ActualAmount: float64(b3RenewalTotal) / 100_000_000,
			Currency: "ETH", Status: model.PaymentStatusPending, WalletAddress: wallet(10 + i), Network: network, OrderID: order("order.renewal.x402", i)}, Days(12+i))
	}
	// user2.
	create([]string{"payment.record.user2"}, "payment.trade_no.user2", model.PaymentRecord{GatewayID: epay, GatewayType: "epay", Provider: "epay",
		UserID: user2ID, Amount: 19.9, Status: model.PaymentStatusPending, OrderID: order("order.user2.status.0", 0)}, Days(-3))
	create([]string{"payment.record.user2"}, "payment.trade_no.user2", model.PaymentRecord{GatewayID: stripe, GatewayType: "stripe", Provider: "stripe",
		UserID: user2ID, Amount: 19.93, FeeAmount: 0.9, Currency: "USD", Status: model.PaymentStatusPaid, OrderID: order("order.user2.status.3", 0)}, Days(-2))

	// Top-ups of other members, spread around the seed base.
	members := s.World.IDs["user.member"][30:]
	types := []struct{ gatewayType, provider string }{{"epay", "epay"}, {"stripe", "stripe"}, {"crypto", "x402"}, {"fiat", "paypal"}}
	for i := 0; i < 60*s.Scale; i++ {
		kind := types[i%len(types)]
		record := model.PaymentRecord{GatewayType: kind.gatewayType, Provider: kind.provider, UserID: members[s.Rand.Intn(len(members))],
			Amount: float64(100*(1+s.Rand.Intn(300))) / 100, Status: s.Rand.Intn(5)}
		switch kind.gatewayType {
		case "epay":
			record.GatewayID = epay
		case "stripe":
			record.GatewayID = stripe
			record.FeeAmount = float64(int(record.Amount*3)+30) / 100
			record.Currency = "USD"
		case "crypto":
			record.ActualAmount, record.Currency = record.Amount/1000, "USDC"
			record.WalletAddress, record.Network = wallet(100+i), network
		}
		create([]string{"payment.record.other"}, "payment.trade_no.other", record, Days(-40)+time.Duration(i)*17*time.Hour)
	}
}

// seedB3Affiliate writes the invite configuration and frontend settings,
// commission balances, invite codes (unused, used, expired, public),
// commission records in every status and type, and withdrawals in every
// status, including a reservation (-1), a fractional amount and a
// withdrawal whose user does not exist.
func seedB3Affiliate(s *Seeder) {
	userID, user2ID := s.World.PersonaID(User), s.World.PersonaID(User2)

	// The configuration: the routes create the default one when there is
	// none, so it must exist before shadow traffic (legacy and native would
	// both create one).
	config := map[string]any{
		"enabled": true, "auto_generate": true, "code_count": 5, "code_expire_days": 30, "commission_enabled": true,
		"commission_type": 1, "commission_rate": 0.2, "commission_fixed": 0, "commission_min_amount": 10,
		"first_order_bonus": 1.5, "first_traffic_bonus": int64(1) << 30,
	}
	var existing model.InviteConfig
	err := s.DB.Order("id").First(&existing).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		cfg := model.InviteConfig{Enabled: true, AutoGenerate: true, CodeCount: 5, CodeExpireDays: 30, CommissionEnabled: true,
			CommissionType: 1, CommissionRate: 0.2, CommissionMinAmount: 10, FirstOrderBonus: 1.5, FirstTrafficBonus: 1 << 30,
			CreatedAt: s.At(Days(-200)), UpdatedAt: s.At(Days(-200))}
		s.Create(&cfg)
	case err != nil:
		panic(err)
	default:
		config["created_at"], config["updated_at"] = s.At(Days(-200)), s.At(Days(-200))
		if err := s.DB.Model(&existing).UpdateColumns(config).Error; err != nil {
			panic(err)
		}
	}
	frontend := `{"code_prefix":"STG","code_length":8,"withdraw_fee":1.5,"withdraw_methods":["alipay","bank"]}`
	var setting model.SystemConfig
	err = s.DB.Where("key = ?", "invite.frontend.config").Take(&setting).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		s.Create(&model.SystemConfig{Key: "invite.frontend.config", Value: frontend, Type: "json", Group: "invite",
			Remark: "Invite frontend configuration fields", CreatedAt: s.At(Days(-200)), UpdatedAt: s.At(Days(-200))})
	case err != nil:
		panic(err)
	default:
		if err := s.DB.Model(&setting).UpdateColumns(map[string]any{"value": frontend, "updated_at": s.At(Days(-200))}).Error; err != nil {
			panic(err)
		}
	}

	// Commission balances (cents) the withdrawal routes debit.
	for id, balance := range map[uint]int64{userID: 50000, user2ID: 1500} {
		if err := s.DB.Model(&model.User{}).Where("id = ?", id).UpdateColumn("commission_balance", balance).Error; err != nil {
			panic(err)
		}
	}

	// Invite codes.
	invited := s.World.IDs["user.invited"]
	n := 0
	code := func(key string, owner *uint, status int, usedBy *uint, expired *time.Time) {
		n++
		created := s.At(Days(-80) + time.Duration(n)*7*time.Hour)
		invite := model.InviteCode{Code: fmt.Sprintf("stg3%05x", 0x1000+n*37), UserID: owner, Status: status, UsedBy: usedBy,
			ExpiredAt: expired, CreatedAt: created, UpdatedAt: created}
		if usedBy != nil {
			used := created.Add(30 * time.Hour)
			invite.UsedAt = &used
		}
		s.Create(&invite)
		s.World.Add(key, invite.ID)
		s.World.AddString(key, invite.Code)
	}
	past, future := s.At(Days(-5)), s.At(Days(3000))
	code("invite.user", &userID, 0, nil, nil)
	code("invite.user", &userID, 0, nil, &future)
	code("invite.user", &userID, 0, nil, nil)
	code("invite.user.expired", &userID, 0, nil, &past)
	code("invite.user.used", &userID, 1, &invited[0], nil)
	code("invite.user.used", &userID, 1, &invited[1], nil)
	for i := 0; i < 5; i++ {
		code("invite.user2", &user2ID, 0, nil, nil)
	}
	for i := 0; i < 3; i++ {
		code("invite.public", nil, 0, nil, nil)
	}
	code("invite.public", nil, 1, &invited[2], nil)
	members := s.World.IDs["user.member"]
	for i := 0; i < 20*s.Scale; i++ {
		owner := members[30+s.Rand.Intn(len(members)-30)]
		code("invite.other", &owner, s.Rand.Intn(2), nil, nil)
	}

	// Commission records of the inviters, from their invitees' orders.
	var buyers []model.User
	if err := s.DB.Select("id", "invite_user_id").Where("id IN ?", invited).Order("id").Find(&buyers).Error; err != nil {
		panic(err)
	}
	inviter := map[uint]uint{}
	for _, buyer := range buyers {
		if buyer.InviteUserID != nil {
			inviter[buyer.ID] = *buyer.InviteUserID
		}
	}
	var orders []model.Order
	if err := s.DB.Select("id", "user_id", "total_amount").Where("id IN ?", s.World.IDs["order.invited"]).Order("id").Find(&orders).Error; err != nil {
		panic(err)
	}
	for i, order := range orders {
		owner, ok := inviter[order.UserID]
		if !ok {
			continue
		}
		created := s.At(Days(-70) + time.Duration(i)*11*time.Hour)
		record := model.CommissionRecord{UserID: owner, OrderID: order.ID, FromUserID: order.UserID,
			Amount: float64(order.TotalAmount) / 10, Type: 1 + i%3, Status: i % 4,
			Remark: fmt.Sprintf("订单 %d 佣金 commission", order.ID), CreatedAt: created, UpdatedAt: created}
		s.Create(&record)
		key := "commission.user2"
		if owner == userID {
			key = "commission.user"
		}
		s.World.Add(key, record.ID)
	}

	// Withdrawals.
	w := 0
	withdraw := func(key string, owner uint, amount float64, status int, method string) {
		w++
		created := s.At(Days(-60) + time.Duration(w)*13*time.Hour)
		record := model.CommissionWithdraw{UserID: owner, Amount: amount, Method: method,
			Account: fmt.Sprintf("account-%03d@example.com", w), Name: fmt.Sprintf("收款人 %d", w), Status: status,
			CreatedAt: created, UpdatedAt: created}
		switch status {
		case 1:
			record.Remark, record.ProcessedAt = "paid out", ptr(created.Add(24*time.Hour))
		case 2:
			record.Remark, record.ProcessedAt = "rejected: wrong account", ptr(created.Add(20*time.Hour))
		}
		s.Create(&record)
		s.World.Add(key, record.ID)
	}
	methods := []string{"alipay", "wechat", "bank"}
	for i := 0; i < 8; i++ {
		withdraw("withdraw.pending", userID, float64(100+50*i), 0, methods[i%3])
	}
	withdraw("withdraw.approved", userID, 300, 1, "alipay")
	withdraw("withdraw.approved", userID, 120, 1, "bank")
	withdraw("withdraw.rejected", userID, 500, 2, "wechat")
	withdraw("withdraw.rejected", userID, 80, 2, "alipay")
	withdraw("withdraw.reserving", userID, 60, -1, "bank")
	withdraw("withdraw.fractional", userID, 150.5, 0, "bank")
	withdraw("withdraw.user2.pending", user2ID, 200, 0, "alipay")
	withdraw("withdraw.user2.pending", user2ID, 90, 0, "wechat")
	withdraw("withdraw.orphan", Missing, 70, 0, "bank")
	for i := 0; i < 30*s.Scale; i++ {
		owner := members[30+s.Rand.Intn(len(members)-30)]
		withdraw("withdraw.other", owner, float64(10*(1+s.Rand.Intn(100))), s.Rand.Intn(3), methods[s.Rand.Intn(3)])
	}
}
