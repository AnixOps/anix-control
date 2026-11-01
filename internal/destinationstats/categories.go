package destinationstats

import (
	"net/netip"
	"sort"
	"strings"
)

// The categories a destination falls in. A domain no rule names is web; a
// destination without a domain that no network rule names, and the rest of
// a user's traffic beyond the Agent's limit, are unknown.
const (
	CategoryVideo     = "video"
	CategoryMusic     = "music"
	CategoryMessaging = "messaging"
	CategorySocial    = "social"
	CategoryGaming    = "gaming"
	CategoryAI        = "ai"
	CategoryDownload  = "download"
	CategoryWeb       = "web"
	CategoryUnknown   = "unknown"
)

// Categories lists every category in display order.
var Categories = []string{
	CategoryVideo, CategoryMusic, CategoryMessaging, CategorySocial, CategoryGaming,
	CategoryAI, CategoryDownload, CategoryWeb, CategoryUnknown,
}

// domainRules maps a registrable domain to its category. Agents report
// registrable domains only (www.example.com counts as example.com), so a
// rule names one. The lists start from v2fly's domain-list-community
// categories, narrowed to the domains that serve one kind of traffic; a
// shared domain (google.com, qq.com, apple.com, microsoft.com, CDNs) stays
// web.
var domainRules = buildDomainRules(map[string][]string{
	CategoryVideo: {
		"youtube.com", "googlevideo.com", "ytimg.com", "youtu.be", "youtube-nocookie.com",
		"netflix.com", "netflix.net", "nflxvideo.net", "nflximg.net", "nflxext.com", "nflxso.net",
		"bilibili.com", "bilivideo.com", "bilivideo.cn", "hdslb.com", "biliapi.net", "biliapi.com",
		"iqiyi.com", "iq.com", "youku.com", "mgtv.com",
		"tiktok.com", "tiktokcdn.com", "tiktokv.com", "tiktokcdn-us.com", "douyin.com", "douyinvod.com",
		"twitch.tv", "ttvnw.net", "jtvnw.net", "vimeo.com", "vimeocdn.com",
		"disneyplus.com", "disney-plus.net", "dssott.com", "bamgrid.com",
		"hulu.com", "hulustream.com", "hbomax.com", "max.com", "primevideo.com", "aiv-cdn.net", "aiv-delivery.net",
		"dazn.com", "abema.tv", "ameba.jp", "nicovideo.jp", "nimg.jp", "dmc.nico",
		"viu.com", "viu.tv", "kktv.me", "linetv.tw", "mytvsuper.com",
		"crunchyroll.com", "funimation.com", "dailymotion.com", "dmcdn.net", "rumble.com", "kick.com",
		"pornhub.com", "phncdn.com", "xvideos.com", "xvideos-cdn.com",
	},
	CategoryMusic: {
		"spotify.com", "scdn.co", "spotifycdn.com", "spotifycdn.net", "soundcloud.com", "sndcdn.com",
		"kkbox.com", "kkbox.com.tw", "deezer.com", "dzcdn.net", "tidal.com", "pandora.com",
	},
	CategoryMessaging: {
		"telegram.org", "t.me", "telegram.me", "telesco.pe", "tdesktop.com", "telegra.ph", "cdn-telegram.org",
		"whatsapp.com", "whatsapp.net", "wa.me",
		"signal.org", "signal.art",
		"discord.com", "discord.gg", "discordapp.com", "discordapp.net", "discord.media",
		"line.me", "line-apps.com", "line-scdn.net", "naver.jp",
		"messenger.com", "viber.com", "skype.com", "zoom.us", "zoom.com", "slack.com", "slack-edge.com",
		"kakao.com", "kakaocdn.net", "wechat.com", "element.io", "matrix.org",
	},
	CategorySocial: {
		"facebook.com", "fbcdn.net", "fb.com", "instagram.com", "cdninstagram.com", "threads.net", "threads.com",
		"twitter.com", "x.com", "twimg.com", "t.co",
		"reddit.com", "redd.it", "redditmedia.com", "redditstatic.com",
		"pinterest.com", "pinimg.com", "tumblr.com", "linkedin.com", "licdn.com", "snapchat.com", "sc-cdn.net",
		"quora.com", "bsky.app", "bsky.social", "mastodon.social",
		"weibo.com", "weibo.cn", "sinaimg.cn", "xiaohongshu.com", "xhscdn.com", "zhihu.com", "zhimg.com", "douban.com",
	},
	CategoryGaming: {
		"steampowered.com", "steamcommunity.com", "epicgames.com", "unrealengine.com",
		"playstation.com", "playstation.net", "sonyentertainmentnetwork.com",
		"xboxlive.com", "xbox.com", "nintendo.com", "nintendo.net",
		"riotgames.com", "leagueoflegends.com", "blizzard.com", "battle.net", "ea.com", "origin.com",
		"roblox.com", "rbxcdn.com", "hoyoverse.com", "mihoyo.com", "supercell.com", "garena.com",
		"ubisoft.com", "minecraft.net", "rockstargames.com", "valvesoftware.com",
	},
	CategoryAI: {
		"openai.com", "chatgpt.com", "oaistatic.com", "oaiusercontent.com",
		"anthropic.com", "claude.ai", "claude.com", "claudeusercontent.com",
		"perplexity.ai", "poe.com", "x.ai", "grok.com", "mistral.ai", "deepseek.com",
		"huggingface.co", "hf.co", "midjourney.com", "character.ai", "cursor.com", "cursor.sh",
	},
	CategoryDownload: {
		"steamcontent.com", "steamserver.net", "steamstatic.com",
		"windowsupdate.com", "cdn-apple.com",
		"githubusercontent.com",
		"docker.io", "docker.com", "quay.io", "pypi.org", "pythonhosted.org", "npmjs.org", "npmjs.com",
		"debian.org", "ubuntu.com", "archlinux.org", "fedoraproject.org", "centos.org", "alpinelinux.org",
		"mega.nz", "mega.co.nz", "mega.io", "mediafire.com", "dropboxusercontent.com", "terabox.com",
		"baidupcs.com", "pikpak.com", "mypikpak.com", "1fichier.com", "rapidgator.net", "sourceforge.net",
		"thepiratebay.org", "nyaa.si", "rarbg.to", "1337x.to",
	},
})

