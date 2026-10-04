package main

import (
	"errors"
	"flag"
	"fmt"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/v4/internal/kernelforward"
)

// forward dns: entry HA through DNS (forward-sdk.md section 7.4, L2). A
// provider file is {"provider": DnsProvider, "credentials": {...}}, a
// binding file a DnsBinding (protojson); credentials are read from the
// file, never from the command line, and never printed.

// dnsRefusal explains a refused DNS write with its violations.
func dnsRefusal(err error) error {
	var refused *kernelforward.DNSRefusedError
	if !errors.As(err, &refused) {
		return refusal(err)
	}
	lines := []string{"refused:"}
	for _, v := range refused.Violations {
		line := fmt.Sprintf("  %s: %s (%s)", v.GetField(), v.GetMessage(), v.GetCode())
		if v.GetRouteId() != "" {
			line += " [route " + v.GetRouteId() + "]"
		}
		lines = append(lines, line)
	}
	return errors.New(strings.Join(lines, "\n"))
}

func (c *forwardCLI) dns(arguments []string) error {
	if len(arguments) == 0 {
		return forwardUsageError()
	}
	set := flag.NewFlagSet("forward dns", flag.ContinueOnError)
	asJSON := set.Bool("json", false, "")
	file := set.String("f", "", "")
	requestID := set.String("request-id", "", "")
	yes := set.Bool("yes", false, "")
	purge := set.Bool("purge", false, "")
	route := set.String("route", "", "")
	positionals, err := parseFlags(set, arguments[1:])
	if err != nil {
		return err
	}
	switch arguments[0] {
	case "status":
		if len(positionals) != 1 {
			return forwardUsageError()
		}
		return c.dnsStatus(positionals[0], *asJSON)
	case "providers", "bindings":
	default:
		return forwardUsageError()
	}
	if len(positionals) == 0 {
		return forwardUsageError()
	}
	command, rest := positionals[0], positionals[1:]
	var id uint64
	switch command {
	case "list", "create":
		if len(rest) != 0 || (command == "create" && *file == "") {
			return forwardUsageError()
		}
	case "update", "delete":
		if len(rest) != 1 || (command == "update" && *file == "") {
			return forwardUsageError()
		}
		if id, err = strconv.ParseUint(rest[0], 10, 64); err != nil || id == 0 {
			return fmt.Errorf("%q is not a %s id", rest[0], strings.TrimSuffix(arguments[0], "s"))
		}
		if command == "delete" && !*yes {
			return errors.New("deleting stops entry HA for the routes it serves: pass --yes")
		}
	default:
		return forwardUsageError()
	}
	if arguments[0] == "providers" {
		return c.dnsProviders(command, id, *file, cliRequestID(*requestID), *asJSON)
	}
	return c.dnsBindings(command, id, *file, *route, cliRequestID(*requestID), *purge, *asJSON)
}

