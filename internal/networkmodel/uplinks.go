package networkmodel

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	networkv1 "github.com/AnixOps/anix-control/sdk/api/network/v1"
	"github.com/AnixOps/anix-control/sdk/network"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// Uplinks (owner, 2026-10-31: the LQ-SH entry has two NICs, CN2 and CU
// 9929): a device may list the interfaces its traffic to the next device
// of a route can leave by. For each next device the automatic transport
// then gets one hop egress per uplink, auto-<from>-up-<uplink>-<to>
// ("-v6" for an IPv6 next device), leaving from the uplink's source
// address and probing the next device's hop ingress (the kernel's egress
// health check, anixcore node-runtime network.rs), and the transport
// leaves by one of them: the uplink the administrator chose for that next
// device, or in auto mode the one Control chose from the probes' round
// trips (evaluateUplinks). The Agent routes the transport's peer address
// through the chosen egress's line (anix-agent networkroute), so the link
// itself takes that NIC.
const (
	// UplinkAuto picks the uplink with the lowest latency.
	UplinkAuto = "auto"
	// MaxUplinks bounds a device's uplinks.
	MaxUplinks = 8

	uplinkInfix = "-up-"
	v6Suffix    = "-v6"

	// The probes of the uplink egresses.
	uplinkProbeIntervalMs = 10_000
	uplinkProbeTimeoutMs  = 3_000
)

// Auto mode's hysteresis: a better uplink takes over only when it is
// better by uplinkMarginMs and uplinkMarginPercent of the current round
// trip in uplinkStreak status reports in a row, and the current one has
// been active for uplinkHold; an uplink that is down is left at once.
const (
	uplinkMarginMs      = 10
	uplinkMarginPercent = 15
	uplinkStreak        = 3
	uplinkHold          = 3 * time.Minute
)

var uplinkNamePattern = regexp.MustCompile(`^[a-z0-9]{1,12}$`)

// Uplink is one interface a device's traffic may leave by, with its source
// address and gateway per family. Name is its id in component ids (lower
// case letters and digits); Label is what people see ("CN2").
type Uplink struct {
	Name      string `json:"name"`
	Label     string `json:"label,omitempty"`
	Interface string `json:"interface"`
	SourceV4  string `json:"source_v4,omitempty"`
	GatewayV4 string `json:"gateway_v4,omitempty"`
	SourceV6  string `json:"source_v6,omitempty"`
	GatewayV6 string `json:"gateway_v6,omitempty"`
}

func (u Uplink) source(family string) string {
	if family == "v6" {
		return u.SourceV6
	}
	return u.SourceV4
}

// uplinkConfig is model.NodeUplinks.ConfigJSON.
type uplinkConfig struct {
	Uplinks []Uplink          `json:"uplinks,omitempty"`
	Default string            `json:"default,omitempty"`
	Choices map[string]string `json:"choices,omitempty"`
}

// choice answers what the next device to gets: UplinkAuto or an uplink
// name.
func (c uplinkConfig) choice(to string) string {
	if choice := c.Choices[to]; choice != "" {
		return choice
	}
	if c.Default != "" {
		return c.Default
	}
	return UplinkAuto
}

func (c uplinkConfig) uplink(name string) (Uplink, bool) {
	for _, u := range c.Uplinks {
		if u.Name == name {
			return u, true
		}
	}
	return Uplink{}, false
}

// activeUplink is the uplink auto mode chose for one next device and
// family (model.NodeUplinks.ActiveJSON, by hopKey), with the better one it
// is watching.
type activeUplink struct {
	Uplink    string    `json:"uplink"`
	Since     time.Time `json:"since"`
	Reason    string    `json:"reason,omitempty"`
	Candidate string    `json:"candidate,omitempty"`
	Streak    int       `json:"streak,omitempty"`
}

func sameChoice(a, b activeUplink) bool {
	return a.Uplink == b.Uplink && a.Since.Equal(b.Since) && a.Reason == b.Reason && a.Candidate == b.Candidate && a.Streak == b.Streak
}

func hopKey(to, family string) string { return to + "/" + family }

// uplinkEgressID is the hop egress of from's uplink towards to.
func uplinkEgressID(from, uplink, to, family string) string {
	id := AutoPrefix + from + uplinkInfix + uplink + "-" + to
	if family == "v6" {
		id += v6Suffix
	}
	return id
}

