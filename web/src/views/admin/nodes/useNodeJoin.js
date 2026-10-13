// State of the 接入节点 dialog: is this Control ready to hand out a command,
// which node group the command is for, the command itself, and the machines
// that have joined (polled every three seconds while the dialog is open).
//
// Order on open (docs/design/node-onboarding, "The Administrator's Flow"):
//   1. GET /kernel/agents/install-readiness: a Control that cannot work shows
//      each failing check with its fix instead of a command that would fail.
//   2. GET /kernel/node-groups; with none, POST /kernel/node-groups/default
//      makes the default group, so the first node needs no input.
//   3. GET /kernel/node-groups/:id/command (super administrators) and
//      GET /kernel/node-groups/:id/nodes every POLL_MS.
import { computed, onBeforeUnmount, ref } from 'vue'
import {
  approveKernelNode,
  createKernelNodeGroup,
  ensureKernelDefaultNodeGroup,
  getKernelInstallReadiness,
  getKernelNodeGroupCommand,
  getKernelNodeGroupNodes,
  listKernelNodeGroups,
  rejectKernelNode,
  resetKernelNodeGroupToken
} from '@/api/kernel'
import { readKernelError, readNodeGroupNodes, readNodeGroups } from './nodeGroupData'

export const POLL_MS = 3000

