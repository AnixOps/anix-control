package parser

import (
	"fmt"
	"strings"

	"github.com/AnixOps/anix-control/v5/internal/model"
)

// IWANFormatter emits iwand client configuration (INI, one section per
// Panabit iWAN node, named after the TUN interface it creates). iWAN has
// no share-link format; OpenWrt and Panabit clients take the same server,
// port, user name and password by hand. `#` starts a comment only at the
// beginning of a line, so every value stands alone on its line.
type IWANFormatter struct{}

func (f *IWANFormatter) Name() string {
	return "iwan"
}

func (f *IWANFormatter) ContentType() string {
	return "text/plain; charset=utf-8"
}

func (f *IWANFormatter) FileExtension() string {
	return "conf"
}

func (f *IWANFormatter) Format(nodes []*model.ParsedNode, ctx *model.TemplateRenderContext) ([]byte, error) {
	var sections []string
	for _, node := range nodes {
		if node.Type != "iwan" || !node.IsValid() {
			continue
		}
		sections = append(sections, f.formatNode(node, len(sections)))
	}
	if len(sections) == 0 {
		return []byte{}, nil
	}
	header := "# Panabit iWAN (iwand). iWAN does not encrypt traffic; use it only on links you trust.\n"
	return []byte(header + strings.Join(sections, "\n")), nil
}

func (f *IWANFormatter) formatNode(node *model.ParsedNode, index int) string {
	mtu := node.MTU
	if mtu <= 0 {
		mtu = 1400
	}
	lines := []string{fmt.Sprintf("# %s", iwanCommentText(node.Name))}
	if node.PeerIP != "" {
		lines = append(lines, fmt.Sprintf("# fixed address %s", node.PeerIP))
	}
	lines = append(lines,
		fmt.Sprintf("[iwan%d]", index),
		fmt.Sprintf("server=%s", wireGuardHost(node.Server)),
		fmt.Sprintf("port=%d", node.Port),
		fmt.Sprintf("username=%s", node.Username),
		fmt.Sprintf("password=%s", node.Password),
		fmt.Sprintf("mtu=%d", mtu),
		fmt.Sprintf("encrypt=%d", iwanEncrypt(node)),
		"",
	)
	return strings.Join(lines, "\n")
}

// iwanEncrypt asks for XOR masking when the node allows it. The mask only
// hides the payload from casual inspection; it is not encryption.
func iwanEncrypt(node *model.ParsedNode) int {
	if allow, ok := node.Settings["allow_encrypt"].(bool); ok && !allow {
		return 0
	}
	return 1
}

// iwanCommentText keeps a node name on its comment line.
func iwanCommentText(name string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(name)
}