// parseUplinkEgress answers the uplink, next device and family of one of
// node's uplink egress ids.
func parseUplinkEgress(node, id string) (uplink, to, family string, ok bool) {
	rest, found := strings.CutPrefix(id, AutoPrefix+node+uplinkInfix)
	if !found {
		return "", "", "", false
	}
	uplink, to, found = strings.Cut(rest, "-")
	if !found || !uplinkNamePattern.MatchString(uplink) {
		return "", "", "", false
	}
	family = "v4"
	if trimmed, cut := strings.CutSuffix(to, v6Suffix); cut {
		to, family = trimmed, "v6"
	}
	if ref, err := agentcontrol.ParseAgentNode(to); err != nil || ref.String() != to {
		return "", "", "", false
	}
	return uplink, to, family, true
}

// checkUplinks answers settings' uplinks trimmed, or adds violations.
func checkUplinks(v *violations, in NodeSettings) NodeSettings {
	if len(in.Uplinks) > MaxUplinks {
		v.add("uplinks", "at most %d uplinks", MaxUplinks)
		return in
	}
	names := map[string]bool{}
	for i := range in.Uplinks {
		u := &in.Uplinks[i]
		field := fmt.Sprintf("uplinks[%d]", i)
		u.Name = strings.ToLower(strings.TrimSpace(u.Name))
		u.Label = strings.TrimSpace(u.Label)
		u.Interface = strings.TrimSpace(u.Interface)
		if !uplinkNamePattern.MatchString(u.Name) {
			v.add(field+".name", "1 to 12 lower case letters and digits")
		} else if names[u.Name] {
			v.add(field+".name", "%q is listed twice", u.Name)
		}
		names[u.Name] = true
		if len(u.Label) > 32 {
			v.add(field+".label", "at most 32 bytes")
		}
		if !interfacePattern.MatchString(u.Interface) {
			v.add(field+".interface", "an interface name of 1-15 letters, digits, '.', '_', '@' or '-'")
		}
		checkAddress(v, field+".source_v4", &u.SourceV4, false)
		checkAddress(v, field+".gateway_v4", &u.GatewayV4, false)
		checkAddress(v, field+".source_v6", &u.SourceV6, true)
		checkAddress(v, field+".gateway_v6", &u.GatewayV6, true)
		if u.SourceV4 == "" && u.SourceV6 == "" {
			v.add(field, "an uplink needs the interface's source address of a family: its traffic leaves from it")
		}
		if u.GatewayV4 != "" && u.SourceV4 == "" {
			v.add(field+".source_v4", "a gateway needs the interface's source address")
		}
		if u.GatewayV6 != "" && u.SourceV6 == "" {
			v.add(field+".source_v6", "a gateway needs the interface's source address")
		}
	}
	valid := func(choice string) bool { return choice == UplinkAuto || names[choice] }
	in.UplinkDefault = strings.TrimSpace(in.UplinkDefault)
	if in.UplinkDefault != "" && !valid(in.UplinkDefault) {
		v.add("uplink_default", "auto or the name of an uplink")
	}
	choices := map[string]string{}
	for to, choice := range in.UplinkChoices {
		to, choice = strings.TrimSpace(to), strings.TrimSpace(choice)
		if ref, err := agentcontrol.ParseAgentNode(to); err != nil || ref.ID == 0 || ref.String() != to {
			v.add("uplink_choices", "%q is not proxy-<id> or forward-<id>", to)
			continue
		}
		if choice == "" {
			continue
		}
		if !valid(choice) {
			v.add("uplink_choices."+to, "auto or the name of an uplink")
			continue
		}
		choices[to] = choice
	}
	in.UplinkChoices = choices
	if len(in.Uplinks) == 0 {
		in.Uplinks = nil
	}
	if len(in.UplinkChoices) == 0 {
		in.UplinkChoices = nil
	}
	return in
}

func (s NodeSettings) uplinkConfig() uplinkConfig {
	return uplinkConfig{Uplinks: s.Uplinks, Default: s.UplinkDefault, Choices: s.UplinkChoices}
}

// loadUplinks answers node's uplink configuration and auto mode's
// choices; zero without a row.
func loadUplinks(db *gorm.DB, node string) (uplinkConfig, map[string]activeUplink, error) {
	if !db.Migrator().HasTable(&model.NodeUplinks{}) {
		return uplinkConfig{}, nil, nil
	}
	var rows []model.NodeUplinks
	if err := db.Where("node_ref = ?", node).Limit(1).Find(&rows).Error; err != nil {
		return uplinkConfig{}, nil, err
	}
	if len(rows) == 0 {
		return uplinkConfig{}, nil, nil
	}
	return decodeUplinks(rows[0])
}

