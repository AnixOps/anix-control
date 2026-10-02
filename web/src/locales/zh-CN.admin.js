// Messages for the admin console (src/views/admin and the forward suite).
// src/i18n.js loads them when an admin route opens or an admin signs in;
// the rest of the app reads src/locales/zh-CN.js only.
import runtimePages from './modules/zh-CN/runtimePages'
import networkPages from './modules/zh-CN/networkPages'
import miscPages from './modules/zh-CN/miscPages'
import adminSupportPages from './modules/zh-CN/adminSupportPages'
import adminDashboard from './modules/zh-CN/adminDashboard'
import adminMonitor from './modules/zh-CN/adminMonitor'
import adminNodes from './modules/zh-CN/adminNodes'
import forwardNodesPage from './modules/zh-CN/forwardNodesPage'
import adminSettingsPages from './modules/zh-CN/adminSettingsPages'

export default {
  ...runtimePages,
  ...networkPages,
  ...miscPages,
  ...adminSupportPages,
  ...adminDashboard,
  ...adminMonitor,
  ...forwardNodesPage,
  ...adminSettingsPages,
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
  },
  forwardWizard: {
    title: '转发配置向导',
    subtitle: '按步骤创建节点、隧道和转发，不用在多个页面之间来回跳转',
    loading: '正在加载运行模式…',
    stepsLabel: '配置步骤',
    stepCount: '第 {current} 步，共 {total} 步',
    stepDone: '（已完成）',
    back: '上一步',
    next: '下一步',
    shared: {
      existingLabel: '已有可复用的记录',
      useExisting: '使用现有'
    },
    steps: {
      mode: {
        title: '选择转发方式',
        chooseLabel: '转发方式',
        intro: '选一种转发方式，系统会自动配置好对应的运行时后端和隧道类型，无需分别理解这两个概念。',
        cards: {
          local: {
            label: '本地端口转发',
            description: '直接在 Ansible 管理的机器上转发，无需中转节点。'
          },
          gostSingle: {
            label: '中转 · 单节点转发',
            description: '通过一个 NodeX 节点转发，不做协议封装。'
          },
          gostTunnel: {
            label: '中转 · 隧道转发',
            description: '跨入口/出口两个节点转发，支持协议隐藏(tls/ws/grpc 等)。'
          }
        },
        currentBadge: '当前生效',
        nodeXSetupHint: '首次使用中转转发，需要先填写 NodeX 控制面信息才能继续。',
        confirmAndContinue: '确认并继续'
      },
      machine: {
        title: '机器或节点',
        intro: '先注册一台执行机器或节点，后面的隧道会用到它。',
        createAndContinue: '创建并继续'
      },
      node: {
        intro: '先注册一个 NodeX 节点，后面的隧道会用到它。',
        createAndContinue: '创建并继续',
        tokenNotice: '这是节点「{name}」的 API Token，只显示这一次，请立即复制。'
      },
      tunnel: {
        title: '隧道',
        intro: '基于上一步的节点创建隧道，隧道是转发条目的必选项。',
        inheritedNodeHint: '已自动带入上一步创建的节点，无需重复选择。',
        createAndContinue: '创建并继续'
      },
      forward: {
        title: '转发',
        intro: '基于上一步的隧道创建实际的转发条目，填好目标地址即可生效。',
        inheritedTunnelHint: '已自动带入上一步创建的隧道，无需重复选择。',
        createAndFinish: '创建并完成'
      },
      done: {
        title: '完成',
        summary: '转发「{name}」已创建成功，链路已打通。',
        nextTitle: '接下来',
        gotoForward: '前往流量转发管理',
        gotoTunnel: '前往隧道管理',
        gotoNode: '前往机器/节点管理',
        createAnother: '再建一条转发'
      }
    }
  },
  runtime: {
    shared: {
      panelConfig: '\u9762\u677f\u914d\u7f6e',
      reachability: '\u53ef\u8fde\u901a\u6027',
      runtimeReady: '\u8fd0\u884c\u65f6\u5c31\u7eea',
      executor: '\u6267\u884c\u5668',
      files: '\u6587\u4ef6',
      warnings: '\u8b66\u544a',
      powerShell: 'PowerShell',
      bash: 'Bash',
      bootstrapVerify: '\u5f15\u5bfc / \u6821\u9a8c',
      references: '\u53c2\u8003',
      doctorOutput: 'Doctor \u8f93\u51fa',
      doctorNotExecuted: '\u5c1a\u672a\u6267\u884c Doctor\u3002',
      refreshStatus: '\u5237\u65b0\u72b6\u6001',
      runDoctor: '\u8fd0\u884c Doctor',
      runningDoctor: '\u6267\u884c\u4e2d...',
      loading: '\u52a0\u8f7d\u4e2d...',
      yes: '\u662f',
      no: '\u5426',
      present: '\u5b58\u5728',
      missing: '\u7f3a\u5931',
      reachable: '\u53ef\u8fde\u901a',
      notReady: '\u672a\u5c31\u7eea',
      ready: '\u5c31\u7eea',
      unavailable: '\u4e0d\u53ef\u7528',
      pending: '\u5f85\u5904\u7406',
      running: '\u8fd0\u884c\u4e2d',
      success: '\u6210\u529f',
      failed: '\u5931\u8d25',
      unknown: '\u672a\u77e5',
      online: '\u5728\u7ebf',
      offline: '\u79bb\u7ebf',
      enabled: '\u5df2\u542f\u7528',
      disabled: '\u5df2\u7981\u7528'
    },
    localRuntime: {
      heroTextPrimary: '\u8be5\u9875\u9762\u53ea\u7ba1\u7406\u9762\u677f\u4e3b\u673a\u4e0a\u7684 Ansible \u6267\u884c\u5668\u3002\u5b83\u662f\u9762\u677f\u4fa7\u8f6c\u53d1\u7684\u65e0\u72b6\u6001\u8fd0\u884c\u65f6\u8def\u5f84\uff0c\u4e0d\u9700\u8981\u6301\u7eed\u7684 NodeX \u63a7\u5236\u9762\u6216 Node-Agent \u8fde\u63a5\u3002',
      heroTextSecondary: '\u540e\u7aef\u4e3a nftables / Ansible\uff0c\u9762\u677f\u4fa7\u8f6c\u53d1\u7684\u65e0\u72b6\u6001\u63a7\u5236\u8def\u5f84\u3002',
      saveActivate: '\u4fdd\u5b58\u5e76\u542f\u7528\u672c\u5730\u8fd0\u884c\u65f6',
      activeBannerTitle: '\u672c\u5730\u8fd0\u884c\u65f6\u5df2\u542f\u7528',
      standbyBannerTitle: '\u672c\u5730\u8fd0\u884c\u65f6\u5904\u4e8e\u5f85\u547d',
      activeBannerText: '\u5f53\u524d\u8f6c\u53d1\u4efb\u52a1\u4f7f\u7528 {backend}\u3002SSH \u4f20\u8f93\u548c\u63d0\u6743\u7b56\u7565\u90fd\u4ece\u8fd9\u4efd Ansible \u8fd0\u884c\u65f6\u914d\u7f6e\u89e3\u6790\u3002',
      standbyBannerText: 'NodeX/gost \u4ecd\u7136\u662f\u5168\u5c40\u6d3b\u8dc3\u8fd0\u884c\u65f6\u3002\u4f60\u4ecd\u53ef\u5148\u5728\u8fd9\u91cc\u9884\u6f14\u548c\u9a8c\u8bc1\u672c\u5730 Ansible \u8fd0\u884c\u65f6\uff0c\u518d\u5207\u6362\u56de\u53bb\u3002',
      configTitle: '\u9762\u677f\u4e3b\u673a Ansible \u6267\u884c\u5668',
      configCopy: 'Ansible \u6a21\u5f0f\u662f\u65e0\u72b6\u6001\u7684\uff1a\u9762\u677f\u53ea\u5728 tunnel \u548c forward \u8bb0\u5f55\u4e2d\u4fdd\u5b58\u6267\u884c\u8282\u70b9\u6807\u8bc6\uff0cinventory\u3001playbook\u3001sudo \u548c SSH \u884c\u4e3a\u90fd\u5728\u8fd9\u91cc\u914d\u7f6e\u3002',
      recommended: '\u63a8\u8350',
      legacy: '\u65e7\u517c\u5bb9',
      executorEyebrow: '\u6267\u884c\u5668',
      defaultsAction: '\u4f7f\u7528\u540e\u7aef\u9ed8\u8ba4\u503c',
      executorHint: '\u4fdd\u5b58\u540e\u4f1a\u5c06 {backend} \u8bbe\u4e3a\u5f53\u524d\u672c\u5730 runtime\uff0c\u5e76\u5199\u5165 `forward.runtime_backend={backendKey}`\u3001`forward.runtime.ansible.backend={backendKey}` \u4ee5\u53ca `forward.runtime.nodex_mode=false`\u3002',
      fields: {
        inventory: 'Inventory',
        applyPlaybook: '\u4e0b\u53d1 Playbook',
        removePlaybook: '\u79fb\u9664 Playbook',
        command: '\u547d\u4ee4',
        workingDir: '\u5de5\u4f5c\u76ee\u5f55',
        targetPattern: '\u76ee\u6807\u6a21\u5f0f',
        timeoutSeconds: '\u8d85\u65f6\uff08\u79d2\uff09',
        ansibleConfig: 'ANSIBLE_CONFIG',
        useBecome: '\u5728\u6267\u884c\u8282\u70b9\u4e0a\u4f7f\u7528 sudo / become',
        extraVarsJson: '\u989d\u5916 vars JSON',
        environmentJson: '\u73af\u5883\u53d8\u91cf JSON',
        generatedJson: '\u751f\u6210\u7684\u8fd0\u884c\u65f6 JSON'
      },
      extraVarsHint: '\u540e\u7aef\u76f8\u5173\u5b57\u6bb5\uff08\u6bd4\u5982 firewall driver\uff09\u4f1a\u7531\u6240\u9009 backend \u81ea\u52a8\u6ce8\u5165\u3002',
      environmentHint: '\u9762\u677f\u4e3b\u673a\u6267\u884c\u5668\u8fdb\u7a0b\u7684\u989d\u5916\u73af\u5883\u53d8\u91cf\u3002',
      generatedHint: 'JSON \u8d1f\u8f7d\u7531\u4e0a\u9762\u7684\u7ed3\u6784\u5316\u5b57\u6bb5\u751f\u6210\uff0c\u5e76\u5b58\u5165 `forward.runtime.ansible.config`\u3002',
      probeTitle: '\u6267\u884c\u5668\u53ef\u8fde\u901a\u6027\u4e0e\u8fd0\u884c\u65f6\u5c31\u7eea\u5ea6',
      noStatus: '\u8fd8\u672a\u52a0\u8f7d\u672c\u5730\u8fd0\u884c\u65f6\u72b6\u6001\u3002',
      cards: {
        localActiveValue: '\u672c\u5730\u8fd0\u884c\u65f6\u5df2\u542f\u7528',
        standbyValue: '\u5f85\u547d\u914d\u7f6e',
        backend: '\u540e\u7aef',
        preferredLocalBackend: '\u9996\u9009\u672c\u5730 backend',
        attachment: '\u6302\u8f7d\u6a21\u5f0f',
        runtimeReady: '\u8fd0\u884c\u65f6\u5c31\u7eea',
        firewallDriver: 'Firewall driver',
        commandFound: '\u627e\u5230\u547d\u4ee4',
        become: 'Become',
        inventory: 'Inventory',
        applyPlaybook: '\u4e0b\u53d1 Playbook',
        removePlaybook: '\u79fb\u9664 Playbook',
        workingDir: '\u5de5\u4f5c\u76ee\u5f55'
      },
      latestJobs: '\u6700\u65b0 {backend} \u4efb\u52a1',
      noJobs: '\u6682\u65e0\u672c\u5730\u8fd0\u884c\u65f6\u4efb\u52a1\u3002',
      backends: {
        nftables: {
          label: 'nftables / Ansible',
          description: '\u73b0\u4ee3 Linux \u4e3b\u673a\u5e94\u4f18\u5148\u4f7f\u7528 nftables\u3002'
        }
      },
      errors: {
        savedConfigInvalid: '\u5df2\u4fdd\u5b58\u7684\u672c\u5730\u8fd0\u884c\u65f6\u914d\u7f6e\u65e0\u6548\uff0c\u5df2\u56de\u9000\u5230\u9ed8\u8ba4\u503c\uff0c\u8bf7\u91cd\u65b0\u4fdd\u5b58\u4ee5\u4fee\u590d\u3002',
        invalidJson: '{label} \u5fc5\u987b\u662f\u6709\u6548 JSON',
        invalidObject: '{label} \u5fc5\u987b\u662f JSON \u5bf9\u8c61',
        invalidPreview: '\u8fd0\u884c\u65f6\u914d\u7f6e\u65e0\u6548\uff1a{message}',
        invalidRuntimeJson: '\u672c\u5730\u8fd0\u884c\u65f6 JSON \u65e0\u6548',
        saveFailed: '\u4fdd\u5b58\u672c\u5730\u8fd0\u884c\u65f6\u914d\u7f6e\u5931\u8d25',
        fetchStatusFailed: '\u83b7\u53d6\u672c\u5730\u8fd0\u884c\u65f6\u72b6\u6001\u5931\u8d25',
        doctorFailed: '\u672c\u5730\u8fd0\u884c\u65f6 Doctor \u6267\u884c\u5931\u8d25'
      }
    },
    nodeX: {
      heroTextPrimary: '\u8fd9\u662f\u7ed9\u6709\u72b6\u6001 NodeX/gost \u8def\u5f84\u7684\u4e13\u7528\u64cd\u4f5c\u5165\u53e3\u3002\u5373\u4f7f\u5168\u5c40 runtime backend \u4ecd\u662f\u672c\u5730 Ansible\uff0c\u8fd9\u4e2a\u9875\u9762\u4e5f\u4f1a\u76f4\u63a5\u63a2\u6d4b\u5df2\u914d\u7f6e\u7684 NodeX \u63a7\u5236\u9762\u3002',
      heroTextSecondary: '\u672c\u5730 Ansible \u6267\u884c\u73b0\u5728\u653e\u5728 Local Runtime \u548c Ansible Machines \u4e0b\u3002\u8282\u70b9\u201c\u5728\u7ebf\u201d\u4ecd\u7136\u53ea\u8868\u793a TCP \u53ef\u8fde\u901a\uff0c\u4e0d\u4ee3\u8868 NodeX \u6216 relay gost API \u5df2\u7ecf\u6302\u8f7d\u6210\u529f\u3002',
      saveLoading: '\u4fdd\u5b58\u4e2d...',
      save: '\u4fdd\u5b58 NodeX \u914d\u7f6e',
      enabledBannerTitle: 'NodeX \u6a21\u5f0f\u5df2\u542f\u7528',
      disabledBannerTitle: 'NodeX \u6a21\u5f0f\u672a\u542f\u7528',
      enabledBannerText: '\u9762\u677f\u8f6c\u53d1\u4efb\u52a1\u53ef\u4ee5\u7ecf\u7531 NodeX/gost\uff0c\u4f46\u6bcf\u4e2a runtime \u4efb\u52a1\u4ecd\u7136\u5fc5\u987b\u6210\u529f\u624d\u4ee3\u8868 relay \u771f\u6b63\u6302\u8f7d\u5b8c\u6210\u3002',
      disabledBannerText: '\u4f60\u53ef\u4ee5\u5148\u5728\u8fd9\u91cc\u9a8c\u8bc1\u5df2\u914d\u7f6e\u7684 NodeX \u63a7\u5236\u9762\uff0c\u51c6\u5907\u597d\u540e\u518d\u5207\u6362\u5168\u5c40 backend\u3002',
      configTitle: 'NodeX \u63a7\u5236\u9762',
      enableModeTitle: '\u542f\u7528 NodeX \u6a21\u5f0f',
      enableModeHint: '\u4f1a\u5199\u5165 `forward.runtime.nodex_mode=true` \u548c `forward.runtime_backend=gost`\u3002',
      fields: {
        baseUrl: 'NodeX Base URL',
        baseUrlHint: '\u8fd9\u91cc\u5fc5\u987b\u6307\u5411 NodeX \u63a7\u5236\u9762\uff0c\u4e0d\u662f relay gost API \u672c\u8eab\u3002',
        token: 'NodeX Token',
        tokenHint: '\u9700\u4e0e NodeX \u63a7\u5236\u9762 `--forward-api-token` \u7684\u503c\u4e00\u81f4\u3002',
        timeout: '\u8d85\u65f6\uff08\u79d2\uff09',
        timeoutHint: '\u9762\u677f\u63a2\u6d4b\u6216\u6267\u884c NodeX runtime \u8bf7\u6c42\u65f6\u4f1a\u4f7f\u7528\u8be5\u8d85\u65f6\u503c\u3002'
      },
      probeTitle: '\u5065\u5eb7\u5ea6\u4e0e\u8fd0\u884c\u65f6\u72b6\u6001',
      probeCopy: '\u8fd9\u4e9b\u68c0\u67e5\u603b\u662f\u76f4\u63a5\u6307\u5411\u5df2\u914d\u7f6e\u7684 NodeX \u63a7\u5236\u9762\uff0c\u4e0d\u4f9d\u8d56\u5f53\u524d\u5168\u5c40 runtime backend\u3002',
      noStatus: '\u8fd8\u672a\u52a0\u8f7d NodeX \u8fd0\u884c\u65f6\u72b6\u6001\u3002',
      cards: {
        modeOn: 'NodeX \u6a21\u5f0f\u5df2\u5f00',
        modeOff: 'NodeX \u6a21\u5f0f\u5df2\u5173',
        backend: '\u540e\u7aef',
        baseUrl: 'Base URL',
        tokenConfigured: 'Token \u5df2\u914d\u7f6e',
        timeout: '\u8d85\u65f6',
        health: '\u5065\u5eb7\u68c0\u67e5',
        http: 'HTTP',
        version: '\u7248\u672c',
        executePath: '\u6267\u884c\u8def\u5f84'
      },
      jobsTitle: '\u6700\u65b0 gost \u4efb\u52a1',
      jobsCopy: '\u6700\u8fd1\u7684\u9762\u677f\u4fa7 runtime \u5ba1\u8ba1\u8bb0\u5f55\uff0c\u5df2\u6309 `gost` backend \u8fc7\u6ee4\u3002',
      noJobs: '\u6682\u65e0 gost \u8fd0\u884c\u65f6\u4efb\u52a1\u3002',
      backends: {
        gost: 'gost / NodeX'
      },
      errors: {
        baseUrlRequired: '\u542f\u7528 NodeX \u6a21\u5f0f\u65f6\u5fc5\u987b\u586b\u5199 NodeX Base URL',
        tokenRequired: '\u542f\u7528 NodeX \u6a21\u5f0f\u65f6\u5fc5\u987b\u586b\u5199 NodeX Token',
        saveFailed: '\u4fdd\u5b58 NodeX \u914d\u7f6e\u5931\u8d25',
        fetchStatusFailed: '\u83b7\u53d6 NodeX \u8fd0\u884c\u65f6\u72b6\u6001\u5931\u8d25',
        doctorFailed: 'NodeX runtime Doctor \u6267\u884c\u5931\u8d25'
      }
    },
    ansibleMachines: {
      heroText: '\u8fd9\u4e2a\u9875\u9762\u53ea\u7528\u4e8e\u65e0\u72b6\u6001 Ansible \u6267\u884c\u673a\u5668\u3002\u8fd9\u4e9b\u4e3b\u673a\u4e0d\u9700\u8981 Node-Agent\uff0c\u4e5f\u4e0d\u9700\u8981\u6301\u7eed\u63a7\u5236\u9762\u8fde\u63a5\u3002',
      addMachine: '\u6dfb\u52a0\u673a\u5668',
      table: {
        reachability: '\u53ef\u8fbe\u6027',
        lastResult: '\u6700\u8fd1\u7ed3\u679c'
      },
      stats: {
        machines: '\u673a\u5668',
        online: '\u5728\u7ebf',
        enabled: '\u5df2\u542f\u7528'
      },
      sectionTitle: '\u6267\u884c\u76ee\u6807',
      sectionCopy: '\u8fd9\u4e9b\u8bb0\u5f55\u53ea\u7528\u4e8e\u672c\u5730 Ansible \u8fd0\u884c\u65f6\u8bc6\u522b\u6267\u884c\u76ee\u6807\u4e3b\u673a\uff0c\u4e0d\u5c5e\u4e8e NodeX \u63a7\u5236\u9762\u8282\u70b9\u3002',
      inventoryHint: 'SSH \u7528\u6237\u540d\u3001\u5bc6\u7801\u548c\u79c1\u94a5\u4e0d\u5728\u6b64\u9875\u9762\u4fdd\u5b58\uff0c\u8bf7\u5728 Ansible inventory\u3001playbook \u6216 Local Runtime \u73af\u5883\u914d\u7f6e\u4e2d\u63d0\u4f9b\u3002',
      filterLabel: '\u72b6\u6001',
      filters: {
        online: '\u5728\u7ebf',
        offline: '\u79bb\u7ebf'
      },
      empty: '\u8fd8\u6ca1\u6709 Ansible \u6267\u884c\u673a\u5668',
      meta: {
        regionIsp: '\u533a\u57df / ISP',
        currentConn: '\u5f53\u524d\u8fde\u63a5',
        traffic: '\u6d41\u91cf'
      },
      actions: {
        edit: '\u7f16\u8f91',
        check: '\u5065\u5eb7\u68c0\u67e5',
        checking: '\u68c0\u67e5\u4e2d...',
        sync: '\u540c\u6b65\u7edf\u8ba1',
        syncing: '\u540c\u6b65\u4e2d...',
        disable: '\u7981\u7528',
        enable: '\u542f\u7528',
        delete: '\u5220\u9664',
        updating: '\u6b63\u5728\u66f4\u65b0 {name}\u2026'
      },
      modal: {
        titleEdit: '\u7f16\u8f91 Ansible \u673a\u5668',
        titleAdd: '\u6dfb\u52a0 Ansible \u673a\u5668',
        deleteTitle: '\u5220\u9664 Ansible \u673a\u5668 {name}\uff1f',
        deleteConfirm: '\u5b83\u5c06\u4ece Ansible \u6267\u884c\u673a\u7fa4\u4e2d\u79fb\u9664\u3002\u6b64\u64cd\u4f5c\u65e0\u6cd5\u64a4\u9500\u3002',
        deleteAction: '\u5220\u9664\u673a\u5668',
        saveLoading: '\u4fdd\u5b58\u4e2d...',
        save: '\u4fdd\u5b58',
        cancel: '\u53d6\u6d88'
      },
      messages: {
        saved: '\u5df2\u4fdd\u5b58 {name}',
        deleted: '\u5df2\u5220\u9664 {name}'
      },
      fields: {
        name: '\u540d\u79f0',
        host: '\u4e3b\u673a',
        reachabilityPort: '\u8fde\u901a\u6027\u7aef\u53e3',
        reachabilityHelp: '\u63a7\u5236\u9762\u53ea\u68c0\u67e5 host:port \u80fd\u5426\u5efa\u7acb TCP \u8fde\u63a5\u3002',
        weight: '\u6743\u91cd',
        region: '\u533a\u57df',
        isp: 'ISP'
      },
      placeholders: {
        name: 'relay-exec-01',
        host: '1.2.3.4',
        region: 'HK / JP / US',
        isp: 'CMI / NTT / Cogent'
      },
      results: {
        latency: '\u5ef6\u8fdf {value} ms',
        reachable: '\u673a\u5668\u53ef\u8fde\u901a',
        unavailable: '\u673a\u5668\u4e0d\u53ef\u7528',
        synced: '\u7edf\u8ba1\u5df2\u540c\u6b65'
      },
      errors: {
        required: '\u540d\u79f0\u3001\u4e3b\u673a\u548c\u8fde\u901a\u6027\u7aef\u53e3\u4e3a\u5fc5\u586b\u9879\u3002',
        saveFailed: '\u4fdd\u5b58\u673a\u5668\u5931\u8d25',
        loadFailed: '\u52a0\u8f7d Ansible \u673a\u5668\u5931\u8d25',
        detailFailed: '\u52a0\u8f7d\u673a\u5668\u8be6\u60c5\u5931\u8d25',
        deleteFailed: '\u5220\u9664\u673a\u5668\u5931\u8d25',
        checkFailed: '\u5065\u5eb7\u68c0\u67e5\u5931\u8d25',
        syncFailed: '\u540c\u6b65\u673a\u5668\u7edf\u8ba1\u5931\u8d25',
        toggleFailed: '\u5207\u6362\u673a\u5668\u72b6\u6001\u5931\u8d25'
      }
    },
    forward: {
      title: '流量转发管理',
      modeLabelNodeX: '\u5f53\u524d\u8fd0\u884c\u65f6\uff1aNodeX / gost',
      modeLabelLocal: '\u5f53\u524d\u8fd0\u884c\u65f6\uff1aLocal / {backend}',
      modeSummaryNodeX: '\u8bf7\u5728\u4e13\u7528\u7684 NodeX Runtime \u9875\u9762\u7f16\u8f91 NodeX \u63a7\u5236\u9762 URL\u3001Token \u548c gost \u64cd\u4f5c\u68c0\u67e5\u3002',
      modeSummaryLocal: '\u8bf7\u5728\u4e13\u7528\u7684 Local Runtime \u9875\u9762\u7f16\u8f91 inventory\u3001playbook \u548c\u9762\u677f\u4e3b\u673a\u6267\u884c\u5668\u914d\u7f6e\u3002',
      modeCompatibilityHint: '\u8f6c\u53d1\u7f16\u8f91\u5668\u4f1a\u6309\u5f53\u524d runtime \u81ea\u52a8\u8fc7\u6ee4\u53ef\u9009 tunnel\u3002\u672c\u5730 Ansible runtime \u53ea\u63a5\u53d7 Port Forward \u96a7\u9053\uff0cNodeX/gost \u5219\u53ef\u4ee5\u9644\u7740\u517c\u5bb9\u7684 Port Forward \u548c Tunnel Forward \u5e03\u5c40\u3002',
      nftablesHint: '当前运行时为 nftables / Ansible：「主备」和「Hash」策略只转发到第一个目标，限速不会生效。要在多个目标间分流请用「轮询」或「随机」，需要限速请使用 NodeX 运行时。',
      nftablesStrategyHint: '在 nftables / Ansible 运行时上，「主备」和「Hash」只使用第一个目标。',
      modeHintNodeX: 'NodeX/gost \u6a21\u5f0f\u4fdd\u7559 ingress \u548c exit \u8bed\u4e49\u3002\u5373\u4f7f\u5df2\u9009\u62e9 tunnel\uff0c\u4e5f\u4ecd\u9700 NodeX runtime \u4efb\u52a1\u6267\u884c\u6210\u529f\uff0c\u8f6c\u53d1\u624d\u7b97\u771f\u6b63\u6302\u8f7d\u3002',
      modeHintLocal: '\u672c\u5730 Ansible \u6a21\u5f0f\u53ea\u8bb0\u5f55\u6267\u884c\u8282\u70b9\u3002SSH \u8bbf\u95ee\u4f9d\u8d56\u5df2\u914d\u7f6e\u7684 ansible inventory \u548c local runtime \u53c2\u6570\uff0c\u4e0d\u6765\u81ea NodeX \u62d3\u6251\u8bb0\u5f55\u3002',
      tunnelHintNodeX: '{name} \u5c06\u901a\u8fc7 NodeX/gost \u6302\u8f7d\u3002\u9762\u677f\u4fa7\u201c\u5728\u7ebf\u201d\u6216\u72b6\u6001\u68c0\u67e5\u4e0d\u80fd\u8bc1\u660e\u8fdc\u7a0b relay \u5df2\u5b8c\u6210\u6302\u8f7d\u3002',
      tunnelHintLocal: '{name} \u53ea\u4f1a\u5728\u6267\u884c\u8282\u70b9\u4e0a\u88ab\u5e94\u7528\u3002\u8be5\u8def\u5f84\u4fdd\u6301\u65e0\u72b6\u6001\uff0c\u76f4\u5230\u6392\u961f\u7684 ansible \u4efb\u52a1\u6210\u529f\u7ed3\u675f\u3002',
      tunnelHintLocalIncompatible: '{name} \u662f Tunnel Forward \u96a7\u9053\uff0c\u53ea\u80fd\u7531 NodeX/gost \u6302\u8f7d\uff0c\u672c\u5730 Ansible runtime \u4e0d\u80fd\u76f4\u63a5\u9644\u7740\u5b83\u3002',
      portRange: '\u53ef\u7528\u7aef\u53e3\u8303\u56f4\uff1a{start} - {end}',
      portHintNodeX: '\u7aef\u53e3\u7559\u7a7a\u65f6\uff0c\u9762\u677f\u4f1a\u4ece tunnel \u5165\u53e3\u8282\u70b9\u7aef\u53e3\u6bb5\u4e2d\u81ea\u52a8\u5206\u914d\u3002',
      portHintLocal: '\u7aef\u53e3\u7559\u7a7a\u65f6\uff0c\u9762\u677f\u4f1a\u5728\u6240\u9009\u6267\u884c\u8282\u70b9\u4e0a\u81ea\u52a8\u5206\u914d\u3002',
      runtimeLinks: '运行时页面',
      loading: '\u6b63\u5728\u52a0\u8f7d\u8f6c\u53d1\u4e0e\u96a7\u9053\u6570\u636e...',
      view: {
        label: '视图',
        directLabel: '直连',
        groupedLabel: '分组'
      },
      actions: {
        import: '\u5bfc\u5165',
        export: '\u5bfc\u51fa',
        add: '\u65b0\u589e',
        edit: '\u7f16\u8f91',
        diagnose: '\u8bca\u65ad',
        delete: '\u5220\u9664',
        copyAll: '\u590d\u5236\u5168\u90e8',
        more: '更多操作',
        moveUp: '上移',
        moveDown: '下移'
      },
      bulk: {
        resume: '恢复',
        pause: '暂停',
        export: '导出',
        delete: '删除'
      },
      filters: {
        search: '搜索',
        searchPlaceholder: '规则、隧道、用户、地址或端口',
        tunnel: '隧道',
        allTunnels: '全部隧道',
        status: '状态',
        running: '运行中',
        paused: '已暂停',
        error: '错误'
      },
      table: {
        label: '转发规则',
        ruleName: '规则',
        tunnel: '隧道',
        ingress: '入口',
        target: '目标',
        policy: '策略',
        status: '状态',
        traffic: '流量',
        copyAddress: '复制{title}：{address}',
        toggleService: '{name} 的转发服务'
      },
      group: {
        userTag: '用户',
        summary: '{tunnels} 个隧道，{forwards} 个转发',
        tunnelMeta: 'Tunnel #{id}',
        runningCount: '{running}/{total} 运行中'
      },
      emptyGroupedTitle: '\u6682\u65e0\u8f6c\u53d1\u914d\u7f6e',
      emptyGroupedText: '\u5f53\u524d\u7cfb\u7edf\u91cc\u8fd8\u6ca1\u6709\u4efb\u4f55\u517c\u5bb9 flux-panel \u7684\u8f6c\u53d1\u8bb0\u5f55\u3002',
      emptyDirectTitle: '\u6682\u65e0\u8f6c\u53d1\u914d\u7f6e',
      emptyDirectText: '\u521b\u5efa\u7b2c\u4e00\u6761\u8f6c\u53d1\u540e\uff0c\u8fd9\u91cc\u4f1a\u663e\u793a\u5f53\u524d\u8f6c\u53d1\u7684\u76f4\u8fde\u8868\u683c\u89c6\u56fe\u3002',
      editor: {
        titleEdit: '\u7f16\u8f91\u8f6c\u53d1',
        titleAdd: '\u65b0\u589e\u8f6c\u53d1',
        description: '流量从隧道入口进入，转到下面的目标地址。',
        fields: {
          name: '\u8f6c\u53d1\u540d\u79f0',
          tunnel: '\u5173\u8054\u96a7\u9053',
          ingressPort: '\u5165\u53e3\u7aef\u53e3',
          interfaceName: '\u7f51\u5361\u540d\u79f0',
          remoteAddress: '\u76ee\u6807\u5730\u5740',
          strategy: '\u8c03\u5ea6\u7b56\u7565'
        },
        placeholders: {
          name: '\u4f8b\u5982\uff1aHK-Web-01',
          tunnel: '\u8bf7\u9009\u62e9\u96a7\u9053',
          ingressPort: '\u7559\u7a7a\u81ea\u52a8\u5206\u914d',
          interfaceName: '\u53ef\u9009\uff0c\u4f8b\u5982 eth0',
          remoteAddress: '\u6bcf\u884c\u4e00\u4e2a\u76ee\u6807\uff0c\u4f8b\u5982\uff1a\n1.1.1.1:443\nexample.com:8443\n[2001:db8::1]:443'
        },
        remoteHint: '\u652f\u6301 IPv4:port\u3001domain:port\u3001[\u5b8c\u6574 IPv6]:port\u3002\u591a\u5730\u5740\u8bf7\u6bcf\u884c\u4e00\u4e2a\u3002',
        submitUpdate: '\u4fdd\u5b58\u4fee\u6539',
        submitCreate: '\u521b\u5efa\u8f6c\u53d1'
      },
      deleteModal: {
        confirmText: '删除转发 {name}？',
        hint: '常规删除失败时，会再询问是否强制删除。此操作无法撤销。',
        confirmDelete: '删除转发',
        forceDeleteTitle: '强制删除 {name}？',
        forceDeleteAction: '强制删除',
        forceDeleteMessage: '常规删除失败（{message}）。强制删除不会确认节点上的转发服务是否已移除。'
      },
      addressModal: {
        copy: '\u590d\u5236',
        titleWithCount: '{title} ({count})'
      },
      exportModal: {
        title: '\u5bfc\u51fa\u8f6c\u53d1\u6570\u636e',
        subtitle: '格式：兼容 relay-panel 的 JSON：{\'[{ "dest": ["host:port"], "listen_port": 10086, "name": "规则" }\'}]',
        tunnelLabel: '\u9009\u62e9\u5bfc\u51fa\u96a7\u9053',
        tunnelPlaceholder: '\u8bf7\u9009\u62e9\u96a7\u9053',
        regenerate: '\u91cd\u65b0\u751f\u6210',
        generate: '\u751f\u6210\u5bfc\u51fa\u6570\u636e',
        selectionHint: '正在导出已选择的 {count} 条转发。',
        dataLabel: '导出数据'
      },
      importModal: {
        title: '\u5bfc\u5165\u8f6c\u53d1\u6570\u636e',
        subtitle: '支持 relay-panel JSON 和旧格式 remoteAddr{\'|\'}name{\'|\'}inPort，inPort 可留空。',
        subtitleSecondary: 'JSON 示例：{\'[{ "dest": ["3.3.3.3:3", "4.4.4.4:4"], "listen_port": 10086, "name": "业务入口" }\'}]',
        tunnelLabel: '\u9009\u62e9\u5bfc\u5165\u96a7\u9053',
        tunnelPlaceholder: '\u8bf7\u9009\u62e9\u96a7\u9053',
        dataLabel: '\u5bfc\u5165\u6570\u636e',
        placeholder: '{\'[{"dest":["example.com:8080"],"listen_port":10086,"name":"业务入口"}]\'}',
        resultTitle: '\u5bfc\u5165\u7ed3\u679c',
        resultSummary: '\u6210\u529f\uff1a{success} / \u603b\u8ba1\uff1a{total}',
        statusSuccess: '\u6210\u529f',
        statusFailed: '\u5931\u8d25',
        startImport: '\u5f00\u59cb\u5bfc\u5165'
      },
      diagnosis: {
        title: '\u8f6c\u53d1\u8bca\u65ad\u7ed3\u679c',
        loading: '\u6b63\u5728\u8bca\u65ad\u8f6c\u53d1\u8fde\u63a5...',
        connectionSuccess: '\u8fde\u63a5\u6210\u529f',
        connectionFailed: '\u8fde\u63a5\u5931\u8d25',
        nodeMeta: '{name} · {node}',
        nodeMeta: '{name} · {node}',
        nodeMeta: '{name} / {node}',
        targetAddress: '\u76ee\u6807\u5730\u5740',
        averageLatency: '\u5e73\u5747\u5ef6\u8fdf',
        packetLoss: '\u4e22\u5305\u7387',
        quality: '\u8d28\u91cf',
        failedFallback: '\u8bca\u65ad\u5931\u8d25',
        emptyTitle: '\u6682\u65e0\u8bca\u65ad\u6570\u636e',
        emptyText: '\u53d1\u8d77\u4e00\u6b21\u8bca\u65ad\u540e\uff0c\u8fd9\u91cc\u4f1a\u5c55\u793a\u4e0e\u53c2\u8003\u9875\u4e00\u81f4\u7684\u7ed3\u679c\u5361\u7247\u3002',
        rerun: '\u91cd\u65b0\u8bca\u65ad',
        summary: '{passed}/{total} 项通过'
      },
      status: {
        normal: '\u6b63\u5e38',
        paused: '\u6682\u505c',
        error: '\u5f02\u5e38',
        unknown: '\u672a\u77e5'
      },
      runtimeStatus: {
        pending: '\u5f85\u4e0b\u53d1',
        running: '\u6267\u884c\u4e2d',
        synced: '\u5df2\u540c\u6b65',
        applied: '\u5df2\u5e94\u7528',
        failed: '\u540c\u6b65\u5931\u8d25',
        queuedSummary: '\u8fd0\u884c\u65f6\u4efb\u52a1\u5df2\u5165\u961f\uff0c\u7b49\u5f85\u6267\u884c\u5668\u5b8c\u6210\u3002',
        runningSummary: '\u8fd0\u884c\u65f6\u4efb\u52a1\u6b63\u5728\u6267\u884c\u3002'
      },
      strategy: {
        fifo: '\u4e3b\u5907',
        round: '\u8f6e\u8be2',
        rand: '\u968f\u673a',
        hash: 'Hash',
        unknown: '\u672a\u77e5'
      },
      quality: {
        unknown: '\u672a\u77e5',
        excellent: '\u4f18\u79c0',
        veryGood: '\u5f88\u597d',
        good: '\u826f\u597d',
        fair: '\u4e00\u822c',
        poor: '\u8f83\u5dee',
        veryPoor: '\u5f88\u5dee'
      },
      labels: {
        inbound: '\u5165',
        outbound: '\u51fa'
      },
      references: {
        tunnel: '\u96a7\u9053 #{id}',
        node: '\u8282\u70b9 {id}',
        node: '\u8282\u70b9 #{id}'
      },
      messages: {
        loadForwardsFailed: '\u83b7\u53d6\u8f6c\u53d1\u5217\u8868\u5931\u8d25',
        loadTunnelsFailed: '\u83b7\u53d6\u96a7\u9053\u5217\u8868\u5931\u8d25',
        loadDataFailed: '\u52a0\u8f7d\u6570\u636e\u5931\u8d25',
        unknownUser: '\u672a\u77e5\u7528\u6237',
        nameRequired: '\u8bf7\u8f93\u5165\u8f6c\u53d1\u540d\u79f0',
        nameLength: '\u8f6c\u53d1\u540d\u79f0\u957f\u5ea6\u5e94\u5728 2-50 \u4e2a\u5b57\u7b26\u4e4b\u95f4',
        tunnelRequired: '\u8bf7\u9009\u62e9\u5173\u8054\u96a7\u9053',
        remoteAddrRequired: '\u8bf7\u8f93\u5165\u8fdc\u7a0b\u5730\u5740',
        remoteAddrLineInvalid: '\u7b2c {line} \u884c\u5730\u5740\u683c\u5f0f\u9519\u8bef',
        portRange: '\u7aef\u53e3\u53f7\u5fc5\u987b\u5728 1-65535 \u4e4b\u95f4',
        portRangeTunnel: '\u7aef\u53e3\u53f7\u5fc5\u987b\u5728 {start}-{end} \u8303\u56f4\u5185',
        updated: '\u4fee\u6539\u6210\u529f',
        created: '\u521b\u5efa\u6210\u529f',
        actionFailed: '\u64cd\u4f5c\u5931\u8d25',
        runtimeBusy: '\u5f53\u524d\u8fd0\u884c\u65f6\u4efb\u52a1\u4ecd\u5728\u6392\u961f\u6216\u6267\u884c\u4e2d\uff0c\u8bf7\u7b49\u5f85\u5b8c\u6210\u540e\u518d\u64cd\u4f5c',
        invalidStatus: '\u8f6c\u53d1\u72b6\u6001\u5f02\u5e38\uff0c\u65e0\u6cd5\u64cd\u4f5c',
        serviceChanged: '\u670d\u52a1\u53d8\u66f4\u5df2\u63d0\u4ea4',
        servicePaused: '\u6682\u505c\u8bf7\u6c42\u5df2\u63d0\u4ea4',
        networkActionFailed: '\u7f51\u7edc\u9519\u8bef\uff0c\u64cd\u4f5c\u5931\u8d25',
        bulkActionComplete: '批量操作完成：成功 {success} 条，失败 {failed} 条',
        bulkDeleteTitle: '删除选中的 {count} 条转发？',
        bulkDeleteMessage: '批量模式下，常规删除失败的转发不会被强制删除。此操作无法撤销。',
        bulkDeleteAction: '删除转发',
        deleted: '\u5220\u9664\u6210\u529f',
        forceDeleted: '\u5f3a\u5236\u5220\u9664\u6210\u529f',
        forceDeleteFailed: '\u5f3a\u5236\u5220\u9664\u5931\u8d25',
        deleteFailed: '\u5220\u9664\u5931\u8d25',
        diagnosisFailed: '\u8bca\u65ad\u5931\u8d25',
        diagnosisProcessingFailed: '\u8bca\u65ad\u8fc7\u7a0b\u4e2d\u53d1\u751f\u9519\u8bef',
        diagnosisNetworkFailed: '\u7f51\u7edc\u9519\u8bef\uff0c\u8bca\u65ad\u5931\u8d25',
        unableConnectServer: '\u65e0\u6cd5\u8fde\u63a5\u5230\u670d\u52a1\u5668',
        contentCopied: '{label}\u5df2\u590d\u5236',
        copyFailedHttp: '\u590d\u5236\u5931\u8d25\uff1aHTTP \u4e0b\u65e0\u6cd5\u590d\u5236\uff0c\u9700 HTTPS/\u53cd\u4ee3',
        selectExportTunnel: '\u8bf7\u9009\u62e9\u8981\u5bfc\u51fa\u7684\u96a7\u9053',
        noExportData: '\u6240\u9009\u96a7\u9053\u6ca1\u6709\u8f6c\u53d1\u6570\u636e',
        exportFailed: '\u5bfc\u51fa\u5931\u8d25',
        enterImportData: '\u8bf7\u8f93\u5165\u8981\u5bfc\u5165\u7684\u6570\u636e',
        selectImportTunnel: '\u8bf7\u9009\u62e9\u8981\u5bfc\u5165\u7684\u96a7\u9053',
        importCompleted: '\u5bfc\u5165\u6267\u884c\u5b8c\u6210',
        importFailed: '\u5bfc\u5165\u8fc7\u7a0b\u4e2d\u53d1\u751f\u9519\u8bef',
        importFormatError: '\u683c\u5f0f\u9519\u8bef\uff1a\u81f3\u5c11\u9700\u8981\u5305\u542b\u76ee\u6807\u5730\u5740\u548c\u8f6c\u53d1\u540d\u79f0',
        importRequiredFields: '\u76ee\u6807\u5730\u5740\u548c\u8f6c\u53d1\u540d\u79f0\u4e0d\u80fd\u4e3a\u7a7a',
        importAddressInvalid: '\u76ee\u6807\u5730\u5740\u683c\u5f0f\u9519\u8bef\uff0c\u5e94\u4e3a host:port\uff0c\u591a\u4e2a\u5730\u5740\u7528\u9017\u53f7\u5206\u9694',
        importPortInvalid: '\u5165\u53e3\u7aef\u53e3\u683c\u5f0f\u9519\u8bef\uff0c\u5e94\u4e3a 1-65535 \u4e4b\u95f4\u7684\u6570\u5b57',
        importCreateSuccess: '\u521b\u5efa\u6210\u529f',
        importCreateFailed: '\u521b\u5efa\u5931\u8d25',
        importNetworkCreateFailed: '\u7f51\u7edc\u9519\u8bef\uff0c\u521b\u5efa\u5931\u8d25',
        orderSaveFailed: '\u4fdd\u5b58\u6392\u5e8f\u5931\u8d25\uff1a{message}',
        orderSaveRetry: '\u4fdd\u5b58\u6392\u5e8f\u5931\u8d25\uff0c\u8bf7\u91cd\u8bd5',
        unknownError: '\u672a\u77e5\u9519\u8bef',
        localRuntimeTunnelForwardUnsupported: '\u672c\u5730 Ansible runtime \u4e0d\u80fd\u9644\u7740 Tunnel Forward \u96a7\u9053\uff0c\u8bf7\u5207\u6362\u5230 NodeX Runtime \u6216\u6539\u9009 Port Forward \u96a7\u9053'
      },
      card: {
        dragHandleTitle: '\u62d6\u62fd\u6392\u5e8f',
        ingressAddressTitle: '\u5165\u53e3\u5730\u5740',
        targetAddressTitle: '\u76ee\u6807\u5730\u5740',
        targetLabel: '\u76ee\u6807'
      }
    },
    tunnel: {
      note: 'NodeX \u6a21\u5f0f\u4f1a\u62c6\u5206 ingress \u548c\u6267\u884c\u8282\u70b9\uff1b\u672c\u5730 Ansible \u6a21\u5f0f\u53ea\u9700\u8981 inventory \u4e2d\u6620\u5c04\u7684\u6267\u884c\u8282\u70b9\u3002\u96a7\u9053\u201c\u5728\u7ebf\u201d\u53ea\u68c0\u67e5 host:port \u53ef\u8fde\u901a\u6027\uff0c\u4e0d\u80fd\u786e\u8ba4\u8fdc\u7a0b\u6302\u8f7d\u6216\u9632\u706b\u5899\u72b6\u6001\u5df2\u7ecf\u5c31\u4f4d\u3002',
      modeLabelNodeX: '\u5f53\u524d\u8fd0\u884c\u65f6\uff1aNodeX / gost',
      modeLabelLocal: '\u5f53\u524d\u8fd0\u884c\u65f6\uff1aLocal / {backend}',
      modeSummaryNodeX: 'ingress \u4e0e egress \u8bed\u4e49\u7531 NodeX/gost \u63a7\u5236\u3002NodeX Runtime \u9875\u9762\u8d1f\u8d23\u63a7\u5236\u9762 URL\u3001Token \u548c gost \u5c31\u7eea\u6027\u3002',
      modeSummaryLocal: '\u8fd9\u91cc\u53ea\u5b58\u50a8\u6267\u884c\u8282\u70b9\u8eab\u4efd\u3002inventory\u3001playbook \u548c\u9762\u677f\u5bbf\u4e3b ansible \u6267\u884c\u5668\u8bf7\u5230 Local Runtime \u9875\u9762\u7ba1\u7406\u3002',
      modeCompatibilityHint: '下方隧道列表\u662f\u6309\u201c\u5f53\u524d\u8fd0\u884c\u65f6\u80fd\u5426\u6267\u884c\u201d\u6765\u6807\u8bb0\u7684\u3002\u4e00\u4e9b\u5386\u53f2 type-1 \u96a7\u9053\u5728\u8868\u7ed3\u6784\u4e0a\u53ef\u80fd\u540c\u65f6\u517c\u5bb9\u4e24\u79cd\u8fd0\u884c\u65f6\uff0c\u4e0d\u8981\u4ec5\u51ed\u5b58\u50a8\u5b57\u6bb5\u63a8\u65ad\u5b83\u5c5e\u4e8e NodeX \u8fd8\u662f Ansible\u3002',
      runtimeLinks: '运行时页面',
      emptyTitle: '\u6682\u65e0\u96a7\u9053\u914d\u7f6e',
      emptyText: '\u8bf7\u5148\u5b8c\u6210\u6240\u9700\u62d3\u6251\uff0c\u518d\u521b\u5efa\u7b2c\u4e00\u4e2a\u53ef\u88ab\u8f6c\u53d1\u5f15\u7528\u7684\u96a7\u9053\u3002',
      actions: {
        add: '\u65b0\u589e\u96a7\u9053',
        edit: '\u7f16\u8f91',
        diagnose: '\u8bca\u65ad',
        delete: '\u5220\u9664'
      },
      table: {
        label: '隧道列表',
        compatibility: '运行时兼容性',
        status: '状态'
      },
      meta: {
        ingressNode: '\u8f6c\u53d1\u5165\u53e3\u8282\u70b9',
        egressNode: '\u8f6c\u53d1\u51fa\u53e3\u8282\u70b9',
        executionNode: '\u4e2d\u8f6c\u6267\u884c\u8282\u70b9',
        flowAccounting: '\u6d41\u91cf\u8ba1\u7b97',
        trafficRatio: '\u6d41\u91cf\u500d\u7387'
      },
      modal: {
        titleEdit: '\u7f16\u8f91\u96a7\u9053',
        titleAdd: '\u65b0\u589e\u96a7\u9053',
        deleteConfirmMessage: '删除隧道 {name}？',
        deleteHint: '如果该隧道仍被转发规则或用户授权引用，将无法删除。此操作无法撤销。',
        submitUpdate: '\u66f4\u65b0',
        submitCreate: '\u521b\u5efa',
        confirmDelete: '删除隧道'
      },
      fields: {
        name: '\u96a7\u9053\u540d\u79f0',
        tunnelType: '\u96a7\u9053\u7c7b\u578b',
        flowAccounting: '\u6d41\u91cf\u8ba1\u7b97',
        trafficRatio: '\u6d41\u91cf\u500d\u7387',
        ingressNode: 'NodeX \u5165\u53e3\u8282\u70b9',
        executionNode: '\u4e2d\u8f6c\u6267\u884c\u8282\u70b9',
        tcpListenAddr: 'TCP \u76d1\u542c\u5730\u5740',
        udpListenAddr: 'UDP \u76d1\u542c\u5730\u5740',
        interfaceName: '\u51fa\u53e3\u7f51\u5361\u540d\u6216 IP',
        protocol: '\u534f\u8bae\u7c7b\u578b',
        egressNode: '\u8f6c\u53d1\u51fa\u53e3\u8282\u70b9'
      },
      placeholders: {
        name: 'HK-Tunnel-01',
        interfaceName: 'eth0 / 192.0.2.10'
      },
      options: {
        portForward: '\u7aef\u53e3\u8f6c\u53d1',
        tunnelForward: '\u96a7\u9053\u8f6c\u53d1',
        oneWayAccounting: '\u5355\u5411\u8ba1\u7b97',
        twoWayAccounting: '\u53cc\u5411\u8ba1\u7b97'
      },
      hints: {
        ingressNode: '\u53ea\u6709 NodeX/gost \u6a21\u5f0f\u4f1a\u5728\u8fd9\u91cc\u4f7f\u7528 ingress \u8282\u70b9\u3002\u8fd9\u662f\u8f6c\u53d1 relay \u89d2\u8272\uff0c\u4e0e\u4ee3\u7406\u8282\u70b9\u4fdd\u6301\u5206\u79bb\u3002',
        executionNode: '\u672c\u5730 Ansible \u6a21\u5f0f\u53ea\u9700\u8981\u6267\u884c\u8282\u70b9\u8eab\u4efd\u3002SSH \u8bbf\u95ee\u4ecd\u7136\u6765\u81ea\u5df2\u914d\u7f6e\u7684 inventory \u548c local runtime \u53c2\u6570\u3002',
        egressNode: '\u51fa\u53e3\u8282\u70b9\u53ea\u7528\u4e8e NodeX/gost \u96a7\u9053\u8f6c\u53d1\u3002\u9762\u677f\u4fdd\u5b58\u6210\u529f\u540e\uff0c\u4ecd\u7136\u9700\u8981 runtime \u4efb\u52a1\u5728\u8fdc\u7a0b\u5b8c\u6210\u6302\u8f7d\u3002'
      },
      compatibility: {
        nodeXReady: '\u53ef\u7528\u4e8e NodeX Runtime',
        nodeXNeedsIngress: 'NodeX Runtime \u9700\u8981\u5165\u53e3\u8282\u70b9',
        nodeXNeedsEgress: 'NodeX \u96a7\u9053\u8f6c\u53d1\u7f3a\u5c11\u51fa\u53e3\u8282\u70b9',
        localReady: '\u53ef\u7528\u4e8e Local Runtime',
        localNeedsExecution: 'Local Runtime \u9700\u8981\u6267\u884c\u8282\u70b9',
        localOnlyPortForward: 'Local Runtime \u53ea\u652f\u6301\u7aef\u53e3\u8f6c\u53d1\u96a7\u9053'
      },
      messages: {
        loadListFailed: '\u83b7\u53d6\u96a7\u9053\u5217\u8868\u5931\u8d25',
        loadDataFailed: '\u52a0\u8f7d\u6570\u636e\u5931\u8d25',
        created: '\u96a7\u9053\u521b\u5efa\u6210\u529f',
        updated: '\u96a7\u9053\u66f4\u65b0\u6210\u529f',
        actionFailed: '\u64cd\u4f5c\u5931\u8d25',
        deleted: '\u96a7\u9053\u5220\u9664\u6210\u529f',
        deleteFailed: '\u5220\u9664\u5931\u8d25',
        diagnosis: '\u96a7\u9053\u8bca\u65ad',
        diagnosisFailed: '\u8bca\u65ad\u5931\u8d25',
        diagnosisRequestFailed: '\u8bca\u65ad\u8bf7\u6c42\u5931\u8d25'
      },
      diagnosis: {
        title: '\u96a7\u9053\u8bca\u65ad\u7ed3\u679c',
        loading: '\u6b63\u5728\u8bca\u65ad\u96a7\u9053\u8fde\u901a\u6027...',
        targetAddress: '\u76ee\u6807\u5730\u5740',
        duration: '\u8017\u65f6',
        emptyTitle: '\u6682\u65e0\u8bca\u65ad\u7ed3\u679c',
        emptyText: '\u5f53\u524d\u6ca1\u6709\u53ef\u5c55\u793a\u7684\u8282\u70b9\u8bca\u65ad\u6570\u636e\u3002',
        rerun: '\u91cd\u65b0\u8bca\u65ad',
        summary: '{passed}/{total} 项通过'
      },
      validation: {
        nameRequired: '\u8bf7\u8f93\u5165\u96a7\u9053\u540d\u79f0',
        nameLength: '\u96a7\u9053\u540d\u79f0\u957f\u5ea6\u5e94\u5728 2-50 \u4e2a\u5b57\u7b26\u4e4b\u95f4',
        typeInvalid: '\u8bf7\u9009\u62e9\u6709\u6548\u7684\u96a7\u9053\u7c7b\u578b',
        ingressRequired: '\u8bf7\u9009\u62e9\u8f6c\u53d1\u5165\u53e3\u8282\u70b9',
        ingressMustRelay: '\u5165\u53e3\u8282\u70b9\u5fc5\u987b\u662f\u8f6c\u53d1\u4e2d\u7ee7\u8282\u70b9',
        trafficRatio: '\u6d41\u91cf\u500d\u7387\u5fc5\u987b\u5728 0.1-100.0 \u4e4b\u95f4',
        tcpListenRequired: '\u8bf7\u8f93\u5165 TCP \u76d1\u542c\u5730\u5740',
        udpListenRequired: '\u8bf7\u8f93\u5165 UDP \u76d1\u542c\u5730\u5740',
        egressRequired: '\u8bf7\u9009\u62e9\u8f6c\u53d1\u51fa\u53e3\u8282\u70b9',
        ingressEgressDifferent: '\u8f6c\u53d1\u5165\u53e3\u8282\u70b9\u548c\u8f6c\u53d1\u51fa\u53e3\u8282\u70b9\u4e0d\u80fd\u76f8\u540c',
        egressMustExit: '\u51fa\u53e3\u8282\u70b9\u5fc5\u987b\u662f\u8f6c\u53d1\u51fa\u53e3\u8282\u70b9',
        protocolRequired: '\u8bf7\u9009\u62e9\u534f\u8bae\u7c7b\u578b',
        executionRequired: '\u8bf7\u9009\u62e9\u4e2d\u8f6c\u6267\u884c\u8282\u70b9',
        executionMustRelay: '\u4e2d\u8f6c\u6267\u884c\u8282\u70b9\u5fc5\u987b\u662f\u8f6c\u53d1\u4e2d\u7ee7\u8282\u70b9'
      }
    },
    workbench: {
      actions: {
        refreshJobs: '\u5237\u65b0\u4efb\u52a1',
        openAnsibleMachines: '\u6253\u5f00 Ansible Machines',
        runDoctorActiveRuntime: '\u5bf9\u5f53\u524d\u8fd0\u884c\u65f6\u8fd0\u884c Doctor'
      },
      localCard: {
        title: '\u672c\u5730 Ansible \u6267\u884c\u5668',
        description: '\u9762\u677f\u4e3b\u673a\u65e0\u72b6\u6001\u6267\u884c\u3002Inventory\u3001playbook \u548c SSH \u8bbf\u95ee\u72ec\u7acb\u4e8e NodeX \u7ba1\u7406\u3002',
        currentState: '\u5f53\u524d\u72b6\u6001',
        manage: '\u7ba1\u7406 Local Runtime / Ansible'
      },
      nodeXCard: {
        title: '\u6709\u72b6\u6001 gost \u63a7\u5236\u9762',
        description: '\u9762\u677f\u4f1a\u76f4\u63a5\u8fde\u63a5\u5185\u90e8 NodeX \u63a7\u5236\u9762\u3002\u53ea\u6709 gost runtime \u4efb\u52a1\u6210\u529f\u540e\uff0crelay \u624d\u7b97\u771f\u6b63\u6302\u8f7d\u3002',
        manage: '\u7ba1\u7406 NodeX Runtime'
      },
      state: {
        activeBackend: '\u5df2\u6fc0\u6d3b',
        standby: '\u5f85\u547d'
      },
      references: {
        panelRuntimeDoc: '\u9762\u677f\u6587\u6863: docs/reference/runtime.md',
        panelRelayOnboarding: '\u9762\u677f\u6587\u6863: docs/guide/forward-relay-onboarding.md',
        nodeXRepo: 'NodeX \u4ed3\u5e93: https://github.com/zdwtest/NodeX',
        panelNodeXOnboarding: '\u9762\u677f\u6587\u6863: docs/forward-runtime-relay-onboarding.md'
      },
      recentJobsTitle: '\u6700\u8fd1\u8fd0\u884c\u65f6\u4efb\u52a1',
      recentJobsSubtitle: '\u663e\u793a Local Runtime \u548c NodeX Runtime \u4e13\u7528\u9875\u9762\u6700\u8fd1\u6392\u961f\u6216\u5df2\u6267\u884c\u7684\u52a8\u4f5c\u3002',
      noJobs: '\u6682\u65e0\u8fd0\u884c\u65f6\u4efb\u52a1\u3002',
      jobMeta: '{backend} / forward {forwardId} / tunnel {tunnelId} / node {nodeId}',
      doctor: {
        title: '\u5f53\u524d\u8fd0\u884c\u65f6\u5feb\u7167',
        description: '\u53ef\u8fde\u901a\u53ea\u80fd\u8bf4\u660e\u63a7\u5236\u9762\u6216\u672c\u5730\u6267\u884c\u5668\u53ef\u4ee5\u8bbf\u95ee\uff0c\u5e76\u4e0d\u80fd\u8bc1\u660e relay \u5df2\u6302\u8f7d\u6210\u529f\uff0c\u4e5f\u4e0d\u4ee3\u8868 iptables \u89c4\u5219\u5df2\u5b58\u5728\u3002',
        note: '\u8fd9\u4e2a workbench \u53ea\u663e\u793a\u5f53\u524d\u6d3b\u8dc3 backend\u3002\u82e5\u8981\u7f16\u8f91\u914d\u7f6e\u6216\u8fd0\u884c\u6a21\u5f0f\u7279\u5b9a\u63a2\u6d4b\uff0c\u8bf7\u524d\u5f80 Local Runtime \u548c NodeX Runtime \u4e13\u9875\u3002',
        summary: '\u8fd9\u4e2a workbench \u6c47\u603b\u63a7\u5236\u9762\u5065\u5eb7\u3001\u8fd0\u884c\u65f6\u8bca\u65ad\u548c\u4e00\u952e\u547d\u4ee4\uff0c\u7edf\u4e00\u8986\u76d6 NodeX/gost \u548c\u672c\u5730 Ansible \u4e24\u79cd\u6267\u884c\u8def\u5f84\u3002',
        loadingStatus: '\u6b63\u5728\u83b7\u53d6 forward runtime \u72b6\u6001...'
      },
      cards: {
        backend: 'Backend',
        nodeXMode: 'NodeX Mode',
        attachment: '\u6302\u8f7d\u6a21\u5f0f',
        panelVerdict: '\u9762\u677f\u5224\u5b9a',
        nodeXSnapshot: 'NodeX Snapshot',
        baseUrlConfigured: 'Base URL \u5df2\u914d\u7f6e',
        runtimeVersion: '\u8fd0\u884c\u65f6\u7248\u672c',
        localAnsible: '\u672c\u5730 ansible',
        playbooks: 'Playbooks'
      },
      errors: {
        fetchStatusFailed: '\u83b7\u53d6 forward runtime \u72b6\u6001\u5931\u8d25',
        doctorFailed: 'Forward runtime Doctor \u6267\u884c\u5931\u8d25'
      }
    },
    nodeXTopology: {
      emptyTitle: '\u6682\u65e0 NodeX \u62d3\u6251\u8282\u70b9',
      emptyText: '\u8bf7\u5148\u521b\u5efa relay / exit \u8282\u70b9\u7528\u4e8e NodeX \u6a21\u5f0f\u3002\u82e5\u53ea\u505a\u65e0\u72b6\u6001\u6267\u884c\uff0c\u8bf7\u6539\u5230 Ansible Machines \u9875\u9762\u3002',
      legacyText: '\u8fd9\u4e2a\u533a\u5757\u5bf9\u5e94 `/admin/forward/rules*` \u517c\u5bb9\u63a5\u53e3\u3002\u5b83\u53ea\u4fdd\u7559 Legacy \u89c4\u5219\u80fd\u529b\uff0c\u4e0d\u4ee3\u8868 NodeX \u6216 Ansible \u7684\u5f53\u524d\u4e3b\u8fd0\u884c\u8def\u5f84\u3002',
      actions: {
        refresh: '\u5237\u65b0',
        testConnection: '\u6d4b\u8bd5\u8fde\u63a5',
        addNode: '\u65b0\u589e\u8282\u70b9',
        clear: '\u6e05\u7a7a',
        addRule: '\u65b0\u589e\u89c4\u5219',
        edit: '\u7f16\u8f91',
        healthCheck: '\u5065\u5eb7\u68c0\u67e5',
        checking: '\u68c0\u6d4b\u4e2d...',
        syncStats: '\u540c\u6b65\u7edf\u8ba1',
        syncing: '\u540c\u6b65\u4e2d...',
        enable: '\u542f\u7528',
        disable: '\u7981\u7528',
        delete: '\u5220\u9664',
        startTest: '\u5f00\u59cb\u68c0\u6d4b',
        cancel: '\u53d6\u6d88',
        saveChanges: '\u4fdd\u5b58\u4fee\u6539',
        createNode: '\u521b\u5efa\u8282\u70b9',
        createRule: '\u521b\u5efa\u89c4\u5219'
      },
      filters: {
        online: '\u5728\u7ebf',
        offline: '\u79bb\u7ebf',
        userId: '\u7528\u6237 ID',
        userIdPlaceholder: '\u6309\u7528\u6237 ID \u8fc7\u6ee4',
        relay: '中继',
        exit: '出口'
      },
      status: {
        enabled: '\u5df2\u542f\u7528',
        disabled: '\u5df2\u7981\u7528',
        online: '\u5728\u7ebf',
        offline: '\u79bb\u7ebf'
      },
      meta: {
        latency: '\u5ef6\u8fdf',
        currentConnections: '\u5f53\u524d\u8fde\u63a5',
        traffic: '\u4e0a\u884c / \u4e0b\u884c',
        lastCheck: '\u6700\u540e\u68c0\u6d4b',
        uptime: '\u5728\u7ebf\u7387',
        rateLimit: '\u901f\u7387',
        trafficLimit: '\u6d41\u91cf',
        expire: '\u8fc7\u671f',
        upload: '\u4e0a\u884c',
        download: '\u4e0b\u884c',
        connections: '\u8fde\u63a5',
        serviceCount: '\u670d\u52a1\u6570\u91cf'
      },
      legacy: {
        title: '旧版端口转发规则',
        emptyTitle: '\u6682\u65e0 Legacy \u89c4\u5219',
        emptyText: '\u5982\u679c\u9700\u8981\u517c\u5bb9 relay + exit \u7aef\u53e3\u7ea7\u8f6c\u53d1\uff0c\u53ef\u5148\u5728\u8fd9\u91cc\u65b0\u589e\u89c4\u5219\u3002'
      },
      nodeModal: {
        titleEdit: '\u7f16\u8f91\u4e2d\u8f6c\u8282\u70b9',
        titleAdd: '\u65b0\u589e\u4e2d\u8f6c\u8282\u70b9',
        loading: '\u6b63\u5728\u52a0\u8f7d\u8282\u70b9\u8be6\u60c5...',
        saveLoading: '\u4fdd\u5b58\u4e2d...',
        fields: {
          name: '\u8282\u70b9\u540d\u79f0',
          type: '\u8282\u70b9\u7c7b\u578b',
          host: '\u4e3b\u673a\u5730\u5740',
          servicePort: '\u4e1a\u52a1\u7aef\u53e3',
          apiPort: 'API \u7aef\u53e3',
          apiToken: 'API Token',
          metricsPort: 'Metrics \u7aef\u53e3',
          region: '\u5730\u533a',
          isp: 'ISP',
          bandwidth: '\u5e26\u5bbd (Mbps)',
          maxConnections: '\u6700\u5927\u8fde\u63a5',
          weight: '\u6743\u91cd'
        },
        placeholders: {
          name: '\u4f8b\u5982 relay-hk-01',
          host: '1.2.3.4',
          apiToken: '\u7559\u7a7a\u5219\u540e\u7aef\u81ea\u52a8\u751f\u6210',
          apiTokenKept: '\u5df2\u4fdd\u5b58\u7684\u4ee4\u724c\u4e0d\u518d\u663e\u793a\uff1b\u7559\u7a7a\u5373\u4fdd\u7559',
          region: 'HK / JP / US',
          isp: 'CMI / NTT / Cogent'
        },
        hints: {
          apiPort: 'NodeX \u7ba1\u7406 API \u7684\u5065\u5eb7\u68c0\u67e5\u3001\u7edf\u8ba1\u540c\u6b65\u548c\u8fde\u901a\u6d4b\u8bd5\u90fd\u9700\u8981\u8be5\u7aef\u53e3\u3002',
          metricsPort: 'gost Prometheus /metrics \u7aef\u53e3\uff0c\u7528\u4e8e\u91c7\u96c6\u8f6c\u53d1\u6d41\u91cf\u7edf\u8ba1\uff0c\u7559\u7a7a\u8868\u793a\u4e0d\u91c7\u96c6\u3002',
          apiTokenKept: 'API Token \u53ea\u5728\u521b\u5efa\u8282\u70b9\u65f6\u663e\u793a\u4e00\u6b21\u3002\u7559\u7a7a\u5373\u4fdd\u7559\u539f\u4ee4\u724c\uff0c\u586b\u5199\u65b0\u4ee4\u724c\u5219\u66ff\u6362\u3002'
        }
      },
      ruleModal: {
        titleEdit: '\u7f16\u8f91 Legacy \u89c4\u5219',
        titleAdd: '\u65b0\u589e Legacy \u89c4\u5219',
        loading: '\u6b63\u5728\u52a0\u8f7d\u89c4\u5219\u8be6\u60c5...',
        ownerReadOnlyHint: '\u5f53\u524d\u540e\u7aef\u66f4\u65b0\u63a5\u53e3\u4e0d\u652f\u6301\u4fee\u6539\u5f52\u5c5e\u5b57\u6bb5\uff0c\u7f16\u8f91\u65f6\u53ea\u8bfb\u3002',
        fields: {
          name: '\u89c4\u5219\u540d\u79f0',
          protocol: '\u534f\u8bae',
          relayNode: '\u5165\u53e3 Relay \u8282\u70b9',
          listenPort: '\u76d1\u542c\u7aef\u53e3',
          exitNode: '\u51fa\u53e3 Exit \u8282\u70b9',
          targetPort: '\u76ee\u6807\u7aef\u53e3',
          targetHost: '\u76ee\u6807\u4e3b\u673a',
          userId: '\u7528\u6237 ID',
          userGroupId: '\u7528\u6237\u7ec4 ID',
          speedLimit: '\u901f\u7387\u4e0a\u9650 (KB/s)',
          trafficLimit: '\u6d41\u91cf\u4e0a\u9650 (Bytes)',
          expireTime: '\u8fc7\u671f\u65f6\u95f4',
          remark: '\u5907\u6ce8'
        },
        placeholders: {
          name: '\u4f8b\u5982 tcp-11111-hk',
          relayNode: '\u8bf7\u9009\u62e9 Relay \u8282\u70b9',
          exitNode: '\u8bf7\u9009\u62e9 Exit \u8282\u70b9',
          targetHost: '127.0.0.1 \u6216\u76ee\u6807\u4e3b\u673a',
          userId: '\u7559\u7a7a\u8868\u793a\u516c\u5171\u89c4\u5219',
          userGroupId: '\u4e0e\u7528\u6237 ID \u4e8c\u9009\u4e00',
          remark: '\u53ef\u8bb0\u5f55\u4e1a\u52a1\u7528\u9014\u6216\u7ef4\u62a4\u8bf4\u660e'
        }
      },
      connectionModal: {
        title: '\u6d4b\u8bd5\u8282\u70b9\u8fde\u63a5',
        fields: {
          host: '\u4e3b\u673a\u5730\u5740',
          apiPort: 'API \u7aef\u53e3',
          apiToken: 'API Token'
        },
        placeholders: {
          host: '127.0.0.1',
          apiToken: '\u5982\u672a\u542f\u7528\u9274\u6743\u53ef\u7559\u7a7a',
          apiTokenHidden: '\u5df2\u4fdd\u5b58\u7684\u4ee4\u724c\u4e0d\u518d\u663e\u793a\uff1b\u8bf7\u586b\u5199\u4ee4\u724c\u540e\u6d4b\u8bd5'
        },
        success: '\u8fde\u63a5\u6210\u529f',
        failed: '\u8fde\u63a5\u5931\u8d25'
      },
      deleteModal: {
        titleNode: '删除转发节点 {name}？',
        titleRule: '删除转发规则 {name}？',
        deleteNode: '删除转发节点',
        deleteRule: '删除规则',
        warning: '\u5220\u9664\u540e\u65e0\u6cd5\u81ea\u52a8\u6062\u590d\uff0c\u8bf7\u786e\u8ba4\u6ca1\u6709\u4ecd\u5728\u4f7f\u7528\u7684\u8f6c\u53d1\u5173\u7cfb\u3002'
      },
      validation: {
        requestFailed: '\u8bf7\u6c42\u5931\u8d25',
        nodeNameRequired: '\u8282\u70b9\u540d\u79f0\u4e0d\u80fd\u4e3a\u7a7a',
        nodeHostRequired: '\u4e3b\u673a\u5730\u5740\u4e0d\u80fd\u4e3a\u7a7a',
        nodePortRange: '\u4e1a\u52a1\u7aef\u53e3\u5fc5\u987b\u5728 1 \u5230 65535 \u4e4b\u95f4',
        nodeApiPortRequired: 'NodeX \u7ba1\u7406 API \u7aef\u53e3\u4e0d\u80fd\u4e3a\u7a7a',
        nodeApiPortRange: 'API \u7aef\u53e3\u5fc5\u987b\u5728 1 \u5230 65535 \u4e4b\u95f4',
        ruleNameRequired: '\u89c4\u5219\u540d\u79f0\u4e0d\u80fd\u4e3a\u7a7a',
        relayNodeRequired: '\u8bf7\u9009\u62e9\u5165\u53e3 Relay \u8282\u70b9',
        exitNodeRequired: '\u8bf7\u9009\u62e9\u51fa\u53e3 Exit \u8282\u70b9',
        listenPortRange: '\u76d1\u542c\u7aef\u53e3\u5fc5\u987b\u5728 1 \u5230 65535 \u4e4b\u95f4',
        targetHostRequired: '\u76ee\u6807\u4e3b\u673a\u4e0d\u80fd\u4e3a\u7a7a',
        targetPortRange: '\u76ee\u6807\u7aef\u53e3\u5fc5\u987b\u5728 1 \u5230 65535 \u4e4b\u95f4',
        ownerConflict: '\u7528\u6237 ID \u4e0e\u7528\u6237\u7ec4 ID \u53ea\u80fd\u586b\u5199\u4e00\u4e2a',
        connectionHostRequired: '\u4e3b\u673a\u5730\u5740\u4e0d\u80fd\u4e3a\u7a7a',
        connectionApiPortRange: 'API \u7aef\u53e3\u5fc5\u987b\u5728 1 \u5230 65535 \u4e4b\u95f4',
        userIdPositive: '\u7528\u6237 ID \u5fc5\u987b\u4e3a\u6b63\u6574\u6570'
      },
      messages: {
        loadStatsFailed: '\u52a0\u8f7d\u8f6c\u53d1\u7edf\u8ba1\u5931\u8d25',
        loadNodesFailed: '\u52a0\u8f7d\u4e2d\u8f6c\u8282\u70b9\u5931\u8d25',
        loadNodeOptionsFailed: '\u52a0\u8f7d\u8282\u70b9\u9009\u9879\u5931\u8d25',
        loadRulesFailed: '\u52a0\u8f7d\u8f6c\u53d1\u89c4\u5219\u5931\u8d25',
        loadNodeDetailFailed: '\u52a0\u8f7d\u8282\u70b9\u8be6\u60c5\u5931\u8d25',
        saveNodeFailed: '\u4fdd\u5b58\u4e2d\u8f6c\u8282\u70b9\u5931\u8d25',
        nodeUpdated: '\u4e2d\u8f6c\u8282\u70b9\u5df2\u66f4\u65b0',
        nodeCreated: '\u4e2d\u8f6c\u8282\u70b9\u5df2\u521b\u5efa',
        nodeCreatedWithToken: '\u4e2d\u8f6c\u8282\u70b9\u5df2\u521b\u5efa\u3002\u8bf7\u7acb\u5373\u590d\u5236\u5176 API Token\uff0c\u4e4b\u540e\u4e0d\u518d\u663e\u793a\uff1a{token}',
        nodeDeleted: '\u4e2d\u8f6c\u8282\u70b9\u5df2\u5220\u9664',
        nodeCheckFailed: '\u5065\u5eb7\u68c0\u6d4b\u5931\u8d25',
        nodeSyncFailed: '\u540c\u6b65\u7edf\u8ba1\u5931\u8d25',
        nodeToggleFailed: '\u5207\u6362\u8282\u70b9\u72b6\u6001\u5931\u8d25',
        loadRuleDetailFailed: '\u52a0\u8f7d\u89c4\u5219\u8be6\u60c5\u5931\u8d25',
        saveRuleFailed: '\u4fdd\u5b58\u8f6c\u53d1\u89c4\u5219\u5931\u8d25',
        ruleUpdated: '\u8f6c\u53d1\u89c4\u5219\u5df2\u66f4\u65b0',
        ruleCreated: '\u8f6c\u53d1\u89c4\u5219\u5df2\u521b\u5efa',
        ruleDeleted: '\u8f6c\u53d1\u89c4\u5219\u5df2\u5220\u9664',
        ruleToggleFailed: '\u5207\u6362\u89c4\u5219\u72b6\u6001\u5931\u8d25',
        connectionFailed: '\u8fde\u63a5\u68c0\u6d4b\u5931\u8d25',
        deleteFailed: '\u5220\u9664\u5931\u8d25',
        nodeReachable: '\u8282\u70b9\u53ef\u8fbe',
        nodeUnavailable: '\u8282\u70b9\u4e0d\u53ef\u8fbe',
        syncSuccess: '\u7edf\u8ba1\u540c\u6b65\u6210\u529f',
        connectionSuccess: '\u8fde\u63a5\u6210\u529f',
        connectionError: '\u8fde\u63a5\u5931\u8d25',
        nodeEnabled: '{name} \u5df2\u542f\u7528',
        nodeDisabled: '{name} \u5df2\u7981\u7528',
        ruleEnabled: '{name} \u5df2\u542f\u7528',
        ruleDisabled: '{name} \u5df2\u7981\u7528',
        latency: '\u5ef6\u8fdf {value} ms',
        currentConnectionsSuffix: '\uff0c\u5f53\u524d\u8fde\u63a5 {value}'
      },
      owner: {
        user: '\u7528\u6237 #{id}',
        userGroup: '\u7528\u6237\u7ec4 #{id}',
        public: '\u516c\u5171\u89c4\u5219'
      },
      protocols: {
        tcp: 'TCP',
        udp: 'UDP',
        both: 'TCP + UDP'
      },
      labels: {
        none: '\u4e0d\u9650',
        neverExpires: '\u6c38\u4e0d\u8fc7\u671f'
      }
    },
    limitPage: {
      heroEyebrow: '限速管理',
      title: '限速管理',
      note: '限速规则由当前转发运行时强制执行，更改可能需要短暂时间生效。',
      actions: {
        create: '新增',
        createNow: '立即创建',
        edit: '编辑',
        delete: '删除'
      },
      runtimeLinks: '运行时页面',
      table: {
        label: '限速规则',
        status: '状态'
      },
      empty: {
        title: '暂无限速规则',
        text: '还没有创建任何限速规则，点击上方按钮开始创建。'
      },
      status: {
        active: '运行',
        error: '异常'
      },
      cards: {
        speed: '速度限制',
        updatedAt: '更新时间'
      },
      formModal: {
        titleCreate: '新增限速规则',
        titleEdit: '编辑限速规则',
        fields: {
          name: '规则名称',
          speed: '速度限制',
          tunnel: '绑定隧道'
        },
        placeholders: {
          name: '请输入限速规则名称',
          speed: '请输入速度限制（Mbps）',
          tunnel: '请选择要绑定的隧道'
        },
        submitCreate: '创建规则',
        submitUpdate: '保存修改'
      },
      deleteModal: {
        title: '删除限速规则 {name}？',
        hint: '此操作无法撤销，删除后该规则将永久消失。',
        confirmDelete: '删除规则'
      },
      values: {
        unlimited: '不限速',
        tunnelFallback: '隧道 #{id}',
        ruleFallback: '规则 #{id}'
      },
      modeLabelNodeX: 'NodeX 模式',
      modeSummaryNodeX: '限速由 NodeX Agent 同步并强制执行。',
      modeLabelLocal: '本地模式',
      modeSummaryLocal: '限速通过本地 {backend} 运行时应用。',
      messages: {
        fetchTunnelsFailed: '获取隧道列表失败',
        fetchRulesFailed: '获取限速规则失败',
        loadFailed: '加载失败',
        nameRequired: '规则名称不能为空',
        nameLength: '规则名称长度应在 2-50 个字符之间',
        speedInvalid: '请输入有效的速度限制（>= 1 Mbps）',
        tunnelRequired: '请选择要绑定的隧道',
        tunnelMissing: '隧道名称不存在，请刷新后重试',
        submitFailed: '提交失败',
        deleteFailed: '删除限速规则失败',
        created: '限速规则创建成功',
        updated: '限速规则更新成功',
        deleted: '限速规则删除成功'
      }
    },
    nodeXAgents: {
      title: 'NodeX Agents',
      subtitle: '\u53ea\u6709 NodeX \u6a21\u5f0f\u9700\u8981 agents\u3002\u8fd9\u4e2a\u9875\u9762\u7528\u4e8e agent \u72b6\u6001\u3001\u8fdc\u7a0b\u7ec8\u7aef\u548c\u4efb\u52a1\u4e0b\u53d1\u3002',
      tabs: {
        agents: '\u5728\u7ebf Agent',
        terminal: '\u8fdc\u7a0b\u7ec8\u7aef',
        tasks: '\u4efb\u52a1\u5386\u53f2'
      },
      actions: {
        refresh: '\u5237\u65b0',
        execute: '\u8fd0\u884c',
        send: '\u53d1\u9001',
        cancel: '\u53d6\u6d88',
        monitor: '\u67e5\u770b\u76d1\u63a7',
        openTerminal: '\u5728\u7ec8\u7aef\u4e2d\u6253\u5f00'
      },
      table: {
        nodeId: '\u8282\u70b9',
        version: '\u7248\u672c',
        system: '\u7cfb\u7edf',
        lastSeen: '\u6700\u540e\u5728\u7ebf',
        status: '\u72b6\u6001',
        capabilities: '\u80fd\u529b',
        taskId: '\u4efb\u52a1 ID',
        node: '\u8282\u70b9',
        command: '\u547d\u4ee4 / \u52a8\u4f5c',
        duration: '\u8017\u65f6',
        time: '\u65f6\u95f4'
      },
      status: {
        online: '\u5728\u7ebf',
        offline: '\u79bb\u7ebf',
        connected: '\u5df2\u8fde\u63a5',
        disconnected: '\u672a\u8fde\u63a5',
        success: '\u6210\u529f',
        failed: '\u5931\u8d25'
      },
      empty: {
        agents: '\u6ca1\u6709\u5728\u7ebf\u7684 Agent',
        agentsDescription: 'NodeX \u8282\u70b9\u8fde\u4e0a\u63a7\u5236\u9762\u540e\uff0c\u5b83\u7684 Agent \u4f1a\u51fa\u73b0\u5728\u8fd9\u91cc\u3002',
        tasks: '\u8fd8\u6ca1\u6709\u4efb\u52a1\u8bb0\u5f55',
        tasksDescription: '\u5728\u7ec8\u7aef\u91cc\u8fd0\u884c\u7684\u547d\u4ee4\u548c\u4e0b\u53d1\u7684\u4efb\u52a1\u4f1a\u5217\u5728\u8fd9\u91cc\u3002'
      },
      terminal: {
        chooseNode: '\u9009\u62e9\u8282\u70b9',
        nodeLabel: '\u8282\u70b9 #{id}',
        chooseAction: '\u9009\u62e9\u52a8\u4f5c',
        output: '\u7ec8\u7aef\u8f93\u51fa',
        hint: '\u9009\u62e9\u4e00\u4e2a\u5728\u7ebf\u8282\u70b9\u548c\u52a8\u4f5c\u540e\u8fd0\u884c\u3002\u53ea\u80fd\u8fd0\u884c\u63a7\u5236\u9762\u5141\u8bb8\u7684\u8bca\u65ad\u52a8\u4f5c\u3002'
      },
      diagnosticActions: {
        service_status: '\u67e5\u770b\u670d\u52a1\u72b6\u6001',
        service_restart: '\u91cd\u542f\u670d\u52a1',
        log_tail: '\u67e5\u770b\u65e5\u5fd7\u5c3e\u90e8'
      },
      fields: {
        service: '\u670d\u52a1',
        lines: '\u884c\u6570'
      },
      services: {
        gost: 'GOST'
      },
      taskModal: {
        title: '\u4e0b\u53d1\u4efb\u52a1',
        action: '\u52a8\u4f5c',
        timeoutSeconds: '\u8d85\u65f6\uff08\u79d2\uff09'
      },
      hints: {
        monitor: '\u67e5\u770b\u8282\u70b9 #{id} \u7684\u76d1\u63a7\u6570\u636e'
      },
      messages: {
        fetchFailed: 'Agent \u5217\u8868\u6ca1\u6709\u52a0\u8f7d\u51fa\u6765',
        tasksFetchFailed: '\u4efb\u52a1\u5386\u53f2\u6ca1\u6709\u52a0\u8f7d\u51fa\u6765',
        taskIncomplete: '\u8bf7\u586b\u5199\u5b8c\u6574\u4fe1\u606f',
        taskSent: '\u4efb\u52a1\u5df2\u53d1\u9001',
        taskSendFailed: '\u53d1\u9001\u5931\u8d25\uff1a{message}',
        commandError: '\u9519\u8bef\uff1a{message}',
        selectActionFirst: '\u8bf7\u5148\u9009\u62e9\u4e00\u4e2a\u52a8\u4f5c'
      }
    }
  },
  admin: {
    nodes: adminNodes
  },
  adminPayment: {
    title: '支付',
    subtitle: '用户付款用的网关、每一笔支付和收入统计。',
    tabs: {
      label: '支付分区',
      gateways: '支付网关',
      records: '支付记录',
      stats: '统计数据'
    },
    actions: {
      createGateway: '新建网关',
      enable: '启用',
      disable: '禁用',
      edit: '编辑',
      details: '详情'
    },
    gateways: {
      label: '支付网关',
      table: {
        id: 'ID',
        name: '名称',
        type: '类型',
        feeRate: '手续费率',
        minAmount: '最小金额',
        maxAmount: '最大金额',
        status: '状态'
      },
      empty: '还没有支付网关',
      emptyDescription: '添加一个网关，用户才能付款购买套餐。'
    },
    records: {
      label: '支付记录',
      filters: {
        status: '按状态筛选',
        type: '按网关筛选'
      },
      table: {
        id: 'ID',
        tradeNo: '交易号',
        userId: '用户 ID',
        gateway: '网关',
        amount: '金额',
        status: '状态',
        createdAt: '创建时间'
      },
      detail: {
        title: '支付详情',
        tradeNo: '交易号',
        amount: '金额',
        status: '状态'
      },
      empty: '还没有支付记录',
      emptyDescription: '用户为订单付款后，记录会出现在这里。'
    },
    stats: {
      totalAmount: '总收入',
      totalOrders: '总订单数',
      successOrders: '成功订单',
      successRate: '成功率',
      gatewayDistribution: '支付渠道分布',
      empty: '暂无渠道统计'
    },
    modal: {
      createTitle: '新建网关',
      editTitle: '编辑网关',
      fields: {
        name: '名称',
        type: '类型',
        feeRate: '手续费率',
        minAmount: '最小金额',
        maxAmount: '最大金额',
        configJson: '配置 (JSON)'
      },
      placeholders: {
        name: '网关名称',
        feeRate: '例如: 0.01 = 1%',
        minAmount: '最小支付金额',
        maxAmount: '最大支付金额',
        configJson: "{'{'}\"app_id\": \"\", \"private_key\": \"\"{'}'}"
      }
    },
    types: {
      alipay: '支付宝',
      wechat: '微信支付',
      stripe: 'Stripe',
      usdt: 'USDT',
      epay: 'EPay'
    },
    status: {
      enabled: '已启用',
      disabled: '已停用',
      pending: '待支付',
      paid: '已支付',
      failed: '失败',
      refunded: '已退款'
    },
    confirm: {
      deleteTitle: '删除支付网关 {name}？',
      deleteMessage: '用户将无法再通过这个网关付款。此操作无法撤销。',
      deleteAction: '删除网关'
    },
    messages: {
      fetchGatewaysFailed: '支付网关没有加载出来',
      fetchRecordsFailed: '支付记录没有加载出来',
      fetchStatsFailed: '支付统计没有加载出来',
      invalidConfigJson: '配置 JSON 格式错误',
      gatewaySaveSuccess: '网关保存成功',
      gatewaySaveFailed: '网关保存失败: {message}',
      gatewaySaveFailedShort: '保存失败',
      toggleFailed: '切换网关状态失败: {message}',
      toggleFailedShort: '操作失败',
      gatewayEnabled: '已启用 {name}',
      gatewayDisabled: '已禁用 {name}',
      gatewayDeleted: '已删除网关 {name}',
      deleteFailedShort: '删除失败'
    }
  },
  adminMfa: {
    config: {
      enabled: '\u542f\u7528\u591a\u56e0\u7d20\u8ba4\u8bc1',
      enabledHelp: '\u542f\u7528\u540e\uff0c\u7528\u6237\u53ef\u9009\u62e9\u5f00\u542f MFA \u4fdd\u62a4\u8d26\u6237\u5b89\u5168',
      required: '\u5f3a\u5236\u542f\u7528 MFA',
      requiredHelp: '\u5f3a\u5236\u6240\u6709\u7528\u6237\u542f\u7528 MFA\uff0c\u5426\u5219\u65e0\u6cd5\u4f7f\u7528\u670d\u52a1',
      methods: '\u652f\u6301\u7684\u8ba4\u8bc1\u65b9\u5f0f',
      backupCodesCount: '\u6062\u590d\u7801\u6570\u91cf',
      backupCodesHelp: '\u7528\u6237\u542f\u7528 MFA \u65f6\u751f\u6210\u7684\u6062\u590d\u7801\u6570\u91cf',
      maxAttempts: '\u767b\u5f55\u5c1d\u8bd5\u9650\u5236',
      maxAttemptsHelp: '\u8d85\u8fc7\u9650\u5236\u5c06\u4e34\u65f6\u9501\u5b9a\u8d26\u6237',
      lockoutDuration: '\u9501\u5b9a\u65f6\u957f (\u5206\u949f)',
      lockoutDurationHelp: '\u767b\u5f55\u5931\u8d25\u8d85\u8fc7\u9650\u5236\u540e\u7684\u9501\u5b9a\u65f6\u95f4'
    },
    methods: {
      totp: 'TOTP (Google Authenticator / Authy)',
      sms: '\u77ed\u4fe1\u9a8c\u8bc1\u7801',
      email: '\u90ae\u7bb1\u9a8c\u8bc1\u7801'
    },
    info: {
      totpBody: '\u57fa\u4e8e\u65f6\u95f4\u7684\u4e00\u6b21\u6027\u5bc6\u7801\uff0c\u7528\u6237\u53ef\u4f7f\u7528 Google Authenticator\u3001Authy \u7b49\u5e94\u7528\u626b\u63cf\u4e8c\u7ef4\u7801\u7ed1\u5b9a\u3002',
      backupBody: '\u5f53\u7528\u6237\u65e0\u6cd5\u4f7f\u7528\u8ba4\u8bc1\u5668\u65f6\uff0c\u53ef\u4f7f\u7528\u6062\u590d\u7801\u767b\u5f55\u3002\u6bcf\u4e2a\u6062\u590d\u7801\u53ea\u80fd\u4f7f\u7528\u4e00\u6b21\u3002',
      lockoutBody: '\u8fde\u7eed\u591a\u6b21 MFA \u9a8c\u8bc1\u5931\u8d25\u5c06\u89e6\u53d1\u8d26\u6237\u9501\u5b9a\uff0c\u9632\u6b62\u66b4\u529b\u7834\u89e3\u3002',
      userOpsBody: '用户在自己的「账户」页开启、关闭两步验证，并重新生成恢复码。'
    },
    messages: {
      fetchFailed: '\u83b7\u53d6 MFA \u914d\u7f6e\u5931\u8d25',
      saveSuccess: '\u4fdd\u5b58\u6210\u529f',
      saveFailed: '\u4fdd\u5b58\u5931\u8d25: {message}',
      saveFailedShort: '\u4fdd\u5b58\u5931\u8d25'
    }
  },
  adminTemplates: {
    title: '订阅模板',
    subtitle: '免费的订阅模板：流量额度、速率和设备限制，以及授予的订阅分组。分配给用户即可生效。',
    filters: {
      search: '搜索模板'
    },
    table: {
      label: '订阅模板列表'
    },
    actions: {
      create: '新建模板'
    },
    empty: {
      title: '还没有订阅模板',
      description: '创建模板后分配给用户，用户就能获得流量和订阅分组。'
    },
    detail: {
      description: '模板 ID {id}'
    },
    labels: {
      noGroupsHint: '这个模板还没有授予订阅分组。'
    },
    planModal: {
      createTitle: '新建订阅模板',
      editTitle: '编辑订阅模板',
      fields: {
        name: '模板名称'
      },
      placeholders: {
        name: '请输入模板名称'
      }
    },
    assignModal: {
      title: '分配订阅模板'
    },
    groupModal: {
      title: '模板分组 - {name}',
      description: '选择该模板授予的订阅分组。'
    },
    confirm: {
      deleteTitle: '删除订阅模板 {name}？',
      deleteAction: '删除模板'
    },
    messages: {
      loadFailed: '加载订阅模板失败',
      nameRequired: '请输入模板名称',
      deleted: '已删除订阅模板 {name}',
      groupRemoved: '已从模板 {plan} 移除分组 {group}'
    }
  },
  adminPlans: {
    title: '套餐',
    subtitle: '用户可以购买的套餐：流量额度、速率和设备限制、价格，以及授予的订阅分组。',
    filters: {
      search: '搜索套餐'
    },
    table: {
      label: '套餐列表',
      name: '名称',
      transfer: '流量',
      limits: '限制',
      monthPrice: '月付价格',
      subscriptionGroups: '订阅分组'
    },
    actions: {
      create: '新建套餐',
      edit: '编辑',
      delete: '删除',
      assign: '分配',
      manageGroups: '管理分组',
      removeGroupNamed: '移除分组 {name}'
    },
    empty: {
      title: '还没有套餐',
      description: '创建套餐后，用户就能购买流量并获得订阅分组。'
    },
    detail: {
      description: '套餐 ID {id}',
      limits: '额度和限制',
      actions: '操作'
    },
    planModal: {
      createTitle: '新建套餐',
      editTitle: '编辑套餐',
      fields: {
        name: '套餐名称',
        transfer: '流量额度',
        speedLimit: '速率限制',
        deviceLimit: '设备限制',
        monthPrice: '月付价格（分）'
      },
      placeholders: {
        name: '请输入套餐名称'
      },
      help: {
        zeroUnlimited: '0 表示不限制。'
      }
    },
    assignModal: {
      title: '分配套餐',
      fields: {
        userId: '用户 ID',
        expireAt: '到期时间（Unix 秒）'
      },
      placeholders: {
        userId: '请输入用户 ID'
      },
      help: {
        expireAt: '可选，Unix 时间（秒）。'
      }
    },
    groupModal: {
      title: '套餐分组 - {name}',
      description: '选择该套餐可访问的订阅分组。',
      empty: '暂无订阅分组',
      noDescription: '无描述'
    },
    labels: {
      noSpeedLimit: '不限速',
      noDeviceLimit: '不限设备',
      speedLimitMbps: '{value} Mbps',
      deviceLimitCount: '{value} 台',
      noGroups: '无',
      noGroupsHint: '这个套餐还没有授予订阅分组。'
    },
    confirm: {
      deleteTitle: '删除套餐 {name}？',
      deleteMessage: '此操作无法撤销。',
      deleteAction: '删除套餐'
    },
    messages: {
      loadFailed: '加载套餐失败',
      loadGroupsFailed: '加载订阅分组失败',
      deleted: '已删除套餐 {name}',
      deleteFailedShort: '删除失败',
      nameRequired: '请填写套餐名称',
      saveFailed: '保存失败：{message}',
      saveFailedShort: '保存失败',
      saved: '已保存 {name}',
      userIdRequired: '请填写用户 ID',
      assignSuccess: '分配成功',
      assignFailed: '分配失败：{message}',
      assignFailedShort: '分配失败',
      toggleGroupFailed: '切换分组失败：{message}',
      toggleGroupFailedShort: '切换分组失败',
      groupRemoved: '已从套餐 {plan} 移除分组 {group}',
      removeGroupFailed: '移除分组失败：{message}',
      removeGroupFailedShort: '移除失败'
    }
  },
  adminUsers: {
    title: '用户',
    subtitle: '账户、订阅和流量。选中一个用户查看详情。',
    filters: {
      searchEmail: '搜索邮箱',
      label: '按状态筛选',
      exhaustedHint: '「流量用尽」只筛选本页的用户，服务器没有这个筛选条件。'
    },
    table: {
      label: '用户列表',
      id: 'ID',
      email: '邮箱',
      plan: '套餐',
      subscriptionTemplate: '订阅模板',
      traffic: '已用 / 总流量',
      limits: '限制',
      expireAt: '到期时间',
      status: '状态',
      createdAt: '注册时间'
    },
    status: {
      active: '正常',
      expired: '已到期',
      banned: '已封禁',
      exhausted: '流量用尽'
    },
    actions: {
      addUser: '新建用户',
      editUser: '编辑用户',
      manageTunnel: '管理隧道授权',
      ban: '封禁',
      unban: '解封',
      resetTraffic: '重置流量',
      copySubscribe: '复制订阅链接',
      resetSubscribe: '重置订阅链接',
      viewTraffic: '最近 30 天流量'
    },
    empty: {
      title: '还没有用户',
      description: '添加第一个用户后，就能给他分配订阅。'
    },
    detail: {
      description: 'ID {id} · 注册于 {date}',
      subscription: '订阅',
      flowReset: '流量重置',
      actions: '操作',
      danger: '危险操作',
      dangerFooter: '重置订阅链接后，所有客户端都要重新导入；重置流量无法撤销。'
    },
    editModal: {
      title: '编辑用户',
      fields: {
        email: '邮箱',
        balance: '余额（分）',
        transfer: '流量限制（字节）',
        speedLimit: '速率限制（Mbps，0 为不限）',
        deviceLimit: '设备限制（0 为不限）',
        groupId: '订阅分组',
        expiredAt: '到期时间（Unix 秒）',
        flowResetTime: '流量重置日',
        remark: '备注'
      },
      groupOptions: {
        unassigned: '未分配'
      }
    },
    createModal: {
      title: '新建用户',
      submit: '创建用户',
      fields: {
        email: '邮箱',
        password: '密码',
        userType: '用户类型',
        groupId: '订阅分组',
        transferEnable: '流量限制（字节）',
        speedLimit: '速率限制（Mbps，0 为不限）',
        deviceLimit: '设备限制（0 为不限）',
        flowResetTime: '流量重置日'
      },
      placeholders: {
        email: '请输入邮箱',
        password: '请输入密码（至少 6 位）',
        transferEnable: '留空则使用默认值 0',
        speedLimit: '0 表示不限速',
        deviceLimit: '0 表示不限设备'
      },
      userTypes: {
        normal: '普通用户',
        admin: '管理员'
      }
    },
    tunnelModal: {
      title: '隧道授权 - {email}',
      sections: {
        form: '新增授权',
        editForm: '编辑授权 #{id}',
        list: '当前授权列表'
      },
      fields: {
        tunnel: '隧道',
        tunnelReadonlyHint: '编辑授权时不能更换隧道。',
        status: '状态',
        flowQuota: '流量配额',
        numQuota: '数量配额',
        expTime: '到期时间',
        flowResetTime: '流量重置时间',
        rateLimit: '限速规则'
      },
      options: {
        noAssignableTunnel: '暂无可分配隧道',
        selectTunnel: '请选择隧道',
        selectTunnelFirst: '请先选择隧道',
        noRateLimitRules: '当前隧道暂无限速规则'
      },
      actions: {
        cancelEdit: '取消编辑',
        updateGrant: '更新授权',
        addGrant: '新增授权'
      },
      table: {
        id: 'ID',
        tunnel: '隧道',
        status: '状态',
        flow: '流量',
        num: '数量',
        expireAt: '到期',
        reset: '重置',
        usedFlow: '已用流量',
        rateLimit: '限速'
      },
      empty: '暂无授权'
    },
    confirm: {
      resetSubscribeTitle: '重置 {email} 的订阅链接？',
      resetSubscribeMessage: '旧链接会立即失效，用户需要重新导入订阅。此操作无法撤销。',
      resetSubscribeAction: '重置订阅链接',
      deleteGrantTitle: '删除隧道授权 #{id}？',
      deleteGrantMessage: '该用户将不能再使用隧道 {tunnel}。此操作无法撤销。',
      deleteGrantAction: '删除授权'
    },
    copyDialog: {
      title: '复制订阅链接',
      description: '无法自动复制，请手动复制下面的链接。',
      label: '订阅链接'
    },
    resetFlow: {
      userTitle: '重置 {email} 的流量？',
      userMessage: '已用流量将清零。此操作无法撤销。',
      tunnelTitle: '重置隧道授权 #{id} 的流量？',
      tunnelMessage: '这条授权的已用流量将清零。此操作无法撤销。',
      usedFlow: '当前已用',
      quota: '当前配额',
      confirmAction: '重置流量'
    },
    trafficModal: {
      title: '流量详情 - {email}',
      subtitle: '最近 30 天每日汇总和每小时明细。',
      refresh: '刷新',
      dailyTitle: '每日流量',
      hourlyTitle: '每小时流量',
      empty: '暂无流量记录',
      fetchFailed: '加载用户流量失败',
      summary: {
        total30d: '30 天总流量',
        dailyPeak: '单日峰值',
        hourlyPeak: '单小时峰值'
      },
      table: {
        date: '日期',
        hour: '小时',
        traffic: '流量'
      }
    },
    labels: {
      admin: '管理员',
      noLimit: '不限速',
      noSpeedLimit: '不限速',
      noDeviceLimit: '不限设备',
      speedLimitMbps: '{value} Mbps',
      deviceLimitCount: '{value} 台',
      noReset: '不重置',
      monthlyDay: '每月第 {day} 天',
      permanent: '永久',
      trafficUnlimited: '已用 {used} · 不限'
    },
    messages: {
      actionFailed: '操作失败',
      fillEmailPassword: '请填写邮箱和密码',
      passwordTooShort: '密码长度至少 6 位',
      userCreated: '用户创建成功',
      createFailed: '创建失败',
      fetchUsersFailed: '用户列表没有加载出来',
      bulkBanned: '已封禁 {count} 个用户',
      bulkUnbanned: '已解封 {count} 个用户',
      bulkPartial: '{total} 个用户中 {done} 个已更改。{message}',
      fetchStatsFailed: '获取统计失败',
      saveFailed: '保存失败：{message}',
      userSaved: '已保存 {email}',
      userBanned: '已封禁 {email}',
      userUnbanned: '已解封 {email}',
      fetchTunnelListFailed: '获取隧道列表失败',
      fetchSpeedLimitFailed: '获取限速规则失败',
      fetchTunnelGrantFailed: '获取隧道授权失败',
      selectTunnelFirst: '请选择隧道',
      tunnelAlreadyAssigned: '该隧道已授权给当前用户',
      grantUpdateFailed: '授权更新失败',
      grantCreateFailed: '授权创建失败',
      grantUpdated: '授权更新成功',
      grantCreated: '授权创建成功',
      grantActionFailed: '授权操作失败',
      grantDeleted: '已删除隧道授权 #{id}',
      grantDeleteFailed: '删除授权失败',
      resetFailed: '重置失败',
      userFlowReset: '用户流量已重置',
      tunnelFlowReset: '隧道流量已重置',
      fetchUserFailed: '获取用户详情失败',
      noToken: '该用户没有订阅 token',
      subscribeCopied: '订阅链接已复制到剪贴板',
      resetSubscribeSuccess: '订阅链接已重置',
      resetSubscribeFailed: '重置订阅失败'
    }
  },
  control: {
    subtitle: '管理官方签名软件包、Control WebUI 扩展与生命周期操作。',
    actions: {
      refresh: '刷新', refreshing: '刷新中...', importRelease: '导入发行版', install: '安装',
      configure: '配置', enable: '启用', disable: '禁用', upgrade: '升级', update: '升级', rollback: '回滚', cancel: '取消操作', installOfficialOnly: '需要官方发行版',
      saving: '保存中...', newAssignment: '新建角色'
    },
    pluginCenter: {
      filters: { search: '搜索插件', health: '按健康状态筛选', target: '按运行目标筛选' },
      states: { healthy: '健康', attention: '需要关注' },
      listLabel: '插件',
      official: '官方签名软件包',
      loadFailed: '插件目录没有加载出来',
      empty: '没有符合当前筛选条件的插件',
      emptyCatalog: { title: '还没有插件', description: '导入一个官方签名发行版，它的插件就会出现在这里。' },
      detail: { versions: '版本与状态' },
      operations: { title: '最近插件操作', empty: '暂无最近插件操作' }
    },
    tabs: { assignments: '节点角色', topologies: '拓扑' },
    table: {
      plugin: '插件', release: '发行版', installation: '安装目标', version: '版本', state: '状态', actions: '操作',
      scope: '作用域', description: '说明', topology: '拓扑', activeRevision: '激活 revision', deployment: '部署',
      operation: '操作', chain: '链', revision: 'revision', deadline: '截止时间', target: '目标', version: '版本', role: '服务角色', configRevision: '配置 revision', rolloutGroup: '灰度组'
    },
    labels: { releases: '{count} 个发行版', desired: '期望', observed: '实际' },
    states: { catalogued: '已登记', enabled: '已启用', disabled: '已禁用', loading: '正在加载控制状态...' },
    empty: { assignments: '该节点暂无服务角色', topologies: '暂无拓扑', operations: '暂无操作' },
    activity: { scoped: '当前范围活动', all: '全部活动', showAll: '显示全部活动', showScoped: '显示当前范围活动', empty: '当前范围暂无活动' },
    topology: {
      select: '拓扑', new: '新建拓扑', newTitle: '新建拓扑', create: '创建拓扑', name: '名称', edit: '编辑修订', status: '查看状态', noDeployment: '暂无部署', editorTitle: '拓扑修订编辑器', revision: '修订',
      noRevisions: '暂无修订', failurePolicy: '失败策略', stopAndRollback: '停止并回滚', message: '修订说明', graphJSON: '拓扑图 JSON',
      graphHelp: '使用 vertices 和 edges 描述拓扑。秘密必须通过 secret_id 引用，禁止直接写入秘密值。', diagnose: '校验 / 诊断', validating: '校验中...',
      unsavedChanges: '当前修订存在未保存修改。请先保存新的不可变修订，再进行预览或规划。',
      valid: '拓扑校验通过', invalid: '拓扑存在校验问题', invalidJSON: '拓扑 JSON 无效', saveRevision: '保存修订', plan: '规划部署',
      apply: '应用部署', rollback: '回滚部署', deployment: '部署', applyConfirm: '确认应用 {topology}（部署 #{deployment}）吗？', rollbackConfirm: '确认请求回滚 {topology}（部署 #{deployment}）吗？',
      preview: '只读部署预览', previewAction: '预览', previewSteps: '{count} 个计划步骤',
      emptyHint: '新建一个拓扑，写入顶点与连接，保存修订后即可规划部署。',
      viewLabel: '编辑方式', viewJSON: 'JSON', viewGraph: '图示', graphLabel: '拓扑图示', graphSummary: '{nodes} 个顶点，{edges} 条连接',
      graphEmpty: '还没有顶点', graphEmptyHint: '在 JSON 的 vertices 中添加顶点后，这里会画出拓扑。', graphInvalidHint: '修正 JSON 后这里会画出拓扑。', graphFailed: '拓扑图示加载失败'
    },
    extensions: { errorsTitle: 'WebUI 扩展加载失败' },
    assignments: {
      node: '节点', noNodes: '暂无可用节点', agentPlugin: 'Agent 插件', enabled: '启用该节点角色',
      createTitle: '新建节点角色', editTitle: '编辑节点角色', deleteTitle: '从该节点删除 {plugin} / {role}？', deleteMessage: '该节点会停止运行这个插件角色。此操作无法撤销。', deleteAction: '删除节点角色',
      emptyHint: '为该节点新建一个角色，指定插件、作用域与版本。', roleHelp: '常用角色：{roles}'
    },
    install: { title: '安装官方插件', target: '运行目标', version: '发行版本', enableAfterInstall: '安装后立即启用' },
    update: { title: '升级官方插件' },
    config: {
      title: '插件配置', loading: '正在加载配置...', revision: '配置 revision：{revision}', mode: '配置编辑模式', formMode: '表单', jsonMode: 'JSON',
      selectValue: '请选择', addItem: '添加项目', removeItem: '移除项目', emptyArray: '暂无项目', item: '项目', invalidJSON: 'JSON 无效',
      schemaError: '{path} 的值不符合 Schema', requiredError: '{path} 为必填项'
    },
    releaseImport: {
      title: '导入官方签名发行版', manifest: 'Manifest JSON', signature: '签名', artifact: '软件包制品',
      artifactOptional: '可选；未上传制品的发行版不能安装。'
    },
    messages: {
      actionQueued: '{plugin} 的“{action}”操作已提交', operationStatus: '操作 {id} 当前为 {state}，链：{chain}。', installed: '{plugin} 安装意图已保存', configSaved: '{plugin} 配置已保存',
      releaseImported: '{plugin} {version} 已导入', cancelRequested: '已请求取消操作',
      assignmentSaved: '{plugin} 节点角色已保存', assignmentStateSaved: '{plugin} 节点角色状态已保存', assignmentDeleted: '{plugin} 节点角色已删除',
      topologyRevisionSaved: '拓扑修订 {revision} 已保存', topologyPlanned: '部署 #{id} 已规划',
      topologyApplyRequested: '已请求应用拓扑部署', topologyRollbackRequested: '已请求回滚拓扑部署', topologyCreated: '拓扑 {name} 已创建'
    },
    errors: {
      load: '无法加载控制状态', action: '插件操作失败', install: '插件安装失败', configLoad: '无法加载插件配置',
      configSave: '无法保存插件配置', releaseImport: '发行版导入失败', cancel: '无法取消操作', poll: '无法刷新操作状态',
      nodesLoad: '无法加载节点', assignmentsLoad: '无法加载节点角色', assignmentSave: '无法保存节点角色', assignmentDelete: '无法删除节点角色',
      topologyLoad: '无法加载拓扑修订', topologyValidate: '无法校验拓扑', topologySave: '无法保存拓扑修订', topologyPlan: '无法规划拓扑部署',
      topologyPreview: '无法预览拓扑部署', topologyStatus: '无法加载部署状态', topologyApply: '无法应用拓扑部署', topologyRollback: '无法回滚拓扑部署', topologyCreate: '无法创建拓扑'
    }
  },
  routeModes: {
    open: '路由模式',
    subtitle: '按软件包切换 v2 路由的旧版、影子和原生模式，并可回滚到旧版。',
    back: '插件中心',
    package: '软件包',
    packageMeta: '版本 {version} · 配置修订 {revision}',
    packageDisabled: '已停用',
    superAdminOnly: '仅超级管理员可以切换路由模式。',
    loadFailed: '路由模式加载失败',
    empty: { title: '没有声明 v2 路由的软件包', description: '安装并启用声明了 v2 路由的软件包后，即可在这里管理其路由模式。' },
    modes: { legacy: '旧版', shadow: '影子', native: '原生' },
    catalog: { 'native-flagged': '可切原生', bridged: '桥接', 'kernel-owned': '内核接管', native: '原生', none: '未声明' },
    locked: { kernel_owned: '内核接管', identity_group_a: '身份切换', websocket: 'WebSocket', not_declared: '未声明' },
    columns: { route: '路由', endpoint: '方法 / 路径', catalog: '资格', configured: '配置模式', effective: '生效模式', shadow: '影子（总数 / 不一致 / 错误）', mismatch: '不一致率', mode: '切换为' },
    routesLabel: '{package} 的路由',
    routeMode: '{route} 的模式',
    effectiveDiffers: '与配置模式不同',
    hostEffective: '宿主：{mode}',
    packageMode: '整个软件包的模式',
    setPackage: '设置整个软件包',
    rollback: '回滚到旧版',
    wholePackage: '整个软件包',
    wholePackageTarget: '{package}（整个软件包）',
    dialog: {
      setTitle: '将 {target} 切换为{mode}模式？',
      setMessage: '不支持该模式的路由会被跳过。此次变更会记入修订历史。',
      nativeMessage: '原生模式下，请求由软件包自身的实现处理，不再经过内核的旧版处理器。请确认并填写原因，以便审计。',
      rollbackTitle: '将 {package} 回滚到旧版？',
      rollbackMessage: '该软件包的所有路由都会恢复为旧版模式。',
      reason: '原因',
      reasonHelp: '选填，会随修订一并保存。',
      reasonRequired: '切换到原生模式时必须填写。',
      confirmSet: '切换模式',
      confirmNative: '切换到原生',
      confirmRollback: '回滚'
    },
    result: '已变更 {changed} 条路由，跳过 {skipped} 条（配置修订 {revision}）',
    errors: { switch: '路由模式切换失败', rollback: '回滚失败', revisions: '修订历史加载失败' },
    revisions: {
      title: '修订历史',
      label: '路由模式修订',
      empty: '暂无路由模式变更',
      time: '时间', route: '路由', change: '变更', actor: '操作人', reason: '原因', revision: '修订',
      actions: { set: '设置', rollback: '回滚' }
    },
    mismatches: {
      title: '影子不一致样本',
      label: '不一致样本',
      open: '样本（{count}）',
      lastSeen: '最近 {time}',
      privacy: '样本在保存前已脱敏：令牌、密码、密钥、UUID 和订阅链接全部隐藏，邮箱只保留首字母和域名，IP 只保留前两段。每条路由保留最近 {max} 条，保存 {days} 天。',
      loadFailed: '不一致样本加载失败',
      empty: '没有保存的样本',
      emptyDescription: '影子运行的结果与旧版不同时，样本会出现在这里。',
      observed: '发生时间',
      status: '状态码（旧版 → 原生）',
      diffColumn: '差异',
      fields: '{count} 个字段',
      detail: '样本详情',
      request: '请求',
      requestID: '请求 ID',
      version: '包版本',
      truncated: '差异已截断',
      diff: '差异（已脱敏）',
      copyDiff: '复制差异'
    }
  },
  accessGroups: {
    subtitle: '管理独立服务作用域的成员关系、资源授权和插件私有配额策略。',
    actions: { refresh: '刷新', newGroup: '新建访问组', open: '打开', editGroup: '编辑名称和说明', enable: '启用', disable: '禁用', add: '添加', removeNamed: '移除 {name}', addGrant: '添加授权', saveQuota: '保存配额', resolve: '解析有效授权' },
    filters: { label: '按服务作用域筛选', allScopes: '全部服务作用域' },
    table: { group: '访问组', scope: '作用域', state: '状态' },
    states: { enabled: '已启用', disabled: '已禁用' },
    groups: { title: '访问组', empty: '这个作用域还没有访问组', emptyAll: '还没有访问组', emptyDescription: '访问组在一个服务作用域内，为其中的用户和套餐授予资源权限和配额策略。', noDescription: '暂无说明', directUnion: '直接成员和套餐成员按允许并集计算。' },
    detail: { title: '访问组', loading: '正在加载访问组详情…', group: '访问组', danger: '危险操作' },
    members: { title: '用户成员', userID: '用户 ID', empty: '暂无直接用户成员' },
    plans: { title: '套餐成员', planID: '套餐 ID', empty: '暂无套餐成员' },
    grants: { title: '资源授权', resourceType: '资源类型', resourceID: '资源 ID', permissions: '权限 JSON', empty: '暂无资源授权' },
    quotas: { title: '配额策略', key: '策略键', policy: '策略 JSON', empty: '暂无配额策略' },
    resolver: { title: '有效授权预览', description: '预览一个用户、可选套餐和服务作用域在服务端的允许并集结果。', userID: '用户 ID', planID: '套餐 ID（可选）', scope: '服务作用域', result: '命中 {count} 个已启用访问组', none: '没有命中已启用访问组', policySummary: '当前生效 {grants} 条授权和 {quotas} 条配额策略。' },
    editor: { createTitle: '新建访问组', editTitle: '编辑访问组', name: '组名称', description: '说明', enabled: '启用该访问组', scopeFixed: '已有访问组的作用域不能修改。' },
    messages: { groupCreated: '已创建访问组 {name}', groupSaved: '已保存访问组 {name}', groupEnabled: '已启用访问组 {name}', groupDisabled: '已禁用访问组 {name}', groupDeleted: '已删除访问组 {name}', memberAdded: '已添加用户 #{id}', memberRemoved: '已移除用户 #{id}', planAdded: '已添加套餐 #{id}', planRemoved: '已移除套餐 #{id}', grantAdded: '已添加资源授权', grantRemoved: '已移除资源授权', quotaSaved: '已保存配额策略', quotaRemoved: '已移除配额策略' },
    confirm: { deleteGroupTitle: '删除访问组 {name}？', deleteGroup: '组内的成员关系、资源授权和配额策略会一并删除。此操作无法撤销。', deleteGroupAction: '删除访问组', removeGrantTitle: '移除资源授权 #{id}？', removeGrant: '该组成员将失去对 {resource} 的访问权限。此操作无法撤销。', removeGrantAction: '移除授权', removeQuotaTitle: '移除配额策略 {key}？', removeQuota: '该组将不再受这条配额限制。此操作无法撤销。', removeQuotaAction: '移除策略' },
    errors: { load: '无法加载访问控制数据', loadGroups: '无法加载访问组', loadDetail: '无法加载访问组详情', groupRequired: '必须填写服务作用域和组名称', saveGroup: '无法保存访问组', deleteGroup: '无法删除访问组', member: '无法更新用户成员关系', plan: '无法更新套餐成员关系', grant: '无法更新资源授权', quota: '无法更新配额策略', resolve: '无法解析有效授权', invalidID: '{label} 必须为正整数', invalidJSON: '{label} 必须为有效 JSON', scopeRequired: '必须选择服务作用域' }
  },
  agentTransports: {
    open: 'Agent 连接方式',
    subtitle: '查看每个节点的 Agent 如何连接 Control：mTLS 流，或 v4.2 将拒绝的旧版通道。',
    back: 'NodeX Agents',
    mode: 'agent_control.mtls：{mode}',
    modes: {
      off: '已关闭客户端证书；旧版 Agent 照常服务，不发出提示。',
      optional: '提供客户端证书时会校验；旧版 Agent 照常服务，不发出提示。',
      preferred: '旧版 Agent 仍可连接，但会收到弃用提示。',
      required: 'AnixOps Agent 通道只接受已注册（持有客户端证书）的 Agent。'
    },
    sunset: '旧版停用日期：{date}',
    notice: 'v4.2 起 agent_control.mtls 默认为 required。升级前，所有标记为“旧版”的节点都必须运行已完成注册的 Agent；使用 API 密钥的旧版 Agent 将被拒绝。UniProxy 和 v2board gRPC（第三方节点软件）不受影响。',
    guide: '升级指南',
    filterLabel: '按状态筛选',
    statuses: { all: '全部', mtls: 'mTLS', legacy: '旧版', 'third-party': '第三方', unseen: '未出现' },
    legacyHint: 'required 模式下会被拒绝',
    transports: {
      'mtls-stream': 'mTLS 流',
      'apikey-stream': 'API 密钥流',
      'http-legacy': '旧版 HTTP',
      websocket: 'WebSocket',
      'clean-agent': 'Clean Agent',
      uniproxy: 'UniProxy',
      'v2board-grpc': 'v2board gRPC'
    },
    columns: { node: '节点', status: '状态', transport: '最近通道', version: 'Agent 版本', certificate: '证书', lastSeen: '最近出现', seenOn: '出现过的通道' },
    certificateUntil: '有效至 {date}',
    noCertificate: '未注册',
    disabled: '已停用',
    tableLabel: '各节点的 Agent 连接方式',
    empty: { title: '暂无节点', description: '创建代理节点或转发节点后会显示在这里。' },
    loadFailed: 'Agent 连接方式加载失败'
  },
}
