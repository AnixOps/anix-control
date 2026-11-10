package networkmodel

import "testing"

func TestTrunkProxyNodes(t *testing.T) {
	doc := &Document{
		Lines: []Line{
			{ID: "sh-cn2", Node: "proxy-7"},
			{ID: "jp", Node: "proxy-9"},
			{ID: "hk", Node: "proxy-11"},
			{ID: "fw", Node: "forward-3"},
		},
		Ingresses: []Ingress{
			{ID: "sh-in", Line: "sh-cn2", Audience: "user"},
			{ID: "jp-in", Line: "jp", Audience: "user"},
			{ID: "jp-hop", Line: "jp", Audience: "hop"},
			{ID: "hk-in", Line: "hk", Audience: "user", Disabled: true},
			{ID: "fw-in", Line: "fw", Audience: "user"},
		},
		Paths: []Path{
			{ID: "sh-jp", Ingress: "sh-in", Segments: []string{"sh-jp-link"}, Egress: "jp-out"},
			{ID: "jp-local", Ingress: "jp-in", Egress: "jp-out"},
			{ID: "hk-jp", Ingress: "hk-in", Segments: []string{"hk-jp-link"}, Egress: "jp-out"},
			{ID: "fw-jp", Ingress: "fw-in", Segments: []string{"fw-jp-link"}, Egress: "jp-out"},
		},
	}
	got := trunkProxyNodes(doc)
	if len(got) != 1 || !got[7] {
		t.Fatalf("trunk nodes = %v, want only 7", got)
	}
	doc.Paths[0].Disabled = true
	if got := trunkProxyNodes(doc); len(got) != 0 {
		t.Fatalf("disabled path still trunk: %v", got)
	}
}
