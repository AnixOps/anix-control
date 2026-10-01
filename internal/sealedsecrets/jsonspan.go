package sealedsecrets

import (
	"bytes"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
)

// maxDepth bounds the nesting a request or answer document may have; the
// gateway's body limit bounds its size.
const maxDepth = 256

var errNotJSON = errors.New("not a JSON document")

type valueKind uint8

const (
	kindNull valueKind = iota
	kindBool
	kindNumber
	kindString
	kindObject
	kindArray
)

// span is one JSON value of a document and where it is: substitution and
// expansion rewrite the values they change and keep every other byte as it
// was sent.
type span struct {
	kind       valueKind
	start, end int
	// text is a string's decoded value.
	text string
	// keys are an object's member names, decoded, with their own spans in
	// keySpans; items are the member values or the array elements.
	keys     []string
	keySpans []*span
	items    []*span
}

// parseSpans parses one JSON value, with optional surrounding white space.
func parseSpans(source []byte) (*span, error) {
	if !json.Valid(source) {
		return nil, errNotJSON
	}
	parser := spanParser{source: source}
	parser.skipSpace()
	root, err := parser.value(0)
	if err != nil {
		return nil, err
	}
	parser.skipSpace()
	if parser.offset != len(source) {
		return nil, errNotJSON
	}
	return root, nil
}

type spanParser struct {
	source []byte
	offset int
}

func (p *spanParser) skipSpace() {
	for p.offset < len(p.source) {
		switch p.source[p.offset] {
		case ' ', '\t', '\r', '\n':
			p.offset++
		default:
			return
		}
	}
}

// value parses the value at the offset. The source is valid JSON, so the
// parser only finds the spans.
func (p *spanParser) value(depth int) (*span, error) {
	if depth > maxDepth {
		return nil, errNotJSON
	}
	if p.offset >= len(p.source) {
		return nil, errNotJSON
	}
	start := p.offset
	switch p.source[p.offset] {
	case '{':
		return p.object(depth)
	case '[':
		return p.array(depth)
	case '"':
		return p.str()
	case 't':
		p.offset += len("true")
		return &span{kind: kindBool, start: start, end: p.offset}, nil
	case 'f':
		p.offset += len("false")
		return &span{kind: kindBool, start: start, end: p.offset}, nil
	case 'n':
		p.offset += len("null")
		return &span{kind: kindNull, start: start, end: p.offset}, nil
	default:
		for p.offset < len(p.source) && strings.IndexByte("+-0123456789.eE", p.source[p.offset]) >= 0 {
			p.offset++
		}
		if p.offset == start {
			return nil, errNotJSON
		}
		return &span{kind: kindNumber, start: start, end: p.offset}, nil
	}
}

func (p *spanParser) str() (*span, error) {
	start := p.offset
	p.offset++
	for p.offset < len(p.source) {
		switch p.source[p.offset] {
		case '\\':
			p.offset += 2
			continue
		case '"':
			p.offset++
			var text string
			if err := json.Unmarshal(p.source[start:p.offset], &text); err != nil {
				return nil, errNotJSON
			}
			return &span{kind: kindString, start: start, end: p.offset, text: text}, nil
		}
		p.offset++
	}
	return nil, errNotJSON
}

func (p *spanParser) object(depth int) (*span, error) {
	node := &span{kind: kindObject, start: p.offset}
	p.offset++
	p.skipSpace()
	if p.offset < len(p.source) && p.source[p.offset] == '}' {
		p.offset++
		node.end = p.offset
		return node, nil
	}
	for {
		p.skipSpace()
		key, err := p.str()
		if err != nil {
			return nil, err
		}
		p.skipSpace()
		if p.offset >= len(p.source) || p.source[p.offset] != ':' {
			return nil, errNotJSON
		}
		p.offset++
		p.skipSpace()
		item, err := p.value(depth + 1)
		if err != nil {
			return nil, err
		}
		node.keys, node.keySpans, node.items = append(node.keys, key.text), append(node.keySpans, key), append(node.items, item)
		p.skipSpace()
		if p.offset >= len(p.source) {
			return nil, errNotJSON
		}
		switch p.source[p.offset] {
		case ',':
			p.offset++
		case '}':
			p.offset++
			node.end = p.offset
			return node, nil
		default:
			return nil, errNotJSON
		}
	}
}

func (p *spanParser) array(depth int) (*span, error) {
	node := &span{kind: kindArray, start: p.offset}
	p.offset++
	p.skipSpace()
	if p.offset < len(p.source) && p.source[p.offset] == ']' {
		p.offset++
		node.end = p.offset
		return node, nil
	}
	for {
		p.skipSpace()
		item, err := p.value(depth + 1)
		if err != nil {
			return nil, err
		}
		node.items = append(node.items, item)
		p.skipSpace()
		if p.offset >= len(p.source) {
			return nil, errNotJSON
		}
		switch p.source[p.offset] {
		case ',':
			p.offset++
		case ']':
			p.offset++
			node.end = p.offset
			return node, nil
		default:
			return nil, errNotJSON
		}
	}
}

// edit replaces source[start:end] with text.
type edit struct {
	start, end int
	text       []byte
}

// applyEdits returns source with the edits applied; they never overlap.
func applyEdits(source []byte, edits []edit) []byte {
	if len(edits) == 0 {
		return source
	}
	sort.Slice(edits, func(i, j int) bool { return edits[i].start < edits[j].start })
	var out bytes.Buffer
	out.Grow(len(source) + 64*len(edits))
	last := 0
	for _, change := range edits {
		out.Write(source[last:change.start])
		out.Write(change.text)
		last = change.end
	}
	out.Write(source[last:])
	return out.Bytes()
}

// encodeString is a JSON string as the kernel's handlers write one: without
// HTML escaping.
func encodeString(value string) []byte {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		// A Go string always encodes; keep the call total anyway.
		return []byte(strconv.Quote(value))
	}
	return bytes.TrimSuffix(buffer.Bytes(), []byte("\n"))
}

// pointerSegment escapes one RFC 6901 reference token.
func pointerSegment(token string) string {
	return strings.ReplaceAll(strings.ReplaceAll(token, "~", "~0"), "/", "~1")
}

// joinPointer is the RFC 6901 pointer of a path of reference tokens.
func joinPointer(tokens []string) string {
	var builder strings.Builder
	for _, token := range tokens {
		builder.WriteByte('/')
		builder.WriteString(pointerSegment(token))
	}
	return builder.String()
}

// splitPointer reads an RFC 6901 pointer: "" is the whole document.
func splitPointer(pointer string) ([]string, error) {
	if pointer == "" {
		return nil, nil
	}
	if !strings.HasPrefix(pointer, "/") {
		return nil, errors.New("a JSON pointer starts with /")
	}
	tokens := strings.Split(pointer[1:], "/")
	for index, token := range tokens {
		if strings.Contains(strings.ReplaceAll(strings.ReplaceAll(token, "~0", ""), "~1", ""), "~") {
			return nil, errors.New("a JSON pointer escapes ~ as ~0 and / as ~1")
		}
		tokens[index] = strings.ReplaceAll(strings.ReplaceAll(token, "~1", "/"), "~0", "~")
	}
	return tokens, nil
}
