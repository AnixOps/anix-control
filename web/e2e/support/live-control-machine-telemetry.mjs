import { generateKeyPairSync, sign } from 'node:crypto'
import { access, chmod, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { createWriteStream } from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { spawn } from 'node:child_process'
import { createServer } from 'node:net'
import { fileURLToPath } from 'node:url'

import { liveControlAPIURL, liveControlAPIPort, liveControlWebUIPort } from './live-control-options.mjs'

const here = path.dirname(fileURLToPath(import.meta.url))
const webRoot = path.resolve(here, '../..')
const controlRoot = path.resolve(webRoot, '..')
const packageRoot = path.join(controlRoot, 'packages', 'machine-telemetry')
const packageVersion = '1.1.0'
const adminEmail = 'live-control-webui@anixops.test'
const adminPassword = 'LiveControlWebUI!2026'

function trimOutput(output, maximum = 12_000) {
  return output.length <= maximum ? output : output.slice(-maximum)
}

function commandError(label, command, args, code, output) {
  const rendered = [command, ...args].join(' ')
  return new Error(`${label} failed (${code ?? 'spawn error'}): ${rendered}\n${trimOutput(output)}`)
}

function runCommand(label, command, args, options = {}) {
  return new Promise((resolve, reject) => {
    const child = spawn(command, args, {
      cwd: options.cwd,
      env: options.env ?? process.env,
      stdio: ['ignore', 'pipe', 'pipe'],
      windowsHide: true,
    })
    let output = ''
    child.stdout.on('data', chunk => { output += chunk })
    child.stderr.on('data', chunk => { output += chunk })
    child.once('error', error => reject(commandError(label, command, args, error.message, output)))
    child.once('exit', code => {
      if (code === 0) {
        resolve(output)
        return
      }
      reject(commandError(label, command, args, code, output))
    })
  })
}

async function assertPortAvailable(port, name) {
  await new Promise((resolve, reject) => {
    const server = createServer()
    server.once('error', error => reject(new Error(`${name} port ${port} is already unavailable: ${error.message}`)))
    server.listen(port, '127.0.0.1', () => {
      server.close(error => {
        if (error) {
          reject(new Error(`close ${name} port probe ${port}: ${error.message}`))
          return
        }
        resolve()
      })
    })
  })
}

async function waitForHealth(child, timeoutMs, logPath) {
  const deadline = Date.now() + timeoutMs
  let lastError = null
  while (Date.now() < deadline) {
    if (child.exitCode !== null || child.signalCode !== null) {
      break
    }
    try {
      const response = await fetch(`${liveControlAPIURL}/health`)
      if (response.ok) {
        return
      }
      lastError = new Error(`health endpoint returned ${response.status}`)
    } catch (error) {
      lastError = error
    }
    await new Promise(resolve => setTimeout(resolve, 100))
  }
  let log = ''
  try {
    log = await readFile(logPath, 'utf8')
  } catch {
    // The startup process may have failed before its log file was created.
  }
  const processState = child.exitCode !== null || child.signalCode !== null
    ? `Control process exited (code=${child.exitCode}, signal=${child.signalCode})`
    : 'Control process did not become healthy'
  throw new Error(`${processState}: ${lastError?.message ?? 'unknown error'}\n${trimOutput(log)}`)
}

async function apiJSON(url, options = {}) {
  const response = await fetch(url, options)
  const text = await response.text()
  let payload = null
  try {
    payload = text ? JSON.parse(text) : null
  } catch {
    throw new Error(`${options.method ?? 'GET'} ${url} returned non-JSON ${response.status}: ${text.slice(0, 500)}`)
  }
  if (!response.ok) {
    throw new Error(`${options.method ?? 'GET'} ${url} failed with ${response.status}: ${JSON.stringify(payload)}`)
  }
  return payload
}

function apiOptions(token, method, body) {
  return {
    method,
    headers: {
      Authorization: `Bearer ${token}`,
      'Content-Type': 'application/json',
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  }
}

function quotedYAML(value) {
  return JSON.stringify(String(value))
}

function controlConfig({ databasePath, frontendPath, publicKey }) {
  return `env: development
server:
  host: "127.0.0.1"
  port: ${liveControlAPIPort}
  mode: release
  read_timeout: 30
  write_timeout: 30
  trusted_proxies: []
frontend:
  enable: true
  port: ${liveControlWebUIPort}
  path: ${quotedYAML(frontendPath)}
database:
  driver: sqlite
  database: ${quotedYAML(databasePath)}
  log_level: error
cache:
  driver: memory
log:
  level: error
  output: stdout
jwt:
  secret: "live-control-webui-e2e-not-for-production"
  expire: 3600
auth:
  login_rate_limit:
    enabled: false
  register_rate_limit:
    enabled: false
  registration:
    enabled: false
app:
  name: "AnixOps live Control WebUI E2E"
  version: "test"
  subscribe_path: s
plugins:
  official_public_key: ${quotedYAML(publicKey)}
  control_execution_enabled: true
  control_poll_interval: "100ms"
  dispatch_enabled: false
  topology_execution_enabled: false
grpc:
  enabled: false
admin:
  email: ${quotedYAML(adminEmail)}
  password: ${quotedYAML(adminPassword)}
tls:
  enable: false
forward_runtime:
  # The live WebUI gate does not exercise the legacy forward runtime. Select
  # its inert pull-agent mode so Control can bootstrap without a NodeX service.
  backend: clean_agent
  clean_agent:
    legacy_bridge_enabled: false
`
}

function rawEd25519PublicKey(publicKey) {
  const der = publicKey.export({ format: 'der', type: 'spki' })
  const raw = der.subarray(-32)
  if (raw.length !== 32) {
    throw new Error('generated Ed25519 public key did not have the expected raw key length')
  }
  return raw.toString('base64')
}

async function waitForInstallation(token) {
  const deadline = Date.now() + 25_000
  let last = null
  while (Date.now() < deadline) {
    const payload = await apiJSON(`${liveControlAPIURL}/api/v3/plugin-installations`, apiOptions(token, 'GET'))
    const rows = Array.isArray(payload?.data) ? payload.data : []
    const installation = rows.find(row => row.plugin_id === 'machine-telemetry' && row.target === 'control')
    last = installation
    if (installation?.state === 'healthy' && installation?.observed_version === packageVersion && installation?.enabled === true) {
      return installation
    }
    await new Promise(resolve => setTimeout(resolve, 100))
  }
  throw new Error(`machine-telemetry Control installation did not become healthy: ${JSON.stringify(last)}`)
}

async function bootstrapSignedMachineTelemetry(tempRoot) {
  const fixtureAgent = path.join(tempRoot, 'machine-telemetry-agent-fixture')
  const packageOutput = path.join(tempRoot, 'package')
  await writeFile(fixtureAgent, '#!/bin/sh\nprintf "live-control-machine-telemetry-fixture\\n"\n', 'utf8')
  await chmod(fixtureAgent, 0o755)
  await runCommand('build signed package fixture', 'python3', [
    'build.py', 'build',
    '--agent-binary', fixtureAgent,
    '--goos', 'linux',
    '--goarch', 'amd64',
    '--output-dir', packageOutput,
  ], { cwd: packageRoot })

  const manifest = await readFile(path.join(packageOutput, `manifest.json`))
  const artifact = await readFile(path.join(packageOutput, `machine-telemetry-${packageVersion}.tar`))
  const keys = generateKeyPairSync('ed25519')
  return {
    artifact,
    manifest,
    signature: sign(null, manifest, keys.privateKey).toString('base64'),
    publicKey: rawEd25519PublicKey(keys.publicKey),
  }
}

async function installSignedMachineTelemetry(signed) {
  const login = await apiJSON(`${liveControlAPIURL}/api/v2/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email: adminEmail, password: adminPassword }),
  })
  const token = login?.data?.token
  if (typeof token !== 'string' || token.length === 0) {
    throw new Error(`real Control login did not issue an admin token: ${JSON.stringify(login)}`)
  }

  const release = await apiJSON(`${liveControlAPIURL}/api/v3/plugin-releases`, apiOptions(token, 'POST', {
    manifest: signed.manifest.toString('utf8'),
    signature: signed.signature,
  }))
  const releaseID = release?.data?.id
  if (!Number.isSafeInteger(releaseID) || releaseID <= 0) {
    throw new Error(`real Control did not create the signed package release: ${JSON.stringify(release)}`)
  }

  await apiJSON(`${liveControlAPIURL}/api/v3/plugin-releases/${releaseID}/artifact`, apiOptions(token, 'POST', {
    artifact_base64: signed.artifact.toString('base64'),
  }))
  await apiJSON(`${liveControlAPIURL}/api/v3/plugin-installations`, apiOptions(token, 'PUT', {
    plugin_id: 'machine-telemetry',
    target: 'control',
    desired_version: packageVersion,
    enabled: true,
  }))
  await waitForInstallation(token)
}

async function terminate(child) {
  if (!child || child.exitCode !== null || child.signalCode !== null) {
    return
  }
  child.kill('SIGTERM')
  const stopped = await Promise.race([
    new Promise(resolve => child.once('exit', resolve)),
    new Promise(resolve => setTimeout(resolve, 5_000)),
  ])
  if (stopped === undefined && child.exitCode === null && child.signalCode === null) {
    child.kill('SIGKILL')
    await new Promise(resolve => child.once('exit', resolve))
  }
}

async function openLog(pathname) {
  await writeFile(pathname, '')
  const stream = createWriteStream(pathname, { flags: 'a' })
  await new Promise((resolve, reject) => {
    stream.once('open', resolve)
    stream.once('error', reject)
  })
  return stream
}

async function closeLog(stream) {
  if (!stream || stream.closed || stream.destroyed) {
    return
  }
  await new Promise(resolve => stream.end(resolve))
}

// Starts a complete isolated Control process. This is deliberately a separate
// Playwright configuration: normal browser E2E stays fast, while release CI
// can run this gate to prove the signed package/Control/WebUI boundary.
export default async function setupLiveControlMachineTelemetry() {
  const tempRoot = await mkdtemp(path.join(os.tmpdir(), 'anixops-live-control-webui-'))
  let child = null
  let log = null
  try {
    await Promise.all([
      assertPortAvailable(liveControlAPIPort, 'live Control API'),
      assertPortAvailable(liveControlWebUIPort, 'live Control WebUI'),
    ])
    const frontendPath = path.join(tempRoot, 'frontend')
    const controlBinary = path.join(tempRoot, 'anix-control')
    const configPath = path.join(tempRoot, 'config.yaml')
    const databasePath = path.join(tempRoot, 'control.db')
    const logPath = path.join(tempRoot, 'control.log')
    const vite = path.join(webRoot, 'node_modules', '.bin', process.platform === 'win32' ? 'vite.cmd' : 'vite')
    await access(vite)

    await runCommand('build isolated frontend', vite, ['build', '--outDir', frontendPath], { cwd: webRoot })
    await runCommand('build real Control binary', 'go', ['build', '-o', controlBinary, './cmd/server'], { cwd: controlRoot })

    const signed = await bootstrapSignedMachineTelemetry(tempRoot)
    await writeFile(configPath, controlConfig({ databasePath, frontendPath, publicKey: signed.publicKey }), 'utf8')

    log = await openLog(logPath)
    child = spawn(controlBinary, ['-config', configPath], {
      cwd: controlRoot,
      stdio: ['ignore', 'pipe', 'pipe'],
      windowsHide: true,
    })
    child.stdout.pipe(log)
    child.stderr.pipe(log)
    child.once('error', error => log.write(`failed to start Control: ${error.stack || error.message}\n`))
    await waitForHealth(child, 35_000, logPath)
    await installSignedMachineTelemetry(signed)

    return async () => {
      await terminate(child)
      await closeLog(log)
      if (process.env.ANIXOPS_LIVE_CONTROL_WEBUI_KEEP_ARTIFACTS !== '1') {
        await rm(tempRoot, { recursive: true, force: true })
      }
    }
  } catch (error) {
    await terminate(child)
    await closeLog(log)
    if (process.env.ANIXOPS_LIVE_CONTROL_WEBUI_KEEP_ARTIFACTS !== '1') {
      await rm(tempRoot, { recursive: true, force: true })
    }
    throw error
  }
}
