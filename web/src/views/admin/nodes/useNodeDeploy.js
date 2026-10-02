// Agent registration and Ansible deployment of nodes (UI U7): the
// registration key (GET /admin/auth-keys, POST /admin/auth-keys), the
// connection settings an Agent needs, its config.json example, and the
// inventory, group vars and commands of config/deploy/ansible/nodes for
// parent nodes (their API keys from GET /admin/nodes/:id/credentials).
// One instance per page: the key panel and the deployment helper share it.
import { computed, reactive, ref } from 'vue'
import { generateAuthKey, getAuthKeys, getNodeCredentials } from '@/api/admin'
import { MASKED_SECRET, isMaskedSecret } from '@/constants/secrets'
import { AGENT_NAME } from '@/constants/brand'
import { useAppI18n } from '@/composables/useAppI18n'
import { readNodeApiError, readNodeList, readNodePayload } from './nodeData'

const DEFAULT_AMD64 = '/home/dev/anixops/anix-agent/build/inventory/anix-agent_linux_amd64'
const DEFAULT_ARM64 = '/home/dev/anixops/anix-agent/build/inventory/anix-agent_linux_arm64'

export function isLoopbackGRPCHost(value) {
  const endpoint = String(value || '').trim().toLowerCase()
  if (!endpoint || endpoint.includes('://') || endpoint.includes('/') || endpoint.includes('?') || endpoint.includes('#')) {
    return false
  }

  let host = endpoint
  if (endpoint.startsWith('[')) {
    const closingBracket = endpoint.indexOf(']')
    if (closingBracket <= 1 || !/^\](?::\d{1,5})?$/.test(endpoint.slice(closingBracket))) {
      return false
    }
    host = endpoint.slice(1, closingBracket)
  } else {
    const firstColon = endpoint.indexOf(':')
    const lastColon = endpoint.lastIndexOf(':')
    if (firstColon === lastColon && firstColon > 0) {
      const port = endpoint.slice(lastColon + 1)
      if (!/^\d{1,5}$/.test(port)) return false
      host = endpoint.slice(0, lastColon)
    }
  }

  if (host === 'localhost' || host === '::1') return true
  const octets = host.split('.')
  return octets.length === 4 && octets.every((octet) => /^\d{1,3}$/.test(octet) && Number(octet) >= 0 && Number(octet) <= 255) && Number(octets[0]) === 127
}

export function isEd25519PublicKey(value) {
  const encoded = String(value || '').trim()
  if (!encoded || !/^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/.test(encoded) || typeof globalThis.atob !== 'function') return false
  try {
    return globalThis.atob(encoded).length === 32
  } catch {
    return false
  }
}

function defaultDeploySettings() {
  const hasWindow = typeof window !== 'undefined'
  return {
    panelApiHost: hasWindow ? window.location.origin : 'http://127.0.0.1:18080',
    grpcHost: hasWindow
      ? `${window.location.hostname}:${window.location.protocol === 'https:' ? '443' : '50051'}`
      : '127.0.0.1:50051',
    grpcUseTLS: hasWindow ? window.location.protocol === 'https:' : false,
    grpcServerName: hasWindow ? window.location.hostname : '127.0.0.1',
    amd64BinaryPath: DEFAULT_AMD64,
    arm64BinaryPath: DEFAULT_ARM64,
    coreType: 'xray',
    pluginSupervisorEnabled: false,
    pluginRoot: '/var/lib/anixops/plugins',
    pluginSocketDir: '/run/anixops/plugins',
    pluginOfficialPublicKey: ''
  }
}

export function slugifyDeployAlias(name, id) {
  const normalized = String(name || '')
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
  return normalized || `node-${id}`
}

export function buildDeployRow(node, apiKey = '') {
  return {
    id: node.id,
    nodeId: node.id,
    name: node.name,
    alias: slugifyDeployAlias(node.name, node.id),
    host: node.host || node.address || '',
    sshPort: 22,
    sshUser: 'root',
    arch: 'amd64',
    authMode: 'password',
    authValue: '',
    apiKey
  }
}

