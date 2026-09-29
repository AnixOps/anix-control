import { generateKeyPairSync } from 'node:crypto'
import { access, chmod, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { createWriteStream } from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { spawn } from 'node:child_process'
import { createServer } from 'node:net'
import { fileURLToPath } from 'node:url'

const here = path.dirname(fileURLToPath(import.meta.url))
const centerRoot = path.resolve(here, '../../..')
const defaultControlRoot = path.resolve(centerRoot, '../anix-control')
const controlRoot = path.resolve(process.env.ANIX_CONTROL_ROOT || defaultControlRoot)
const goBinary = process.env.ANIXOPS_GO_BIN || 'go'

export const realControlAPIURL = `http://127.0.0.1:${process.env.ANIXOPS_REAL_CONTROL_API_PORT || '38080'}`
export const realControlWebURL = `http://127.0.0.1:${process.env.ANIXOPS_REAL_CONTROL_WEB_PORT || '3010'}`
export const realControlAdmin = {
  email: 'center-real-control@anixops.test',
  password: 'CenterRealControl!2026',
}

const lifecycleGateEnabled = process.env.ANIXOPS_REAL_CONTROL_LIFECYCLE === '1'

function trimOutput(output, maximum = 12_000) {
  return output.length <= maximum ? output : output.slice(-maximum)
}

function commandError(label, command, args, code, output) {
  return new Error(`${label} failed (${code ?? 'spawn error'}): ${command} ${args.join(' ')}\n${trimOutput(output)}`)
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
      if (code === 0) resolve(output)
      else reject(commandError(label, command, args, code, output))
    })
  })
}

async function assertPortAvailable(port, name) {
  await new Promise((resolve, reject) => {
    const server = createServer()
    server.once('error', error => reject(new Error(`${name} port ${port} is unavailable: ${error.message}`)))
    server.listen(port, '127.0.0.1', () => server.close(error => error ? reject(error) : resolve()))
  })
}

async function waitForHealth(child, logPath) {
  const deadline = Date.now() + 45_000
  let lastError = null
  while (Date.now() < deadline) {
    if (child.exitCode !== null || child.signalCode !== null) break
    try {
      const response = await fetch(`${realControlAPIURL}/health`)
      if (response.ok) return
      lastError = new Error(`health endpoint returned ${response.status}`)
    } catch (error) {
      lastError = error
    }
    await new Promise(resolve => setTimeout(resolve, 100))
  }
  let log = ''
  try { log = await readFile(logPath, 'utf8') } catch { /* startup may fail before logging */ }
  throw new Error(`real Control did not become healthy: ${lastError?.message || 'unknown error'}\n${trimOutput(log)}`)
}

async function apiJSON(url, options = {}) {
  const response = await fetch(url, options)
  const text = await response.text()
  let payload = null
  try {
    payload = text ? JSON.parse(text) : null
  } catch {
    throw new Error(`${options.method || 'GET'} ${url} returned non-JSON ${response.status}: ${text.slice(0, 500)}`)
  }
  if (!response.ok) {
    throw new Error(`${options.method || 'GET'} ${url} failed with ${response.status}: ${JSON.stringify(payload)}`)
  }
  return { response, payload }
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

async function loginRealControlAdmin() {
  const deadline = Date.now() + 25_000
  let last = null
  while (Date.now() < deadline) {
    const response = await fetch(`${realControlAPIURL}/api/v2/login`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(realControlAdmin),
    })
    const text = await response.text()
    let payload = null
    try {
      payload = text ? JSON.parse(text) : null
    } catch {
      throw new Error(`real Control login returned non-JSON ${response.status}: ${text.slice(0, 500)}`)
    }
    if (response.ok) return payload
    last = { status: response.status, payload }
    if (response.status !== 503 || payload?.error?.code !== 'package_unavailable') {
      throw new Error(`real Control login failed with ${response.status}: ${JSON.stringify(payload)}`)
    }
    await new Promise(resolve => setTimeout(resolve, 100))
  }
  throw new Error(`real Control identity package did not become available: ${JSON.stringify(last)}`)
}

async function waitForInstallation(token, pluginID, version) {
  const deadline = Date.now() + 45_000
  let last = null
  while (Date.now() < deadline) {
    const { payload } = await apiJSON(`${realControlAPIURL}/api/v3/plugin-installations`, apiOptions(token, 'GET'))
    const rows = Array.isArray(payload?.data) ? payload.data : []
    const installation = rows.find(row => row.plugin_id === pluginID && row.target === 'control')
    last = installation
    if (installation?.state === 'healthy' && installation?.observed_version === version && installation?.enabled === true) {
      return installation
    }
    await new Promise(resolve => setTimeout(resolve, 200))
  }
  throw new Error(`${pluginID} Control installation did not become healthy: ${JSON.stringify(last)}`)
}