export function useNodeJoin({ onChanged } = {}) {
  // idle → checking → blocked | loading → ready | error
  const stage = ref('idle')
  const readiness = ref(null)
  const errorMessage = ref('')
  const groups = ref([])
  const groupId = ref(null)
  const command = ref(null)
  // 'forbidden' (not a super administrator), 'failed' or ''.
  const commandProblem = ref('')
  const commandMessage = ref('')
  const nodes = ref([])
  const nodesLoaded = ref(false)
  const busyNodes = ref([])
  const createdDefault = ref(false)

  let timer = null
  let sequence = 0
  let polling = false
  let signature = null

  const group = computed(() => groups.value.find(item => item.id === groupId.value) || null)
  const failedChecks = computed(() => (readiness.value?.checks || []).filter(check => !check.ok))
  // Failed checks that do not stop the command.
  const warnings = computed(() => failedChecks.value.filter(check => check.level === 'warning'))
  const blockers = computed(() => failedChecks.value.filter(check => check.level !== 'warning'))

  function stopPolling() {
    if (timer) clearInterval(timer)
    timer = null
  }

  function reset() {
    sequence += 1
    stopPolling()
    stage.value = 'idle'
    readiness.value = null
    errorMessage.value = ''
    groups.value = []
    groupId.value = null
    command.value = null
    commandProblem.value = ''
    commandMessage.value = ''
    nodes.value = []
    nodesLoaded.value = false
    busyNodes.value = []
    createdDefault.value = false
    signature = null
  }

  // The dialog's content is its own: a token is never kept after it closes.
  function close() {
    reset()
  }

  async function loadGroups() {
    groups.value = readNodeGroups(await listKernelNodeGroups())
    return groups.value
  }

  async function loadCommand(mine = sequence) {
    const id = groupId.value
    if (!id) return
    commandProblem.value = ''
    commandMessage.value = ''
    try {
      const answer = await getKernelNodeGroupCommand(id)
      if (mine !== sequence || id !== groupId.value) return
      command.value = answer
    } catch (error) {
      if (mine !== sequence || id !== groupId.value) return
      const problem = readKernelError(error)
      command.value = null
      commandProblem.value = problem.status === 403 || problem.code === 'super_admin_required' ? 'forbidden' : 'failed'
      commandMessage.value = problem.message
    }
  }

  async function loadNodes(mine = sequence) {
    const id = groupId.value
    if (!id || polling) return
    polling = true
    try {
      const answer = await getKernelNodeGroupNodes(id)
      if (mine !== sequence || id !== groupId.value) return
      const list = readNodeGroupNodes(answer)
      const next = JSON.stringify(list.map(node => [node.id, node.kind, node.approval, node.online]))
      const first = signature === null
      nodes.value = list
      nodesLoaded.value = true
      if (next !== signature) {
        signature = next
        // The first answer is what was there already; later ones are joins,
        // approvals and nodes coming online: the page's list is stale.
        if (!first) onChanged?.()
      }
    } catch (error) {
      // A poll that fails is tried again; the list stays as it was.
      if (mine === sequence) console.error('Failed to load the nodes of the group:', error)
    } finally {
      polling = false
    }
  }

  function startPolling() {
    stopPolling()
    const mine = sequence
    timer = setInterval(() => { void loadNodes(mine) }, POLL_MS)
  }

  async function selectGroup(id) {
    const mine = ++sequence
    stopPolling()
    groupId.value = id
    command.value = null
    commandProblem.value = ''
    nodes.value = []
    nodesLoaded.value = false
    signature = null
    polling = false
    if (!id) return
    await Promise.all([loadCommand(mine), loadNodes(mine)])
    if (mine === sequence) startPolling()
  }

  // Everything after the readiness check: the groups (the default one when
  // the panel has none) and the first group's command.
  async function prepare(preferredId = null) {
    const mine = ++sequence
    stage.value = 'loading'
    try {
      let list = await loadGroups()
      if (mine !== sequence) return
      if (list.length === 0) {
        const answer = await ensureKernelDefaultNodeGroup()
        if (mine !== sequence) return
        createdDefault.value = Boolean(answer?.created)
        list = await loadGroups()
        if (mine !== sequence) return
        if (list.length === 0 && answer?.group) {
          groups.value = [answer.group]
          list = groups.value
        }
        if (createdDefault.value) onChanged?.()
      }
      const wanted = list.find(item => item.id === preferredId) || list[0] || null
      stage.value = 'ready'
      await selectGroup(wanted ? wanted.id : null)
    } catch (error) {
      if (mine !== sequence) return
      const problem = readKernelError(error)
      errorMessage.value = problem.message
      stage.value = 'error'
    }
  }

  async function open(preferredId = null) {
    reset()
    const mine = sequence
    stage.value = 'checking'
    try {
      const answer = await getKernelInstallReadiness()
      if (mine !== sequence) return
      readiness.value = answer
      if (answer && answer.ready === false) {
        stage.value = 'blocked'
        return
      }
    } catch (error) {
      if (mine !== sequence) return
      errorMessage.value = readKernelError(error).message
      stage.value = 'error'
      return
    }
    await prepare(preferredId)
  }

  // Check again after the administrator fixed the configuration.
  async function recheck() {
    return open(groupId.value)
  }

  // A group made inline: { name, kind, protocol_template,
  // subscription_group_ids }. Selects it; the error is the caller's to show.
  async function createGroup(input) {
    const created = await createKernelNodeGroup(input)
    await loadGroups()
    onChanged?.()
    await selectGroup(created?.id ?? groups.value[groups.value.length - 1]?.id ?? null)
    return created
  }

  // Replaces the group's token; every earlier command stops working.
  async function resetToken() {
    const id = groupId.value
    if (!id) return
    await resetKernelNodeGroupToken(id)
    await Promise.all([loadCommand(), loadGroups()])
  }

  async function decide(node, action) {
    const name = node.node
    if (busyNodes.value.includes(name)) return
    busyNodes.value = [...busyNodes.value, name]
    try {
      await action(name)
      await Promise.all([loadNodes(), loadGroups()])
      onChanged?.()
    } finally {
      busyNodes.value = busyNodes.value.filter(item => item !== name)
    }
  }

  const approve = node => decide(node, approveKernelNode)
  const reject = node => decide(node, rejectKernelNode)

  onBeforeUnmount(stopPolling)

  return {
    stage, readiness, errorMessage, groups, groupId, group, command, commandProblem, commandMessage,
    nodes, nodesLoaded, busyNodes, createdDefault, failedChecks, warnings, blockers,
    open, close, recheck, prepare, selectGroup, createGroup, resetToken, approve, reject, loadNodes, loadGroups
  }
}
