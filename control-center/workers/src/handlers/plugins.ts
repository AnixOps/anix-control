import type { Context } from 'hono'
import type { Env } from '../types'
import { logAudit } from '../utils/audit'

interface Plugin {
  name: string
  version: string
  status: 'running' | 'stopped' | 'error'
  capabilities: string[]
}

// 模拟插件数据 (实际应该从 D1 或配置中读取)
const plugins: Plugin[] = [
  { name: 'ansible', version: '1.0.0', status: 'running', capabilities: ['run', 'validate', 'inventory'] },
  { name: 'v2board', version: '1.0.0', status: 'running', capabilities: ['sync', 'users', 'nodes'] },
  { name: 'v2bx', version: '1.0.0', status: 'running', capabilities: ['sync', 'users', 'nodes'] },
  { name: 'agent', version: '1.0.0', status: 'running', capabilities: ['exec', 'status', 'deploy'] },
]

/**
 * 获取插件列表
 */
export async function listPluginsHandler(c: Context<{ Bindings: Env }>) {
  return c.json({
    success: true,
    data: plugins,
  })
}

/**
 * 获取单个插件
 */
export async function getPluginHandler(c: Context<{ Bindings: Env }>) {
  const name = c.req.param('name') as string
  const plugin = plugins.find(p => p.name === name)

  if (!plugin) {
    return c.json({ success: false, error: 'Plugin not found' }, 404)
  }

  return c.json({
    success: true,
    data: plugin,
  })
}

/**
 * 执行插件操作
 */
export async function executePluginHandler(c: Context<{ Bindings: Env }>) {
  const name = c.req.param('name') as string
  const body = await c.req.json<{ action: string; params?: Record<string, unknown> }>()
  const user = c.get('user')

  const plugin = plugins.find(p => p.name === name)

  if (!plugin) {
    return c.json({ success: false, error: 'Plugin not found' }, 404)
  }

  if (plugin.status !== 'running') {
    return c.json({ success: false, error: 'Plugin is not running' }, 400)
  }

  if (!plugin.capabilities.includes(body.action)) {
    return c.json({ success: false, error: `Action '${body.action}' not supported by plugin` }, 400)
  }

  // 记录审计日志
  await logAudit(c, user.sub, 'execute_plugin', 'plugin', {
    plugin: name,
    action: body.action,
    params: body.params,
  })

  // 模拟执行结果 (实际应该调用插件的真实实现)
  return c.json({
    success: true,
    data: {
      plugin: name,
      action: body.action,
      result: 'success',
      timestamp: new Date().toISOString(),
    },
  })
}