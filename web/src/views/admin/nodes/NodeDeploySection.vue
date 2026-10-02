<template>
  <UiSection :title="t('admin.nodes.deploySection.title')" :description="t('admin.nodes.deploySection.description')">
    <div class="node-deploy-section">
      <UiCard :title="t('admin.nodes.deploySection.registration')" heading-tag="h3">
        <NodeAuthKeyPanel :deploy="deploy" />
      </UiCard>
      <div class="node-deploy-section__side">
        <UiCard :title="t('admin.nodes.deploySection.connection')" :description="t('admin.nodes.deploySection.connectionHint')" heading-tag="h3">
          <NodeDeploySettings :deploy="deploy" />
        </UiCard>
        <UiCard :title="t('admin.nodes.deploySection.ansible')" heading-tag="h3">
          <p class="node-deploy-section__text">
            {{ isRoot ? t('admin.nodes.deploySection.ansibleRoot') : t('admin.nodes.deploySection.ansibleChild') }}
          </p>
          <template v-if="isRoot" #actions>
            <UiButton size="sm" :icon="Terminal" data-testid="open-deploy-helper" @click="deployOpen = true">{{ t('admin.nodes.deploySection.openHelper') }}</UiButton>
          </template>
        </UiCard>
      </div>
    </div>
    <NodeDeploySheet v-if="isRoot" v-model:open="deployOpen" :nodes="[node]" :deploy="deploy" />
  </UiSection>
</template>

<script setup>
// 部署 section of the node page: the Agent registration key with its
// config.json, the connection settings it uses, and, for a parent node, the
// Ansible helper for this node.
import { computed, onMounted, ref } from 'vue'
import { Terminal } from '@lucide/vue'
import UiButton from '@/ui/UiButton.vue'
import UiCard from '@/ui/UiCard.vue'
import UiSection from '@/ui/UiSection.vue'
import { useAppI18n } from '@/composables/useAppI18n'
import NodeAuthKeyPanel from './NodeAuthKeyPanel.vue'
import NodeDeploySettings from './NodeDeploySettings.vue'
import NodeDeploySheet from './NodeDeploySheet.vue'

const props = defineProps({
  node: { type: Object, required: true },
  // reactive(useNodeDeploy()), owned by the page
  deploy: { type: Object, required: true }
})
const { t } = useAppI18n()
const deployOpen = ref(false)
const isRoot = computed(() => !props.node.parent_id)

onMounted(() => props.deploy.loadAuthKeysPreview())
</script>

<style scoped>
.node-deploy-section {
  display: grid;
  grid-template-columns: minmax(0, 1.2fr) minmax(0, 1fr);
  gap: var(--space-6);
  align-items: start;
}

.node-deploy-section__side {
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
  min-width: 0;
}

.node-deploy-section__text {
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-callout-size);
  line-height: var(--type-callout-line);
}

@media (max-width: 1067.98px) {
  .node-deploy-section {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
