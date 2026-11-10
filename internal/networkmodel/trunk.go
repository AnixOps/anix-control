package networkmodel

import (
	"strconv"
	"strings"

	"gorm.io/gorm"
)

// TrunkProxyNodes answers the proxy nodes (by id) whose users reach the
// internet over a trunk: an enabled user ingress on one of the node's lines
// has an enabled path with at least one transport segment (ANX-Line or
// ANX-A). Subscribers of such a node dial a domestic ingress and share the
// cross-border link with other users, which hides the cross-layer RTT a
// single user's direct connection shows (anx docs FUTURE-IMPROVEMENTS F-51,
// 默认路径规则). Subscriptions put these nodes first.
func TrunkProxyNodes(db *gorm.DB) (map[uint]bool, error) {
	doc, err := LoadDocument(db)
	if err != nil {
		return nil, err
	}
	return trunkProxyNodes(doc), nil
}

func trunkProxyNodes(doc *Document) map[uint]bool {
	lineNode := make(map[string]string, len(doc.Lines))
	for _, line := range doc.Lines {
		lineNode[line.ID] = line.Node
	}
	ingressNode := make(map[string]string, len(doc.Ingresses))
	for _, ingress := range doc.Ingresses {
		if ingress.Disabled || ingress.Audience != "user" {
			continue
		}
		ingressNode[ingress.ID] = lineNode[ingress.Line]
	}
	result := map[uint]bool{}
	for _, path := range doc.Paths {
		if path.Disabled || len(path.Segments) == 0 {
			continue
		}
		ref, ok := ingressNode[path.Ingress]
		if !ok {
			continue
		}
		raw, ok := strings.CutPrefix(ref, "proxy-")
		if !ok {
			continue
		}
		id, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || id == 0 {
			continue
		}
		result[uint(id)] = true
	}
	return result
}
