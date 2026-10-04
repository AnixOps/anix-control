package kernelforward

import (
	"context"
	"log/slog"
	"strconv"
	"strings"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
)

// WritePrometheus renders the forwarding gauges in the Prometheus text
// format, from db: the nodes behind their desired generation and the
// largest lag, the hop errors of the nodes' latest reports, and whether
// the last plan was refused, and the counter of generation recoveries. A
// database without the tables writes nothing.
func WritePrometheus(body *strings.Builder, db *gorm.DB) {
	if db == nil {
		return
	}
	nodes, err := NodeConvergence(context.Background(), db)
	if err != nil {
		slog.Debug("kernel forward: convergence could not be read", "component", "kernel-forward", "error", err)
		return
	}
	var lagging, unreported, hopErrors int
	var maxLag uint64
	for _, node := range nodes {
		if !node.Reported {
			unreported++
		}
		if lag := node.Lag(); lag > 0 {
			lagging++
			if lag > maxLag {
				maxLag = lag
			}
		}
		hopErrors += node.HopErrors
	}
	var plans []model.KernelForwardPlan
	refused := 0
	if err := db.Where("id = ?", planRowID).Limit(1).Find(&plans).Error; err == nil && len(plans) == 1 && plans[0].Refused {
		refused = 1
	}
	gauge := func(name, help string, value string) {
		body.WriteString("# HELP " + name + " " + help + "\n")
		body.WriteString("# TYPE " + name + " gauge\n")
		body.WriteString(name + " " + value + "\n")
	}
	gauge("anixops_forward_nodes", "Nodes with a desired forwarding state.", strconv.Itoa(len(nodes)))
	gauge("anixops_forward_lagging_nodes", "Nodes whose latest forward report runs an older generation than their desired one, or that never reported.", strconv.Itoa(lagging))
	gauge("anixops_forward_unreported_nodes", "Nodes with a desired forwarding state and no forward report yet.", strconv.Itoa(unreported))
	gauge("anixops_forward_generation_lag_max", "The largest number of generations a node runs behind its desired forwarding state.", strconv.FormatUint(maxLag, 10))
	gauge("anixops_forward_hop_errors", "Hops the nodes' latest forward reports could not apply.", strconv.Itoa(hopErrors))
	gauge("anixops_forward_plan_refused", "1 when the last forwarding replan was refused and every node kept its state.", strconv.Itoa(refused))
	name := "anixops_forward_generation_recoveries_total"
	body.WriteString("# HELP " + name + " Node forwarding generations this process moved above the one the node's Agent holds, by reason: report (a report ahead of the stored generation, after a Control database reset or restore), plan (a plan stamped from such a report) or operator (forward reset-node).\n")
	body.WriteString("# TYPE " + name + " counter\n")
	for _, reason := range []string{RecoveryOperator, RecoveryPlan, RecoveryReport} {
		body.WriteString(name + `{reason="` + reason + `"} ` + strconv.FormatUint(recoveries[reason].Load(), 10) + "\n")
	}
}
