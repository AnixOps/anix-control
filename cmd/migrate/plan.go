package main

import (
	"fmt"
	"sort"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
)

// migrationPlan holds every row this tool will write, already converted to
// GORM model structs, plus the node-protocol -> old-group-id links needed to
// populate the SubscriptionGroup<->NodeProtocol many2many join table.
type migrationPlan struct {
	Plans                  []*model.Plan
	SubscriptionGroups     []*model.SubscriptionGroup
	Nodes                  []*model.Node
	NodeProtocolLinks      []nodeProtocolGroups
	Users                  []*model.User
	UserSubscriptionGroups []*model.UserSubscriptionGroup
	InviteCodes            []*model.InviteCode
	SystemConfigs          []*model.SystemConfig
}

// buildPlan runs every per-table conversion against the parsed dump tables.
// Tables absent from the dump (e.g. no vless rows) are treated as empty,
// not an error.
func buildPlan(tables map[string]*DumpTable) (*migrationPlan, error) {
	plan := &migrationPlan{}

	if t, ok := tables["v2_plan"]; ok {
		plans, err := buildPlans(t)
		if err != nil {
			return nil, err
		}
		plan.Plans = plans
	}

	if t, ok := tables["v2_server_group"]; ok {
		groups, err := buildSubscriptionGroups(t)
		if err != nil {
			return nil, err
		}
		plan.SubscriptionGroups = groups
	}

	nodes, links, err := buildNodesAndProtocols(tables)
	if err != nil {
		return nil, err
	}
	plan.Nodes = nodes
	plan.NodeProtocolLinks = links

	if t, ok := tables["v2_user"]; ok {
		users, err := buildUsers(t)
		if err != nil {
			return nil, err
		}
		plan.Users = users
		plan.UserSubscriptionGroups = buildUserSubscriptionGroups(users)
	}

	if t, ok := tables["v2_invite_code"]; ok {
		codes, err := buildInviteCodes(t)
		if err != nil {
			return nil, err
		}
		plan.InviteCodes = codes
	}

	if t, ok := tables["v2_settings"]; ok {
		configs, err := buildSystemConfigs(t)
		if err != nil {
			return nil, err
		}
		plan.SystemConfigs = configs
	}

	return plan, nil
}

// printSummary prints the row counts this run will import (or imported),
// plus every table that was explicitly skipped, so nothing is silently
// dropped without being reported.
func (p *migrationPlan) printSummary(report *importReport) {
	fmt.Println("Migration plan:")
	fmt.Printf("  v2_plan                -> Plan:                   %d rows\n", len(p.Plans))
	fmt.Printf("  v2_server_group        -> SubscriptionGroup:      %d rows\n", len(p.SubscriptionGroups))
	fmt.Printf("  v2_server_shadowsocks/vless -> Node+NodeProtocol: %d rows\n", len(p.Nodes))
	fmt.Printf("  v2_user                -> User:                  %d rows\n", len(p.Users))
	fmt.Printf("  (user.group_id)        -> UserSubscriptionGroup: %d rows\n", len(p.UserSubscriptionGroups))
	fmt.Printf("  v2_invite_code         -> InviteCode:             %d rows\n", len(p.InviteCodes))
	fmt.Printf("  v2_settings            -> SystemConfig:           %d rows\n", len(p.SystemConfigs))

	if len(report.skipped) > 0 {
		fmt.Println("\nSkipped tables (not migrated):")
		names := make([]string, 0, len(report.skipped))
		for name := range report.skipped {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Printf("  %-28s %d rows\n", name, report.skipped[name])
		}
	}
}

// importReport tracks which dump tables were intentionally skipped, and how
// many rows each contained, purely for the printed summary.
type importReport struct {
	skipped map[string]int
}

func newReport() *importReport {
	return &importReport{skipped: make(map[string]int)}
}

func (r *importReport) skip(table string, rows int) {
	r.skipped[table] = rows
}

// runImport writes every table in the plan to db in FK dependency order,
// inside a single transaction so a failure partway through leaves the
// target database untouched.
func runImport(db *gorm.DB, plan *migrationPlan) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if len(plan.Plans) > 0 {
			if err := tx.Create(&plan.Plans).Error; err != nil {
				return fmt.Errorf("insert plans: %w", err)
			}
		}
		if len(plan.SubscriptionGroups) > 0 {
			if err := tx.Create(&plan.SubscriptionGroups).Error; err != nil {
				return fmt.Errorf("insert subscription groups: %w", err)
			}
		}
		if len(plan.Nodes) > 0 {
			if err := tx.Create(&plan.Nodes).Error; err != nil {
				return fmt.Errorf("insert nodes: %w", err)
			}
		}
		for _, link := range plan.NodeProtocolLinks {
			if err := tx.Create(link.Protocol).Error; err != nil {
				return fmt.Errorf("insert node protocol (node_id=%d): %w", link.Protocol.NodeID, err)
			}
			if len(link.GroupIDs) == 0 {
				continue
			}
			var groups []model.SubscriptionGroup
			if err := tx.Where("id IN ?", link.GroupIDs).Find(&groups).Error; err != nil {
				return fmt.Errorf("load subscription groups %v for protocol %d: %w", link.GroupIDs, link.Protocol.ID, err)
			}
			if err := tx.Model(link.Protocol).Association("SubscriptionGroups").Append(groups); err != nil {
				return fmt.Errorf("link protocol %d to groups %v: %w", link.Protocol.ID, link.GroupIDs, err)
			}
		}
		if len(plan.Users) > 0 {
			if err := tx.Create(&plan.Users).Error; err != nil {
				return fmt.Errorf("insert users: %w", err)
			}
		}
		if len(plan.UserSubscriptionGroups) > 0 {
			if err := tx.Create(&plan.UserSubscriptionGroups).Error; err != nil {
				return fmt.Errorf("insert user subscription groups: %w", err)
			}
		}
		if len(plan.InviteCodes) > 0 {
			if err := tx.Create(&plan.InviteCodes).Error; err != nil {
				return fmt.Errorf("insert invite codes: %w", err)
			}
		}
		if len(plan.SystemConfigs) > 0 {
			if err := tx.Create(&plan.SystemConfigs).Error; err != nil {
				return fmt.Errorf("insert system configs: %w", err)
			}
		}
		return nil
	})
}
