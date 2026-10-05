import { beforeEach, describe, expect, it } from 'vitest'
import { setLocale } from '@/i18n'
import { useBulkReport } from '@/composables/useBulkReport'
import { readBulkResult } from '@/utils/bulkResult'
import { runAction } from '@/ui/composables/useToast'
import { toasts } from './helpers/feedback'

// The toast that closes a bulk action: success (with undo), or what was done,
// why the rest was not (a count per cause) and Retry for the ids worth it.
const outcomeOf = results => readBulkResult({ results }, results.map(item => item.id))
const ok = id => ({ id, ok: true })
const refused = (id, code, message = '') => ({ id, ok: false, error: { code, message } })

describe('useBulkReport', () => {
  beforeEach(async () => {
    await setLocale('en')
  })

  it('words every error code the bulk routes answer with, in a fixed order', () => {
    const { failureText } = useBulkReport()
    const failed = outcomeOf([
      refused(1, 'failed', 'database is locked'), refused(2, 'not_attempted'), refused(3, 'conflict'),
      refused(4, 'not_found'), refused(5, 'forbidden_self'), refused(6, 'not_found'), refused(7, 'plugin_host_unavailable', 'no host')
    ]).failed
    expect(failureText(failed)).toBe('2 not found; 1 already used and kept; your own account can’t be banned; 1 not attempted (out of time); 2 failed (database is locked).')
    expect(failureText([])).toBe('')
  })

  it('words the same codes in Chinese', async () => {
    await setLocale('zh-CN')
    const { failureText } = useBulkReport()
    expect(failureText(outcomeOf([refused(1, 'not_found'), refused(2, 'not_attempted')]).failed)).toBe('1 个不存在；1 个未执行（请求超时）。')
    await setLocale('en')
  })

  it('shows a success toast with undo only when something was done', () => {
    const { report } = useBulkReport()
    const undo = () => {}
    report(outcomeOf([ok(1), ok(2)]), { success: 'Done', partial: 'Some', none: 'None', undo })
    const [toast] = toasts('success')
    expect(toast.message).toBe('Done')
    expect(toast.action).toMatchObject({ undo: true, onAction: undo })
  })

  it('keeps a partial failure on screen with Retry for the retryable ids only', async () => {
    const { report } = useBulkReport()
    let retried = 0
    report(outcomeOf([ok(1), refused(2, 'not_found'), refused(3, 'failed', 'boom')]), { success: 'Done', partial: 'Did 1 of 3.', none: 'Did none.', retry: () => { retried += 1 } })
    const [toast] = toasts('warning')
    expect(toast.message).toBe('Did 1 of 3. 1 not found; 1 failed (boom).')
    expect(toast.duration).toBe(0)
    expect(toast.action.label).toBe('Retry 1')
    await runAction(toast.id)
    expect(retried).toBe(1)
  })

  it('uses an error toast with no Retry when nothing was done and nothing can change', () => {
    const { report } = useBulkReport()
    report(outcomeOf([refused(1, 'forbidden_self'), refused(2, 'conflict')]), { success: 'Done', partial: 'Some', none: 'Did none.', retry: () => {} })
    const [toast] = toasts('error')
    expect(toast.message).toBe('Did none. 1 already used and kept; your own account can’t be banned.')
    expect(toast.action).toBeNull()
  })
})
