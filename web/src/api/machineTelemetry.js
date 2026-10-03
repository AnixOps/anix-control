// The machine-telemetry package's per-node systemd services table
// (docs/architecture/package-reports.md). The table is the package's own
// route; the per-node switch and unit globs live in the package's Agent
// installation configuration, saved through the kernel's plugin
// configuration API like any other package setting.
import request from '@/utils/request'
import {
  getKernelInstallationConfig,
  getKernelInstallations,
  getKernelNodeAssignments,
  getKernelPluginReleases,
  updateKernelInstallationConfig
} from './kernel'

export const MACHINE_TELEMETRY_ID = 'machine-telemetry'
// The manifest capability of a release whose collector reports the table.
export const SERVICES_CAPABILITY = 'telemetry.systemd.read'
// The configuration key of the services settings
// (sdk/telemetry/systemdreport.ConfigKey).
export const SERVICES_CONFIG_KEY = 'systemd_services'

function unwrap(response) {
  return response?.data ?? response
}

function list(value) {
  if (Array.isArray(value)) return value
  if (value && Array.isArray(value.list)) return value.list
  return []
}

export async function getNodeServices(nodeID) {
  return unwrap(await request({
    baseURL: '/api/v3',
    url: `/plugins/${MACHINE_TELEMETRY_ID}/nodes/${encodeURIComponent(nodeID)}/services`,
    method: 'get'
  }))
}

function manifestOf(release) {
  const manifest = release?.manifest
  if (manifest && typeof manifest === 'object') return manifest
  if (typeof manifest !== 'string' || !manifest) return null
  try {
    return JSON.parse(manifest)
  } catch {
    return null
  }
}

// Whether the release of machine-telemetry assigned to the node declares
// the services capability: false when the package is not assigned to the
// node, the assignment is disabled, or its release predates the table.
export async function nodeHasServicesTable(nodeID) {
  const assignments = list(unwrap(await getKernelNodeAssignments(nodeID)))
  const versions = new Set(assignments
    .filter(item => item?.plugin_id === MACHINE_TELEMETRY_ID && item.enabled && !item.delete_pending && item.desired_version)
    .map(item => item.desired_version))
  if (!versions.size) return false
  const releases = list(unwrap(await getKernelPluginReleases(MACHINE_TELEMETRY_ID)))
  return releases.some((release) => {
    if (release?.plugin_id !== MACHINE_TELEMETRY_ID || !versions.has(release.version)) return false
    const capabilities = manifestOf(release)?.capabilities
    return Array.isArray(capabilities) && capabilities.includes(SERVICES_CAPABILITY)
  })
}

async function agentInstallation() {
  const installations = list(unwrap(await getKernelInstallations()))
  const installation = installations.find(item => item?.plugin_id === MACHINE_TELEMETRY_ID && item.target === 'agent')
  if (!installation) {
    throw Object.assign(new Error('machine-telemetry has no Agent installation'), { code: 'no_installation' })
  }
  return installation
}

function cleanGlobs(globs) {
  return [...new Set((globs || []).map(glob => String(glob).trim()).filter(Boolean))]
}

// Returns the configuration document with the node's settings replaced:
// globs trimmed and deduplicated, empty lists left out.
export function withNodeServices(config, nodeID, { enabled, include, exclude }) {
  const document = config && typeof config === 'object' && !Array.isArray(config) ? { ...config } : {}
  const current = document[SERVICES_CONFIG_KEY] && typeof document[SERVICES_CONFIG_KEY] === 'object' ? document[SERVICES_CONFIG_KEY] : {}
  const nodes = { ...(current.nodes || {}) }
  const entry = { enabled: Boolean(enabled) }
  const includeGlobs = cleanGlobs(include)
  const excludeGlobs = cleanGlobs(exclude)
  if (includeGlobs.length) entry.include = includeGlobs
  if (excludeGlobs.length) entry.exclude = excludeGlobs
  nodes[String(nodeID)] = entry
  document[SERVICES_CONFIG_KEY] = { ...current, nodes }
  return document
}

// Saves one node's services settings: reads the Agent installation's
// configuration, replaces the node's entry and saves it at the revision it
// read, so a concurrent change answers 409 instead of being overwritten.
// Control then pushes the configuration to the package's nodes.
export async function saveNodeServicesSettings(nodeID, settings) {
  const installation = await agentInstallation()
  const current = unwrap(await getKernelInstallationConfig(installation.id)) || {}
  const config = withNodeServices(current.config, nodeID, settings)
  return updateKernelInstallationConfig(installation.id, config, Number(current.revision || 0))
}

// The unit name alphabet plus path.Match's * ? [ ] ^, with every class
// closed and not empty. Control checks the full path.Match syntax
// (sdk/telemetry/systemdreport.ValidGlob) when the settings are saved.
const GLOB_PATTERN = /^[A-Za-z0-9:_.\\@*?[\]^-]+$/
export const MAX_GLOBS = 32
export const MAX_GLOB_LENGTH = 256

export function validGlob(glob) {
  if (!glob || glob.length > MAX_GLOB_LENGTH || !GLOB_PATTERN.test(glob)) return false
  let open = false
  for (let index = 0; index < glob.length; index += 1) {
    const char = glob[index]
    if (char === '\\') {
      if (index === glob.length - 1) return false
      index += 1
    } else if (char === '[') {
      if (open) return false
      open = true
      // A class needs one character before its closing bracket.
      if (glob[index + 1] === '^') index += 1
      if (glob[index + 1] === ']') return false
    } else if (char === ']' && open) {
      open = false
    }
  }
  return !open
}