func (c *forwardCLI) dnsProviders(command string, id uint64, file, requestID string, asJSON bool) error {
	switch command {
	case "list":
		providers, err := c.service.ListDNSProviders(c.ctx)
		if err != nil {
			return err
		}
		if asJSON {
			return c.printMessage(&forwardv1.ListDnsProvidersResponse{Providers: providers})
		}
		table := tabwriter.NewWriter(c.stdout, 0, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(table, "ID\tNAME\tKIND\tCREDENTIALS\tBINDINGS")
		for _, p := range providers {
			_, _ = fmt.Fprintf(table, "%d\t%s\t%s\t%s\t%d\n", p.GetId(), p.GetName(),
				strings.ToLower(strings.TrimPrefix(p.GetKind().String(), "DNS_PROVIDER_KIND_")), strings.Join(p.GetCredentialNames(), ","), p.GetBindings())
		}
		return table.Flush()
	case "delete":
		if err := c.service.DeleteDNSProvider(c.ctx, requestID, id); err != nil {
			return dnsRefusal(err)
		}
		audited := c.audit("forward.dns_provider_delete", "forward_dns_provider", map[string]any{"provider_id": id})
		return c.printJSON(map[string]any{"deleted": id, "audited": audited})
	}
	raw, err := c.readFile(file)
	if err != nil {
		return err
	}
	var provider *forwardv1.DnsProvider
	if command == "create" {
		body := &forwardv1.CreateDnsProviderRequest{}
		if err := forwardJSONIn.Unmarshal(raw, body); err != nil {
			return fmt.Errorf("%s: %w", file, err)
		}
		provider, err = c.service.CreateDNSProvider(c.ctx, requestID, body.GetProvider(), body.GetCredentials())
	} else {
		body := &forwardv1.UpdateDnsProviderRequest{}
		if err := forwardJSONIn.Unmarshal(raw, body); err != nil {
			return fmt.Errorf("%s: %w", file, err)
		}
		if body.Provider == nil {
			body.Provider = &forwardv1.DnsProvider{}
		}
		body.Provider.Id = id
		provider, err = c.service.UpdateDNSProvider(c.ctx, requestID, body.GetProvider(), body.GetCredentials())
	}
	if err != nil {
		return dnsRefusal(err)
	}
	c.audit("forward.dns_provider_"+command, "forward_dns_provider", map[string]any{
		"provider_id": provider.GetId(), "name": provider.GetName(), "credential_names": provider.GetCredentialNames(), "request_id": requestID,
	})
	return c.printMessage(provider)
}

func (c *forwardCLI) dnsBindings(command string, id uint64, file, routeID, requestID string, purge, asJSON bool) error {
	switch command {
	case "list":
		bindings, err := c.service.ListDNSBindings(c.ctx, routeID, 0)
		if err != nil {
			return err
		}
		if asJSON {
			return c.printMessage(&forwardv1.ListDnsBindingsResponse{Bindings: bindings})
		}
		table := tabwriter.NewWriter(c.stdout, 0, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(table, "ID\tROUTE\tPROVIDER\tMODE\tNAME\tTYPES\tTTL\tPAUSED")
		for _, b := range bindings {
			types := make([]string, 0, len(b.GetRecordTypes()))
			for _, t := range b.GetRecordTypes() {
				types = append(types, strings.TrimPrefix(t.String(), "DNS_RECORD_TYPE_"))
			}
			_, _ = fmt.Fprintf(table, "%d\t%s\t%d\t%s\t%s\t%s\t%d\t%t\n", b.GetId(), b.GetRouteId(), b.GetProviderId(),
				strings.ToLower(strings.TrimPrefix(b.GetMode().String(), "DNS_BINDING_MODE_")), b.GetRecordName(), strings.Join(types, ","), b.GetTtl(), b.GetPaused())
		}
		return table.Flush()
	case "delete":
		if err := c.service.DeleteDNSBinding(c.ctx, requestID, id, purge); err != nil {
			return dnsRefusal(err)
		}
		audited := c.audit("forward.dns_binding_delete", "forward_dns_binding", map[string]any{"binding_id": id, "purge": purge})
		return c.printJSON(map[string]any{"deleted": id, "purged": purge, "audited": audited})
	}
	raw, err := c.readFile(file)
	if err != nil {
		return err
	}
	binding := &forwardv1.DnsBinding{}
	if err := forwardJSONIn.Unmarshal(raw, binding); err != nil {
		return fmt.Errorf("%s: %w", file, err)
	}
	if command == "create" {
		binding, err = c.service.CreateDNSBinding(c.ctx, requestID, binding)
	} else {
		binding.Id = id
		binding, err = c.service.UpdateDNSBinding(c.ctx, requestID, binding)
	}
	if err != nil {
		return dnsRefusal(err)
	}
	c.audit("forward.dns_binding_"+command, "forward_dns_binding", map[string]any{"binding_id": binding.GetId(), "route_id": binding.GetRouteId(), "request_id": requestID})
	return c.printMessage(binding)
}

func (c *forwardCLI) dnsStatus(routeID string, asJSON bool) error {
	status, err := c.service.RouteDNS(c.ctx, routeID)
	if err != nil {
		return err
	}
	if asJSON {
		return c.printMessage(status)
	}
	out := c.stdout
	_, _ = fmt.Fprintf(out, "route %s  entry %s  state %s\n", status.GetRouteId(), status.GetEntryHostname(), status.GetState())
	if binding := status.GetBinding(); binding != nil {
		_, _ = fmt.Fprintf(out, "binding %d  provider %d  %s %s  ttl %d\n", binding.GetId(), binding.GetProviderId(),
			strings.ToLower(strings.TrimPrefix(binding.GetMode().String(), "DNS_BINDING_MODE_")), binding.GetRecordName(), binding.GetTtl())
	}
	if target := status.GetCnameTarget(); target != "" {
		_, _ = fmt.Fprintf(out, "point %s CNAME %s\n", status.GetEntryHostname(), target)
	}
	for _, record := range status.GetRecords() {
		_, _ = fmt.Fprintf(out, "%s published [%s] desired [%s]\n", strings.TrimPrefix(record.GetType().String(), "DNS_RECORD_TYPE_"),
			strings.Join(record.GetPublished(), " "), strings.Join(record.GetDesired(), " "))
	}
	table := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	if len(status.GetNodes()) > 0 {
		_, _ = fmt.Fprintln(table, "NODE\tIN ROTATION\tHEALTHY\tREASON\tGOOD\tBAD\tADDRESSES")
	}
	for _, node := range status.GetNodes() {
		_, _ = fmt.Fprintf(table, "%s\t%t\t%t\t%s\t%d\t%d\t%s\n", node.GetNodeRef(), node.GetInRotation(), node.GetHealthy(), node.GetReason(),
			node.GetGoodStreak(), node.GetBadStreak(), strings.Join(node.GetAddresses(), " "))
	}
	if err := table.Flush(); err != nil {
		return err
	}
	if status.GetLastError() != "" {
		_, _ = fmt.Fprintf(out, "last error (%s): %s\n", time.UnixMilli(status.GetLastErrorAtUnixMs()).UTC().Format(time.RFC3339), status.GetLastError())
	}
	return nil
}
