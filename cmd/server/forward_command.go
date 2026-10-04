package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/v4/internal/kernelforward"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

const forwardCommandUsage = `usage:
  anix-control forward routes list [--owner <owner>] [--node <node_ref>] [--json]
  anix-control forward routes get <route_id>
  anix-control forward routes create -f <route.json> [--request-id <id>]
  anix-control forward routes delete <route_id> --yes [--request-id <id>]
  anix-control forward routes pause <route_id>
  anix-control forward routes resume <route_id>
  anix-control forward routes diagnose <route_id> [--timeout <ms>] [--json]
  anix-control forward nodes list [--kind forward|proxy] [--json]
  anix-control forward nodes set <node_ref> (-f <settings.json> | [--port-range <first>-<last>] [--reserved <port,...>] [--address <ip>]... [--label <key=value>]... | --defaults)
  anix-control forward stats [--route <route_id>] [--node <node_ref>] [--since <time>] [--until <time>] [--json]
  anix-control forward reset-node <node_ref>

The commands run on Control's database through the kernel's forwarding
state (forward-sdk.md section 8), as /api/v4/forward/* does through
ForwardControl. Routes and settings are protojson with the contract's field
names (sdk/api/forward/v1): routes create reads a Route, nodes set -f a
NodeSettings. A refused write prints each violation with its code
(sdk/forward/validate). nodes set replaces the node's settings: the flags
it is not given go back to the defaults. Times are RFC 3339 or Unix
milliseconds; stats defaults to the last 24 hours, at most 31 days.

routes diagnose runs the route diagnosis (forward-sdk.md section 7.6):
each node's planned against applied generation, hop errors, upstream health
and circuit breakers from the latest reports, and dials from Control to the
entries and the public targets. The command line holds no Agent sessions,
so the node probes (listen, port conflicts, next-hop connect, delivery) are
SKIPPED; POST /api/v4/forward/routes/{id}/diagnose on the running Control
runs them. It exits non-zero when a step failed.

reset-node moves a node's forwarding generation (node_ref proxy-<id> or
forward-<id>) above both the one Control stored and the one the node's
latest report says its Agent holds, keeping the node's hops, so the Agent
applies the node's current state again (after its rules were changed by
hand, or to drive a push again). A reset or restored Control database needs
no command: a node that reports a generation ahead of the stored one is
recovered on that report, within a minute (forward-sdk.md section 8.2).

Writes are written to the audit log as system/cli. The running Control
sends the nodes their new state at its next configuration refresh, within a
minute.

The config file comes from ANIX_CONTROL_CONFIG or config/config.yaml.`

func forwardUsageError() error {
	return fmt.Errorf("invalid forward command\n%s", forwardCommandUsage)
}

// forwardResetOutput is what forward reset-node prints.
type forwardResetOutput struct {
	kernelforward.NodeReset
	Audited bool `json:"audited"`
}

var (
	forwardJSONOut = protojson.MarshalOptions{UseProtoNames: true}
	forwardJSONIn  = protojson.UnmarshalOptions{}
)

// forwardCommandProbes are routes diagnose's dials from Control; tests
// replace them.
var forwardCommandProbes service.DiagnosisProbes

// forwardCLI runs the forward commands on one database.
type forwardCLI struct {
	ctx     context.Context
	db      *gorm.DB
	service *kernelforward.Service
	stdout  io.Writer
	// readFile reads -f; os.ReadFile by default.
	readFile func(string) ([]byte, error)
}

// runForwardCommand administers the kernel's forwarding state.
func runForwardCommand(ctx context.Context, db *gorm.DB, arguments []string, stdout io.Writer) error {
	forward := kernelforward.New(db)
	forward.Probes = forwardCommandProbes
	cli := &forwardCLI{ctx: ctx, db: db, service: forward, stdout: stdout, readFile: os.ReadFile}
	if len(arguments) == 0 {
		return forwardUsageError()
	}
	switch arguments[0] {
	case "reset-node":
		if len(arguments) != 2 {
			return forwardUsageError()
		}
		return cli.resetNode(arguments[1])
	case "routes":
		if len(arguments) < 2 {
			return forwardUsageError()
		}
		return cli.routes(arguments[1], arguments[2:])
	case "nodes":
		if len(arguments) < 2 {
			return forwardUsageError()
		}
		return cli.nodes(arguments[1], arguments[2:])
	case "stats":
		return cli.stats(arguments[1:])
	}
	return forwardUsageError()
}

