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
        currentRepoRuntime: 'Current repo: docs/reference/runtime.md',
        currentRepoOnboarding: 'Current repo: docs/guide/forward-relay-onboarding.md',
        nodeXRepo: 'NodeX repo: https://github.com/zdwtest/NodeX',
        nodeXDoc: 'NodeX doc: docs/forward-runtime-relay-onboarding.md'
      }
    }
  }
}
