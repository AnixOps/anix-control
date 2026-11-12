// Package netline is the network as lines (docs/design/network-lines):
// devices, the entries subscribers connect to, the transports between
// devices, the exits traffic leaves by, and lines, each one a way from
// entries over transports to an exit with the protocol it serves or the
// targets it forwards to. A line is the unit of subscription, rate, traffic
// and health.
//
// This is step 1 of the move to that model: Import reads what Control
// stores today (nodes and their protocols, node entries, forward routes and
// their subscriptions, egress routes, exit policies, node network settings)
// as a Snapshot, Forwards plans a snapshot's lines as the network model's
// forwards, and Shadow compares those with the forwards Control plans
// today. Nothing here is stored or reaches a node yet.
package netline

import (
	networkv1 "github.com/AnixOps/anix-control/sdk/api/network/v1"
	"github.com/AnixOps/anix-control/v5/internal/networkmodel"
	"github.com/AnixOps/anix-control/v5/internal/nodeentry"
	"google.golang.org/protobuf/proto"
)

// Device is a machine with an Agent, by node reference (proxy-<id>).
type Device struct {
	Ref  string `json:"ref"`
	Name string `json:"name"`
	Host string `json:"host"`
	// Region is where the device is (its network settings), a place key.
	Region string `json:"region,omitempty"`
	// Kernel devices forward in the kernel (hosts on clouds that scan
	// their machines); the others run the userspace kernel.
	Kernel bool `json:"kernel"`
}

// Uplink is an interface of a device with its source address and gateway
// per family.
type Uplink struct {
	Device    string `json:"device"`
	Name      string `json:"name"`
	Interface string `json:"interface"`
	SourceV4  string `json:"source_v4,omitempty"`
	GatewayV4 string `json:"gateway_v4,omitempty"`
	SourceV6  string `json:"source_v6,omitempty"`
	GatewayV6 string `json:"gateway_v6,omitempty"`
}

// Entry kinds.
const (
	EntryDirect = "direct"
	EntryNAT    = "nat"
	EntryIWAN   = "iwan"
)

// Entry is an address subscribers reach a device at. Every device has a
// default direct entry at its host, with the device's reference as id.
type Entry struct {
	ID      string                `json:"id"`
	Device  string                `json:"device"`
	Name    string                `json:"name,omitempty"`
	Kind    string                `json:"kind"`
	Host    string                `json:"host"`
	PortMap []nodeentry.PortRange `json:"port_map,omitempty"`
	// IWANLine is the iWAN line of an iwan entry.
	IWANLine uint   `json:"iwan_line,omitempty"`
	Pool     string `json:"pool,omitempty"`
}

// Transport kinds.
const (
	// TransportJacket is the jacketed link, for segments across borders.
	TransportJacket = "jacket"
	// TransportLine is the bare link of a trusted segment.
	TransportLine = "line"
	// TransportReverse is the bare link the far device dials back.
	TransportReverse = "reverse"
)

// Transport is the link from one device to the next of a line. Lines
// between the same two devices share it.
type Transport struct {
	ID        string   `json:"id"`
	From      string   `json:"from"`
	To        string   `json:"to"`
	Kind      string   `json:"kind"`
	Carriers  []string `json:"carriers,omitempty"`
	Multipath bool     `json:"multipath,omitempty"`
	FEC       bool     `json:"fec,omitempty"`
	// Declared transports are components of the network model document,
	// by their id there; the others Control links itself.
	Declared bool `json:"declared,omitempty"`
}

// Exit is where traffic leaves a device for the internet. Every device has
// a default exit, with the device's reference as id.
type Exit struct {
	ID            string `json:"id"`
	Device        string `json:"device"`
	BlockMainland bool   `json:"block_mainland,omitempty"`
	// Declared exits are internet egresses of the network model document.
	Declared bool `json:"declared,omitempty"`
}

// Rule sends the domains of a line out by another exit.
type Rule struct {
	Name    string   `json:"name"`
	Domains []string `json:"domains"`
	Exit    string   `json:"exit"`
	Enabled bool     `json:"enabled"`
}

// Hop is a step of a line's chain: its devices (several on the entry hop
// are entry HA) and how the hop before reaches it.
type Hop struct {
	Devices []string `json:"devices"`
	// Port is where a hop after the entry listens for a kernel hop before
	// it; 0 is the line's port.
	Port uint32 `json:"port,omitempty"`
	// Dial replaces the device's host for the hop before.
	Dial string `json:"dial,omitempty"`
	// Transport is the transport from the hop before; empty after a kernel
	// device, which forwards without one, and on the entry hop.
	Transport string `json:"transport,omitempty"`
}

// Protocol is the protocol a line serves: a protocol of the device it is
// served at.
type Protocol struct {
	ID     uint   `json:"id"`
	Type   string `json:"type"`
	Device string `json:"device"`
	Port   uint32 `json:"port"`
}