func decodeUplinks(row model.NodeUplinks) (uplinkConfig, map[string]activeUplink, error) {
	var config uplinkConfig
	if row.ConfigJSON != "" {
		if err := json.Unmarshal([]byte(row.ConfigJSON), &config); err != nil {
			return uplinkConfig{}, nil, fmt.Errorf("network model: %s: stored uplinks are corrupt: %w", row.NodeRef, err)
		}
	}
	active := map[string]activeUplink{}
	if row.ActiveJSON != "" {
		if err := json.Unmarshal([]byte(row.ActiveJSON), &active); err != nil {
			return uplinkConfig{}, nil, fmt.Errorf("network model: %s: stored uplink choices are corrupt: %w", row.NodeRef, err)
		}
	}
	return config, active, nil
}

// saveUplinkConfig stores node's uplink configuration, keeping auto
// mode's choices of the uplinks that are still there.
func saveUplinkConfig(tx *gorm.DB, node string, config uplinkConfig, now time.Time) error {
	row := model.NodeUplinks{NodeRef: node}
	_, active, err := loadUplinks(tx, node)
	if err != nil {
		return err
	}
	for key, choice := range active {
		if _, ok := config.uplink(choice.Uplink); !ok {
			delete(active, key)
		}
	}
	encoded, err := json.Marshal(config)
	if err != nil {
		return err
	}
	row.ConfigJSON = string(encoded)
	if len(config.Uplinks) == 0 && config.Default == "" && len(config.Choices) == 0 {
		row.ConfigJSON = ""
	}
	if row.ActiveJSON, err = encodeActive(active); err != nil {
		return err
	}
	row.UpdatedAt = now
	return tx.Save(&row).Error
}

func encodeActive(active map[string]activeUplink) (string, error) {
	if len(active) == 0 {
		return "", nil
	}
	encoded, err := json.Marshal(active)
	return string(encoded), err
}

// uplinkEgresses adds from's uplink egresses towards to (family, the next
// device's hop ingress dialed at dial) and answers the one the transport
// leaves by; "" when from has no uplink of family.
func (a *autoCompleter) uplinkEgresses(from, to, family, dial string) string {
	d := a.devices[from]
	var candidates []Uplink
	for _, u := range d.Uplinks.Uplinks {
		if u.source(family) != "" {
			candidates = append(candidates, u)
		}
	}
	if len(candidates) == 0 {
		return ""
	}
	for _, u := range candidates {
		id := uplinkEgressID(from, u.Name, to, family)
		if !a.add("e:" + id) {
			continue
		}
		a.doc.Egresses = append(a.doc.Egresses, Egress{ID: id, Line: a.uplinkLine(from, u), Kind: network.EgressHop,
			Audience: "hop", Family: family, Source: u.source(family),
			Health: &HealthCheck{Targets: []string{dial}, IntervalMs: uplinkProbeIntervalMs, TimeoutMs: uplinkProbeTimeoutMs}})
	}
	chosen := candidates[0].Name
	choice := d.Uplinks.choice(to)
	if u, ok := d.Uplinks.uplink(choice); ok && u.source(family) != "" {
		chosen = u.Name
	} else if active, ok := d.Active[hopKey(to, family)]; ok {
		if u, ok := d.Uplinks.uplink(active.Uplink); ok && u.source(family) != "" {
			chosen = u.Name
		}
	}
	return uplinkEgressID(from, chosen, to, family)
}

// uplinkLine answers the line auto-<node>-up-<uplink>, adding it.
func (a *autoCompleter) uplinkLine(node string, u Uplink) string {
	id := AutoPrefix + node + uplinkInfix + u.Name
	if a.add("l:" + id) {
		line := Line{ID: id, Node: node, Name: u.Label, Interface: u.Interface, GatewayV4: u.GatewayV4, GatewayV6: u.GatewayV6}
		if line.Name == "" {
			line.Name = u.Name
		}
		if u.SourceV4 != "" {
			line.Families = append(line.Families, "v4")
		}
		if u.SourceV6 != "" {
			line.Families = append(line.Families, "v6")
		}
		a.doc.Lines = append(a.doc.Lines, line)
		a.nodeOf[id] = node
	}
	return id
}

// UplinkProbe is one uplink's view of a next device: the round trip of
// its latest probe (0 before one succeeded) and whether it is up.
type UplinkProbe struct {
	Uplink    string `json:"uplink"`
	Egress    string `json:"egress"`
	State     string `json:"state"` // up, down, unknown
	RTTMs     uint32 `json:"rtt_ms"`
	Detail    string `json:"detail,omitempty"`
	ChangedAt int64  `json:"changed_at_unix_ms,omitempty"`
}

