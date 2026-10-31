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
//   - an ANX-A node is an anx:// link (anxLink); other formats skip ANX;
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
		} else if node.Type == "anx" {
			link = anxLink(node)
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

// anxLink is an ANX-A node as an anx:// link:
// anx://<user key>@host:port?sni=&pk=<server public key>&alpn=&fp=chrome#name.
// A node whose server has not reported its public key has no link yet.
func anxLink(node *model.ParsedNode) string {
	if node.Server == "" || node.Port <= 0 || node.Password == "" || node.PublicKey == "" || node.ServerName == "" {
		return ""
	}
	query := url.Values{}
	query.Set("sni", node.ServerName)
	query.Set("pk", node.PublicKey)
	if node.ALPN != "" {
		query.Set("alpn", node.ALPN)
	}
	query.Set("fp", "chrome")
	host := node.Server
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return fmt.Sprintf("anx://%s@%s:%d?%s#%s", node.Password, host, node.Port, query.Encode(), url.PathEscape(node.Name))
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
