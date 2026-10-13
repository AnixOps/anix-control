// The first administrator's claim page (/claim#anixclaim_…), signed out:
// the form while the panel has no administrator ('form'), and the neutral
// message when nothing can be claimed ('none').
const TOKEN = 'anixclaim_' + 'q7Z-_'.repeat(8) + 'Kp3'

export default {
  signedOut: () => true,
  pathFor: () => `/claim#${TOKEN}`,
  api(p, { scenario }) {
    if (p === '/api/v5/claim/status') return { data: { claimable: scenario !== 'none' } }
    return undefined
  },
  // The page settles once the status answered: the form, or the message.
  async after(page, { scenario }) {
    await page.locator(scenario === 'none' ? '[data-claim-note]' : '[data-claim-form]').waitFor()
  }
}
