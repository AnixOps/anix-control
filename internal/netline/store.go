package netline

import (
	"time"

	"github.com/AnixOps/anix-control/v5/internal/kernelforward"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

func init() { kernelforward.ProjectLines(Project) }

// lineColumns are the columns of a forward route's line that Project
// writes; the route, its revision and enforcement are the forward API's.
var lineColumns = []string{
	"sort", "name", "enabled", "show", "entries", "entry_hostname", "listen", "port", "l4", "chain",
	"exit", "rules", "protocol_id", "targets", "policy", "hop_balance", "rate", "serve_at",
}

// Project writes each forward route's line as Import reads the route into
// its row of v5_line, and the transports of the lines in place of the
// stored ones. A route Import skips keeps its columns.
func Project(db *gorm.DB) error {
	s, err := Import(db)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if err := db.Where("1 = 1").Delete(&model.Transport{}).Error; err != nil {
		return err
	}
	for _, t := range s.Transports {
		row := model.Transport{ID: t.ID, From: t.From, To: t.To, Kind: t.Kind, Carriers: t.Carriers, Multipath: t.Multipath, FEC: t.FEC, Declared: t.Declared, UpdatedAt: now}
		if err := db.Create(&row).Error; err != nil {
			return err
		}
	}
	for i, line := range s.Lines {
		row := model.Line{
			Sort: i, Name: line.Name, Enabled: line.Enabled, Show: line.Show, Entries: line.Entries, EntryHostname: line.EntryHostname,
			Listen: line.Listen, Port: line.Port, L4: line.L4, Chain: line.Chain, Exit: line.Exit, Rules: line.Rules,
			Targets: line.Targets, Policy: line.Policy, HopBalance: line.HopBalance, Rate: line.Rate, ServeAt: line.ServeAt,
		}
		if line.Protocol != nil {
			row.ProtocolID = &line.Protocol.ID
		}
		if err := db.Model(&model.Line{}).Where("id = ? AND route <> ''", line.ID).Select(lineColumns).Updates(&row).Error; err != nil {
			return err
		}
	}
	return nil
}
