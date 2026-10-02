<script>
// Renders the blocks of utils/articleMarkup.js as elements (no v-html):
// links open in a new tab with noopener; headings carry ids for the
// article's table of contents and take focus when it jumps to them.
import { h } from 'vue'

function inline(tokens) {
  return tokens.map((token) => {
    switch (token.type) {
      case 'strong': return h('strong', token.text)
      case 'code': return h('code', token.text)
      case 'break': return h('br')
      case 'link': return h('a', { href: token.href, target: '_blank', rel: 'noopener noreferrer' }, token.text)
      default: return token.text
    }
  })
}

export default {
  name: 'ArticleBody',
  props: {
    blocks: { type: Array, default: () => [] }
  },
  setup(props) {
    return () => h('div', { class: 'article-body' }, props.blocks.map((block, index) => {
      switch (block.type) {
        case 'heading':
          return h(`h${block.level}`, { key: index, id: block.id, tabindex: '-1' }, block.text)
        case 'list':
          return h(block.ordered ? 'ol' : 'ul', { key: index }, block.items.map((item, itemIndex) => h('li', { key: itemIndex }, inline(item))))
        case 'code':
          return h('pre', { key: index, tabindex: '0' }, [h('code', block.text)])
        case 'quote':
          return h('blockquote', { key: index }, [h('p', inline(block.inline))])
        default:
          return h('p', { key: index }, inline(block.inline))
      }
    }))
  }
}
</script>

<style scoped>
.article-body {
  font-size: var(--type-body-size);
  line-height: 1.75;
}

.article-body > * + * {
  margin-top: var(--space-4);
}

.article-body > :first-child {
  margin-top: 0;
}

.article-body :deep(h2),
.article-body :deep(h3),
.article-body :deep(h4) {
  margin-top: var(--space-10);
  scroll-margin-top: calc(48px + var(--space-6));
}

.article-body :deep(h2) {
  font-size: var(--type-title-2-size);
  font-weight: var(--type-title-2-weight);
  line-height: var(--type-title-2-line);
}

.article-body :deep(h3) {
  font-size: var(--type-title-3-size);
  font-weight: var(--type-title-3-weight);
  line-height: var(--type-title-3-line);
}

.article-body :deep(h4) {
  font-size: var(--type-body-size);
  font-weight: var(--weight-semibold);
}

.article-body :deep(:is(h2, h3, h4):focus) {
  outline: none;
}

.article-body :deep(ul),
.article-body :deep(ol) {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding-left: var(--space-6);
}

.article-body :deep(code) {
  padding: 1px var(--space-1);
  border-radius: var(--radius-xs);
  background: var(--fill-1);
  font-family: var(--font-mono);
  font-size: var(--type-callout-size);
}

.article-body :deep(pre) {
  overflow-x: auto;
  padding: var(--space-4);
  border-radius: var(--radius-sm);
  background: var(--bg-grouped);
  font-size: var(--type-callout-size);
  line-height: 1.6;
}

.article-body :deep(pre code) {
  padding: 0;
  background: none;
  font-size: inherit;
}

.article-body :deep(pre:focus-visible) {
  outline: 2px solid var(--accent);
  outline-offset: 2px;
}

.article-body :deep(blockquote) {
  padding: var(--space-3) var(--space-4);
  border-left: 3px solid var(--accent);
  border-radius: 0 var(--radius-xs) var(--radius-xs) 0;
  background: var(--accent-soft);
  color: var(--label-1);
}

.article-body :deep(a) {
  overflow-wrap: anywhere;
}
</style>