// UplinkHop is a next device of the device's routes as its uplinks see it.
type UplinkHop struct {
	To     string        `json:"to"`
	ToName string        `json:"to_name,omitempty"`
	Family string        `json:"family"`
	Target string        `json:"target"`
	Choice string        `json:"choice"` // auto or an uplink name
	Active string        `json:"active"` // the uplink the transport leaves by
	Since  *time.Time    `json:"since,omitempty"`
	Reason string        `json:"reason,omitempty"`
	Probes []UplinkProbe `json:"probes"`
}

// uplinkHops answers node's next devices from its plan, with the probes of
// status (nil before one).
func uplinkHops(node string, plan *networkv1.NetworkPlan, status *networkv1.NetworkStatus, config uplinkConfig, active map[string]activeUplink) []UplinkHop {
	components := map[string]*networkv1.ComponentStatus{}
	for _, c := range status.GetComponents() {
		if c.GetComponent() == networkv1.Component_COMPONENT_EGRESS {
			components[c.GetId()] = c
		}
	}
	using := map[string]bool{}
	for _, t := range plan.GetTransports() {
		if t.GetRole() == networkv1.TransportRole_TRANSPORT_ROLE_DIAL {
			using[t.GetFromEgressId()] = true
		}
	}
	hops := map[string]*UplinkHop{}
	var keys []string
	for _, egress := range plan.GetEgresses() {
		uplink, to, family, ok := parseUplinkEgress(node, egress.GetId())
		if !ok {
			continue
		}
		key := hopKey(to, family)
		hop := hops[key]
		if hop == nil {
			hop = &UplinkHop{To: to, Family: family, Choice: config.choice(to), Probes: []UplinkProbe{}}
			if targets := egress.GetHealth().GetTargets(); len(targets) > 0 {
				hop.Target = targets[0]
			}
			if choice, ok := active[key]; ok && hop.Choice == UplinkAuto {
				since := choice.Since
				hop.Since, hop.Reason = &since, choice.Reason
			}
			hops[key] = hop
			keys = append(keys, key)
		}
		if using[egress.GetId()] {
			hop.Active = uplink
		}
		probe := UplinkProbe{Uplink: uplink, Egress: egress.GetId(), State: "unknown"}
		if c := components[egress.GetId()]; c != nil {
			probe.RTTMs, probe.Detail, probe.ChangedAt = c.GetRttMs(), c.GetDetail(), c.GetChangedAtUnixMs()
			switch c.GetState() {
			case networkv1.ComponentState_COMPONENT_STATE_DOWN:
				probe.State = "down"
			case networkv1.ComponentState_COMPONENT_STATE_APPLIED, networkv1.ComponentState_COMPONENT_STATE_DEGRADED:
				if probe.RTTMs > 0 {
					probe.State = "up"
				}
			}
		}
		hop.Probes = append(hop.Probes, probe)
	}
	sort.Strings(keys)
	out := make([]UplinkHop, 0, len(keys))
	for _, key := range keys {
		out = append(out, *hops[key])
	}
	return out
}

// nextChoice is auto mode's decision for one next device: the uplink it
// should leave by now, given the current one and the probes.
func nextChoice(current activeUplink, probes []UplinkProbe, now time.Time) (activeUplink, bool) {
	var best *UplinkProbe
	var cur *UplinkProbe
	for i := range probes {
		p := &probes[i]
		if p.Uplink == current.Uplink {
			cur = p
		}
		if p.State == "up" && (best == nil || p.RTTMs < best.RTTMs) {
			best = p
		}
	}
	if best == nil {
		current.Candidate, current.Streak = "", 0
		return current, false
	}
	take := func(reason string) (activeUplink, bool) {
		return activeUplink{Uplink: best.Uplink, Since: now, Reason: reason}, true
	}
	switch {
	case current.Uplink == "" || cur == nil:
		return take("lowest latency")
	case cur.State == "down":
		return take(fmt.Sprintf("%s is down", current.Uplink))
	case cur.State != "up":
		// The current one is not probed yet: start on the fastest.
		return take("lowest latency")
	case best.Uplink == current.Uplink:
		current.Candidate, current.Streak = "", 0
		return current, false
	}
	margin := max(uint32(uplinkMarginMs), cur.RTTMs*uplinkMarginPercent/100)
	if best.RTTMs+margin > cur.RTTMs {
		current.Candidate, current.Streak = "", 0
		return current, false
	}
	if current.Candidate != best.Uplink {
		current.Candidate, current.Streak = best.Uplink, 0
	}
	current.Streak++
	if current.Streak >= uplinkStreak && now.Sub(current.Since) >= uplinkHold {
		return take(fmt.Sprintf("lower latency: %d ms against %d ms", best.RTTMs, cur.RTTMs))
	}
	return current, false
}

