package meshregion

import (
	_ "embed"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// nameTableText is the character data the name rule uses, generated from
// Unicode 15.1.0 by config/scripts/gen_mesh_name_table.py. The node
// kernel reads the same generated file, so a name gets the same label in
// both whichever Unicode version Go ships.
//
//go:embed nametable-unicode-15.1.0.txt
var nameTableText string

type nameTable struct {
	keep  [][2]rune
	lower map[rune]rune
}

var (
	nameTableOnce sync.Once
	nameTableData nameTable
)

func loadNameTable() nameTable {
	nameTableOnce.Do(func() {
		table, err := parseNameTable(nameTableText)
		if err != nil {
			panic(err)
		}
		nameTableData = table
	})
	return nameTableData
}

func parseNameTable(text string) (nameTable, error) {
	table := nameTable{lower: map[rune]rune{}}
	for number, line := range strings.Split(text, "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 3 {
			return nameTable{}, fmt.Errorf("name table line %d: %q", number+1, line)
		}
		from, err := strconv.ParseUint(fields[1], 16, 32)
		if err != nil {
			return nameTable{}, fmt.Errorf("name table line %d: %w", number+1, err)
		}
		to, err := strconv.ParseUint(fields[2], 16, 32)
		if err != nil {
			return nameTable{}, fmt.Errorf("name table line %d: %w", number+1, err)
		}
		switch fields[0] {
		case "keep":
			if to < from || len(table.keep) > 0 && rune(from) <= table.keep[len(table.keep)-1][1] {
				return nameTable{}, fmt.Errorf("name table line %d: range out of order", number+1)
			}
			table.keep = append(table.keep, [2]rune{rune(from), rune(to)})
		case "lower":
			table.lower[rune(from)] = rune(to)
		default:
			return nameTable{}, fmt.Errorf("name table line %d: %q", number+1, fields[0])
		}
	}
	if len(table.keep) == 0 || len(table.lower) == 0 {
		return nameTable{}, fmt.Errorf("name table is empty")
	}
	return table, nil
}

func (t nameTable) kept(r rune) bool {
	i := sort.Search(len(t.keep), func(i int) bool { return t.keep[i][1] >= r })
	return i < len(t.keep) && t.keep[i][0] <= r
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// normalizeName turns a node's name into its intranet name label the way
// the node kernel does (R28, docs/design/mesh-regions/README.md), with
// the character data of Unicode 15.1.0: each code point to its simple
// lowercase mapping; every code point other than an ASCII letter, digit
// or hyphen, or a non-ASCII letter (L*) or decimal digit (Nd), to a
// hyphen (combining marks and unassigned code points too); no hyphen at
// either end. An ASCII label is cut to 63 bytes. Any other label becomes
// xn-- and Punycode; while that is over 63 bytes its last character is
// dropped, trailing hyphens are removed, and it is encoded again. No
// label ends with a hyphen. It answers "" for a name with no label.
func normalizeName(raw string) string {
	table := loadNameTable()
	runes := make([]rune, 0, len(raw))
	for _, r := range raw {
		if lower, ok := table.lower[r]; ok {
			r = lower
		}
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
		case r >= utf8.RuneSelf && table.kept(r):
		default:
			r = '-'
		}
		runes = append(runes, r)
	}
	label := strings.Trim(string(runes), "-")
	for {
		if isASCII(label) {
			if len(label) > 63 {
				label = label[:63]
			}
			return strings.TrimRight(label, "-")
		}
		encoded := "xn--" + punycode(label)
		if len(encoded) <= 63 {
			return encoded
		}
		_, size := utf8.DecodeLastRuneInString(label)
		label = strings.TrimRight(label[:len(label)-size], "-")
	}
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
