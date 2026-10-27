// 入口 of a node: the port map text, and the section saving the whole list.
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import NodeEntriesSection from '@/views/admin/nodes/NodeEntriesSection.vue'
import UiHost from '@/ui/UiHost.vue'
import { setLocale } from '@/i18n'
import { entriesRequest, entryForm, formatPortMap, lineForm, lineRequest, parsePortMap, validateEntries, validateLine } from '@/views/admin/nodes/nodeEntriesModel'

const kernelApi = vi.hoisted(() => ({
  getKernelNodeEntries: vi.fn(),
  replaceKernelNodeEntries: vi.fn(),
  listIWANLines: vi.fn(),
  createIWANLine: vi.fn(),
  updateIWANLine: vi.fn(),
  deleteIWANLine: vi.fn()
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

describe('iWAN entries and lines', () => {
  it('needs a line for an iwan entry and sends its id', () => {
    const iwan = { ...entryForm(null), label: 'via HK', kind: 'iwan', host: '198.51.100.20' }
    expect(validateEntries([iwan], MESSAGES)[iwan.key]).toEqual({ line: 'required' })
    iwan.lineId = 3
    expect(validateEntries([iwan], MESSAGES)).toEqual({})
    expect(entriesRequest([iwan])).toEqual([{ label: 'via HK', kind: 'iwan', host: '198.51.100.20', port_map: [], enabled: true, iwan_line_id: 3 }])
    expect(entryForm({ label: 'via HK', kind: 'iwan', host: 'h', iwan_line_id: 3 }).lineId).toBe(3)
  })

  it('validates a line and keeps the password on an edit', () => {
    const messages = { required: 'required', noPipe: 'pipe', port: 'port', mtu: 'mtu' }
    const created = { ...lineForm(null), name: 'HK', host: '203.0.113.10', port: 4567, username: 'u' }
    expect(validateLine(created, messages)).toEqual({ password: 'required' })
    created.password = 'p'
    created.mtu = 100
    expect(validateLine(created, messages)).toEqual({ mtu: 'mtu' })
    const edited = lineForm({ id: 3, name: 'HK', host: '203.0.113.10', port: 4567, username: 'u', mtu: 0, encrypt: true })
    expect(validateLine(edited, messages)).toEqual({})
    expect(lineRequest(edited)).toEqual({ name: 'HK', host: '203.0.113.10', port: 4567, username: 'u', password: '', mtu: 0, encrypt: true })
  })
})

describe('NodeEntriesSection', () => {
  beforeEach(async () => {
    await setLocale('zh-CN')
    Object.values(kernelApi).forEach(fn => fn.mockReset())
    kernelApi.listIWANLines.mockResolvedValue({ lines: [] })
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

  it('warns about protocols no entry offers', async () => {
    const nat = { id: 3, label: 'BGP', kind: 'nat', host: '203.0.113.5', family: 'ipv4', port_map: [{ from: 55556, to: 55557, external: 51820 }], enabled: true, sort: 0 }
    kernelApi.getKernelNodeEntries.mockResolvedValue({ node_id: 58, entries: [nat], uncovered_protocols: [{ id: 47, name: 'SS', type: 'shadowsocks', port: 11111 }] })
    render({
      components: { NodeEntriesSection, UiHost },
      template: '<div><NodeEntriesSection :node="{ id: 58, name: \'le sh\', host: \'sh.example.com\' }" /><UiHost /></div>'
    })
    const notice = await screen.findByTestId('node-entries-uncovered')
    expect(notice.textContent).toContain('SS :11111')
  })

  it('names the line of an iwan entry and adds lines from its sheet', async () => {
    const iwan = { id: 4, label: 'via HK', kind: 'iwan', host: '198.51.100.20', family: 'ipv4', port_map: [], enabled: true, sort: 0, iwan_line_id: 3 }
    kernelApi.getKernelNodeEntries.mockResolvedValue({ node_id: 70, entries: [iwan] })
    kernelApi.listIWANLines.mockResolvedValue({ lines: [{ id: 3, name: 'HK iWAN', host: '203.0.113.10', port: 4567, username: 'u', mtu: 0, encrypt: false, entries: 1 }] })
    kernelApi.createIWANLine.mockResolvedValue({ line: { id: 5 } })
    render({
      components: { NodeEntriesSection, UiHost },
      template: '<div><NodeEntriesSection :node="{ id: 70, name: \'US\', host: \'198.51.100.20\' }" /><UiHost /></div>'
    })
    expect(await screen.findByText('经由 HK iWAN')).toBeTruthy()

    const user = userEvent.setup()
    await user.click(screen.getByTestId('edit-iwan-lines'))
    expect(await screen.findByTestId('iwan-line-3')).toBeTruthy()
    await user.click(screen.getByTestId('add-iwan-line'))
    const form = await screen.findByTestId('iwan-line-form')
    const fields = form.querySelectorAll('input')
    await user.type(fields[0], 'JP iWAN')
    await user.type(fields[1], '203.0.113.20')
    await user.type(fields[2], '4568')
    await user.type(fields[3], 'shared')
    await user.type(fields[4], 'secret')
    await user.click(screen.getByTestId('iwan-line-save'))
    await waitFor(() => expect(kernelApi.createIWANLine).toHaveBeenCalledTimes(1))
    expect(kernelApi.createIWANLine).toHaveBeenCalledWith({ name: 'JP iWAN', host: '203.0.113.20', port: 4568, username: 'shared', password: 'secret', mtu: 0, encrypt: false })
    await waitFor(() => expect(kernelApi.listIWANLines).toHaveBeenCalledTimes(2))
  })
})
