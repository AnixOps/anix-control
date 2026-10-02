import { describe, it, expect, beforeEach, vi } from 'vitest'
import { reactive } from 'vue'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import Knowledge from '@/views/user/Knowledge.vue'
import { articleExcerpt, parseArticle } from '@/utils/articleMarkup'

const mockGetKnowledgeList = vi.fn()

vi.mock('@/api/user', () => ({
  getKnowledgeList: (...args) => mockGetKnowledgeList(...args)
}))

const route = vi.hoisted(() => ({ current: null }))
vi.mock('vue-router', async () => {
  const { h } = await vi.importActual('vue')
  const go = ({ query = {} }) => {
    route.current.query = Object.fromEntries(Object.entries(query).filter(([, value]) => value !== undefined))
  }
  const RouterLink = {
    props: ['to'],
    setup(props, { slots }) {
      return () => h('a', {
        href: `${props.to.path}?${new URLSearchParams(Object.entries(props.to.query || {}).filter(([, v]) => v !== undefined))}`,
        onClick: (event) => { event.preventDefault(); go(props.to) }
      }, slots.default?.())
    }
  }
  return { RouterLink, useRoute: () => route.current, useRouter: () => ({ replace: go, push: go }) }
})

const ARTICLES = [
  { id: 1, title: 'Billing overview', body: 'Use this to track payments.', category: 'Billing', updated_at: 1710000000 },
  {
    id: 2,
    title: 'Connectivity tips',
    body: '# Check the basics\nMake sure the **firewall** allows port 443.\n\n## DNS\n- Use 1.1.1.1\n- Or [the guide](https://example.com/dns)\n\n## Clients\n[bad](javascript:alert(1))\n\n```\nping example.com\n```',
    category: 'Networking',
    updated_at: 1710001000
  },
  { id: 3, title: 'Shadowrocket import fails', body: 'Update the subscription.', category: 'Networking', updated_at: 1710002000 },
]

function renderPage(query = {}) {
  route.current = reactive({ query })
  const user = userEvent.setup()
  render(Knowledge)
  return { user }
}