func (c *forwardCLI) audit(action, targetType string, content any) bool {
	encoded, _ := json.Marshal(content)
	err := service.NewOperationLogService(c.db.WithContext(c.ctx)).Record(&service.OperationLogInput{
		Username: service.RouteModeCLIActor, Action: action, Module: "forward", TargetType: targetType, Content: string(encoded),
	})
	return err == nil
}

func (c *forwardCLI) printJSON(value any) error {
	encoder := json.NewEncoder(c.stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

// printMessage prints a contract message as indented protojson, with
// stable spacing (protojson's own varies on purpose).
func (c *forwardCLI) printMessage(message proto.Message) error {
	encoded, err := forwardJSONOut.Marshal(message)
	if err != nil {
		return err
	}
	var indented bytes.Buffer
	if err := json.Indent(&indented, encoded, "", "  "); err != nil {
		return err
	}
	_, err = fmt.Fprintln(c.stdout, indented.String())
	return err
}

// refusal explains a refused write with its violations.
func refusal(err error) error {
	var refused *kernelforward.RefusedError
	if !errors.As(err, &refused) {
		return err
	}
	lines := []string{"refused:"}
	for _, v := range refused.ProtoViolations() {
		line := fmt.Sprintf("  %s: %s (%s)", v.GetField(), v.GetMessage(), v.GetCode())
		if v.GetRouteId() != "" {
			line += " [route " + v.GetRouteId() + "]"
		}
		lines = append(lines, line)
	}
	return errors.New(strings.Join(lines, "\n"))
}

func cliRequestID(given string) string {
	if given != "" {
		return given
	}
	var raw [12]byte
	_, _ = rand.Read(raw[:])
	return "cli-" + hex.EncodeToString(raw[:])
}

// parseFlags parses arguments, interleaved with positionals, into set and
// answers the positionals.
func parseFlags(set *flag.FlagSet, arguments []string) ([]string, error) {
	set.SetOutput(io.Discard)
	var positionals []string
	for {
		if err := set.Parse(arguments); err != nil {
			return nil, forwardUsageError()
		}
		rest := set.Args()
		if len(rest) == 0 {
			return positionals, nil
		}
		positionals = append(positionals, rest[0])
		arguments = rest[1:]
	}
}

func (c *forwardCLI) resetNode(nodeRef string) error {
	reset, err := c.service.ResetNode(c.ctx, nodeRef)
	if err != nil {
		return err
	}
	audited := c.audit("forward.reset_node", "forward_node_state", reset)
	return c.printJSON(forwardResetOutput{NodeReset: reset, Audited: audited})
}

func (c *forwardCLI) routes(command string, arguments []string) error {
	set := flag.NewFlagSet("forward routes", flag.ContinueOnError)
	owner := set.String("owner", "", "")
	node := set.String("node", "", "")
	asJSON := set.Bool("json", false, "")
	file := set.String("f", "", "")
	requestID := set.String("request-id", "", "")
	yes := set.Bool("yes", false, "")
	timeout := set.Uint("timeout", 0, "")
	positionals, err := parseFlags(set, arguments)
	if err != nil {
		return err
	}
	switch command {
	case "list":
		if len(positionals) != 0 {
			return forwardUsageError()
		}
		return c.listRoutes(*owner, *node, *asJSON)
	case "create":
		if len(positionals) != 0 || *file == "" {
			return forwardUsageError()
		}
		return c.createRoute(*file, cliRequestID(*requestID))
	}
	if len(positionals) != 1 {
		return forwardUsageError()
	}
	routeID := positionals[0]
	switch command {
	case "get":
		route, err := c.service.GetRoute(c.ctx, routeID)
		if err != nil {
			return err
		}
		return c.printMessage(route)
	case "delete":
		if !*yes {
			return errors.New("deleting a route removes it from every node: pass --yes")
		}
		if err := c.service.DeleteRoute(c.ctx, cliRequestID(*requestID), routeID); err != nil {
			return refusal(err)
		}
		audited := c.audit("forward.route_delete", "forward_route", map[string]string{"route_id": routeID})
		return c.printJSON(map[string]any{"deleted": routeID, "audited": audited})
	case "pause", "resume":
		return c.setPaused(routeID, command == "pause")
	case "diagnose":
		return c.diagnoseRoute(routeID, time.Duration(min(*timeout, 60000))*time.Millisecond, *asJSON)
	}
	return forwardUsageError()
}

// errDiagnosisFailed ends routes diagnose when a step failed.
var errDiagnosisFailed = errors.New("the diagnosis found failures")

func (c *forwardCLI) diagnoseRoute(routeID string, timeout time.Duration, asJSON bool) error {
	answer, err := c.service.DiagnoseRoute(c.ctx, routeID, timeout)
	if err != nil {
		return err
	}
	c.audit("forward.route_diagnose", "forward_route", map[string]any{"route_id": routeID, "ok": answer.GetOk(), "steps": len(answer.GetSteps())})
	if asJSON {
		if err := c.printMessage(answer); err != nil {
			return err
		}
	} else if err := c.printDiagnosis(answer); err != nil {
		return err
	}
	if !answer.GetOk() {
		return errDiagnosisFailed
	}
	return nil
}

// printDiagnosis prints a diagnosis as a table of steps, then the nodes.
func (c *forwardCLI) printDiagnosis(answer *forwardv1.DiagnoseRouteResponse) error {
	writer := tabwriter.NewWriter(c.stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(writer, "HOP\tNODE\tKIND\tVANTAGE\tTARGET\tSTATUS\tCODE\tMESSAGE")
	for _, step := range answer.GetSteps() {
		node := step.GetNodeRef()
		if node == "" {
			node = "-"
		}
		target := step.GetTarget()
		if target == "" {
			target = "-"
		}
		if protocol := step.GetProtocol(); protocol == forwardv1.L4Protocol_L4_PROTOCOL_UDP {
			target += "/udp"
		}
		result := step.GetResult()
		_, _ = fmt.Fprintf(writer, "%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", step.GetHopIndex(), node,
			strings.TrimPrefix(step.GetKind().String(), "PROBE_KIND_"), strings.TrimPrefix(step.GetVantage().String(), "DIAGNOSE_VANTAGE_"),
			target, strings.TrimPrefix(result.GetStatus().String(), "PROBE_STATUS_"), result.GetCode(), result.GetMessage())
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(c.stdout)
	writer = tabwriter.NewWriter(c.stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(writer, "NODE\tCONNECTED\tNODE PROBES\tNOTE")
	for _, node := range answer.GetNodes() {
		_, _ = fmt.Fprintf(writer, "%s\t%t\t%t\t%s\n", node.GetNodeRef(), node.GetConnected(), node.GetNodeVantage(), node.GetNote())
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	verdict := "ok"
	if !answer.GetOk() {
		verdict = "FAILED"
	}
	if answer.GetCached() {
		verdict += " (cached)"
	}
	_, err := fmt.Fprintf(c.stdout, "\nroute %s: %s\n", answer.GetRouteId(), verdict)
	return err
}

func (c *forwardCLI) listRoutes(owner, node string, asJSON bool) error {
	var routes []*forwardv1.Route
	token := ""
	for {
		page, next, err := c.service.ListRoutes(c.ctx, kernelforward.ListOptions{Owner: owner, NodeRef: node, PageSize: 1000, PageToken: token})
		if err != nil {
			return err
		}
		routes = append(routes, page...)
		if token = next; token == "" {
			break
		}
	}
	ids := make([]string, 0, len(routes))
	for _, route := range routes {
		ids = append(ids, route.GetId())
	}
	enforced, err := c.service.Enforcement(c.ctx, ids)
	if err != nil {
		return err
	}
	if asJSON {
		answer := &forwardv1.ListRoutesResponse{Routes: routes, Enforced: enforced}
		return c.printMessage(answer)
	}
	writer := tabwriter.NewWriter(c.stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(writer, "ID\tNAME\tLISTEN\tHOPS\tTARGETS\tSTATE\tREVISION")
	for _, route := range routes {
		state := "active"
		if route.GetPaused() {
			state = "paused"
		}
		if reason := enforced[route.GetId()]; reason != "" {
			state += " (" + reason + ")"
		}
		var hops []string
		for _, hop := range route.GetHops() {
			hops = append(hops, strings.Join(hop.GetNodeRefs(), "|"))
		}
		listen := strconv.FormatUint(uint64(route.GetListen().GetPort()), 10)
		if listen == "0" {
			listen = "auto"
		}
		_, _ = fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%d\t%s\t%d\n", route.GetId(), route.GetName(), listen,
			strings.Join(hops, " > "), len(route.GetTargets()), state, route.GetRevision())
	}
	return writer.Flush()
}

func (c *forwardCLI) createRoute(file, requestID string) error {
	raw, err := c.readFile(file)
	if err != nil {
		return err
	}
	route := &forwardv1.Route{}
	if err := forwardJSONIn.Unmarshal(raw, route); err != nil {
		return fmt.Errorf("%s: %w", file, err)
	}
	if route.GetOwner() == "" {
		route.Owner = "admin"
	}
	created, err := c.service.CreateRoute(c.ctx, requestID, route)
	if err != nil {
		return refusal(err)
	}
	c.audit("forward.route_create", "forward_route", map[string]any{"route_id": created.GetId(), "name": created.GetName(), "request_id": requestID})
	return c.printMessage(created)
}

func (c *forwardCLI) setPaused(routeID string, paused bool) error {
	route, err := c.service.GetRoute(c.ctx, routeID)
	if err != nil {
		return err
	}
	if route.GetPaused() != paused {
		candidate := proto.Clone(route).(*forwardv1.Route)
		candidate.Paused = paused
		route, err = c.service.UpdateRoute(c.ctx, cliRequestID(""), candidate, route.GetRevision())
		if err != nil {
			return refusal(err)
		}
		action := "forward.route_resume"
		if paused {
			action = "forward.route_pause"
		}
		c.audit(action, "forward_route", map[string]any{"route_id": routeID, "revision": route.GetRevision()})
	}
	return c.printMessage(route)
}

// stringList collects a repeated flag.
type stringList []string

func (l *stringList) String() string { return strings.Join(*l, ",") }

func (l *stringList) Set(value string) error {
	*l = append(*l, value)
	return nil
}

func (c *forwardCLI) nodes(command string, arguments []string) error {
	set := flag.NewFlagSet("forward nodes", flag.ContinueOnError)
	kind := set.String("kind", "", "")
	asJSON := set.Bool("json", false, "")
	file := set.String("f", "", "")
	portRange := set.String("port-range", "", "")
	reserved := set.String("reserved", "", "")
	defaults := set.Bool("defaults", false, "")
	var addresses, labels stringList
	set.Var(&addresses, "address", "")
	set.Var(&labels, "label", "")
	positionals, err := parseFlags(set, arguments)
	if err != nil {
		return err
	}
	switch command {
	case "list":
		if len(positionals) != 0 {
			return forwardUsageError()
		}
		return c.listNodes(*kind, *asJSON)
	case "set":
		if len(positionals) != 1 {
			return forwardUsageError()
		}
		flagged := *portRange != "" || *reserved != "" || len(addresses) > 0 || len(labels) > 0
		modes := 0
		for _, mode := range []bool{*file != "", flagged, *defaults} {
			if mode {
				modes++
			}
		}
		if modes != 1 {
			// Exactly one of -f, the flags or --defaults.
			return forwardUsageError()
		}
		settings := &forwardv1.NodeSettings{}
		if *file != "" {
			raw, err := c.readFile(*file)
			if err != nil {
				return err
			}
			if err := forwardJSONIn.Unmarshal(raw, settings); err != nil {
				return fmt.Errorf("%s: %w", *file, err)
			}
		} else if flagged {
			if settings, err = settingsFromFlags(*portRange, *reserved, addresses, labels); err != nil {
				return err
			}
		}
		return c.setNode(positionals[0], settings)
	}
	return forwardUsageError()
}

func settingsFromFlags(portRange, reserved string, addresses, labels []string) (*forwardv1.NodeSettings, error) {
	settings := &forwardv1.NodeSettings{Addresses: addresses}
	if portRange != "" {
		first, last, ok := strings.Cut(portRange, "-")
		low, errLow := strconv.ParseUint(first, 10, 16)
		high, errHigh := strconv.ParseUint(last, 10, 16)
		if !ok || errLow != nil || errHigh != nil {
			return nil, fmt.Errorf("--port-range %q is not <first>-<last>", portRange)
		}
		settings.PortRange = &forwardv1.PortRange{First: uint32(low), Last: uint32(high)}
	}
	if reserved != "" {
		for _, item := range strings.Split(reserved, ",") {
			port, err := strconv.ParseUint(strings.TrimSpace(item), 10, 16)
			if err != nil {
				return nil, fmt.Errorf("--reserved %q is not a port list", reserved)
			}
			settings.ReservedPorts = append(settings.ReservedPorts, uint32(port))
		}
	}
	for _, label := range labels {
		key, value, ok := strings.Cut(label, "=")
		if !ok || key == "" {
			return nil, fmt.Errorf("--label %q is not key=value", label)
		}
		if settings.Labels == nil {
			settings.Labels = map[string]string{}
		}
		settings.Labels[key] = value
	}
	return settings, nil
}

func (c *forwardCLI) listNodes(kind string, asJSON bool) error {
	nodes, err := c.service.ListNodes(c.ctx, kernelforward.NodeFilter{Kind: kind})
	if err != nil {
		return err
	}
	if asJSON {
		return c.printMessage(&forwardv1.ListNodesResponse{Nodes: nodes})
	}
	writer := tabwriter.NewWriter(c.stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(writer, "NODE\tNAME\tHOST\tTRANSPORT\tINVENTORY\tPORTS\tDESIRED\tREPORTED\tHOP_ERRORS")
	for _, node := range nodes {
		transport := "agent"
		if node.GetRecord().GetTransport() == forwardv1.NodeTransport_NODE_TRANSPORT_ANSIBLE {
			transport = "ansible"
		}
		inventory := "no"
		if node.GetInInventory() {
			inventory = "yes"
		} else if !node.GetEnabled() {
			inventory = "disabled"
		}
		ports := "-"
		if r := node.GetInfo().GetPortRange(); r != nil {
			ports = fmt.Sprintf("%d-%d", r.GetFirst(), r.GetLast())
		}
		reported := "-"
		if node.GetReported() {
			reported = strconv.FormatUint(node.GetReportedGeneration(), 10)
			if !node.GetApplied() {
				reported += " (not applied)"
			}
		}
		_, _ = fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\t%d\t%s\t%d\n", node.GetNodeRef(), node.GetName(), node.GetHost(),
			transport, inventory, ports, node.GetDesiredGeneration(), reported, node.GetHopErrors())
	}
	return writer.Flush()
}

func (c *forwardCLI) setNode(nodeRef string, settings *forwardv1.NodeSettings) error {
	if _, err := agentcontrol.ParseAgentNode(nodeRef); err != nil {
		return fmt.Errorf("%w: node_ref: %v", kernelforward.ErrInvalidRequest, err)
	}
	answer, err := c.service.SetNodeSettingsAnswer(c.ctx, nodeRef, kernelforward.SettingsFromProto(settings))
	if err != nil {
		return err
	}
	encodedSettings, _ := protojson.Marshal(settings)
	c.audit("forward.node_settings", "forward_node", map[string]any{"node_ref": nodeRef, "settings": json.RawMessage(encodedSettings)})
	if err := c.printMessage(answer); err != nil {
		return err
	}
	if len(answer.GetViolations()) > 0 {
		return errors.New("the settings are stored, but the routes no longer plan: every node keeps its state until they do (see violations)")
	}
	return nil
}

// parseCLITime reads RFC 3339 or Unix milliseconds; "" is the zero time.
func parseCLITime(raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
	}
	if ms, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return time.UnixMilli(ms), nil
	}
	return time.Parse(time.RFC3339, raw)
}

// forwardStatsTotal is one route hop's traffic on one node over the window.
type forwardStatsTotal struct {
	RouteID   string `json:"route_id"`
	HopIndex  uint32 `json:"hop_index"`
	NodeRef   string `json:"node_ref"`
	UpBytes   uint64 `json:"up_bytes"`
	DownBytes uint64 `json:"down_bytes"`
	NewConns  uint64 `json:"new_conns"`
}

func (c *forwardCLI) stats(arguments []string) error {
	set := flag.NewFlagSet("forward stats", flag.ContinueOnError)
	routeID := set.String("route", "", "")
	nodeRef := set.String("node", "", "")
	sinceRaw := set.String("since", "", "")
	untilRaw := set.String("until", "", "")
	asJSON := set.Bool("json", false, "")
	positionals, err := parseFlags(set, arguments)
	if err != nil || len(positionals) != 0 {
		return forwardUsageError()
	}
	since, err := parseCLITime(*sinceRaw)
	if err != nil {
		return fmt.Errorf("--since: %w", err)
	}
	until, err := parseCLITime(*untilRaw)
	if err != nil {
		return fmt.Errorf("--until: %w", err)
	}
	buckets, truncated, err := c.service.TrafficBuckets(c.ctx, *routeID, *nodeRef, since, until)
	if err != nil {
		return err
	}
	byKey := map[string]*forwardStatsTotal{}
	for _, bucket := range buckets {
		key := fmt.Sprintf("%s\x00%d\x00%s", bucket.GetRouteId(), bucket.GetHopIndex(), bucket.GetNodeRef())
		total := byKey[key]
		if total == nil {
			total = &forwardStatsTotal{RouteID: bucket.GetRouteId(), HopIndex: bucket.GetHopIndex(), NodeRef: bucket.GetNodeRef()}
			byKey[key] = total
		}
		total.UpBytes += bucket.GetUpBytes()
		total.DownBytes += bucket.GetDownBytes()
		total.NewConns += bucket.GetNewConns()
	}
	totals := make([]*forwardStatsTotal, 0, len(byKey))
	for _, total := range byKey {
		totals = append(totals, total)
	}
	sort.Slice(totals, func(i, j int) bool {
		a, b := totals[i], totals[j]
		if a.RouteID != b.RouteID {
			return a.RouteID < b.RouteID
		}
		if a.HopIndex != b.HopIndex {
			return a.HopIndex < b.HopIndex
		}
		return a.NodeRef < b.NodeRef
	})
	if *asJSON {
		series := make([]json.RawMessage, 0, len(buckets))
		for _, bucket := range buckets {
			encoded, _ := protojson.MarshalOptions{UseProtoNames: true}.Marshal(bucket)
			series = append(series, encoded)
		}
		return c.printJSON(map[string]any{"totals": totals, "series": series, "truncated": truncated})
	}
	writer := tabwriter.NewWriter(c.stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(writer, "ROUTE\tHOP\tNODE\tUP_BYTES\tDOWN_BYTES\tNEW_CONNS")
	for _, total := range totals {
		_, _ = fmt.Fprintf(writer, "%s\t%d\t%s\t%d\t%d\t%d\n", total.RouteID, total.HopIndex, total.NodeRef, total.UpBytes, total.DownBytes, total.NewConns)
	}
	if truncated {
		_, _ = fmt.Fprintln(writer, "(truncated: narrow the filter or the window)")
	}
	return writer.Flush()
}
