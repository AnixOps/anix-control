import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { nextTick } from 'vue'
import ForwardSuiteNav from '@/components/admin/ForwardSuiteNav.vue'

const routes = [
  { path: '/admin/forward', component: { template: '<div />' } },
  { path: '/admin/forward/tunnel', component: { template: '<div />' } },
  { path: '/admin/forward/limit', component: { template: '<div />' } },
  { path: '/admin/forward/ansible-machines', component: { template: '<div />' } },
  { path: '/admin/forward/local', component: { template: '<div />' } },
  { path: '/admin/forward/nodes', component: { template: '<div />' } },
  { path: '/admin/forward/nodex', component: { template: '<div />' } },
  { path: '/admin/forward/agents', component: { template: '<div />' } },
  { path: '/admin/forward/observability', component: { template: '<div />' } }
]

function createTestRouter(startPath = '/admin/forward') {
  const router = createRouter({
    history: createMemoryHistory(),
    routes
  })
  router.push(startPath)
  return router
}

describe('ForwardSuiteNav.vue', () => {
  it('renders structured navigation semantics and descriptive link labels', async () => {
    const router = createTestRouter()
    await router.isReady()

    const wrapper = mount(ForwardSuiteNav, {
      global: {
        plugins: [router]
      }
    })

    const nav = wrapper.find('nav.forward-suite-nav')
    const listItems = wrapper.findAll('li.forward-suite-item')
    const ansibleLink = wrapper.find('a[href="/admin/forward/ansible-machines"]')

    expect(nav.exists()).toBe(true)
    expect(nav.attributes('aria-label')).toContain('Forward')
    expect(wrapper.find('ul.forward-suite-list').exists()).toBe(true)
    expect(listItems).toHaveLength(9)
    expect(ansibleLink.attributes('aria-label')).toContain('Stateless execution machines')
  })

  it('exposes clear active-state semantics via active class and aria-current', async () => {
    const router = createTestRouter('/admin/forward/nodex')
    await router.isReady()

    const wrapper = mount(ForwardSuiteNav, {
      global: {
        plugins: [router]
      }
    })

    await nextTick()

    const activeLink = wrapper.find('a[href="/admin/forward/nodex"]')
    expect(activeLink.classes()).toContain('forward-suite-link-active')
    expect(activeLink.attributes('aria-current')).toBe('page')
  })
})
