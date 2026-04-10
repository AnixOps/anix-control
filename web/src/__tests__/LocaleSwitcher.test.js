import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'

const mockI18n = vi.hoisted(() => {
  const makeRef = (initialValue) => ({ __v_isRef: true, value: initialValue })
  const currentLocale = makeRef('en')
  const localeOptions = makeRef([
    { value: 'en', label: 'English', shortLabel: 'EN' },
    { value: 'zh-CN', label: 'Simplified Chinese', shortLabel: '中' }
  ])
  const switchLocale = vi.fn((value) => {
    currentLocale.value = value
  })
  const messages = {
    'common.locale.label': 'Language',
    'common.locale.switch': 'Switch language'
  }

  return {
    currentLocale,
    localeOptions,
    switchLocale,
    t: (key) => messages[key] || key
  }
})

vi.mock('@/composables/useAppI18n', () => ({
  useAppI18n: () => mockI18n
}))

describe('LocaleSwitcher.vue', () => {
  beforeEach(() => {
    mockI18n.currentLocale.value = 'en'
    mockI18n.switchLocale.mockClear()
  })

  it('renders as an accessible radio group', () => {
    const wrapper = mount(LocaleSwitcher, {
      props: { compact: true }
    })

    const group = wrapper.find('.locale-switcher')
    const options = wrapper.findAll('button.locale-option')

    expect(group.attributes('role')).toBe('radiogroup')
    expect(group.attributes('aria-label')).toBe('Language')
    expect(options).toHaveLength(2)
    expect(options[0].attributes('role')).toBe('radio')
    expect(options[0].attributes('aria-checked')).toBe('true')
    expect(options[0].attributes('tabindex')).toBe('0')
    expect(options[1].text()).toBe('中')
    expect(options[1].attributes('aria-label')).toContain('Simplified Chinese')
  })

  it('supports arrow/home/end keyboard navigation and selection', async () => {
    const wrapper = mount(LocaleSwitcher)
    const options = wrapper.findAll('button.locale-option')

    await options[0].trigger('keydown', { key: 'ArrowRight' })
    expect(mockI18n.switchLocale).toHaveBeenLastCalledWith('zh-CN')

    await options[1].trigger('keydown', { key: 'Home' })
    expect(mockI18n.switchLocale).toHaveBeenLastCalledWith('en')

    await options[0].trigger('keydown', { key: 'End' })
    expect(mockI18n.switchLocale).toHaveBeenLastCalledWith('zh-CN')
  })
})