export function useNodeDeploy() {
  const { t } = useAppI18n()

  // Registration key ------------------------------------------------------
  // Keys are shown once, when generated: the key list masks them
  // (MASKED_SECRET). A key generated here stays shown while the page is open.
  const authKey = ref('')
  const authKeyUsed = ref(0)
  const authKeyMasked = ref(false)
  const authKeyGenerating = ref(false)
  const authKeyLoading = ref(false)
  const authKeyError = ref('')
  const issuedAuthKey = ref(null)

  function clearAuthKey() {
    authKey.value = ''
    authKeyMasked.value = false
    authKeyUsed.value = 0
  }

  async function loadAuthKeysPreview() {
    authKeyLoading.value = true
    try {
      const keys = readNodeList(await getAuthKeys())
      if (keys.length === 0) {
        clearAuthKey()
        return
      }
      const latest = keys[0]
      const key = String(latest.key || '')
      authKeyUsed.value = latest.used || 0
      if (key && !isMaskedSecret(key)) {
        authKey.value = key
        authKeyMasked.value = false
      } else if (issuedAuthKey.value && issuedAuthKey.value.id === latest.id) {
        authKey.value = issuedAuthKey.value.key
        authKeyMasked.value = false
      } else {
        authKey.value = ''
        authKeyMasked.value = key === MASKED_SECRET
      }
    } catch (e) {
      console.error('Failed to preload auth keys:', e)
      clearAuthKey()
    } finally {
      authKeyLoading.value = false
    }
  }

  // Generates a registration key. The answer is the only one that shows it.
  async function createAuthKey() {
    if (authKeyGenerating.value) return
    authKeyGenerating.value = true
    authKeyError.value = ''
    try {
      const res = await generateAuthKey({
        name: t('admin.nodes.authKey.defaultName', { date: new Date().toISOString().slice(0, 10) }),
        expire_days: 0
      })
      const payload = readNodePayload(res) || {}
      const key = String(payload.key || '')
      if (!key || isMaskedSecret(key)) {
        throw new Error(t('admin.nodes.authKey.noKey'))
      }
      issuedAuthKey.value = { id: payload.id, key }
      authKey.value = key
      authKeyMasked.value = false
      authKeyUsed.value = 0
    } catch (e) {
      authKeyError.value = t('admin.nodes.messages.generateFailed', { message: readNodeApiError(e) })
    } finally {
      authKeyGenerating.value = false
    }
  }

  // Connection settings ---------------------------------------------------
  const deploySettings = reactive(defaultDeploySettings())

  const agentControlEnabled = computed(() => deploySettings.grpcUseTLS || isLoopbackGRPCHost(deploySettings.grpcHost))

  const pluginSupervisorCanaryReady = computed(() => {
    if (!deploySettings.pluginSupervisorEnabled) return true
    return agentControlEnabled.value && [
      deploySettings.pluginRoot,
      deploySettings.pluginSocketDir
    ].every((value) => String(value || '').trim() !== '') && isEd25519PublicKey(deploySettings.pluginOfficialPublicKey)
  })

  const pluginSupervisorCanaryError = computed(() => {
    if (!agentControlEnabled.value) {
      return t('admin.nodes.deploy.pluginSupervisorControlRequired')
    }
    if (String(deploySettings.pluginOfficialPublicKey || '').trim() && !isEd25519PublicKey(deploySettings.pluginOfficialPublicKey)) {
      return t('admin.nodes.deploy.pluginSupervisorKeyInvalid')
    }
    return t('admin.nodes.deploy.pluginSupervisorKeyRequired')
  })

  const configSnippet = computed(() => {
    const host = String(deploySettings.panelApiHost || '').trim() || (typeof window !== 'undefined' ? window.location.origin : 'http://127.0.0.1:18080')
    const config = {
      Log: { Level: 'info', Output: '' },
      Cores: [{ Type: deploySettings.coreType || 'xray', Log: { Level: 'info' } }],
      Nodes: [
        {
          Core: deploySettings.coreType || 'xray',
          ApiHost: host,
          Transport: 'http',
          GRPCHost: deploySettings.grpcHost,
          GRPCUseTLS: Boolean(deploySettings.grpcUseTLS),
          ...(deploySettings.grpcUseTLS && deploySettings.grpcServerName
            ? { GRPCServerName: deploySettings.grpcServerName }
            : {}),
          GRPCKeepalive: 30,
          AgentControlEnabled: agentControlEnabled.value,
          AgentControlAllowInsecure: false,
          ...(deploySettings.pluginSupervisorEnabled && pluginSupervisorCanaryReady.value
            ? {
                PluginSupervisorEnabled: true,
                PluginRoot: deploySettings.pluginRoot,
                PluginSocketDir: deploySettings.pluginSocketDir,
                PluginOfficialPublicKey: deploySettings.pluginOfficialPublicKey
              }
            : {}),
          AuthKey: authKey.value || '<your-auth-key>',
          NodeID: 0,
          AutoRegister: true,
          Timeout: 30,
          ListenIP: '0.0.0.0',
          SendIP: '0.0.0.0',
          CertConfig: { CertMode: 'none' }
        }
      ]
    }
    return `# ${AGENT_NAME} config example\n${JSON.stringify(config, null, 2)}`
  })

  // Ansible deployment of parent nodes ------------------------------------
  const deployRows = ref([])
  const deployLoading = ref(false)
  const deployError = ref('')

  // Reads each parent node's API key (every read is audited as a reveal)
  // and prepares one inventory row per node.
  async function loadDeployRows(roots) {
    deployLoading.value = true
    deployError.value = ''
    try {
      if (roots.length === 0) {
        deployRows.value = []
        return
      }
      const credentials = await Promise.allSettled(roots.map(async (node) => {
        const payload = readNodePayload(await getNodeCredentials(node.id))
        return { nodeId: node.id, apiKey: payload?.api_key || '' }
      }))
      const apiKeysByNode = new Map(
        credentials
          .filter((item) => item.status === 'fulfilled')
          .map((item) => [item.value.nodeId, item.value.apiKey])
      )
      deployRows.value = roots.map((node) => buildDeployRow(node, apiKeysByNode.get(node.id) || ''))
    } catch (e) {
      console.error('Failed to load deploy credentials:', e)
      deployError.value = t('admin.nodes.messages.deployLoadFailed')
      deployRows.value = roots.map((node) => buildDeployRow(node, ''))
    } finally {
      deployLoading.value = false
    }
  }

  const deployInventoryPreview = computed(() => {
    const lines = [
      '[v2bx_nodes]',
      '# Generated from parent nodes (nodes without parent_id)'
    ]
    for (const row of deployRows.value) {
      const authValue = row.authValue || (row.authMode === 'password' ? '<PASSWORD>' : '~/.ssh/id_ed25519')
      const authField = row.authMode === 'password'
        ? `ansible_ssh_pass=${authValue}`
        : `ansible_ssh_private_key_file=${authValue}`
      const binaryPath = row.arch === 'arm64'
        ? (deploySettings.arm64BinaryPath || DEFAULT_ARM64)
        : (deploySettings.amd64BinaryPath || DEFAULT_AMD64)
      lines.push(
        `${row.alias} ansible_host=${row.host} ansible_port=${row.sshPort || 22} ansible_user=${row.sshUser || 'root'} ${authField} node_id=${row.nodeId} api_key=${row.apiKey || '<API_KEY>'} v2bx_arch=${row.arch} v2bx_binary_local=${binaryPath}`
      )
    }
    return lines.join('\n')
  })

  const deployGroupVarsPreview = computed(() => {
    const lines = [
      '---',
      `panel_api_host: "${deploySettings.panelApiHost}"`,
      `grpc_host: "${deploySettings.grpcHost}"`,
      `grpc_use_tls: ${deploySettings.grpcUseTLS ? 'true' : 'false'}`,
      `agent_control_enabled: ${agentControlEnabled.value ? 'true' : 'false'}`,
      'agent_control_allow_insecure: false',
      `plugin_supervisor_enabled: ${pluginSupervisorCanaryReady.value ? 'true' : 'false'}`
    ]
    if (deploySettings.grpcUseTLS && deploySettings.grpcServerName) {
      lines.push(`grpc_server_name: "${deploySettings.grpcServerName}"`)
    }
    if (pluginSupervisorCanaryReady.value) {
      lines.push(
        `plugin_root: ${JSON.stringify(deploySettings.pluginRoot)}`,
        `plugin_socket_dir: ${JSON.stringify(deploySettings.pluginSocketDir)}`,
        `plugin_official_public_key: ${JSON.stringify(deploySettings.pluginOfficialPublicKey)}`
      )
    }
    lines.push(
      '',
      'panel_api_base: "http://127.0.0.1:18080"',
      '',
      `v2bx_binary_amd64_local: "${deploySettings.amd64BinaryPath}"`,
      `v2bx_binary_arm64_local: "${deploySettings.arm64BinaryPath}"`,
      '',
      'push_geodata: false',
      'v2bx_geodata_dir: "/home/dev/anixops/anix-agent/example"',
      '',
      `core_type: "${deploySettings.coreType}"`,
      'v2bx_log_level: "info"',
      'listen_ip: "0.0.0.0"',
      'send_ip: "0.0.0.0"',
      'cert_mode: "none"'
    )
    return lines.join('\n')
  })

  const deployCommandsPreview = computed(() => {
    if (deployRows.value.length === 0) {
      return 'cd config/deploy/ansible/nodes'
    }
    const firstAlias = deployRows.value[0]?.alias || '<node-alias>'
    return [
      'cd config/deploy/ansible/nodes',
      'export ANSIBLE_CONFIG=../ansible.cfg',
      `# Download and verify matching ${AGENT_NAME} artifacts from GitHub Actions into the configured paths`,
      `# AMD64 artifact: ${deploySettings.amd64BinaryPath}`,
      `# ARM64 artifact: ${deploySettings.arm64BinaryPath}`,
      '# No local build is performed by this deployment flow',
      'ansible-playbook -i inventory.ini deploy_v2bx.yml',
      `ansible-playbook -i inventory.ini deploy_v2bx.yml -l ${firstAlias}`,
      `ansible-playbook -i inventory.ini bootstrap_ssh_key.yml -l ${firstAlias}`
    ].join('\n')
  })

  return {
    authKey,
    authKeyUsed,
    authKeyMasked,
    authKeyGenerating,
    authKeyLoading,
    authKeyError,
    loadAuthKeysPreview,
    createAuthKey,
    deploySettings,
    agentControlEnabled,
    pluginSupervisorCanaryReady,
    pluginSupervisorCanaryError,
    configSnippet,
    deployRows,
    deployLoading,
    deployError,
    loadDeployRows,
    deployInventoryPreview,
    deployGroupVarsPreview,
    deployCommandsPreview
  }
}
