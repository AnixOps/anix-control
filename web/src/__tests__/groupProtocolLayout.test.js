// The order and display names of a subscription group's node protocols
// (views/admin/subscriptions/groupProtocolLayout.js) and the sheet that
// edits them.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { setLocale } from '@/i18n'
import GroupProtocolLayoutSheet from '@/views/admin/subscriptions/GroupProtocolLayoutSheet.vue'
import { applyProtocolLayout, defaultProtocolName, layoutRequest, moveItem } from '@/views/admin/subscriptions/groupProtocolLayout'

const saveLayoutMock = vi.hoisted(() => vi.fn())
vi.mock('@/api/admin', () => ({
  default: {},
  saveGroupProtocolLayout: (...args) => saveLayoutMock(...args)
}))

enableAutoUnmount(afterEach)

const PROTOCOLS = [
  { id: 1, node_id: 5, node: { name: 'HK' }, name: 'ss', type: 'shadowsocks', port: 1, display_name: '' },
  { id: 2, node_id: 6, node: { name: 'US' }, name: 'ss', type: 'shadowsocks', port: 2, display_name: '美国' },
  { id: 3, node_id: 7, node: null, name: '', type: 'vless', port: 443, display_name: '' }
]

describe('group protocol layout', () => {
  it('orders the rows by the layout, the others by id', () => {
    const rows = applyProtocolLayout([{ id: 4 }, { id: 2 }, { id: 3 }, { id: 1 }], [{ protocol_id: 3, display_name: 'C' }, { protocol_id: 1 }])
    expect(rows.map(row => [row.id, row.display_name])).toEqual([[3, 'C'], [1, ''], [2, ''], [4, '']])
    expect(applyProtocolLayout([{ id: 2 }, { id: 1 }], null).map(row => row.id)).toEqual([1, 2])
  })

  it('names a protocol as the subscription does by default', () => {
    expect(defaultProtocolName(PROTOCOLS[0])).toBe('HK - ss')
    expect(defaultProtocolName(PROTOCOLS[2])).toBe('#7 - ')
  })

  it('moves an item and builds the request', () => {
    expect(moveItem([1, 2, 3], 2, 0)).toEqual([3, 1, 2])
    expect(moveItem([1, 2, 3], 0, 5)).toEqual([1, 2, 3])
    expect(layoutRequest([{ id: 2, display_name: ' 美国 ' }, { id: 1 }])).toEqual([{ protocol_id: 2, display_name: '美国' }, { protocol_id: 1, display_name: '' }])
  })
})

describe('GroupProtocolLayoutSheet', () => {
  beforeEach(async () => {
    vi.resetAllMocks()
    setActivePinia(createPinia())
    await setLocale('en')
  })

  function mountSheet() {
    return mount(GroupProtocolLayoutSheet, {
      props: { open: true, group: { id: 9, name: 'ok' }, protocols: PROTOCOLS },
      attachTo: document.body
    })
  }

  it('reorders by drag and by the arrow buttons, then saves the whole list', async () => {
    saveLayoutMock.mockResolvedValueOnce({ group_id: 9, protocols: [{ protocol_id: 3, display_name: 'Oracle US' }] })
    const wrapper = mountSheet()
    await flushPromises()

    // Drag the last row onto the first.
    wrapper.vm.onDragStart({ dataTransfer: { setData() {} } }, 2)
    wrapper.vm.onDragEnter(0)
    wrapper.vm.onDragEnd()
    expect(wrapper.vm.rows.map(row => row.id)).toEqual([3, 1, 2])

    wrapper.vm.move(2, 1)
    expect(wrapper.vm.rows.map(row => row.id)).toEqual([3, 2, 1])
    expect(wrapper.vm.announcement).toContain('position 2 of 3')

    wrapper.vm.rows[0].display_name = '  Oracle US '
    await wrapper.vm.save()
    await flushPromises()
    expect(saveLayoutMock).toHaveBeenCalledWith(9, [
      { protocol_id: 3, display_name: 'Oracle US' },
      { protocol_id: 2, display_name: '美国' },
      { protocol_id: 1, display_name: '' }
    ])
    expect(wrapper.emitted('saved')[0][0]).toEqual([{ protocol_id: 3, display_name: 'Oracle US' }])
    expect(wrapper.emitted('update:open').at(-1)).toEqual([false])
  })

  it('refuses a name with a line break and keeps the sheet open on a server error', async () => {
    const wrapper = mountSheet()
    await flushPromises()
    wrapper.vm.rows[0].display_name = 'a\nb'
    await wrapper.vm.save()
    expect(saveLayoutMock).not.toHaveBeenCalled()

    wrapper.vm.rows[0].display_name = 'ok'
    saveLayoutMock.mockRejectedValueOnce({ response: { data: { error: { message: 'protocol 1 is not linked to the group' } } } })
    await wrapper.vm.save()
    await flushPromises()
    expect(wrapper.vm.saveError).toBe('protocol 1 is not linked to the group')
    expect(wrapper.emitted('update:open')).toBeUndefined()
  })
})