describe('User Knowledge flow', () => {
  beforeEach(() => {
    mockGetKnowledgeList.mockReset()
  })

  it('shows the search, category cards with counts and the articles', async () => {
    mockGetKnowledgeList.mockResolvedValue({ data: ARTICLES })
    const { user } = renderPage()
    expect(screen.getByRole('heading', { level: 1, name: 'Help Center' })).toBeTruthy()
    expect(screen.getByRole('searchbox', { name: 'Search help articles' })).toBeTruthy()

    const all = await screen.findByRole('button', { name: /^All 3 articles$/ })
    expect(all.getAttribute('aria-pressed')).toBe('true')
    const networking = screen.getByRole('button', { name: /^Networking 2 articles$/ })
    expect(screen.getByRole('button', { name: /^Billing 1 article$/ })).toBeTruthy()
    expect(screen.getAllByRole('link').filter(link => link.hasAttribute('data-help-article-link'))).toHaveLength(3)

    await user.click(networking)
    expect(route.current.query).toEqual({ category: 'Networking' })
    await waitFor(() => expect(networking.getAttribute('aria-pressed')).toBe('true'))
    expect(screen.getByRole('heading', { level: 2, name: 'Networking' })).toBeTruthy()
    expect(screen.queryByText('Billing overview')).toBeNull()
  })

  it('searches titles and bodies, keeps the search in the URL and can clear it', async () => {
    mockGetKnowledgeList.mockResolvedValue({ code: 0, msg: 'ok', data: ARTICLES })
    const { user } = renderPage()
    const search = await screen.findByRole('searchbox', { name: 'Search help articles' })
    await screen.findByText('Billing overview')

    await user.type(search, 'FIREWALL')
    expect(await screen.findByRole('heading', { level: 2, name: 'Results for “FIREWALL”' })).toBeTruthy()
    expect(screen.getByText('Connectivity tips')).toBeTruthy()
    expect(screen.queryByText('Billing overview')).toBeNull()
    await waitFor(() => expect(route.current.query).toEqual({ q: 'FIREWALL' }))

    await user.type(search, 'zzz')
    expect(await screen.findByText('No articles match “FIREWALLzzz”. Try another word.')).toBeTruthy()
    await user.click(screen.getByRole('button', { name: 'Clear search' }))
    expect(search.value).toBe('')
    expect(document.activeElement).toBe(search)
  })

  it('focuses the search with "/"', async () => {
    mockGetKnowledgeList.mockResolvedValue({ data: ARTICLES })
    const { user } = renderPage()
    await screen.findByText('Billing overview')
    await user.keyboard('/')
    expect(document.activeElement).toBe(screen.getByRole('searchbox'))
  })

  it('reads an article at reading width, with contents, safe links and previous / next', async () => {
    mockGetKnowledgeList.mockResolvedValue({ data: ARTICLES })
    const { user } = renderPage({ article: '2' })
    const article = await screen.findByRole('article', { name: 'Connectivity tips' })
    expect(screen.getAllByRole('heading', { level: 1 }).map(h => h.textContent)).toEqual(['Connectivity tips'])
    expect(within(article).getByRole('heading', { level: 2, name: 'Check the basics' })).toBeTruthy()
    expect(within(article).getByRole('heading', { level: 3, name: 'DNS' })).toBeTruthy()
    expect(within(article).getByText('firewall').tagName).toBe('STRONG')
    const guide = within(article).getByRole('link', { name: 'the guide' })
    expect(guide.getAttribute('href')).toBe('https://example.com/dns')
    expect(guide.getAttribute('rel')).toContain('noopener')
    // A javascript: link stays text.
    expect(within(article).queryByRole('link', { name: 'bad' })).toBeNull()
    expect(within(article).getByText('ping example.com').closest('pre')).toBeTruthy()

    const toc = within(article).getByRole('navigation', { name: 'In this article' })
    expect(within(toc).getAllByRole('link').map(link => link.textContent)).toEqual(['Check the basics', 'DNS', 'Clients'])

    const pager = within(article).getByRole('navigation', { name: 'Previous / Next' })
    expect(within(pager).getByRole('link', { name: /Previous\s*Billing overview/ })).toBeTruthy()
    await user.click(within(pager).getByRole('link', { name: /Next\s*Shadowrocket import fails/ }))
    expect(route.current.query).toEqual({ article: '3' })
    expect(await screen.findByRole('article', { name: 'Shadowrocket import fails' })).toBeTruthy()

    await user.click(screen.getByRole('link', { name: 'Help Center' }))
    expect(route.current.query).toEqual({})
    expect(await screen.findByRole('searchbox')).toBeTruthy()
  })

  it('says when an article does not exist', async () => {
    mockGetKnowledgeList.mockResolvedValue({ data: ARTICLES })
    renderPage({ article: '99' })
    expect(await screen.findByRole('heading', { level: 1, name: 'Article not found' })).toBeTruthy()
    expect(screen.getByRole('link', { name: 'Help Center' })).toBeTruthy()
  })

  it('has an empty state, and a retryable error state', async () => {
    mockGetKnowledgeList.mockResolvedValueOnce({ data: null })
    renderPage()
    expect(await screen.findByRole('heading', { level: 2, name: /No help articles yet/ })).toBeTruthy()
  })

  it('retries after a failed load', async () => {
    mockGetKnowledgeList.mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce({ data: ARTICLES })
    const { user } = renderPage()
    expect(await screen.findByRole('heading', { name: 'Could not load the help articles.' })).toBeTruthy()
    await user.click(screen.getByRole('button', { name: 'Try again' }))
    expect(await screen.findByText('Billing overview')).toBeTruthy()
  })
})

describe('articleMarkup', () => {
  it('parses the common Markdown subset into blocks, keeping unknown text as text', () => {
    const blocks = parseArticle('Intro line\nsecond line\n\n1. one\n2. two\n\n> note **here**\n\n### Deep')
    expect(blocks.map(block => block.type)).toEqual(['paragraph', 'list', 'quote', 'heading'])
    expect(blocks[0].inline).toEqual([{ type: 'text', text: 'Intro line' }, { type: 'break' }, { type: 'text', text: 'second line' }])
    expect(blocks[1]).toMatchObject({ ordered: true })
    expect(blocks[3]).toMatchObject({ level: 4, text: 'Deep', id: 'section-1' })
  })

  it('links only http(s) and mailto targets, and bare https URLs', () => {
    const [block] = parseArticle('[x](javascript:alert(1)) [y](mailto:a@b.c) see https://example.com/a.')
    expect(block.inline.filter(token => token.type === 'link').map(token => token.href)).toEqual(['mailto:a@b.c', 'https://example.com/a'])
  })

  it('makes a plain excerpt', () => {
    expect(articleExcerpt('# Title\n**Bold** and [link](https://x.y) `code`', 40)).toBe('Title Bold and link code')
    expect(articleExcerpt('a'.repeat(50), 10)).toBe(`${'a'.repeat(10)}…`)
  })
})
