package netline

import (
	"time"

	networkv1 "github.com/AnixOps/anix-control/sdk/api/network/v1"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// LineRow is a line as stored (v5_line). The protocol it serves is a
// protocol of a node, by id; its other fields are Line's.
type LineRow struct {
	ID            string                     `gorm:"primaryKey;size:64"`
	Sort          int                        `gorm:"not null;default:0"`
	Name          string                     `gorm:"size:191;not null;default:''"`
	Enabled       bool                       `gorm:"not null;default:false"`
	Show          bool                       `gorm:"not null;default:false"`
	Entries       []string                   `gorm:"type:text;serializer:json"`
	EntryHostname string                     `gorm:"size:255;not null;default:''"`
	Listen        string                     `gorm:"size:64;not null;default:''"`
	Port          uint32                     `gorm:"not null;default:0"`
	L4            []string                   `gorm:"type:text;serializer:json"`
	Chain         []Hop                      `gorm:"type:text;serializer:json"`
	Exit          string                     `gorm:"size:64;not null;default:''"`
	Rules         []Rule                     `gorm:"type:text;serializer:json"`
	ProtocolID    *uint                      `gorm:"index"`
	Targets       []*networkv1.ForwardTarget `gorm:"type:text;serializer:json"`
	Policy        *networkv1.ForwardPolicy   `gorm:"type:text;serializer:json"`
	HopBalance    string                     `gorm:"size:32;not null;default:''"`
	Rate          float64                    `gorm:"not null;default:1"`
	// ServeAt is the device the line's protocol is served at when the
	// administrator chose it; empty is the first userspace device.
	ServeAt   string    `gorm:"size:32;not null;default:''"`
	UpdatedAt time.Time `gorm:"not null"`
}

func (LineRow) TableName() string { return "v5_line" }

// TransportRow is a transport as stored (v5_transport).
type TransportRow struct {
	ID        string    `gorm:"primaryKey;size:191"`
	From      string    `gorm:"column:from_device;size:32;not null"`
	To        string    `gorm:"column:to_device;size:32;not null"`
	Kind      string    `gorm:"size:16;not null"`
	Carriers  []string  `gorm:"type:text;serializer:json"`
	Multipath bool      `gorm:"not null;default:false"`
	FEC       bool      `gorm:"column:fec;not null;default:false"`
	Declared  bool      `gorm:"not null;default:false"`
	UpdatedAt time.Time `gorm:"not null"`
}

func (TransportRow) TableName() string { return "v5_transport" }

// Save stores a snapshot's lines and transports in place of the stored
// ones, and which devices forward in the kernel. Devices, entries, uplinks
// and exits stay where they are (nodes, node entries, uplinks, exit
// policies).
func Save(db *gorm.DB, s *Snapshot) error {
	now := time.Now().UTC()
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("1 = 1").Delete(&LineRow{}).Error; err != nil {
			return err
		}
		if err := tx.Where("1 = 1").Delete(&TransportRow{}).Error; err != nil {
			return err
		}
		for i, line := range s.Lines {
			row := LineRow{
				ID: line.ID, Sort: i, Name: line.Name, Enabled: line.Enabled, Show: line.Show, Entries: line.Entries,
				EntryHostname: line.EntryHostname, Listen: line.Listen, Port: line.Port, L4: line.L4, Chain: line.Chain,
				Exit: line.Exit, Rules: line.Rules, Targets: line.Targets, Policy: line.Policy, HopBalance: line.HopBalance,
				Rate: line.Rate, ServeAt: line.ServeAt, UpdatedAt: now,
			}
			if line.Protocol != nil {
				row.ProtocolID = &line.Protocol.ID
			}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}
		for _, t := range s.Transports {
			row := TransportRow{ID: t.ID, From: t.From, To: t.To, Kind: t.Kind, Carriers: t.Carriers, Multipath: t.Multipath, FEC: t.FEC, Declared: t.Declared, UpdatedAt: now}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}
		for _, device := range s.Devices {
			row := model.NodeNetwork{NodeRef: device.Ref, ScanCloud: device.Kernel, UpdatedAt: now}
			if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "node_ref"}}, DoUpdates: clause.AssignmentColumns([]string{"scan_cloud"})}).Create(&row).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// Load reads the stored network: the devices as Import reads them, the
// lines and transports from their tables.
func Load(db *gorm.DB) (*Snapshot, error) {
	in, _, _, err := readDevices(db)
	if err != nil {
		return nil, err
	}
	var transports []TransportRow
	if err := db.Order("id").Find(&transports).Error; err != nil {
		return nil, err
	}
	for _, t := range transports {
		in.s.Transports = append(in.s.Transports, Transport{ID: t.ID, From: t.From, To: t.To, Kind: t.Kind, Carriers: t.Carriers, Multipath: t.Multipath, FEC: t.FEC, Declared: t.Declared})
	}
	var lines []LineRow
	if err := db.Order("sort, id").Find(&lines).Error; err != nil {
		return nil, err
	}
	for _, row := range lines {
		line := Line{
			ID: row.ID, Name: row.Name, Enabled: row.Enabled, Show: row.Show, Entries: row.Entries, EntryHostname: row.EntryHostname,
			Listen: row.Listen, Port: row.Port, L4: row.L4, Chain: row.Chain, Exit: row.Exit, Rules: row.Rules,
			Targets: row.Targets, Policy: row.Policy, HopBalance: row.HopBalance, Rate: row.Rate, ServeAt: row.ServeAt,
		}
		if row.ProtocolID != nil {
			line.Protocol = in.protocol(*row.ProtocolID)
		}
		if !in.exits[line.Exit] && len(line.Chain) > 0 {
			in.exits[line.Exit] = true
			in.s.Exits = append(in.s.Exits, Exit{ID: line.Exit, Device: line.Chain[len(line.Chain)-1].Devices[0], Declared: true})
		}
		in.s.Lines = append(in.s.Lines, line)
	}
	return &in.s, nil
}

// protocol answers a node protocol as the protocol a line serves.
func (in *importer) protocol(id uint) *Protocol {
	for _, p := range in.protocols {
		if p.ID == id {
			return &Protocol{ID: p.ID, Type: string(p.Type), Device: ref(p.NodeID), Port: uint32(p.Port)} // #nosec G115 -- a port.
		}
	}
	return nil
}
