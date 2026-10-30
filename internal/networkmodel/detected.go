package networkmodel

import (
	"context"
	"net/netip"
	"sort"
	"time"
)

// DetectedInterface is an interface the node's Agent reported in its
// network status (InterfaceFact), offered on the node page so the network
// settings are picked rather than typed: its addresses per family without
// the prefix length, and its default gateways.
type DetectedInterface struct {
	Name        string   `json:"name"`
	AddressesV4 []string `json:"addresses_v4"`
	AddressesV6 []string `json:"addresses_v6"`
	GatewayV4   string   `json:"gateway_v4"`
	GatewayV6   string   `json:"gateway_v6"`
}

// NodeNetworkView is a device's network settings with the interfaces its
// Agent last reported (detected, empty before an Agent that reports them;
// detected_at is that report's time).
type NodeNetworkView struct {
	NodeSettings
	Detected   []DetectedInterface `json:"detected"`
	DetectedAt *time.Time          `json:"detected_at"`
}

// NodeNetwork answers node's network settings and detected interfaces.
func (s *Service) NodeNetwork(ctx context.Context, node string) (NodeNetworkView, error) {
	settings, err := s.NodeSettings(ctx, node)
	if err != nil {
		return NodeNetworkView{}, err
	}
	view := NodeNetworkView{NodeSettings: settings, Detected: []DetectedInterface{}}
	status, at, err := s.latestStatus(ctx, node)
	if err != nil || status == nil {
		return view, err
	}
	for _, fact := range status.GetInterfaces() {
		detected := DetectedInterface{Name: fact.GetName(), AddressesV4: []string{}, AddressesV6: []string{},
			GatewayV4: fact.GetGatewayV4(), GatewayV6: fact.GetGatewayV6()}
		for _, text := range fact.GetAddresses() {
			addr, ok := plainAddress(text)
			switch {
			case !ok:
			case addr.Is4():
				detected.AddressesV4 = append(detected.AddressesV4, addr.String())
			default:
				detected.AddressesV6 = append(detected.AddressesV6, addr.String())
			}
		}
		view.Detected = append(view.Detected, detected)
	}
	sort.Slice(view.Detected, func(i, j int) bool { return view.Detected[i].Name < view.Detected[j].Name })
	view.DetectedAt = at
	return view, nil
}

// plainAddress answers an address or a prefix's address.
func plainAddress(text string) (netip.Addr, bool) {
	if prefix, err := netip.ParsePrefix(text); err == nil {
		return prefix.Addr().Unmap(), true
	}
	addr, err := netip.ParseAddr(text)
	return addr.Unmap(), err == nil
}