// networkRules are the address networks of services that clients reach
// without a domain (Telegram's MTProto connects to addresses).
var networkRules = []struct {
	prefix   netip.Prefix
	category string
}{
	{netip.MustParsePrefix("91.108.4.0/22"), CategoryMessaging},
	{netip.MustParsePrefix("91.108.8.0/22"), CategoryMessaging},
	{netip.MustParsePrefix("91.108.12.0/22"), CategoryMessaging},
	{netip.MustParsePrefix("91.108.16.0/22"), CategoryMessaging},
	{netip.MustParsePrefix("91.108.20.0/22"), CategoryMessaging},
	{netip.MustParsePrefix("91.108.56.0/22"), CategoryMessaging},
	{netip.MustParsePrefix("95.161.64.0/20"), CategoryMessaging},
	{netip.MustParsePrefix("149.154.160.0/20"), CategoryMessaging},
	{netip.MustParsePrefix("185.76.151.0/24"), CategoryMessaging},
	{netip.MustParsePrefix("2001:67c:4e8::/48"), CategoryMessaging},
	{netip.MustParsePrefix("2001:b28:f23c::/47"), CategoryMessaging},
	{netip.MustParsePrefix("2001:b28:f23f::/48"), CategoryMessaging},
	{netip.MustParsePrefix("2a0a:f280::/32"), CategoryMessaging},
}

// protocolRules are the sniffed protocols that decide the category
// whatever the destination.
var protocolRules = map[string]string{
	"bittorrent": CategoryDownload,
}

func buildDomainRules(lists map[string][]string) map[string]string {
	rules := map[string]string{}
	for category, domains := range lists {
		for _, domain := range domains {
			rules[domain] = category
		}
	}
	return rules
}

// builtin is the classifier of the built-in rules alone.
var builtin = newClassifierBuilder().addBuiltin().build()

// Classify answers the category of a destination by the built-in rules
// alone; Summarize judges by the administrator's rules and the remote
// sources too (CurrentClassifier).
func Classify(domain, protocol string) string {
	return builtin.Classify(domain, protocol)
}

// IsCategory reports whether category is one of Categories.
func IsCategory(category string) bool {
	for _, known := range Categories {
		if known == category {
			return true
		}
	}
	return false
}

// Classifier judges destinations by layered rules: the built-in ones, then
// the remote sources', then the administrator's, a later layer replacing
// an earlier one's rule for the same domain, network or protocol.
type Classifier struct {
	protocols map[string]string
	domains   map[string]string
	// networks are the network rules, the longest prefix first.
	networks []networkRule
}

type networkRule struct {
	prefix   netip.Prefix
	category string
}

// Classify answers the category of a destination: its sniffed protocol's
// first, then the longest rule domain the domain ends with, then the most
// specific network rule for an address or network. A domain no rule names
// is web; a destination without a domain that no rule names is unknown.
func (c *Classifier) Classify(domain, protocol string) string {
	if category, ok := c.protocols[protocol]; ok {
		return category
	}
	if domain == "" {
		return CategoryUnknown
	}
	if prefix, err := netip.ParsePrefix(domain); err == nil {
		return c.classifyNetwork(prefix)
	}
	if address, err := netip.ParseAddr(domain); err == nil {
		return c.classifyNetwork(netip.PrefixFrom(address, address.BitLen()))
	}
	for name := strings.ToLower(domain); name != ""; {
		if category, ok := c.domains[name]; ok {
			return category
		}
		dot := strings.IndexByte(name, '.')
		if dot < 0 {
			break
		}
		name = name[dot+1:]
	}
	return CategoryWeb
}

func (c *Classifier) classifyNetwork(prefix netip.Prefix) string {
	for _, rule := range c.networks {
		if rule.prefix.Bits() <= prefix.Bits() && rule.prefix.Contains(prefix.Addr()) {
			return rule.category
		}
	}
	return CategoryUnknown
}

// classifierBuilder collects the layers of a Classifier.
type classifierBuilder struct {
	protocols map[string]string
	domains   map[string]string
	networks  map[netip.Prefix]string
}

func newClassifierBuilder() *classifierBuilder {
	return &classifierBuilder{protocols: map[string]string{}, domains: map[string]string{}, networks: map[netip.Prefix]string{}}
}

func (b *classifierBuilder) addBuiltin() *classifierBuilder {
	for protocol, category := range protocolRules {
		b.protocols[protocol] = category
	}
	for domain, category := range domainRules {
		b.domains[domain] = category
	}
	for _, rule := range networkRules {
		b.networks[rule.prefix.Masked()] = rule.category
	}
	return b
}

func (b *classifierBuilder) build() *Classifier {
	c := &Classifier{protocols: b.protocols, domains: b.domains, networks: make([]networkRule, 0, len(b.networks))}
	for prefix, category := range b.networks {
		c.networks = append(c.networks, networkRule{prefix: prefix, category: category})
	}
	sort.Slice(c.networks, func(i, j int) bool {
		a, z := c.networks[i].prefix, c.networks[j].prefix
		if a.Bits() != z.Bits() {
			return a.Bits() > z.Bits()
		}
		return a.Addr().Less(z.Addr())
	})
	return c
}
