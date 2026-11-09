// Package l2standby is the cross-border emergency channel (anx
// docs/FUTURE-IMPROVEMENTS.md F-54 items 1 and 4): for every cross-border
// transport of the network model (kind anx, the A2 jacket; not trusted)
// Control keeps public-protocol standby channels ready, one per pair of
// proxy nodes the transport's paths join (the user ingress node and the
// node the path leaves from), in two tiers:
//
//   - reality: VLESS + REALITY + Vision to a relay port of the exit node.
//     Quickest, but the same borrowed-handshake family as A2: it covers a
//     fault of the A2 implementation, not a rule against the family.
//   - xhttp: VLESS over XHTTP to our own domain on the exit node, through
//     a TLS front (nginx with a real certificate) that proxies a path to the
//     relay on loopback. A different family from A2.
//
// Nothing runs while a channel is on standby. Switching a transport to a
// tier makes that tier's channels active: the source node gets a catch-all
// VLESS egress route and the exit node the relay, in the member "egress"
// of their desired configurations (internal/egressroute standby.go), which
// Control sends within a minute. Switching back ("off") removes them, and
// the traffic takes the transport again. Every switch is audited; the
// drill and the manual plan are in docs/guide/cross-border-standby.md.
//
// Limits: the route matches domains (the requested name, the SNI / Host,
// or a name a relayed DNS answer gave the address), so traffic to a bare
// address the node cannot name still takes the transport.
package l2standby

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/networkmodel"
	"gorm.io/gorm"
)

var (
	// ErrInvalid: an input is invalid; the error says which.
	ErrInvalid = errors.New("invalid standby request")
	// ErrNotReady: a transport has no usable channel of the tier asked for.
	ErrNotReady = errors.New("standby channel not ready")
	// ErrNoPort: the exit node has no free relay port left in the range.
	ErrNoPort = errors.New("no free relay port")
)

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

// Relay ports are taken from this range on the exit node.
const (
	PortFirst = 31000
	PortLast  = 31999
)

// TierOff switches a transport back to itself.
const TierOff = "off"

// Audit entries of the operation log (module l2_standby); never a key.
const (
	auditModule         = "l2_standby"
	auditTarget         = "l2_standby"
	AuditActionPrepare  = "l2_standby_prepare"
	AuditActionSwitch   = "l2_standby_switch"
	defaultRealityDest  = ":443"
	maxServerNameLength = 253
)

// Actor is who made an administrator's change, for the audit entry.
type Actor struct {
	UserID uint
	Name   string
	IP     string
}

// Service manages the standby channels on one database.
type Service struct {
	DB *gorm.DB
	// Now defaults to time.Now.
	Now func() time.Time
	// Document answers the network model; nil reads it with networkmodel.
	Document func(ctx context.Context) (*networkmodel.Document, error)
}

// New returns the service on db.
func New(db *gorm.DB) *Service { return &Service{DB: db} }

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s *Service) document(ctx context.Context) (*networkmodel.Document, error) {
	if s.Document != nil {
		return s.Document(ctx)
	}
	return networkmodel.LoadDocument(s.DB.WithContext(ctx))
}

// Pair is a source and exit proxy node a cross-border transport joins.
type Pair struct {
	SourceNodeID uint `json:"source_node_id"`
	ExitNodeID   uint `json:"exit_node_id"`
}

// Link is a cross-border transport and the node pairs its standby channels
// serve.
type Link struct {
	TransportID string `json:"transport_id"`
	From        string `json:"from"`
	To          string `json:"to"`
	Pairs       []Pair `json:"pairs"`
	// Skipped says why a path of the transport has no standby channel
	// (an end that is not a proxy node).
	Skipped []string `json:"skipped,omitempty"`
}

