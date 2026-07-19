const PACKAGE_ID = 'subscription'
const PACKAGE_VERSION = '__ANIXOPS_PACKAGE_VERSION__'
const BUNDLE_PATH = 'webui/index.mjs'

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
    name: 'SubscriptionPackageExtension',
    setup() {
      return () => h('section', { class: 'control-page package-extension' }, [
        h('div', { class: 'page-header' }, [
          h('div', [h('h1', 'Subscription'), h('p', { class: 'description-cell' }, `Package ${PACKAGE_VERSION}`)]),
        ]),
      ])
    },
  })
}

export default mount
