export default {
  runtimePages: {
    localRuntime: {
      placeholders: {
        command: 'ansible-playbook'
      },
      defaults: {
        command: 'ansible-playbook'
      }
    },
    nodeX: {
      placeholders: {
        token: 'FORWARD_API_TOKEN'
      },
      references: {
        currentRepoRuntime: '当前仓库：docs/reference/runtime.md',
        currentRepoOnboarding: '当前仓库：docs/guide/forward-relay-onboarding.md',
        nodeXRepo: 'NodeX 仓库：https://github.com/zdwtest/NodeX',
        nodeXDoc: 'NodeX 文档：docs/forward-runtime-relay-onboarding.md'
      }
    }
  }
}