// evaluateUplinks runs auto mode on node's latest status: each next device
// in auto mode gets the uplink nextChoice picks, and a change replans.
func (s *Service) evaluateUplinks(ctx context.Context, node string, status *networkv1.NetworkStatus) error {
	db := s.DB.WithContext(ctx)
	config, _, err := loadUplinks(db, node)
	if err != nil || len(config.Uplinks) < 2 {
		return err
	}
	var changed []string
	err = db.Transaction(func(tx *gorm.DB) error {
		_, doc, err := lockModel(tx, s.now())
		if err != nil {
			return err
		}
		config, active, err := loadUplinks(tx, node)
		if err != nil {
			return err
		}
		plan, found, err := s.devicePlan(tx, node)
		if err != nil || !found {
			return err
		}
		now := s.now().UTC()
		moved, touched := false, false
		next := map[string]activeUplink{}
		for _, hop := range uplinkHops(node, plan, status, config, active) {
			if hop.Choice != UplinkAuto {
				continue
			}
			key := hopKey(hop.To, hop.Family)
			// Without a choice yet the fastest uplink is taken at once.
			current := active[key]
			choice, switched := nextChoice(current, hop.Probes, now)
			if switched {
				moved = true
				slog.Info("network model: uplink switched", "component", "network-model", "node", node, "to", hop.To,
					"family", hop.Family, "from", current.Uplink, "uplink", choice.Uplink, "reason", choice.Reason)
			}
			if !sameChoice(choice, active[key]) {
				touched = true
			}
			next[key] = choice
		}
		if len(next) != len(active) {
			touched = true
		}
		if !touched {
			return nil
		}
		encoded, err := encodeActive(next)
		if err != nil {
			return err
		}
		if err := tx.Model(&model.NodeUplinks{}).Where("node_ref = ?", node).
			Updates(map[string]any{"active_json": encoded}).Error; err != nil {
			return err
		}
		if moved {
			changed, err = s.replan(tx, doc)
		}
		return err
	})
	if err != nil {
		return err
	}
	notify(changed)
	return nil
}

// devicePlan answers node's current plan on db, unstamped.
func (s *Service) devicePlan(db *gorm.DB, node string) (*networkv1.NetworkPlan, bool, error) {
	devices, _, err := loadDevices(db)
	if err != nil {
		return nil, false, err
	}
	keys, err := s.openKeys(db)
	if err != nil {
		return nil, false, err
	}
	_, doc, err := loadDocument(db)
	if err != nil {
		return nil, false, err
	}
	full, forwards, err := s.completed(db, doc)
	if err != nil {
		return nil, false, err
	}
	result, err := Plan(full, keys, devices, forwards)
	if err != nil {
		return nil, false, err
	}
	plan := result.Plans[node]
	return plan, plan != nil, nil
}

// NodeUplinkHops answers node's next devices as its uplinks see them.
func (s *Service) NodeUplinkHops(ctx context.Context, node string) ([]UplinkHop, error) {
	db := s.DB.WithContext(ctx)
	config, active, err := loadUplinks(db, node)
	if err != nil {
		return nil, err
	}
	if len(config.Uplinks) == 0 {
		return []UplinkHop{}, nil
	}
	plan, _, err := s.devicePlan(db, node)
	if err != nil {
		return nil, err
	}
	status, _, err := s.latestStatus(ctx, node)
	if err != nil {
		return nil, err
	}
	hops := uplinkHops(node, plan, status, config, active)
	if len(hops) > 0 {
		names := map[string]string{}
		nodes, err := agentNodes(db)
		if err != nil {
			return nil, err
		}
		for _, n := range nodes {
			names[n.Node] = n.Name
		}
		for i := range hops {
			hops[i].ToName = names[hops[i].To]
		}
	}
	return hops, nil
}

// dialTarget is host:port.
func dialTarget(host string, port uint32) string {
	return net.JoinHostPort(strings.Trim(host, "[]"), strconv.FormatUint(uint64(port), 10))
}