// Line is a way from entries to an exit.
type Line struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	// Show offers the line in subscriptions.
	Show bool `json:"show"`
	// Entries are entry ids, on the devices of Chain[0].
	Entries []string `json:"entries"`
	// EntryHostname is the one name over every entry (entry HA).
	EntryHostname string `json:"entry_hostname,omitempty"`
	// Listen is the address the entry devices listen on; empty is every
	// address.
	Listen string   `json:"listen,omitempty"`
	Port   uint32   `json:"port"`
	L4     []string `json:"l4"`
	// Chain is the entry hop and every hop after it; the last one's
	// device is the exit's.
	Chain []Hop  `json:"chain"`
	Exit  string `json:"exit"`
	Rules []Rule `json:"rules,omitempty"`
	// Protocol is what the line serves; nil on a line that forwards to
	// fixed targets.
	Protocol *Protocol `json:"protocol,omitempty"`
	// Targets are where the last device connects to: the protocol's port
	// on a line that serves one.
	Targets []*networkv1.ForwardTarget `json:"targets,omitempty"`
	// Policy balances over the targets, checks them and limits the line;
	// HopBalance balances a kernel hop over the next hop's devices.
	Policy     *networkv1.ForwardPolicy `json:"policy,omitempty"`
	HopBalance string                   `json:"hop_balance,omitempty"`
	Rate       float64                  `json:"rate"`
	// ServeAt is the device the protocol is served at when the
	// administrator chose it; empty is the first userspace device.
	ServeAt string `json:"serve_at,omitempty"`
}

// Skipped is something Import could not read as a line, and why.
type Skipped struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// Snapshot is the whole network.
type Snapshot struct {
	Devices    []Device    `json:"devices"`
	Uplinks    []Uplink    `json:"uplinks"`
	Entries    []Entry     `json:"entries"`
	Transports []Transport `json:"transports"`
	Exits      []Exit      `json:"exits"`
	Lines      []Line      `json:"lines"`
	Skipped    []Skipped   `json:"skipped"`
}

// direct reports whether the line is served where its subscribers connect:
// one device, the protocol's own port. Such a line needs no forward.
func (l Line) direct() bool {
	return len(l.Chain) == 1 && l.Protocol != nil && l.Port == l.Protocol.Port
}

// Forwards plans the lines that forward as the network model's forwards:
// the leading kernel devices of a chain forward in the kernel, the first
// other hop runs the port forward, every hop after it is a segment.
func Forwards(s *Snapshot) []networkmodel.Forward {
	kernel := map[string]bool{}
	for _, device := range s.Devices {
		kernel[device.Ref] = device.Kernel
	}
	transports := map[string]Transport{}
	for _, transport := range s.Transports {
		transports[transport.ID] = transport
	}
	declaredExit := map[string]bool{}
	for _, exit := range s.Exits {
		declaredExit[exit.ID] = exit.Declared
	}
	var forwards []networkmodel.Forward
	for _, line := range s.Lines {
		if line.direct() || len(line.Chain) == 0 {
			continue
		}
		address := line.Listen
		if address == "" {
			address = "::"
		}
		listen := func(i int) networkmodel.Endpoint {
			switch {
			case i == 0:
				return networkmodel.Endpoint{Address: address, Port: line.Port}
			case line.Chain[i].Port == 0:
				return networkmodel.Endpoint{Address: "::", Port: line.Port}
			}
			return networkmodel.Endpoint{Address: "::", Port: line.Chain[i].Port}
		}
		forward := networkmodel.Forward{
			ID: line.ID, Protocols: line.L4, Targets: line.Targets, Policy: line.Policy, Paused: !line.Enabled,
		}
		k := 0
		for k < len(line.Chain) && kernel[line.Chain[k].Devices[0]] {
			hop := line.Chain[k]
			forward.Kernel = append(forward.Kernel, networkmodel.KernelHop{
				Nodes: hop.Devices, Listen: listen(k), DialAddress: hop.Dial, Policy: line.kernelPolicy(k == len(line.Chain)-1),
			})
			k++
		}
		if k < len(line.Chain) {
			forward.Entries, forward.Listen = line.Chain[k].Devices, listen(k)
			if k > 0 {
				forward.EntryDial = line.Chain[k].Dial
			}
			if declaredExit[line.Exit] {
				forward.Egress = line.Exit
			}
			for _, hop := range line.Chain[k+1:] {
				transport := transports[hop.Transport]
				segment, link := "", networkmodel.Link{}
				switch {
				case transport.Declared:
					segment = transport.ID
				case transport.Kind == TransportLine:
					link = networkmodel.Link{Mode: networkmodel.LinkAnxLine, Carriers: transport.Carriers, Multipath: transport.Multipath, FEC: transport.FEC}
				case transport.Kind == TransportReverse:
					link = networkmodel.Link{Mode: networkmodel.LinkAnxLineReverse}
				}
				forward.Segments = append(forward.Segments, segment)
				forward.Links = append(forward.Links, link)
				forward.Via = append(forward.Via, hop.Devices[0])
			}
		}
		forwards = append(forwards, forward)
	}
	return forwards
}

// kernelPolicy answers how a kernel hop balances and checks where it
// forwards: the next hop's devices, or the targets on the last hop.
func (l Line) kernelPolicy(last bool) *networkv1.ForwardPolicy {
	out := &networkv1.ForwardPolicy{Balance: l.HopBalance, Health: l.Policy.GetHealth()}
	if last {
		out.Balance = l.Policy.GetBalance()
	}
	if proto.Equal(out, &networkv1.ForwardPolicy{}) {
		return nil
	}
	return out
}
