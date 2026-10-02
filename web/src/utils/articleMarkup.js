// Help-center article bodies are plain text that administrators often write
// in Markdown. parseArticle turns the common subset into blocks a component
// renders as elements (never as HTML, so a body cannot inject markup):
// headings (#, ##, ###), paragraphs (single line breaks kept), bulleted and
// numbered lists, fenced code, quotes, and inline **bold**, `code` and links
// ([text](https://…) or a bare https:// URL). Anything else stays text.

const SAFE_LINK = /^(https?:\/\/|mailto:)/i

// Inline tokens: { type: 'text' | 'strong' | 'code' | 'link' | 'break', text, href }
export function parseInline(source) {
  const out = []
  const pattern = /(`[^`\n]+`)|(\*\*[^*\n]+\*\*)|(\[[^\]\n]+\]\([^)\s]+\))|(https?:\/\/[^\s<>()]+[^\s<>().,;:!?，。；：！？）])|(\n)/g
  let last = 0
  let match
  while ((match = pattern.exec(source)) !== null) {
    if (match.index > last) out.push({ type: 'text', text: source.slice(last, match.index) })
    const [token] = match
    if (match[1]) {
      out.push({ type: 'code', text: token.slice(1, -1) })
    } else if (match[2]) {
      out.push({ type: 'strong', text: token.slice(2, -2) })
    } else if (match[3]) {
      const split = token.indexOf('](')
      const text = token.slice(1, split)
      const href = token.slice(split + 2, -1)
      out.push(SAFE_LINK.test(href) ? { type: 'link', text, href } : { type: 'text', text })
    } else if (match[4]) {
      out.push({ type: 'link', text: token, href: token })
    } else {
      out.push({ type: 'break' })
    }
    last = match.index + token.length
  }
  if (last < source.length) out.push({ type: 'text', text: source.slice(last) })
  return out
}

function slug(index) {
  return `section-${index + 1}`
}

// Blocks: { type: 'heading', level (2–4), text, id } | { type: 'paragraph',
// inline } | { type: 'list', ordered, items: [inline] } | { type: 'code', text }
// | { type: 'quote', inline }
export function parseArticle(body) {
  const lines = String(body || '').replace(/\r\n?/g, '\n').split('\n')
  const blocks = []
  let paragraph = []
  let list = null
  let quote = []
  let headings = 0

  function flushParagraph() {
    if (paragraph.length) blocks.push({ type: 'paragraph', inline: parseInline(paragraph.join('\n')) })
    paragraph = []
  }
  function flushList() {
    if (list) blocks.push(list)
    list = null
  }
  function flushQuote() {
    if (quote.length) blocks.push({ type: 'quote', inline: parseInline(quote.join('\n')) })
    quote = []
  }
  function flushAll() {
    flushParagraph()
    flushList()
    flushQuote()
  }

  for (let index = 0; index < lines.length; index += 1) {
    const line = lines[index]
    const trimmed = line.trim()

    if (/^```/.test(trimmed)) {
      flushAll()
      const code = []
      index += 1
      while (index < lines.length && !/^```/.test(lines[index].trim())) {
        code.push(lines[index])
        index += 1
      }
      blocks.push({ type: 'code', text: code.join('\n') })
      continue
    }

    const heading = /^(#{1,6})\s+(.+?)\s*#*$/.exec(trimmed)
    if (heading) {
      flushAll()
      blocks.push({ type: 'heading', level: Math.min(4, heading[1].length + 1), text: heading[2], id: slug(headings) })
      headings += 1
      continue
    }

    const bullet = /^[-*+]\s+(.*)$/.exec(trimmed)
    const numbered = /^\d+[.)]\s+(.*)$/.exec(trimmed)
    if (bullet || numbered) {
      flushParagraph()
      flushQuote()
      const ordered = Boolean(numbered)
      if (!list || list.ordered !== ordered) {
        flushList()
        list = { type: 'list', ordered, items: [] }
      }
      list.items.push(parseInline((bullet || numbered)[1]))
      continue
    }

    const quoted = /^>\s?(.*)$/.exec(trimmed)
    if (quoted) {
      flushParagraph()
      flushList()
      quote.push(quoted[1])
      continue
    }

    if (!trimmed) {
      flushAll()
      continue
    }

    flushList()
    flushQuote()
    paragraph.push(trimmed)
  }
  flushAll()
  return blocks
}

// A one-line excerpt without Markdown marks.
export function articleExcerpt(body, length = 120) {
  const text = String(body || '')
    .replace(/```[\s\S]*?```/g, ' ')
    .replace(/\[([^\]]+)\]\([^)]+\)/g, '$1')
    .replace(/^\s*(#{1,6}|>|[-*+]|\d+[.)])\s+/gm, '')
    .replace(/[*`_]+/g, '')
    .replace(/\s+/g, ' ')
    .trim()
  return text.length > length ? `${text.slice(0, length).trimEnd()}…` : text
}
