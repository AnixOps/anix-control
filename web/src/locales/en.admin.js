// Messages for the admin shell and the dashboard: navigation, ⌘K, the
// settings save bar and the dashboard page. src/i18n.js loads them when
// an admin signs in or an admin route opens; the other admin pages read
// src/locales/en.adminPages.js, loaded before any of them opens.
// The rest of the app reads src/locales/en.js only.
import adminDashboard from './modules/en/adminDashboard'
import adminMonitor from './modules/en/adminMonitor'
import adminSettingsPages from './modules/en/adminSettingsPages'

export default {
  ...adminSettingsPages,
  ...adminDashboard,
  ...adminMonitor,
  forwardSuite: {
    nav: {
      setupWizard: 'Setup Wizard',
      forwards: 'Forwards',
      tunnels: 'Tunnels',
      limits: 'Limits',
      ansibleMachines: 'Ansible Machines',
      localRuntime: 'Local Runtime',
      nodeXTopology: 'NodeX Topology',
      nodeXRuntime: 'NodeX Runtime',
      nodeXAgents: 'NodeX Agents'
    },
    hints: {
      setupWizard: 'Configure nodes, tunnels and forwards step by step',
      ansibleMachines: 'Stateless execution machines',
      localRuntime: 'Stateless panel-host executor',
      nodeXTopology: 'Stateful relay/exit topology',
      nodeXRuntime: 'Stateful gost control-plane',
      nodeXAgents: 'Stateful agent task channel'
    }
  }
}
