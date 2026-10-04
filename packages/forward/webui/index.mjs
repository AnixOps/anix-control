const PACKAGE_ID = 'forward'
const PACKAGE_VERSION = '__ANIXOPS_PACKAGE_VERSION__'
const BUNDLE_PATH = 'webui/index.mjs'
// The core app's forwarding pages (F5b).
const FORWARD_UI_PATH = '/admin/forward/overview'

export const anixopsExtension = Object.freeze({
  webuiApiVersion: 'anixops.webui/v1',
  pluginId: PACKAGE_ID,
  version: PACKAGE_VERSION,
  bundle: Object.freeze({ path: BUNDLE_PATH }),
})

export function mount(host) {
  if (!host || typeof host.defineComponent !== 'function' || typeof host.h !== 'function') {
    throw new Error('AnixOps WebUI host is required')
  }
  const { defineComponent, h } = host
  return defineComponent({
    name: 'ForwardPackageExtension',
    setup() {
      // The forwarding pages live in the core app (F5b, D1): they need the
      // Ui* components and the router, which this contract does not offer.
      // This entry links to them.
      return () => h('section', { class: 'control-page package-extension' }, [
        h('div', { class: 'page-header' }, [
          h('div', [h('h1', 'Forward'), h('p', { class: 'description-cell' }, `Package ${PACKAGE_VERSION}`)]),
        ]),
        h('p', { class: 'description-cell' }, [
          h('a', { href: FORWARD_UI_PATH, 'data-forward-ui-link': '' }, 'Open forwarding (routes, nodes and traffic)'),
        ]),
      ])
    },
  })
}

export default mount
