import { test, expect } from '@playwright/test'

function setSession(page) {
  return page.addInitScript(() => {
    localStorage.setItem('token', 'test-token')
    localStorage.setItem('user', JSON.stringify({ id: 1, email: 'admin@example.com', role: 'admin' }))
  })
}

test.describe('Realtime UI smoke', () => {
  test('nodes page subscribes once and cleans up on route change', async ({ page }) => {
    await setSession(page)

    await page.route('**/api/v1/nodes', async route => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          success: true,
          data: [{ id: 1, name: 'node-a', host: '10.0.0.1', status: 'offline' }],
        }),
      })
    })

    await page.route('**/api/v1/tasks', async route => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ success: true, data: [] }) })
    })

    await page.route('**/api/v1/playbooks', async route => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ success: true, data: [] }) })
    })

    const subscribeCalls = []
    const unsubscribeCalls = []

    await page.route('**/api/v1/sse/subscribe', async route => {
      subscribeCalls.push(route.request().postDataJSON())
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ success: true }) })
    })

    await page.route('**/api/v1/sse/unsubscribe', async route => {
      unsubscribeCalls.push(route.request().postDataJSON())
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ success: true }) })
    })

    await page.addInitScript(() => {
      class MockEventSource {
        static instances = []

        constructor(url) {
          this.url = url
          this.readyState = 1
          this.onopen = null
          this.onerror = null
          this.onmessage = null
          this.closed = false
          MockEventSource.instances.push(this)
          window.__mockEventSource = MockEventSource
          setTimeout(() => this.onopen && this.onopen({}), 0)
        }

        emit(data) {
          this.onmessage && this.onmessage({ data: JSON.stringify(data) })
        }

        close() {
          this.closed = true
        }
      }

      window.EventSource = MockEventSource
    })

    await page.goto('/nodes')
    await expect(page.getByRole('heading', { name: 'Nodes' })).toBeVisible()
    await expect(page.locator('text=node-a')).toBeVisible()

    await page.waitForFunction(() => window.__mockEventSource?.instances?.length === 1)
    await page.waitForFunction(() => window.__mockEventSource.instances[0].url.includes('/api/v1/sse?token=test-token'))
    await page.waitForFunction(() => window.__mockEventSource.instances[0].closed === false)

    await page.evaluate(() => {
      window.__mockEventSource.instances[0].emit({
        type: 'node_update',
        payload: { node_id: 1, status: 'online' },
      })
    })

    await expect(page.getByText('online', { exact: true })).toBeVisible()
    await expect.poll(() => subscribeCalls.length).toBe(1)
    await expect.poll(() => subscribeCalls[0]?.channel).toBe('nodes')

    await page.goto('/tasks')
    await expect(page.getByRole('heading', { name: 'Tasks' })).toBeVisible()
    await expect.poll(() => subscribeCalls.map(call => call.channel)).toEqual(['nodes', 'tasks', 'logs'])
    expect(unsubscribeCalls.length).toBe(0)
  })

  test('tasks page subscribes to task and log channels and renders realtime updates', async ({ page }) => {
    await setSession(page)

    await page.route('**/api/v1/tasks', async route => {
      const url = route.request().url()
      if (/\/api\/v1\/tasks\/\d+(\/logs)?$/.test(url)) {
        await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ success: true, data: [] }) })
        return
      }

      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          success: true,
          data: [{ id: 7, playbook_name: 'Deploy', status: 'pending', target_nodes: [], created_at: '2026-03-23T00:00:00Z' }],
        }),
      })
    })

    await page.route('**/api/v1/nodes', async route => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ success: true, data: [] }) })
    })

    await page.route('**/api/v1/playbooks', async route => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ success: true, data: [] }) })
    })

    const subscribeCalls = []
    await page.route('**/api/v1/sse/subscribe', async route => {
      subscribeCalls.push(route.request().postDataJSON())
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ success: true }) })
    })

    await page.addInitScript(() => {
      class MockEventSource {
        static instances = []

        constructor(url) {
          this.url = url
          this.onopen = null
          this.onerror = null
          this.onmessage = null
          MockEventSource.instances.push(this)
          window.__mockEventSource = MockEventSource
          setTimeout(() => this.onopen && this.onopen({}), 0)
        }

        emit(data) {
          this.onmessage && this.onmessage({ data: JSON.stringify(data) })
        }

        close() {}
      }

      window.EventSource = MockEventSource
    })

    await page.goto('/tasks')
    await expect(page.getByRole('heading', { name: 'Tasks' })).toBeVisible()
    await expect(page.locator('text=Deploy')).toBeVisible()

    await page.waitForFunction(() => window.__mockEventSource?.instances?.length === 1)
    await expect.poll(() => subscribeCalls.map(call => call.channel)).toEqual(['tasks', 'logs'])

    await page.evaluate(() => {
      window.__mockEventSource.instances[0].emit({
        type: 'task_update',
        payload: { task_id: 7, status: 'running' },
      })
    })

    await expect(page.getByText('running', { exact: true })).toBeVisible()
  })
})
