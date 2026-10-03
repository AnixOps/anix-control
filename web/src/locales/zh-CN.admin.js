// Messages for the admin shell and the dashboard: navigation, ⌘K, the
// settings save bar and the dashboard page. src/i18n.js loads them when
// an admin signs in or an admin route opens; the other admin pages read
// src/locales/zh-CN.adminPages.js, loaded before any of them opens.
// The rest of the app reads src/locales/zh-CN.js only.
import adminDashboard from './modules/zh-CN/adminDashboard'
import adminMonitor from './modules/zh-CN/adminMonitor'
import adminSettingsPages from './modules/zh-CN/adminSettingsPages'

export default {
  ...adminSettingsPages,
  ...adminDashboard,
  ...adminMonitor,
  forwardSuite: {
    nav: {
      setupWizard: '快速配置向导',
      forwards: '流量转发',
      tunnels: '隧道管理',
      limits: '限速管理',
      ansibleMachines: 'Ansible 机器',
      localRuntime: '本地运行时',
      nodeXTopology: 'NodeX 拓扑',
      nodeXRuntime: 'NodeX 运行时',
      nodeXAgents: 'NodeX Agents'
    },
    hints: {
      setupWizard: '一步步配置节点、隧道和转发',
      ansibleMachines: '无状态执行机器',
      localRuntime: '无状态面板宿主执行器',
      nodeXTopology: '有状态 relay/exit 拓扑',
      nodeXRuntime: '有状态 gost 控制面',
      nodeXAgents: '有状态 agent 任务通道'
    }
  }
}
