package meshregion

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// normalizeName turns a node's name into its intranet name label the way
// the node kernel does (R28): letters to lowercase, a name with other than
// ASCII characters to its IDNA form (xn-- and Punycode), every character
// other than a letter, digit or hyphen to a hyphen, no hyphen at either
// end, at most 63 bytes. It answers "" for a name with no label.
func normalizeName(raw string) string {
	var builder strings.Builder
	ascii := true
	for _, r := range strings.ToLower(strings.TrimSpace(raw)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			builder.WriteRune(r)
		case r < utf8.RuneSelf || !(unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r)):
			builder.WriteByte('-')
		default:
			ascii = false
			builder.WriteRune(r)
		}
		if builder.Len() > 4*63 {
			break
		}
	}
	label := strings.Trim(builder.String(), "-")
	if !ascii && label != "" {
		label = "xn--" + punycode(label)
	}
	if len(label) > 63 {
		label = label[:63]
	}
	return strings.Trim(label, "-")
}

// Punycode (RFC 3492) parameters.
const (
	punyBase        = 36
	punyTMin        = 1
	punyTMax        = 26
	punySkew        = 38
	punyDamp        = 700
	punyInitialBias = 72
	punyInitialN    = 128
)

// punycode encodes s (RFC 3492 section 6.3).
func punycode(s string) string {
	runes := []rune(s)
	out := make([]byte, 0, len(s)+8)
	for _, r := range runes {
		if r < utf8.RuneSelf {
			out = append(out, string(r)...)
		}
	}
	basic := len(out)
	handled := basic
	if basic > 0 {
		out = append(out, '-')
	}
	n, delta, bias := punyInitialN, 0, punyInitialBias
	for handled < len(runes) {
		next := int(unicode.MaxRune) + 1
		for _, r := range runes {
			if int(r) >= n && int(r) < next {
				next = int(r)
			}
		}
		delta += (next - n) * (handled + 1)
		n = next
		for _, r := range runes {
			if int(r) < n {
				delta++
			}
			if int(r) != n {
				continue
			}
			q := delta
			for k := punyBase; ; k += punyBase {
				t := min(max(k-bias, punyTMin), punyTMax)
				if q < t {
					break
				}
				out = append(out, punyDigit(t+(q-t)%(punyBase-t)))
				q = (q - t) / (punyBase - t)
			}
			out = append(out, punyDigit(q))
			bias = punyAdapt(delta, handled+1, handled == basic)
			delta = 0
			handled++
		}
		delta++
		n++
	}
	return string(out)
}

const punyDigits = "abcdefghijklmnopqrstuvwxyz0123456789"

func punyDigit(d int) byte { return punyDigits[d] }

func punyAdapt(delta, points int, first bool) int {
	if first {
		delta /= punyDamp
	} else {
		delta /= 2
	}
	delta += delta / points
	k := 0
	for delta > ((punyBase-punyTMin)*punyTMax)/2 {
		delta /= punyBase - punyTMin
		k += punyBase
	}
	return k + (punyBase-punyTMin+1)*delta/(delta+punySkew)
}
