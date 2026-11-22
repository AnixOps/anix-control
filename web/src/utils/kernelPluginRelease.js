export function parseManifest(release) {
  try {
    const manifest = typeof release?.manifest === 'string'
      ? JSON.parse(release.manifest)
      : release?.manifest
    return manifest && typeof manifest === 'object' && !Array.isArray(manifest) ? manifest : {}
  } catch {
    return {}
  }
}
