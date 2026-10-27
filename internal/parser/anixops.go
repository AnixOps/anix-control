package parser

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/AnixOps/anix-control/v5/internal/model"
)

// AnixOpsFormatter emits AnixOps Net's subscription: the V2Ray link list,
// base64 encoded, plus two AnixOps extensions its importer reads
// (networkcore docs/architecture/outbound-via.md, 订阅导入):
//
//   - a Panabit iWAN node is an iwan://user:password@host:port?mtu=&encrypt=#name
//     link;
//   - a node reached through an iWAN line carries via=<line name> in its
//     link's query (after the base64 body of a vmess:// link).
//
// Older App versions skip iwan:// links and ignore via.
type AnixOpsFormatter struct {
	links V2RayFormatter
}

func (f *AnixOpsFormatter) Name() string {
	return "anixops"
}

func (f *AnixOpsFormatter) ContentType() string {
	return "text/plain; charset=utf-8"
}

func (f *AnixOpsFormatter) FileExtension() string {
	return "txt"
}

func (f *AnixOpsFormatter) Format(nodes []*model.ParsedNode, ctx *model.TemplateRenderContext) ([]byte, error) {
	var links []string
	for _, node := range nodes {
		var link string
		var err error
		if node.Type == "iwan" {
			link = iwanLink(node)
		} else {
			link, err = f.links.link(node, ctx)
		}
		if err != nil || link == "" {
			continue
		}
		if node.Via != "" {
			link = withVia(link, node.Via)
		}
		links = append(links, link)
	}
	content := strings.Join(links, "\n")
	return []byte(base64.StdEncoding.EncodeToString([]byte(content))), nil
}

// iwanLink is a Panabit iWAN login as an iwan:// link.
func iwanLink(node *model.ParsedNode) string {
	if !node.IsValid() {
		return ""
	}
	query := url.Values{}
	if node.MTU > 0 {
		query.Set("mtu", strconv.Itoa(node.MTU))
	}
	query.Set("encrypt", strconv.Itoa(iwanEncrypt(node)))
	login := url.UserPassword(node.Username, node.Password).String()
	host := node.Server
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return fmt.Sprintf("iwan://%s@%s:%d?%s#%s", login, host, node.Port, query.Encode(), url.PathEscape(node.Name))
}

// withVia adds via=<name> to a link's query, before its fragment.
func withVia(link, via string) string {
	fragment := ""
	if i := strings.Index(link, "#"); i >= 0 {
		link, fragment = link[:i], link[i:]
	}
	separator := "?"
	if strings.Contains(link, "?") {
		separator = "&"
	}
	// The importer decodes percent escapes only, so a space is %20, not +.
	return link + separator + "via=" + strings.ReplaceAll(url.QueryEscape(via), "+", "%20") + fragment
}
