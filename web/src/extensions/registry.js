const localExtensionLoaders = import.meta.glob('./modules/*/index.js')

// Extension code is selected exclusively from this build-time registry. The
// signed catalog's bundle path is identity metadata and is never imported.
export function getLocalExtensionModuleLoader(pluginID) {
  return localExtensionLoaders[`./modules/${pluginID}/index.js`] || null
}