async function writeControlConfig(configPath, databasePath, publicKey, bootstrapDir, runtimeDir, artifactDir) {
  const quote = value => JSON.stringify(String(value))
  const config = `env: development
server:
  host: "127.0.0.1"
  port: ${new URL(realControlAPIURL).port}
  mode: release
  read_timeout: 30
  write_timeout: 30
  trusted_proxies: []
  cors:
    allowed_origins: []
    allowed_methods: ["GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"]
    allowed_headers: ["Content-Type", "Authorization"]
    allow_credentials: false
frontend:
  enable: false
database:
  driver: sqlite
  database: ${quote(databasePath)}
  log_level: error
cache:
  driver: memory
log:
  level: error
  output: stdout
jwt:
  secret: "center-real-control-test-secret"
  expire: 3600
auth:
  login_rate_limit:
    enabled: false
  register_rate_limit:
    enabled: false
  registration:
    enabled: false
app:
  name: "Control Center real integration test"
  version: "test"
  subscribe_path: s
plugins:
  official_public_key: ${quote(publicKey)}
  identity_bootstrap_package_dir: ${quote(bootstrapDir)}
  control_execution_enabled: true
  control_poll_interval: "100ms"
  control_host_runtime_dir: ${quote(runtimeDir)}
  control_host_artifact_dir: ${quote(artifactDir)}
  control_host_startup_timeout: "10s"
  control_host_request_timeout: "10s"
  dispatch_enabled: false
  topology_execution_enabled: false
grpc:
  enabled: false
admin:
  email: ${quote(realControlAdmin.email)}
  password: ${quote(realControlAdmin.password)}
tls:
  enable: false
forward_runtime:
  backend: clean_agent
  clean_agent:
    legacy_bridge_enabled: false
`
  await writeFile(configPath, config, 'utf8')
}

function rawEd25519PublicKey(publicKey) {
  const der = publicKey.export({ format: 'der', type: 'spki' })
  return der.subarray(-32).toString('base64')
}

async function createIdentityBootstrap(tempRoot) {
  const keys = generateKeyPairSync('ed25519')
  const privateKeyPath = path.join(tempRoot, 'signing-key.pem')
  const publicKeyPath = path.join(tempRoot, 'public-key.raw')
  const output = path.join(tempRoot, 'identity-package')
  const publicKey = rawEd25519PublicKey(keys.publicKey)
  await writeFile(privateKeyPath, keys.privateKey.export({ format: 'pem', type: 'pkcs8' }), { mode: 0o600 })
  await writeFile(publicKeyPath, `${publicKey}\n`, { mode: 0o600 })
  await runCommand('build signed identity bootstrap package', 'python3', [
    'packages/shared/build_package.py', '--package', 'identity-platform', '--version', '4.0.0',
    '--out', output, '--signing-key', privateKeyPath, '--formal-release',
    '--official-public-key', publicKeyPath, '--platform', 'linux/amd64', '--platform', 'linux/arm64',
  ], { cwd: controlRoot, env: { ...process.env, GOWORK: 'off' } })
  await chmod(output, 0o700)
  return { output, publicKey, privateKeyPath, publicKeyPath }
}

async function buildFormalAgentBinary(tempRoot, packageID, goarch) {
  const agentRoot = process.env.ANIXOPS_AGENT_ROOT
  if (!agentRoot) {
    throw new Error('ANIXOPS_AGENT_ROOT is required for the real Control Center lifecycle gate')
  }
  await access(agentRoot)
  const output = path.join(tempRoot, `${packageID}-agent-linux-${goarch}`)
  await runCommand(`build ${packageID} Agent binary for linux/${goarch}`, goBinary, [
    'build', '-trimpath', '-buildvcs=false', '-o', output, `./cmd/${packageID}`,
  ], {
    cwd: agentRoot,
    env: {
      ...process.env,
      CGO_ENABLED: '0',
      GOOS: 'linux',
      GOARCH: goarch,
      GOEXPERIMENT: 'jsonv2',
      GOWORK: 'off',
    },
  })
  await chmod(output, 0o755)
  return output
}

async function buildMachineTelemetryPackage(tempRoot, bootstrap) {
  const output = path.join(tempRoot, 'machine-telemetry-package')
  const args = [
    'packages/shared/build_package.py',
    '--package', 'machine-telemetry',
    '--version', '4.0.0',
    '--out', output,
    '--signing-key', bootstrap.privateKeyPath,
    '--formal-release',
    '--official-public-key', bootstrap.publicKeyPath,
    '--platform', 'linux/amd64',
    '--platform', 'linux/arm64',
  ]
  for (const goarch of ['amd64', 'arm64']) {
    const binary = await buildFormalAgentBinary(tempRoot, 'machine-telemetry', goarch)
    args.push('--agent-binary', `machine-telemetry@linux/${goarch}=${binary}`)
  }
  await runCommand('build signed machine-telemetry package', 'python3', args, {
    cwd: controlRoot,
    env: { ...process.env, GOWORK: 'off' },
  })
  const stem = 'machine-telemetry-4.0.0'
  return {
    artifact: await readFile(path.join(output, `${stem}.anxp`)),
    manifest: await readFile(path.join(output, `${stem}.manifest.json`), 'utf8'),
    signature: (await readFile(path.join(output, `${stem}.manifest.sig`), 'utf8')).trim(),
  }
}