// Links answers the cross-border transports of doc: kind anx and not
// trusted. A transport's pairs are its enabled paths' (ingress node, egress
// node), or its own two ends when no path takes it.
func Links(doc *networkmodel.Document) []Link {
	lineNode := map[string]string{}
	for _, line := range doc.Lines {
		lineNode[line.ID] = line.Node
	}
	ingressNode := map[string]string{}
	for _, ingress := range doc.Ingresses {
		ingressNode[ingress.ID] = lineNode[ingress.Line]
	}
	egressNode := map[string]string{}
	for _, egress := range doc.Egresses {
		egressNode[egress.ID] = lineNode[egress.Line]
	}
	var links []Link
	for _, transport := range doc.Transports {
		if transport.Kind != networkmodel.KindAnx || transport.Trusted {
			continue
		}
		link := Link{TransportID: transport.ID, From: egressNode[transport.FromEgress], To: ingressNode[transport.ToIngress]}
		seen := map[Pair]bool{}
		add := func(source, exit, what string) {
			s, okS := proxyID(source)
			e, okE := proxyID(exit)
			if !okS || !okE || s == e {
				link.Skipped = append(link.Skipped, fmt.Sprintf("%s: %s → %s is not a pair of two proxy nodes", what, source, exit))
				return
			}
			pair := Pair{SourceNodeID: s, ExitNodeID: e}
			if !seen[pair] {
				seen[pair] = true
				link.Pairs = append(link.Pairs, pair)
			}
		}
		for _, path := range doc.Paths {
			if path.Disabled || !contains(path.Segments, transport.ID) {
				continue
			}
			add(ingressNode[path.Ingress], egressNode[path.Egress], "path "+path.ID)
		}
		if len(link.Pairs) == 0 && len(link.Skipped) == 0 {
			add(link.From, link.To, "transport "+transport.ID)
		}
		links = append(links, link)
	}
	sort.Slice(links, func(i, j int) bool { return links[i].TransportID < links[j].TransportID })
	return links
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

func proxyID(ref string) (uint, bool) {
	node, err := agentcontrol.ParseAgentNode(ref)
	if err != nil || node.Kind != agentcontrol.NodeKindProxy {
		return 0, false
	}
	return uint(node.ID), true
}

// ChannelView is a standby channel as the API answers it: never the
// relay's uuid or the REALITY private key.
type ChannelView struct {
	ID                uint       `json:"id"`
	Tier              string     `json:"tier"`
	SourceNodeID      uint       `json:"source_node_id"`
	ExitNodeID        uint       `json:"exit_node_id"`
	ExitHost          string     `json:"exit_host"`
	RelayPort         int        `json:"relay_port"`
	RealityServerName string     `json:"reality_server_name,omitempty"`
	RealityDest       string     `json:"reality_dest,omitempty"`
	RealityPublicKey  string     `json:"reality_public_key,omitempty"`
	XHTTPDomain       string     `json:"xhttp_domain,omitempty"`
	XHTTPPath         string     `json:"xhttp_path,omitempty"`
	Ready             bool       `json:"ready"`
	Active            bool       `json:"active"`
	ActivatedAt       *time.Time `json:"activated_at"`
}

// LinkView is a cross-border transport with its channels.
type LinkView struct {
	Link
	// ActiveTier is the tier its traffic takes now, or "off".
	ActiveTier string `json:"active_tier"`
	// Ready lists the tiers every pair has a usable channel of.
	Ready    []string      `json:"ready"`
	Channels []ChannelView `json:"channels"`
}

// Overview is every cross-border transport, and channels left from
// transports that are gone.
type Overview struct {
	Links []LinkView    `json:"links"`
	Stale []ChannelView `json:"stale"`
}

func ready(row model.L2Standby) bool {
	switch row.Tier {
	case model.L2StandbyTierReality:
		return row.RealityServerName != "" && row.RealityDest != "" && row.RealityPrivateKey != ""
	case model.L2StandbyTierXHTTP:
		return row.XHTTPDomain != "" && row.XHTTPPath != ""
	}
	return false
}

func viewOf(row model.L2Standby) ChannelView {
	return ChannelView{ID: row.ID, Tier: row.Tier, SourceNodeID: row.SourceNodeID, ExitNodeID: row.ExitNodeID,
		ExitHost: row.ExitHost, RelayPort: row.RelayPort, RealityServerName: row.RealityServerName,
		RealityDest: row.RealityDest, RealityPublicKey: row.RealityPublicKey, XHTTPDomain: row.XHTTPDomain,
		XHTTPPath: row.XHTTPPath, Ready: ready(row), Active: row.Active, ActivatedAt: row.ActivatedAt}
}

// Overview answers the cross-border transports and their channels.
func (s *Service) Overview(ctx context.Context) (*Overview, error) {
	doc, err := s.document(ctx)
	if err != nil {
		return nil, err
	}
	var rows []model.L2Standby
	if err := s.DB.WithContext(ctx).Order("transport_id ASC, tier ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return overviewOf(Links(doc), rows), nil
}

func overviewOf(links []Link, rows []model.L2Standby) *Overview {
	byTransport := map[string][]model.L2Standby{}
	for _, row := range rows {
		byTransport[row.TransportID] = append(byTransport[row.TransportID], row)
	}
	out := &Overview{Links: []LinkView{}, Stale: []ChannelView{}}
	known := map[string]map[Pair]bool{}
	for _, link := range links {
		pairs := map[Pair]bool{}
		for _, pair := range link.Pairs {
			pairs[pair] = true
		}
		known[link.TransportID] = pairs
		view := LinkView{Link: link, ActiveTier: TierOff, Ready: []string{}, Channels: []ChannelView{}}
		for _, tier := range []string{model.L2StandbyTierReality, model.L2StandbyTierXHTTP} {
			if tierReady(link, byTransport[link.TransportID], tier) {
				view.Ready = append(view.Ready, tier)
			}
		}
		for _, row := range byTransport[link.TransportID] {
			if !pairs[Pair{row.SourceNodeID, row.ExitNodeID}] {
				continue
			}
			if row.Active {
				view.ActiveTier = row.Tier
			}
			view.Channels = append(view.Channels, viewOf(row))
		}
		out.Links = append(out.Links, view)
	}
	for _, row := range rows {
		if !known[row.TransportID][Pair{row.SourceNodeID, row.ExitNodeID}] {
			out.Stale = append(out.Stale, viewOf(row))
		}
	}
	return out
}

// tierReady reports whether every pair of link has a ready channel of tier.
func tierReady(link Link, rows []model.L2Standby, tier string) bool {
	if len(link.Pairs) == 0 {
		return false
	}
	for _, pair := range link.Pairs {
		found := false
		for _, row := range rows {
			if row.Tier == tier && row.SourceNodeID == pair.SourceNodeID && row.ExitNodeID == pair.ExitNodeID && ready(row) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// PrepareInput is what an administrator gives for the channels. Fields left
// empty keep what prepared channels have.
type PrepareInput struct {
	// Transports limits the preparation; empty prepares every cross-border
	// transport.
	Transports []string `json:"transports"`
	// RealityServerName is the borrowed site of the reality tier (SNI) and
	// RealityDest its address (server_name:443 when empty).
	RealityServerName string `json:"reality_server_name"`
	RealityDest       string `json:"reality_dest"`
	// XHTTPDomains is our own domain on each exit node's TLS front, by
	// exit node id ("12" or "proxy-12"). An exit node without one gets no
	// xhttp channel yet.
	XHTTPDomains map[string]string `json:"xhttp_domains"`
	// ExitHosts overrides the address a source node dials for the reality
	// tier, by exit node id; the node's host when absent.
	ExitHosts map[string]string `json:"exit_hosts"`
}

var hostPattern = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?)*$`)

func checkHost(field, host string) error {
	if len(host) > maxServerNameLength || !hostPattern.MatchString(host) {
		return invalid("%s: %q is not a host name", field, host)
	}
	return nil
}

func nodeKeyed(field string, values map[string]string) (map[uint]string, error) {
	out := map[uint]string{}
	for key, value := range values {
		id, err := strconv.ParseUint(strings.TrimPrefix(key, agentcontrol.NodeKindProxy+"-"), 10, 32)
		if err != nil || id == 0 {
			return nil, invalid("%s: %q is not a proxy node id", field, key)
		}
		value = strings.ToLower(strings.TrimSpace(value))
		if err := checkHost(field, value); err != nil {
			return nil, err
		}
		out[uint(id)] = value
	}
	return out, nil
}

// PrepareResult is what Prepare did.
type PrepareResult struct {
	Created  int       `json:"created"`
	Updated  int       `json:"updated"`
	Removed  int       `json:"removed"`
	Overview *Overview `json:"overview"`
}

// Prepare makes sure every pair of the cross-border transports has a
// channel of each tier it can have: reality once a borrowed site is known,
// xhttp once the exit node's domain is. New channels get a fresh uuid,
// REALITY key pair, short id, path and relay port; existing ones keep them
// and take the new site or domain. Inactive channels of pairs that are gone
// are removed. Audited.
func (s *Service) Prepare(ctx context.Context, input PrepareInput, actor Actor) (*PrepareResult, error) {
	serverName := strings.ToLower(strings.TrimSpace(input.RealityServerName))
	dest := strings.TrimSpace(input.RealityDest)
	if serverName != "" {
		if err := checkHost("reality_server_name", serverName); err != nil {
			return nil, err
		}
		if dest == "" {
			dest = serverName + defaultRealityDest
		}
	}
	if dest != "" {
		host, port, ok := strings.Cut(dest, ":")
		if serverName == "" {
			return nil, invalid("reality_dest needs reality_server_name")
		}
		if n, err := strconv.Atoi(port); !ok || err != nil || n < 1 || n > 65535 || checkHost("reality_dest", host) != nil {
			return nil, invalid("reality_dest: %q is not host:port", dest)
		}
	}
	domains, err := nodeKeyed("xhttp_domains", input.XHTTPDomains)
	if err != nil {
		return nil, err
	}
	exitHosts, err := nodeKeyed("exit_hosts", input.ExitHosts)
	if err != nil {
		return nil, err
	}
	doc, err := s.document(ctx)
	if err != nil {
		return nil, err
	}
	links := Links(doc)
	if len(input.Transports) > 0 {
		wanted := map[string]bool{}
		for _, id := range input.Transports {
			wanted[id] = true
		}
		var picked []Link
		for _, link := range links {
			if wanted[link.TransportID] {
				picked = append(picked, link)
				delete(wanted, link.TransportID)
			}
		}
		for id := range wanted {
			return nil, invalid("transports: %q is not a cross-border transport (kind anx, not trusted)", id)
		}
		links = picked
	}
	result := &PrepareResult{}
	now := s.now()
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, link := range links {
			var rows []model.L2Standby
			if err := tx.Where("transport_id = ?", link.TransportID).Find(&rows).Error; err != nil {
				return err
			}
			for _, pair := range link.Pairs {
				for _, tier := range []string{model.L2StandbyTierReality, model.L2StandbyTierXHTTP} {
					var row *model.L2Standby
					for i := range rows {
						if rows[i].Tier == tier && rows[i].SourceNodeID == pair.SourceNodeID && rows[i].ExitNodeID == pair.ExitNodeID {
							row = &rows[i]
						}
					}
					created := row == nil
					if created {
						if tier == model.L2StandbyTierXHTTP && domains[pair.ExitNodeID] == "" {
							continue
						}
						if tier == model.L2StandbyTierReality && serverName == "" {
							continue
						}
						fresh, err := newChannel(tx, link.TransportID, tier, pair)
						if err != nil {
							return err
						}
						row = fresh
					}
					changed := created
					if tier == model.L2StandbyTierReality && serverName != "" &&
						(row.RealityServerName != serverName || row.RealityDest != dest) {
						row.RealityServerName, row.RealityDest, changed = serverName, dest, true
					}
					if host, ok := exitHosts[pair.ExitNodeID]; ok && tier == model.L2StandbyTierReality && row.ExitHost != host {
						row.ExitHost, changed = host, true
					}
					if domain := domains[pair.ExitNodeID]; tier == model.L2StandbyTierXHTTP && domain != "" && row.XHTTPDomain != domain {
						row.XHTTPDomain, changed = domain, true
					}
					if !changed {
						continue
					}
					row.UpdatedAt = now
					if created {
						row.CreatedAt = now
						if err := tx.Create(row).Error; err != nil {
							return err
						}
						result.Created++
					} else {
						if err := tx.Save(row).Error; err != nil {
							return err
						}
						result.Updated++
					}
				}
			}
		}
		// Inactive channels of pairs that are gone.
		if len(input.Transports) == 0 {
			var all []model.L2Standby
			if err := tx.Where("active = ?", false).Find(&all).Error; err != nil {
				return err
			}
			known := map[string]map[Pair]bool{}
			for _, link := range links {
				known[link.TransportID] = map[Pair]bool{}
				for _, pair := range link.Pairs {
					known[link.TransportID][pair] = true
				}
			}
			for _, row := range all {
				if !known[row.TransportID][Pair{row.SourceNodeID, row.ExitNodeID}] {
					if err := tx.Delete(&model.L2Standby{}, row.ID).Error; err != nil {
						return err
					}
					result.Removed++
				}
			}
		}
		return writeAudit(tx, actor, AuditActionPrepare, map[string]any{
			"transports": len(links), "created": result.Created, "updated": result.Updated, "removed": result.Removed,
			"reality_server_name": serverName, "xhttp_domains": len(domains),
		})
	})
	if err != nil {
		return nil, err
	}
	if result.Overview, err = s.Overview(ctx); err != nil {
		return nil, err
	}
	return result, nil
}

// newChannel makes a channel with fresh secrets and a free relay port.
func newChannel(tx *gorm.DB, transportID, tier string, pair Pair) (*model.L2Standby, error) {
	port, err := freePort(tx, pair.ExitNodeID)
	if err != nil {
		return nil, err
	}
	row := &model.L2Standby{TransportID: transportID, Tier: tier, SourceNodeID: pair.SourceNodeID,
		ExitNodeID: pair.ExitNodeID, RelayPort: port, UUID: newUUID()}
	switch tier {
	case model.L2StandbyTierReality:
		key, err := ecdh.X25519().GenerateKey(rand.Reader)
		if err != nil {
			return nil, err
		}
		row.RealityPrivateKey = base64.RawURLEncoding.EncodeToString(key.Bytes())
		row.RealityPublicKey = base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes())
		row.RealityShortID = randomHex(8)
	case model.L2StandbyTierXHTTP:
		row.XHTTPPath = "/" + randomHex(6) + "/"
	}
	return row, nil
}

// freePort is the first port of the range no relay, standby channel or
// protocol of the exit node uses.
func freePort(tx *gorm.DB, exitNodeID uint) (int, error) {
	used := map[int]bool{}
	var ports []int
	if err := tx.Model(&model.L2Standby{}).Where("exit_node_id = ?", exitNodeID).Pluck("relay_port", &ports).Error; err != nil {
		return 0, err
	}
	for _, p := range ports {
		used[p] = true
	}
	ports = nil
	if tx.Migrator().HasTable(&model.EgressRoute{}) {
		if err := tx.Model(&model.EgressRoute{}).Where("exit_node_id = ?", exitNodeID).Pluck("relay_port", &ports).Error; err != nil {
			return 0, err
		}
	}
	for _, p := range ports {
		used[p] = true
	}
	ports = nil
	if tx.Migrator().HasTable(&model.NodeProtocol{}) {
		if err := tx.Model(&model.NodeProtocol{}).Where("node_id = ?", exitNodeID).Pluck("port", &ports).Error; err != nil {
			return 0, err
		}
	}
	for _, p := range ports {
		used[p] = true
	}
	for port := PortFirst; port <= PortLast; port++ {
		if !used[port] {
			return port, nil
		}
	}
	return 0, fmt.Errorf("%w: node %d, %d-%d", ErrNoPort, exitNodeID, PortFirst, PortLast)
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand never fails on supported platforms
	}
	return hex.EncodeToString(b)
}

func newUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	h := hex.EncodeToString(b)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

// SwitchInput moves transports to a tier.
type SwitchInput struct {
	// Transports to switch; All switches every cross-border transport
	// (A2 targeted as a whole).
	Transports []string `json:"transports"`
	All        bool     `json:"all"`
	// Tier is reality, xhttp, or off (back to the transport).
	Tier string `json:"tier"`
}

// SwitchResult lists the transports switched.
type SwitchResult struct {
	Tier       string    `json:"tier"`
	Transports []string  `json:"transports"`
	Overview   *Overview `json:"overview"`
}

// Switch moves the transports' traffic to their tier channels, or back
// ("off"). All or nothing: when a transport has no ready channel of the
// tier for every pair, nothing changes. Audited.
func (s *Service) Switch(ctx context.Context, input SwitchInput, actor Actor) (*SwitchResult, error) {
	tier := strings.TrimSpace(input.Tier)
	if tier != model.L2StandbyTierReality && tier != model.L2StandbyTierXHTTP && tier != TierOff {
		return nil, invalid("tier must be reality, xhttp or off")
	}
	if input.All == (len(input.Transports) > 0) {
		return nil, invalid("give transports or all, not both or neither")
	}
	doc, err := s.document(ctx)
	if err != nil {
		return nil, err
	}
	links := Links(doc)
	byID := map[string]Link{}
	for _, link := range links {
		byID[link.TransportID] = link
	}
	var picked []Link
	if input.All {
		picked = links
	} else {
		for _, id := range input.Transports {
			link, ok := byID[id]
			if !ok {
				return nil, invalid("transports: %q is not a cross-border transport (kind anx, not trusted)", id)
			}
			picked = append(picked, link)
		}
	}
	if len(picked) == 0 {
		return nil, invalid("the network model has no cross-border transport")
	}
	now := s.now()
	result := &SwitchResult{Tier: tier, Transports: []string{}}
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var notReady []string
		for _, link := range picked {
			var rows []model.L2Standby
			if err := tx.Where("transport_id = ?", link.TransportID).Find(&rows).Error; err != nil {
				return err
			}
			if tier != TierOff && !tierReady(link, rows, tier) {
				notReady = append(notReady, link.TransportID)
				continue
			}
			pairs := map[Pair]bool{}
			for _, pair := range link.Pairs {
				pairs[pair] = true
			}
			for _, row := range rows {
				want := tier != TierOff && row.Tier == tier && pairs[Pair{row.SourceNodeID, row.ExitNodeID}] && ready(row)
				if row.Active == want {
					continue
				}
				updates := map[string]any{"active": want, "updated_at": now}
				if want {
					updates["activated_at"] = now
				}
				if err := tx.Model(&model.L2Standby{}).Where("id = ?", row.ID).Updates(updates).Error; err != nil {
					return err
				}
			}
			result.Transports = append(result.Transports, link.TransportID)
		}
		if len(notReady) > 0 {
			return fmt.Errorf("%w: %s has no %s channel for every pair (prepare it first)", ErrNotReady, strings.Join(notReady, ", "), tier)
		}
		return writeAudit(tx, actor, AuditActionSwitch, map[string]any{"tier": tier, "transports": result.Transports, "all": input.All})
	})
	if err != nil {
		return nil, err
	}
	if result.Overview, err = s.Overview(ctx); err != nil {
		return nil, err
	}
	return result, nil
}

// ActiveTiers answers the tier each transport with an active channel is
// switched to (for the alerts).
func ActiveTiers(db *gorm.DB) (map[string]string, error) {
	out := map[string]string{}
	if !db.Migrator().HasTable(&model.L2Standby{}) {
		return out, nil
	}
	var rows []model.L2Standby
	if err := db.Where("active = ?", true).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.TransportID] = row.Tier
	}
	return out, nil
}

func writeAudit(tx *gorm.DB, actor Actor, action string, content map[string]any) error {
	encoded, err := json.Marshal(content)
	if err != nil {
		return err
	}
	var userID *uint
	if actor.UserID != 0 {
		id := actor.UserID
		userID = &id
	}
	return tx.Create(&model.OperationLog{
		UserID: userID, Username: truncate(actor.Name, 100), Action: action, Module: auditModule,
		TargetType: auditTarget, Content: string(encoded), IP: truncate(actor.IP, 45), Status: 1,
	}).Error
}

func truncate(value string, limit int) string {
	if len(value) > limit {
		return value[:limit]
	}
	return value
}
