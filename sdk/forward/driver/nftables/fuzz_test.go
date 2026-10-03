package nftables_test

import (
	"bytes"
	"errors"
	"regexp"
	"slices"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/driver"
	"github.com/AnixOps/anix-control/sdk/forward/driver/conformance"
	"github.com/AnixOps/anix-control/sdk/forward/driver/nftables"
)

// The script's grammar, as far as the fuzzer checks it: the fixed header,
// blank lines, and statements made of names, numbers, addresses,
// punctuation and quoted names only.
var (
	statementLine = regexp.MustCompile(`^\t*[A-Za-z0-9_ .:,/{}@!=&|()\-;"]+$`)
	quoted        = regexp.MustCompile(`"[^"]*"`)
	quotedOK      = regexp.MustCompile(`^"[A-Za-z0-9_. -]+"$`)
	tableLine     = regexp.MustCompile(`^table inet anixops_fwd \{$`)
	flushLine     = regexp.MustCompile(`^flush (table inet anixops_fwd|(set|map) inet anixops_fwd r_[0-9A-Za-z]{1,64}_h[0-9]+_(src4|src6|lb4|lb6))$`)
	forbidden     = regexp.MustCompile(`\b(ruleset|delete|destroy|include|define|reset|insert|replace|rename)\b|\$`)
)

// checkScript reports what in a rendered script is outside the grammar.
// manifestLine is the grammar of the manifest comments Apply reads.
var manifestLine = regexp.MustCompile(`^# anixops-(hop route=[0-9A-Za-z]{1,64} hop=[0-9]+ mark=[0-9]+ balance=BALANCE_STRATEGY_[A-Z_]+ listen=(tcp|udp|tcp,udp)/[0-9]+ address=(any|[0-9a-fA-F:.]+) bandwidth=[0-9]+|upstream route=[0-9A-Za-z]{1,64} hop=[0-9]+ address=[0-9a-fA-F:.]+ port=[0-9]+ weight=[0-9]+ priority=[0-9]+)$`)

func checkScript(content []byte, hops int) error {
	lines := strings.Split(string(content), "\n")
	if lines[len(lines)-1] != "" {
		return errors.New("no final newline")
	}
	depth, statements := 0, 0
	for _, l := range lines[:len(lines)-1] {
		switch {
		case l == "":
			continue
		case strings.HasPrefix(l, "#"):
			if depth == 0 && manifestLine.MatchString(l) {
				continue
			}
			if depth != 0 || !bytes.Contains(headerAndNote, []byte(l+"\n")) {
				return errors.New("unexpected comment line " + l)
			}
			continue
		case !statementLine.MatchString(l):
			return errors.New("line outside the grammar: " + l)
		case forbidden.MatchString(l):
			return errors.New("forbidden token: " + l)
		}
		for _, q := range quoted.FindAllString(l, -1) {
			if !quotedOK.MatchString(q) {
				return errors.New("unexpected quoted text " + q)
			}
		}
		if strings.Count(l, `"`)%2 != 0 {
			return errors.New("unbalanced quote: " + l)
		}
		statements++
		if depth == 0 {
			if !tableLine.MatchString(l) && !flushLine.MatchString(l) {
				return errors.New("top-level statement outside the grammar: " + l)
			}
		}
		depth += strings.Count(l, "{") - strings.Count(l, "}")
		if depth < 0 {
			return errors.New("unbalanced braces at line " + l)
		}
	}
	if depth != 0 {
		return errors.New("unbalanced braces")
	}
	if (hops == 0) != (statements == 0) {
		return errors.New("statements do not match the hops")
	}
	return nil
}

// headerAndNote are the comment lines a script may contain.
var headerAndNote = func() []byte {
	d, err := nftables.New(testConfig())
	if err != nil {
		panic(err)
	}
	a, _ := d.Render(&forwardv1.NodeForwardState{})
	return a.Content
}()

// FuzzRender: Render never panics, answers an artifact that verifies with
// every nftables hop either run or rejected, and a script inside the
// grammar, the same on every call.
func FuzzRender(f *testing.F) {
	for _, c := range goldenCases(f) {
		b, err := proto.Marshal(c.state)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(b, false)
		f.Add(b, true)
	}
	b := builder(f)
	h := b.Relay(`r"; flush ruleset #`, 0, b.Upstreams(b.Top.UpstreamsV4, 2))
	h.IngressSources = []string{"198.51.100.0/24 } ; drop", "::/0"}
	h.Upstreams[0].Address = "1.2.3.4 . 5, 0 : 6.6.6.6"
	seed, _ := proto.Marshal(conformance.State("forward-11", 1, h))
	f.Add(seed, true)

	d := newDriver(f, nil)
	f.Fuzz(func(t *testing.T, data []byte, allNFT bool) {
		var s forwardv1.NodeForwardState
		if proto.Unmarshal(data, &s) != nil {
			return
		}
		if allNFT {
			for _, h := range s.GetHops() {
				if h != nil {
					h.Engine = nftE
				}
			}
		}
		a, err := d.Render(&s)
		var re *driver.RenderError
		if err != nil && !errors.As(err, &re) {
			t.Fatalf("Render of a non-nil state: %v", err)
		}
		if verr := a.Verify(nftE); verr != nil {
			t.Fatal(verr)
		}
		keys := map[driver.HopKey]bool{}
		for _, h := range s.GetHops() {
			if h.GetEngine() == nftE {
				keys[driver.KeyOf(h)] = true
			}
		}
		accounted := map[driver.HopKey]bool{}
		for _, k := range a.Hops {
			accounted[k] = true
		}
		if re != nil {
			for _, he := range re.Hops {
				if slices.Contains(a.Hops, he.Key) {
					t.Fatalf("hop %s both run and rejected", he.Key)
				}
				accounted[he.Key] = true
			}
		}
		if len(accounted) != len(keys) {
			t.Fatalf("%d nftables hops, %d run or rejected", len(keys), len(accounted))
		}
		if err := checkScript(a.Content, len(a.Hops)); err != nil {
			t.Fatalf("%v\n%s", err, a.Content)
		}
		if err := nftables.CheckManifest(a); err != nil {
			t.Fatalf("manifest: %v\n%s", err, a.Content)
		}
		again, _ := d.Render(&s)
		if again.Digest != a.Digest {
			t.Fatal("two renders differ")
		}
	})
}
