// 入口 of a node: the port map text, and the section saving the whole list.
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import NodeEntriesSection from '@/views/admin/nodes/NodeEntriesSection.vue'
import UiHost from '@/ui/UiHost.vue'
import { setLocale } from '@/i18n'
import { entriesRequest, entryForm, formatPortMap, parsePortMap, validateEntries } from '@/views/admin/nodes/nodeEntriesModel'

const kernelApi = vi.hoisted(() => ({
  getKernelNodeEntries: vi.fn(),
  replaceKernelNodeEntries: vi.fn()
}))

vi.mock('@/api/kernel', () => kernelApi)

const MESSAGES = { required: 'required', noPipe: 'pipe', duplicate: 'dup', badLine: line => `line ${line}` }

describe('nodeEntriesModel', () => {
  it('parses public -> machine ranges and formats them back', () => {
    const { ranges, error } = parsePortMap('21000-21099 -> 30000-30099\n\n20443 = 443')
    expect(error).toBeNull()
    expect(ranges).toEqual([{ from: 30000, to: 30099, external: 21000 }, { from: 443, to: 443, external: 20443 }])
    expect(formatPortMap(ranges)).toBe('21000-21099 -> 30000-30099\n20443 -> 443')
  })

  it('names the line that does not parse or whose sides differ', () => {
    expect(parsePortMap('20443 -> 443\n21000-21010 -> 30000-30099').error).toBe(2)
    expect(parsePortMap('70000 -> 443').error).toBe(1)
    expect(parsePortMap('443').error).toBe(1)
  })

  it('validates labels, hosts and NAT maps and builds the request', () => {
    const v6 = { ...entryForm(null), label: 'CN2 v6', host: '2001:db8::10' }
    const nat = { ...entryForm(null), label: 'CN2 NAT', kind: 'nat', host: '203.0.113.7', portMap: '21000-21099 -> 30000-30099' }
    expect(validateEntries([v6, nat], MESSAGES)).toEqual({})
    const twice = { ...v6, key: 999 }
    expect(validateEntries([v6, twice], MESSAGES)[999]).toEqual({ label: 'dup' })
    const empty = { ...entryForm(null), kind: 'nat' }
    expect(validateEntries([empty], MESSAGES)[empty.key]).toEqual({ label: 'required', host: 'required', portMap: 'required' })
    expect(entriesRequest([v6, nat])).toEqual([
      { label: 'CN2 v6', kind: 'direct', host: '2001:db8::10', port_map: [], enabled: true },
      { label: 'CN2 NAT', kind: 'nat', host: '203.0.113.7', port_map: [{ from: 30000, to: 30099, external: 21000 }], enabled: true }
    ])
  })
})

describe('NodeEntriesSection', () => {
  beforeEach(async () => {
    await setLocale('zh-CN')
    Object.values(kernelApi).forEach(fn => fn.mockReset())
  })

  it('lists the entries and replaces the list from the sheet', async () => {
    const existing = { id: 1, label: 'CN2 v6', kind: 'direct', host: '2001:db8::10', family: 'ipv6', port_map: [], enabled: true, sort: 0 }
    kernelApi.getKernelNodeEntries.mockResolvedValue({ node_id: 60, entries: [existing] })
    kernelApi.replaceKernelNodeEntries.mockImplementation(async (_node, entries) => ({
      node_id: 60,
      entries: entries.map((entry, i) => ({ id: i + 1, family: 'ipv6', sort: i, ...entry }))
    }))
    render({
      components: { NodeEntriesSection, UiHost },
      template: '<div><NodeEntriesSection :node="{ id: 60, name: \'SH\', host: \'relay.example.com\' }" /><UiHost /></div>'
    })
    expect(await screen.findByText('2001:db8::10')).toBeTruthy()
    expect(kernelApi.getKernelNodeEntries).toHaveBeenCalledWith('proxy-60')

    const user = userEvent.setup()
    await user.click(screen.getByTestId('edit-node-entries'))
    await user.click(await screen.findByTestId('add-node-entry'))
    const labels = screen.getAllByLabelText(/名称/)
    await user.type(labels[labels.length - 1], '9929 v6')
    const hosts = screen.getAllByLabelText(/地址/)
    await user.type(hosts[hosts.length - 1], '2001:db8:1::20')
    await user.click(screen.getByTestId('node-entries-save'))

    await waitFor(() => expect(kernelApi.replaceKernelNodeEntries).toHaveBeenCalledTimes(1))
    expect(kernelApi.replaceKernelNodeEntries).toHaveBeenCalledWith('proxy-60', [
      { label: 'CN2 v6', kind: 'direct', host: '2001:db8::10', port_map: [], enabled: true },
      { label: '9929 v6', kind: 'direct', host: '2001:db8:1::20', port_map: [], enabled: true }
    ])
    expect(await screen.findByText('2001:db8:1::20')).toBeTruthy()
  })
})