async function installMachineTelemetryPackage(packageArtifact) {
  const login = await loginRealControlAdmin()
  const token = login?.data?.token
  if (!token) throw new Error(`real Control login did not issue a token: ${JSON.stringify(login)}`)
  const release = await apiJSON(`${realControlAPIURL}/api/v3/plugin-releases`, apiOptions(token, 'POST', {
    manifest: packageArtifact.manifest,
    signature: packageArtifact.signature,
  }))
  const releaseID = release.payload?.data?.id
  if (!Number.isSafeInteger(releaseID) || releaseID <= 0) {
    throw new Error(`real Control did not create the machine-telemetry release: ${JSON.stringify(release.payload)}`)
  }
  await apiJSON(`${realControlAPIURL}/api/v3/plugin-releases/${releaseID}/artifact`, apiOptions(token, 'POST', {
    artifact_base64: packageArtifact.artifact.toString('base64'),
  }))
  await apiJSON(`${realControlAPIURL}/api/v3/plugin-installations`, apiOptions(token, 'PUT', {
    plugin_id: 'machine-telemetry',
    target: 'control',
    desired_version: '4.0.0',
    enabled: true,
  }))
  await waitForInstallation(token, 'machine-telemetry', '4.0.0')
}

async function terminate(child) {
  if (!child || child.exitCode !== null || child.signalCode !== null) return
  child.kill('SIGTERM')
  await new Promise(resolve => {
    const timer = setTimeout(() => {
      if (child.exitCode === null && child.signalCode === null) child.kill('SIGKILL')
      resolve()
    }, 5_000)
    child.once('exit', () => { clearTimeout(timer); resolve() })
  })
}

export default async function setupRealControl() {
  await access(controlRoot)
  // Playwright owns the Center Vite port through `webServer`; only the
  // separate Control listener must be proven free before it is spawned.
  await assertPortAvailable(Number(new URL(realControlAPIURL).port), 'real Control API')
  // pluginhost verifies every runtime ancestor. Keep the fixture below the
  // private operator home rather than /tmp, whose sticky world-writable mode
  // is intentionally rejected by Control's host supervisor.
  const tempRoot = await mkdtemp(path.join(os.homedir(), '.anixops-control-center-real-'))
  let control = null
  let log = null
  try {
    const controlBinary = path.join(tempRoot, 'anix-control')
    const configPath = path.join(tempRoot, 'config.yaml')
    const databasePath = path.join(tempRoot, 'control.db')
    const runtimeDir = path.join(tempRoot, 'plugin-hosts')
    const artifactDir = path.join(tempRoot, 'plugin-artifacts')
    const logPath = path.join(tempRoot, 'control.log')
    const bootstrap = await createIdentityBootstrap(tempRoot)
    const machineTelemetry = lifecycleGateEnabled
      ? await buildMachineTelemetryPackage(tempRoot, bootstrap)
      : null
    await writeControlConfig(configPath, databasePath, bootstrap.publicKey, bootstrap.output, runtimeDir, artifactDir)
    await runCommand('build real Control binary', goBinary, ['build', '-o', controlBinary, './cmd/server'], {
      cwd: controlRoot, env: { ...process.env, GOWORK: 'off' },
    })
    await writeFile(logPath, '')
    log = createWriteStream(logPath, { flags: 'a' })
    control = spawn(controlBinary, ['-config', configPath], {
      cwd: controlRoot, env: { ...process.env, GOWORK: 'off' },
      stdio: ['ignore', 'pipe', 'pipe'], windowsHide: true,
    })
    control.stdout.pipe(log)
    control.stderr.pipe(log)
    await waitForHealth(control, logPath)
    if (machineTelemetry) await installMachineTelemetryPackage(machineTelemetry)
    return async () => {
      await terminate(control)
      if (log && !log.destroyed) await new Promise(resolve => log.end(resolve))
      if (process.env.ANIXOPS_REAL_CONTROL_KEEP_ARTIFACTS !== '1') await rm(tempRoot, { recursive: true, force: true })
    }
  } catch (error) {
    await terminate(control)
    if (log && !log.destroyed) await new Promise(resolve => log.end(resolve))
    if (process.env.ANIXOPS_REAL_CONTROL_KEEP_ARTIFACTS !== '1') await rm(tempRoot, { recursive: true, force: true })
    throw error
  }
}
