// Keep the default ports away from the normal development listener (8080),
// Playwright's Vite listener (4173), and the common production test listener
// (18080). Callers can override both values for a shared CI executor.
const DEFAULT_WEBUI_PORT = 34175
const DEFAULT_API_PORT = 38080

function parsePort(raw, fallback, name) {
  if (raw === undefined || raw === '') {
    return fallback
  }
  const value = Number(raw)
  if (!Number.isSafeInteger(value) || value < 1024 || value > 65535) {
    throw new Error(`${name} must be a TCP port between 1024 and 65535`)
  }
  return value
}

export const liveControlWebUIPort = parsePort(
  process.env.ANIXOPS_LIVE_CONTROL_WEBUI_PORT,
  DEFAULT_WEBUI_PORT,
  'ANIXOPS_LIVE_CONTROL_WEBUI_PORT'
)

export const liveControlAPIPort = parsePort(
  process.env.ANIXOPS_LIVE_CONTROL_API_PORT,
  DEFAULT_API_PORT,
  'ANIXOPS_LIVE_CONTROL_API_PORT'
)

if (liveControlWebUIPort === liveControlAPIPort) {
  throw new Error('ANIXOPS live Control API and WebUI ports must differ')
}

export const liveControlWebUIURL = `http://127.0.0.1:${liveControlWebUIPort}`
export const liveControlAPIURL = `http://127.0.0.1:${liveControlAPIPort}`
