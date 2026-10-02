<template>
  <div class="subscriptions-page">
    <div class="page-header">
      <h1>{{ $t('admin.subscriptions.title') }}</h1>
      <p class="subtitle">{{ $t('admin.subscriptions.subtitle') }}</p>
    </div>

    <!-- Overview stats -->
    <div class="overview-stats">
      <div class="stat-card">
        <div class="stat-value">{{ groups.length }}</div>
        <div class="stat-label">{{ $t('admin.subscriptions.stats.totalGroups') }}</div>
      </div>
      <div class="stat-card">
        <div class="stat-value">{{ totalUsers }}</div>
        <div class="stat-label">{{ $t('admin.subscriptions.stats.totalUsers') }}</div>
      </div>
      <div class="stat-card">
        <div class="stat-value">{{ totalTemplates }}</div>
        <div class="stat-label">{{ $t('admin.subscriptions.stats.totalTemplates') }}</div>
      </div>
      <div class="stat-card">
        <div class="stat-value">{{ formatTotalTraffic(totalTraffic) }}</div>
        <div class="stat-label">{{ $t('admin.subscriptions.stats.totalTraffic') }}</div>
      </div>
    </div>

    <!-- Group usage table -->
    <div class="stats-table-section">
      <h2>{{ $t('admin.subscriptions.stats.groupUsage') }}</h2>
      <div class="stats-table-wrapper" v-if="groupStats.length > 0">
        <table class="stats-table">
          <thead>
            <tr>
              <th>{{ $t('admin.subscriptions.stats.groupName') }}</th>
              <th>{{ $t('admin.subscriptions.stats.users') }}</th>
              <th>{{ $t('admin.subscriptions.stats.enabledUsers') }}</th>
              <th>{{ $t('admin.subscriptions.stats.templates') }}</th>
              <th>{{ $t('admin.subscriptions.stats.protocols') }}</th>
              <th>{{ $t('admin.subscriptions.stats.onlineNodes') }}</th>
              <th>{{ $t('admin.subscriptions.stats.trafficUsed') }}</th>
              <th>{{ $t('admin.subscriptions.stats.plans') }}</th>
              <th>{{ $t('admin.subscriptions.actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="stat in sortedGroupStats" :key="stat.group_id" class="clickable-row" @click="selectGroupById(stat.group_id)">
              <td>
                <span class="group-name">{{ stat.group_name }}</span>
              </td>
              <td>{{ stat.user_count }}</td>
              <td>{{ stat.enabled_users }}</td>
              <td>{{ stat.template_count }}</td>
              <td>{{ stat.protocol_count }}</td>
              <td>
                <span class="node-count" :class="stat.online_nodes > 0 ? 'online' : 'offline'">
                  {{ stat.online_nodes }}
                </span>
              </td>
              <td>{{ formatBytes(stat.total_traffic) }}</td>
              <td>{{ stat.plan_count }}</td>
              <td>
                <button class="btn btn-sm btn-outline" @click.stop="selectGroupById(stat.group_id)">
                  {{ $t('admin.subscriptions.stats.view') }}
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <div v-else class="empty-state stats-empty">
        <p>{{ $t('admin.subscriptions.stats.empty') }}</p>
      </div>
    </div>

    <!-- Group list -->
    <div class="groups-section">
      <div class="section-header">
        <h2>{{ $t('admin.subscriptions.groups') }}</h2>
        <button class="btn btn-primary" @click="showCreateGroupModal = true">
          <svg class="icon" viewBox="0 0 24 24" width="16" height="16" aria-hidden="true"><path fill="currentColor" d="M19 11h-6V5h-2v6H5v2h6v6h2v-6h6v-2z"/></svg>
          {{ $t('admin.subscriptions.createGroup') }}
        </button>
      </div>

      <div class="groups-grid">
        <div 
          v-for="group in groups" 
          :key="group.id" 
          class="group-card"
          :class="{ active: selectedGroup?.id === group.id }"
          @click="selectGroup(group)"
        >
          <div class="group-header">
            <h3>{{ group.name }}</h3>
            <span class="badge" :class="group.enable ? 'badge-success' : 'badge-secondary'">
              {{ group.enable ? $t('admin.subscriptions.enabled') : $t('admin.subscriptions.disabled') }}
            </span>
          </div>
          <p class="group-desc">{{ group.description || $t('admin.subscriptions.noDescription') }}</p>
          <div class="group-meta">
            <span>{{ $t('admin.subscriptions.priority') }}: {{ group.priority }}</span>
            <span>{{ $t('admin.subscriptions.templates') }}: {{ group.template_count || 0 }}</span>
          </div>
          <div class="group-actions">
            <button class="btn btn-sm btn-outline" @click.stop="copyGroupSubscription(group)">
              <svg class="icon" viewBox="0 0 24 24" width="16" height="16" aria-hidden="true"><path fill="currentColor" d="M16 1H4a2 2 0 0 0-2 2v12h2V3h12V1zM20 5H8a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7a2 2 0 0 0-2-2zm-1 15H9V8h10v12z"/></svg>
              {{ $t('admin.subscriptions.copySubscription') }}
            </button>
            <button class="btn btn-sm btn-outline" @click.stop="copyGroupCombined(group)">
              <svg class="icon" viewBox="0 0 24 24" width="16" height="16" aria-hidden="true"><path fill="currentColor" d="M16 1H4a2 2 0 0 0-2 2v12h2V3h12V1zM20 5H8a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7a2 2 0 0 0-2-2zm-1 15H9V8h10v12z"/></svg>
              {{ $t('admin.subscriptions.copyCombinedSubscription') }}
            </button>
            <button class="btn btn-sm btn-outline" :aria-label="`${$t('common.actions.edit')} ${group.name}`" :title="$t('common.actions.edit')" @click.stop="editGroup(group)">
              <svg class="icon" viewBox="0 0 24 24" width="14" height="14" aria-hidden="true"><path fill="currentColor" d="M3 17.25V21h3.75L17.81 9.94l-3.75-3.75L3 17.25zM20.71 7.04a1 1 0 0 0 0-1.41l-2.34-2.34a1 1 0 0 0-1.41 0l-1.83 1.83 3.75 3.75 1.83-1.83z"/></svg>
            </button>
            <button class="btn btn-sm btn-outline btn-danger" :aria-label="`${$t('common.actions.delete')} ${group.name}`" :title="$t('common.actions.delete')" @click.stop="deleteGroup(group)">
              <svg class="icon" viewBox="0 0 24 24" width="14" height="14" aria-hidden="true"><path fill="currentColor" d="M6 19a2 2 0 0 0 2 2h8a2 2 0 0 0 2-2V7H6v12zM19 4h-3.5l-1-1h-5l-1 1H5v2h14V4z"/></svg>
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- Template list -->
    <div class="templates-section" v-if="selectedGroup">
      <div class="section-header">
        <h2>{{ $t('admin.subscriptions.templatesFor') }} {{ selectedGroup.name }}</h2>
        <div class="header-actions">
          <button class="btn btn-secondary" @click="previewSubscription">
              <svg class="icon" viewBox="0 0 24 24" width="16" height="16" aria-hidden="true"><path fill="currentColor" d="M12 5c-7 0-11 7-11 7s4 7 11 7 11-7 11-7-4-7-11-7zm0 12a5 5 0 1 1 0-10 5 5 0 0 1 0 10zM12 9a3 3 0 1 0 .001 6.001A3 3 0 0 0 12 9z"/></svg>
              {{ $t('admin.subscriptions.preview') }}
          </button>
            <button class="btn btn-primary" @click="showCreateTemplateModal = true">
            <svg class="icon" viewBox="0 0 24 24" width="16" height="16" aria-hidden="true"><path fill="currentColor" d="M19 11h-6V5h-2v6H5v2h6v6h2v-6h6v-2z"/></svg>
            {{ $t('admin.subscriptions.createTemplate') }}
          </button>
        </div>
      </div>

      <div class="templates-table">
        <table>
          <thead>
            <tr>
              <th>{{ $t('admin.subscriptions.nodeName') }}</th>
              <th>{{ $t('admin.subscriptions.protocol') }}</th>
              <th>{{ $t('admin.subscriptions.server') }}</th>
              <th>{{ $t('admin.subscriptions.port') }}</th>
              <th>{{ $t('admin.subscriptions.tls') }}</th>
              <th>{{ $t('admin.subscriptions.status') }}</th>
              <th>{{ $t('admin.subscriptions.actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="template in templates" :key="template.id">
              <td>{{ template.name }}</td>
              <td>
                <span class="protocol-badge" :class="'protocol-' + template.type">
                  {{ template.type.toUpperCase() }}
                </span>
              </td>
              <td>{{ template.server }}</td>
              <td>{{ template.port }}</td>
              <td>
                <span class="tls-badge" :class="getTLSClass(template.tls)">
                  {{ getTLSLabel(template.tls) }}
                </span>
              </td>
              <td>
                <label class="switch">
                  <input type="checkbox" :checked="template.enable === 1" @change="toggleTemplate(template)">
                  <span class="slider"></span>
                </label>
              </td>
              <td>
                <button class="btn btn-sm btn-outline" :aria-label="`${$t('common.actions.copy')} ${template.name}`" :title="$t('common.actions.copy')" @click="copyTemplateLink(template)">
                  <svg class="icon" viewBox="0 0 24 24" width="14" height="14" aria-hidden="true"><path fill="currentColor" d="M10.59 13.41L9.17 12l4.24-4.24 1.41 1.41L10.59 13.41zM6 18h12v2H6v-2z"/></svg>
                </button>
                <button class="btn btn-sm btn-outline" :aria-label="`${$t('common.actions.edit')} ${template.name}`" :title="$t('common.actions.edit')" @click="editTemplate(template)">
                  <svg class="icon" viewBox="0 0 24 24" width="14" height="14" aria-hidden="true"><path fill="currentColor" d="M3 17.25V21h3.75L17.81 9.94l-3.75-3.75L3 17.25zM20.71 7.04a1 1 0 0 0 0-1.41l-2.34-2.34a1 1 0 0 0-1.41 0l-1.83 1.83 3.75 3.75 1.83-1.83z"/></svg>
                </button>
                <button class="btn btn-sm btn-outline btn-danger" :aria-label="`${$t('common.actions.delete')} ${template.name}`" :title="$t('common.actions.delete')" @click="deleteTemplate(template)">
                  <svg class="icon" viewBox="0 0 24 24" width="14" height="14" aria-hidden="true"><path fill="currentColor" d="M6 19a2 2 0 0 0 2 2h8a2 2 0 0 0 2-2V7H6v12zM19 4h-3.5l-1-1h-5l-1 1H5v2h14V4z"/></svg>
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Production node protocol list -->
    <div class="templates-section" v-if="selectedGroup">
      <div class="section-header">
        <h2>{{ $t('admin.subscriptions.productionNodes') }}</h2>
        <div class="header-actions">
           <button class="btn btn-secondary" @click="openManageProtocolsModal">
              <svg class="icon" viewBox="0 0 24 24" width="16" height="16" aria-hidden="true"><path fill="currentColor" d="M12 22C6.477 22 2 17.523 2 12S6.477 2 12 2s10 4.477 10 10-4.477 10-10 10zm0-2a8 8 0 1 0 0-16 8 8 0 0 0 0 16zm-1-7h2v2h-2v-2zm0-4h2v2h-2V9z"/></svg>
              {{ $t('admin.subscriptions.manageRelations') }}
           </button>
           <span class="info-badge">{{ $t('admin.subscriptions.linkedProtocolsInfo') }}</span>
        </div>
      </div>

      <div class="templates-table">
        <table v-if="protocols.length > 0">
          <thead>
            <tr>
              <th>{{ $t('admin.subscriptions.productionTable.node') }}</th>
              <th>{{ $t('admin.subscriptions.productionTable.protocol') }}</th>
              <th>{{ $t('admin.subscriptions.productionTable.name') }}</th>
              <th>{{ $t('admin.subscriptions.productionTable.port') }}</th>
              <th>{{ $t('admin.subscriptions.productionTable.visibility') }}</th>
              <th>{{ $t('admin.subscriptions.productionTable.status') }}</th>
              <th>{{ $t('admin.subscriptions.productionTable.actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="protocol in protocols" :key="protocol.id">
              <td>{{ protocol.node?.name || $t('networkPages.subscriptions.nodeIdFallback', { id: protocol.node_id }) }}</td>
              <td>
                <span class="protocol-badge" :class="'protocol-' + protocol.type">
                  {{ (protocol.type || '').toUpperCase() }}
                </span>
              </td>
              <td>{{ protocol.name }}</td>
              <td>{{ protocol.port }}</td>
              <td>
                <span :class="['status-badge', protocol.show ? 'status-online' : 'status-disabled']">
                  {{ protocol.show ? $t('admin.subscriptions.visibilityShown') : $t('admin.subscriptions.visibilityHidden') }}
                </span>
              </td>
              <td>
                <span :class="['status-badge', protocol.enable ? 'status-online' : 'status-disabled']">
                  {{ protocol.enable ? $t('admin.subscriptions.protocolOnline') : $t('admin.subscriptions.protocolOffline') }}
                </span>
              </td>
              <td>
                <button class="btn btn-sm btn-outline" @click="goToNode(protocol.node_id)" :title="$t('admin.subscriptions.goToNode')">
                  <svg class="icon" viewBox="0 0 24 24" width="14" height="14" aria-hidden="true"><path fill="currentColor" d="M19 19H5V5h7V3H5c-1.11 0-2 .9-2 2v14c0 1.1.89 2 2 2h14c1.1 0 2-.9 2-2v-7h-2v7zM14 3v2h3.59l-9.83 9.83 1.41 1.41L19 6.41V10h2V3h-7z"/></svg>
                </button>
              </td>
            </tr>
          </tbody>
        </table>
        <div v-else class="empty-state">
          <p>{{ $t('admin.subscriptions.productionNodesEmpty') }}</p>
        </div>
      </div>
    </div>

    <!-- Manage linked protocols -->
    <UiDialog v-model:open="showManageProtocolsModal" size="lg" :title="$t('admin.subscriptions.manageProtocolsTitle')" :description="$t('admin.subscriptions.manageProtocolsDescription', { group: selectedGroup?.name || '' })">
      <div class="protocol-pool-table">
        <table>
          <thead>
            <tr>
              <th width="40"><input type="checkbox" :aria-label="$t('admin.subscriptions.protocolPool.selectAll')" @change="toggleAllAvailable" :checked="isAllSelected"></th>
              <th>{{ $t('admin.subscriptions.protocolPool.node') }}</th>
              <th>{{ $t('admin.subscriptions.protocolPool.protocolName') }}</th>
              <th>{{ $t('admin.subscriptions.protocolPool.port') }}</th>
              <th>{{ $t('admin.subscriptions.protocolPool.linkedGroups') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="p in availableProtocols" :key="p.id" @click="toggleProtocolSelection(p.id)" class="clickable-row">
              <td><input type="checkbox" :aria-label="$t('admin.subscriptions.protocolPool.select', { name: p.name })" :checked="selectedProtocolIds.includes(p.id)" @change="toggleProtocolSelection(p.id)" @click.stop></td>
              <td>{{ p.node?.name || $t('admin.subscriptions.unknownNode') }}</td>
              <td>
                <span class="protocol-badge" :class="'protocol-' + p.type">{{ p.type.toUpperCase() }}</span>
                <span class="ml-2">{{ p.name }}</span>
              </td>
              <td>{{ p.port }}</td>
              <td>
                <div class="group-badges">
                  <span v-for="g in p.subscription_groups" :key="g.id" class="mini-badge">
                    {{ g.name }}
                  </span>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <p v-if="protocolsError" class="dialog-error" role="alert">{{ protocolsError }}</p>
      <template #footer="{ close }">
        <UiButton :disabled="protocolsSaving" @click="close">{{ $t('common.actions.cancel') }}</UiButton>
        <UiButton variant="primary" data-test="save-group-protocols" :loading="protocolsSaving" @click="saveGroupProtocols">{{ $t('admin.subscriptions.confirmSave') }}</UiButton>
      </template>
    </UiDialog>

    <!-- Preview -->
    <UiDialog v-model:open="showPreviewModal" size="lg" :title="$t('admin.subscriptions.previewTitle')">
      <div class="preview-format-selector">
        <label for="subscription-field-1">{{ $t('admin.subscriptions.format') }}:</label>
        <select id="subscription-field-1" v-model="previewFormat" @change="loadPreview">
          <option value="v2ray">{{ $t('admin.subscriptions.formats.v2ray') }}</option>
          <option value="clash">{{ $t('admin.subscriptions.formats.clash') }}</option>
          <option value="stash">{{ $t('admin.subscriptions.formats.stash') }}</option>
          <option value="egern">{{ $t('admin.subscriptions.formats.egern') }}</option>
          <option value="surge">{{ $t('admin.subscriptions.formats.surge') }}</option>
          <option value="loon">{{ $t('admin.subscriptions.formats.loon') }}</option>
          <option value="shadowrocket">{{ $t('admin.subscriptions.formats.shadowrocket') }}</option>
          <option value="quantumultx">{{ $t('admin.subscriptions.formats.quantumultx') }}</option>
          <option value="json">{{ $t('admin.subscriptions.formats.json') }}</option>
          <option value="base64json">{{ $t('admin.subscriptions.formats.base64json') }}</option>
        </select>
      </div>
      <div class="preview-content">
        <pre>{{ previewContent }}</pre>
      </div>
      <template #footer>
        <UiButton :icon="Download" @click="downloadPreview">{{ $t('admin.subscriptions.download') }}</UiButton>
        <UiButton variant="primary" :icon="Copy" @click="copyPreviewContent">{{ $t('admin.subscriptions.copyContent') }}</UiButton>
      </template>
    </UiDialog>

    <!-- Subscription links -->
    <UiDialog v-model:open="showSubscriptionModal" :title="$t('admin.subscriptions.subscriptionLinks')">
      <div class="subscription-links">
        <UiCopyField
          v-for="format in subscriptionFormats"
          :key="format.value"
          :value="getSubscriptionUrl(format.value)"
          :label="format.label"
          size="md"
        />
      </div>
    </UiDialog>

    <!-- Create/edit group -->
    <UiDialog :open="showCreateGroupModal || showEditGroupModal" :title="showEditGroupModal ? $t('admin.subscriptions.editGroup') : $t('admin.subscriptions.createGroup')" :dismissible="!groupSaving" @update:open="value => { if (!value) closeGroupModal() }">
      <div class="dialog-fields">
        <div class="form-group">
          <label for="subscription-field-2">{{ $t('admin.subscriptions.groupName') }} *</label>
          <input id="subscription-field-2" type="text" v-model="groupForm.name" :placeholder="$t('admin.subscriptions.groupNamePlaceholder')">
        </div>
        <div class="form-group">
          <label for="subscription-field-3">{{ $t('admin.subscriptions.description') }}</label>
          <textarea id="subscription-field-3" v-model="groupForm.description" rows="3"></textarea>
        </div>
        <div class="form-row">
          <div class="form-group">
            <label for="subscription-field-4">{{ $t('admin.subscriptions.priority') }}</label>
            <input id="subscription-field-4" type="number" v-model.number="groupForm.priority">
          </div>
          <div class="form-group">
            <label>{{ $t('admin.subscriptions.enabled') }}</label>
            <label class="switch">
              <input type="checkbox" v-model="groupForm.enable" :aria-label="$t('admin.subscriptions.enabled')">
              <span class="slider"></span>
            </label>
          </div>
        </div>
        <p v-if="groupFormError" class="dialog-error" role="alert" data-test="group-form-error">{{ groupFormError }}</p>
      </div>
      <template #footer="{ close }">
        <UiButton :disabled="groupSaving" @click="close">{{ $t('common.actions.cancel') }}</UiButton>
        <UiButton variant="primary" data-test="save-group" :loading="groupSaving" @click="saveGroup">{{ $t('common.actions.save') }}</UiButton>
      </template>
    </UiDialog>

    <!-- Create/edit template -->
    <UiDialog :open="showCreateTemplateModal || showEditTemplateModal" size="lg" :title="showEditTemplateModal ? $t('admin.subscriptions.editTemplate') : $t('admin.subscriptions.createTemplate')" :dismissible="!templateSaving" @update:open="value => { if (!value) closeTemplateModal() }">
      <div class="dialog-fields">
        <div class="form-row">
          <div class="form-group">
            <label for="subscription-field-5">{{ $t('admin.subscriptions.nodeName') }} *</label>
            <input id="subscription-field-5" type="text" v-model="templateForm.name" :placeholder="$t('admin.subscriptions.nodeNamePlaceholder')">
          </div>
          <div class="form-group">
            <label for="subscription-field-6">{{ $t('admin.subscriptions.protocol') }} *</label>
            <select id="subscription-field-6" v-model="templateForm.type">
              <option value="vless">{{ $t('networkPages.subscriptions.protocols.vless') }}</option>
              <option value="vmess">{{ $t('networkPages.subscriptions.protocols.vmess') }}</option>
              <option value="trojan">{{ $t('networkPages.subscriptions.protocols.trojan') }}</option>
              <option value="shadowsocks">{{ $t('networkPages.subscriptions.protocols.shadowsocks') }}</option>
              <option value="hysteria2">{{ $t('networkPages.subscriptions.protocols.hysteria2') }}</option>
              <option value="tuic">{{ $t('networkPages.subscriptions.protocols.tuic') }}</option>
            </select>
          </div>
        </div>
        <div class="form-row">
          <div class="form-group">
            <label for="subscription-field-7">{{ $t('admin.subscriptions.server') }} *</label>
            <input id="subscription-field-7" type="text" v-model="templateForm.server" :placeholder="$t('networkPages.subscriptions.placeholders.server')">
          </div>
          <div class="form-group">
            <label for="subscription-field-8">{{ $t('admin.subscriptions.port') }} *</label>
            <input id="subscription-field-8" type="number" v-model.number="templateForm.port" :placeholder="$t('networkPages.subscriptions.placeholders.port')">
          </div>
        </div>
        <div class="form-row">
          <div class="form-group">
            <label for="subscription-field-9">{{ $t('admin.subscriptions.tls') }}</label>
            <select id="subscription-field-9" v-model.number="templateForm.tls">
              <option :value="0">{{ $t('admin.subscriptions.tlsNone') }}</option>
              <option :value="1">{{ $t('networkPages.subscriptions.tlsModes.tls') }}</option>
              <option :value="2">{{ $t('networkPages.subscriptions.tlsModes.reality') }}</option>
            </select>
          </div>
          <div class="form-group">
            <label for="subscription-field-10">{{ $t('admin.subscriptions.transport') }}</label>
            <select id="subscription-field-10" v-model="templateForm.transport">
              <option value="tcp">{{ $t('networkPages.subscriptions.transports.tcp') }}</option>
              <option value="ws">{{ $t('networkPages.subscriptions.transports.ws') }}</option>
              <option value="grpc">{{ $t('networkPages.subscriptions.transports.grpc') }}</option>
              <option value="h2">{{ $t('networkPages.subscriptions.transports.h2') }}</option>
              <option value="quic">{{ $t('networkPages.subscriptions.transports.quic') }}</option>
            </select>
          </div>
        </div>
        <div class="form-group" v-if="templateForm.tls > 0">
          <label for="subscription-field-11">{{ $t('admin.subscriptions.sni') }}</label>
          <input id="subscription-field-11" type="text" v-model="templateForm.server_name" :placeholder="$t('networkPages.subscriptions.placeholders.sni')">
        </div>
        <div class="form-row" v-if="templateForm.tls === 2">
          <div class="form-group">
            <label for="subscription-field-12">{{ $t('admin.subscriptions.realityPublicKey') }}</label>
            <input id="subscription-field-12" type="text" v-model="templateForm.reality_public_key">
          </div>
          <div class="form-group">
            <label for="subscription-field-13">{{ $t('admin.subscriptions.realityShortId') }}</label>
            <input id="subscription-field-13" type="text" v-model="templateForm.reality_short_id">
          </div>
        </div>
        <div class="form-group" v-if="templateForm.tls > 0">
          <label for="subscription-field-14">{{ $t('admin.subscriptions.tlsFingerprint') }}</label>
          <select id="subscription-field-14" v-model="templateForm.tls_fingerprint">
            <option value="">{{ $t('admin.subscriptions.defaultOption') }}</option>
            <option value="chrome">{{ $t('networkPages.subscriptions.fingerprints.chrome') }}</option>
            <option value="firefox">{{ $t('networkPages.subscriptions.fingerprints.firefox') }}</option>
            <option value="safari">{{ $t('networkPages.subscriptions.fingerprints.safari') }}</option>
            <option value="edge">{{ $t('networkPages.subscriptions.fingerprints.edge') }}</option>
            <option value="random">{{ $t('networkPages.subscriptions.fingerprints.random') }}</option>
          </select>
        </div>
        <div class="form-group" v-if="templateForm.transport === 'ws'">
          <label for="subscription-field-15">{{ $t('admin.subscriptions.websocketPath') }}</label>
          <input id="subscription-field-15" type="text" v-model="wsPath" :placeholder="$t('networkPages.subscriptions.placeholders.wsPath')">
        </div>
        <div class="form-group" v-if="templateForm.type === 'vless' && templateForm.tls === 2">
          <label for="subscription-field-16">{{ $t('admin.subscriptions.flow') }}</label>
          <select id="subscription-field-16" v-model="vlessFlow">
            <option value="">{{ $t('admin.subscriptions.noneOption') }}</option>
            <option value="xtls-rprx-vision">{{ $t('networkPages.subscriptions.flows.xtlsRprxVision') }}</option>
          </select>
        </div>
        <div class="form-group">
          <label>{{ $t('admin.subscriptions.enabled') }}</label>
          <label class="switch">
            <input type="checkbox" v-model="templateForm.enable" :aria-label="$t('admin.subscriptions.enabled')">
            <span class="slider"></span>
          </label>
        </div>
        <p v-if="templateFormError" class="dialog-error" role="alert" data-test="template-form-error">{{ templateFormError }}</p>
      </div>
      <template #footer="{ close }">
        <UiButton :disabled="templateSaving" @click="close">{{ $t('common.actions.cancel') }}</UiButton>
        <UiButton variant="primary" data-test="save-template" :loading="templateSaving" @click="saveTemplate">{{ $t('common.actions.save') }}</UiButton>
      </template>
    </UiDialog>
  </div>
</template>

<script>
import { ref, reactive, onMounted, onUnmounted, computed } from 'vue'
import adminApi, { getSubscriptionStats } from '@/api/admin'
import { useUserStore } from '@/stores/user'
import { useAppI18n } from '@/composables/useAppI18n'
import { Copy, Download } from '@lucide/vue'
import { UiButton, UiCopyField, UiDialog, copyText, useConfirm, useToast } from '@/ui'

export default {
  name: 'Subscriptions',
  components: { UiButton, UiCopyField, UiDialog },
  setup() {
    const { t } = useAppI18n()
    const toast = useToast()
    const confirm = useConfirm()
    const userStore = useUserStore()
    
    // Data
    const groups = ref([])
    const groupStats = ref([])
    const templates = ref([])
    const protocols = ref([])
    const selectedGroup = ref(null)
    
    // Modals
    const showPreviewModal = ref(false)
    const showSubscriptionModal = ref(false)
    const showCreateGroupModal = ref(false)
    const showEditGroupModal = ref(false)
    const showCreateTemplateModal = ref(false)
    const showEditTemplateModal = ref(false)
    const showManageProtocolsModal = ref(false)
    
    // Protocol Pool Data
    const availableProtocols = ref([])
    const selectedProtocolIds = ref([])
    
    // Forms
    const groupForm = reactive({
      id: null,
      name: '',
      description: '',
      priority: 0,
      enable: true
    })
    
    const templateForm = reactive({
      id: null,
      name: '',
      type: 'vless',
      server: '',
      port: 443,
      server_name: '',
      tls: 1,
      tls_fingerprint: 'chrome',
      transport: 'tcp',
      reality_public_key: '',
      reality_short_id: '',
      enable: true
    })
    
    const wsPath = ref('/ws')
    const vlessFlow = ref('xtls-rprx-vision')
    // Preview
    const previewFormat = ref('v2ray')
    const previewContent = ref('')
    
    // Dialog state: saving flags and inline errors
    const protocolsSaving = ref(false)
    const protocolsError = ref('')
    const groupSaving = ref(false)
    const groupFormError = ref('')
    const templateSaving = ref(false)
    const templateFormError = ref('')
    
    const subscriptionFormats = computed(() => [
      { value: 'auto', label: t('admin.subscriptions.formats.auto') },
      { value: 'v2ray', label: t('admin.subscriptions.formats.v2ray') },
      { value: 'clash', label: t('admin.subscriptions.formats.clash') },
      { value: 'stash', label: t('admin.subscriptions.formats.stash') },
      { value: 'egern', label: t('admin.subscriptions.formats.egern') },
      { value: 'surge', label: t('admin.subscriptions.formats.surge') },
      { value: 'loon', label: t('admin.subscriptions.formats.loon') },
      { value: 'shadowrocket', label: t('admin.subscriptions.formats.shadowrocket') },
      { value: 'quantumultx', label: t('admin.subscriptions.formats.quantumultx') },
      { value: 'json', label: t('admin.subscriptions.formats.json') },
      { value: 'base64json', label: t('admin.subscriptions.formats.base64json') }
    ])

    const totalUsers = computed(() => groupStats.value.reduce((sum, s) => sum + s.user_count, 0))
    const totalTemplates = computed(() => groupStats.value.reduce((sum, s) => sum + s.template_count, 0))
    const totalTraffic = computed(() => groupStats.value.reduce((sum, s) => sum + s.total_traffic, 0))
    const sortedGroupStats = computed(() => [...groupStats.value].sort((a, b) => b.user_count - a.user_count))

    const readSubscriptionEnvelopeError = (res) => {
      const candidates = [res, res?.data]
      for (const candidate of candidates) {
        if (!candidate || typeof candidate !== 'object') continue
        if (!Object.prototype.hasOwnProperty.call(candidate, 'code')) continue
        if (Number(candidate.code) === 0) return null
        return candidate.msg || candidate.message || candidate.error || ''
      }
      return null
    }

    const ensureSubscriptionSuccess = (res, fallbackMessage) => {
      const message = readSubscriptionEnvelopeError(res)
      if (message !== null) {
        throw new Error(message || fallbackMessage)
      }
      return res
    }

    const readSubscriptionPayload = (res, fallbackMessage = t('admin.subscriptions.loadError')) => {
      const payload = ensureSubscriptionSuccess(res, fallbackMessage)
      if (!payload || typeof payload !== 'object') return null
      if (Object.prototype.hasOwnProperty.call(payload, 'code')) return payload.data ?? null
      if (payload.data && typeof payload.data === 'object' && Object.prototype.hasOwnProperty.call(payload.data, 'code')) {
        return payload.data.data ?? null
      }
      if (payload.data && typeof payload.data === 'object' && Object.prototype.hasOwnProperty.call(payload.data, 'data')) {
        return payload.data.data ?? null
      }
      return payload.data ?? payload
    }

    const readSubscriptionList = (res) => {
      const payload = readSubscriptionPayload(res)
      return Array.isArray(payload) ? payload : []
    }

    const readSubscriptionStats = (res) => {
      if (!res || typeof res !== 'object') return []
      return readSubscriptionList(res)
    }
    
    // Methods
    const loadGroups = async () => {
      try {
        const res = await adminApi.getSubscriptionGroups()
        groups.value = readSubscriptionList(res)
        if (groups.value.length > 0 && !selectedGroup.value) {
          selectGroup(groups.value[0])
        }
      } catch (error) {
        showToast(t('admin.subscriptions.loadError'), 'error')
      }
    }

    const loadStats = async () => {
      try {
        const res = await getSubscriptionStats()
        groupStats.value = readSubscriptionStats(res)
      } catch (error) {
        console.error('Failed to load subscription stats:', error)
      }
    }

    const selectGroupById = (groupId) => {
      const group = groups.value.find(g => g.id === groupId)
      if (group) selectGroup(group)
    }

    const formatBytes = (bytes) => {
      if (!bytes || bytes === 0) return '0 B'
      const units = ['B', 'KB', 'MB', 'GB', 'TB']
      let i = 0
      let val = bytes
      while (val >= 1024 && i < units.length - 1) {
        val /= 1024
        i++
      }
      return val.toFixed(2) + ' ' + units[i]
    }

    const formatTotalTraffic = (bytes) => {
      if (!bytes || bytes === 0) return '0 GB'
      const gb = (bytes / (1024 * 1024 * 1024)).toFixed(2)
      return gb + ' GB'
    }
    
    const loadTemplates = async (groupId) => {
      try {
        const res = await adminApi.getSubscriptionTemplates(groupId)
        templates.value = readSubscriptionList(res)
      } catch (error) {
        showToast(t('admin.subscriptions.loadError'), 'error')
      }
    }

    const loadProtocols = async (groupId) => {
      try {
        const res = await adminApi.getSubscriptionProtocols(groupId)
        protocols.value = readSubscriptionList(res)
      } catch (error) {
        console.error('Failed to load protocols:', error)
      }
    }

    const openManageProtocolsModal = async () => {
      if (!selectedGroup.value) return
      await loadAvailableProtocols()
      selectedProtocolIds.value = protocols.value.map((p) => p.id)
      protocolsError.value = ''
      showManageProtocolsModal.value = true
    }

    const loadAvailableProtocols = async () => {
      try {
        const res = await adminApi.getAvailableProtocols()
        availableProtocols.value = readSubscriptionList(res)
      } catch (error) {
        showToast(t('admin.subscriptions.availableProtocolsLoadError'), 'error')
      }
    }

    const toggleProtocolSelection = (id) => {
      const index = selectedProtocolIds.value.indexOf(id)
      if (index > -1) {
        selectedProtocolIds.value.splice(index, 1)
      } else {
        selectedProtocolIds.value.push(id)
      }
    }

    const isAllSelected = computed(() => {
      return availableProtocols.value.length > 0 && selectedProtocolIds.value.length === availableProtocols.value.length
    })

    const toggleAllAvailable = () => {
      if (isAllSelected.value) {
        selectedProtocolIds.value = []
      } else {
        selectedProtocolIds.value = availableProtocols.value.map(p => p.id)
      }
    }

    const saveGroupProtocols = async () => {
      if (protocolsSaving.value) return
      protocolsSaving.value = true
      protocolsError.value = ''
      try {
        ensureSubscriptionSuccess(
          await adminApi.updateGroupProtocols(selectedGroup.value.id, selectedProtocolIds.value),
          t('admin.subscriptions.groupProtocolsUpdateFailed')
        )
        showToast(t('admin.subscriptions.groupProtocolsUpdated'), 'success')
        showManageProtocolsModal.value = false
        loadProtocols(selectedGroup.value.id)
      } catch (error) {
        protocolsError.value = t('admin.subscriptions.groupProtocolsUpdateFailed')
      } finally {
        protocolsSaving.value = false
      }
    }
    
    const selectGroup = (group) => {
      selectedGroup.value = group
      loadTemplates(group.id)
      loadProtocols(group.id)
    }
    
    const copyGroupSubscription = (group) => {
      selectedGroup.value = group
      showSubscriptionModal.value = true
    }

    // Copy merged subscription content for a group
    const copyGroupCombined = async (group) => {
      selectedGroup.value = group

      // Prefer the server-side merged result first
      try {
        const res = await adminApi.previewSubscription({ group_ids: [group.id], format: 'v2ray' })
        const payload = readSubscriptionPayload(res)
        const content = payload?.content || ''
        if (content) {
          await copyToClipboard(content)
          showToast(t('admin.subscriptions.copied'), 'success')
          return
        }
      } catch (e) {
        // Fall through to the panel-side fallback merge
      }

      try {
        const tplRes = await adminApi.getSubscriptionTemplates(group.id)
        const tplList = readSubscriptionList(tplRes)
        const lines = []
        for (const tpl of tplList) {
          const link = generateNodeLink(tpl)
          if (link) lines.push(link)
        }
        if (lines.length === 0) {
          showToast(t('admin.subscriptions.copyError'), 'error')
          return
        }
        const combined = lines.join('\n')
        const encoded = btoa(combined)
        await copyToClipboard(encoded)
        showToast(t('admin.subscriptions.copyFallbackNotice'), 'success')
      } catch (e) {
        showToast(t('admin.subscriptions.copyError'), 'error')
      }
    }
    
    const getSubscriptionUrl = (format) => {
      // Use the current logged-in administrator subscription token
      const baseUrl = window.location.origin
      const token = userStore.userInfo?.token || ''
      if (!token) return ''
      const groupQuery = `groups=${selectedGroup.value?.id || ''}`
      if (!format || format === 'auto' || format === 'ua') {
        return `${baseUrl}/s/${token}?${groupQuery}`
      }
      return `${baseUrl}/s/${token}?type=${format}&${groupQuery}`
    }
    
    const copyToClipboard = async (text) => {
      if (!text) {
        showToast(t('admin.subscriptions.copyError'), 'error')
        return
      }
      if (await copyText(text)) {
        showToast(t('admin.subscriptions.copied'), 'success')
      } else {
        showToast(t('admin.subscriptions.copyError'), 'error')
      }
    }
    
    const previewSubscription = async () => {
      showPreviewModal.value = true
      await loadPreview()
    }
    
    const loadPreview = async () => {
      try {
        const res = await adminApi.previewSubscription({
          group_ids: [selectedGroup.value.id],
          format: previewFormat.value
        })
        const payload = readSubscriptionPayload(res)
        previewContent.value = payload?.content || ''
      } catch (error) {
        previewContent.value = t('admin.subscriptions.previewError')
      }
    }
    
    const copyPreviewContent = () => {
      copyToClipboard(previewContent.value)
    }

    const getSubscriptionFileExt = (format) => {
      if (format === 'auto' || format === 'ua') return 'txt'
      if (format === 'clash' || format === 'stash' || format === 'egern') return 'yaml'
      if (format === 'json' || format === 'sing-box') return 'json'
      if (format === 'surge') return 'conf'
      return 'txt'
    }
    
    const downloadPreview = () => {
      const ext = getSubscriptionFileExt(previewFormat.value)
      const blob = new Blob([previewContent.value], { type: 'text/plain' })
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `subscription.${ext}`
      a.click()
      URL.revokeObjectURL(url)
    }
    
    const editGroup = (group) => {
      Object.assign(groupForm, {
        id: group.id,
        name: group.name,
        description: group.description || '',
        priority: group.priority,
        enable: group.enable === 1
      })
      groupFormError.value = ''
      showEditGroupModal.value = true
    }
    
    // Danger confirmation that runs the request; a failure stays inline.
    const confirmDelete = (options, request) => confirm({
      tone: 'danger',
      ...options,
      onConfirm: async () => {
        try {
          ensureSubscriptionSuccess(await request(), t('admin.subscriptions.deleteError'))
        } catch (error) {
          throw new Error(error?.message || t('admin.subscriptions.deleteError'))
        }
      }
    })

    const deleteGroup = async (group) => {
      const confirmed = await confirmDelete({
        title: t('admin.subscriptions.confirm.deleteGroupTitle', { name: group.name }),
        message: t('admin.subscriptions.confirm.deleteGroupMessage'),
        confirmLabel: t('admin.subscriptions.confirm.deleteGroupAction')
      }, () => adminApi.deleteSubscriptionGroup(group.id))
      if (!confirmed) return
      showToast(t('admin.subscriptions.groupDeleted'), 'success')
      loadGroups()
      if (selectedGroup.value?.id === group.id) {
        selectedGroup.value = null
        templates.value = []
      }
    }
    
    const closeGroupModal = () => {
      if (groupSaving.value) return
      groupFormError.value = ''
      showCreateGroupModal.value = false
      showEditGroupModal.value = false
      Object.assign(groupForm, { id: null, name: '', description: '', priority: 0, enable: true })
    }
    
    const saveGroup = async () => {
      if (groupSaving.value) return
      groupSaving.value = true
      groupFormError.value = ''
      try {
        const data = {
          name: groupForm.name,
          description: groupForm.description,
          priority: groupForm.priority,
          enable: groupForm.enable ? 1 : 0
        }
        
        if (groupForm.id) {
          ensureSubscriptionSuccess(
            await adminApi.updateSubscriptionGroup(groupForm.id, data),
            t('admin.subscriptions.saveError')
          )
        } else {
          ensureSubscriptionSuccess(
            await adminApi.createSubscriptionGroup(data),
            t('admin.subscriptions.saveError')
          )
        }
        
        groupSaving.value = false
        showToast(t('admin.subscriptions.groupSaved'), 'success')
        closeGroupModal()
        loadGroups()
      } catch (error) {
        groupFormError.value = t('admin.subscriptions.saveError')
      } finally {
        groupSaving.value = false
      }
    }
    
    const editTemplate = (template) => {
      Object.assign(templateForm, {
        id: template.id,
        name: template.name,
        type: template.type,
        server: template.server,
        port: template.port,
        server_name: template.server_name || '',
        tls: template.tls,
        tls_fingerprint: template.tls_fingerprint || '',
        transport: template.transport || 'tcp',
        reality_public_key: template.reality_public_key || '',
        reality_short_id: template.reality_short_id || '',
        enable: template.enable === 1
      })
      
      // Parse transport_settings
      if (template.transport_settings) {
        try {
          const settings = JSON.parse(template.transport_settings)
          wsPath.value = settings.path || '/ws'
        } catch (e) {}
      }
      
      // Parse protocol_settings
      if (template.protocol_settings) {
        try {
          const settings = JSON.parse(template.protocol_settings)
          vlessFlow.value = settings.flow || ''
        } catch (e) {}
      }
      
      templateFormError.value = ''
      showEditTemplateModal.value = true
    }
    
    const deleteTemplate = async (template) => {
      const confirmed = await confirmDelete({
        title: t('admin.subscriptions.confirm.deleteTemplateTitle', { name: template.name }),
        message: t('admin.subscriptions.confirm.deleteTemplateMessage'),
        confirmLabel: t('admin.subscriptions.confirm.deleteTemplateAction')
      }, () => adminApi.deleteSubscriptionTemplate(template.id))
      if (!confirmed) return
      showToast(t('admin.subscriptions.templateDeleted'), 'success')
      loadTemplates(selectedGroup.value.id)
    }
    
    const toggleTemplate = async (template) => {
      try {
        ensureSubscriptionSuccess(
          await adminApi.updateSubscriptionTemplate(template.id, {
            enable: template.enable === 1 ? 0 : 1
          }),
          t('admin.subscriptions.updateError')
        )
        loadTemplates(selectedGroup.value.id)
      } catch (error) {
        showToast(t('admin.subscriptions.updateError'), 'error')
      }
    }
    
    const closeTemplateModal = () => {
      if (templateSaving.value) return
      templateFormError.value = ''
      showCreateTemplateModal.value = false
      showEditTemplateModal.value = false
      Object.assign(templateForm, {
        id: null, name: '', type: 'vless', server: '', port: 443,
        server_name: '', tls: 1, tls_fingerprint: 'chrome', transport: 'tcp',
        reality_public_key: '', reality_short_id: '', enable: true
      })
      wsPath.value = '/ws'
      vlessFlow.value = 'xtls-rprx-vision'
    }
    
    const saveTemplate = async () => {
      if (templateSaving.value) return
      templateSaving.value = true
      templateFormError.value = ''
      try {
        const data = {
          name: templateForm.name,
          type: templateForm.type,
          server: templateForm.server,
          port: templateForm.port,
          server_name: templateForm.server_name || null,
          tls: templateForm.tls,
          tls_fingerprint: templateForm.tls_fingerprint || null,
          transport: templateForm.transport,
          enable: templateForm.enable ? 1 : 0
        }
        
        // Reality settings
        if (templateForm.tls === 2) {
          data.reality_public_key = templateForm.reality_public_key
          data.reality_short_id = templateForm.reality_short_id
        }
        
        // Transport settings
        if (templateForm.transport === 'ws') {
          data.transport_settings = JSON.stringify({ path: wsPath.value, host: templateForm.server_name || templateForm.server })
        }
        
        // Protocol settings
        if (templateForm.type === 'vless' && vlessFlow.value) {
          data.protocol_settings = JSON.stringify({ flow: vlessFlow.value })
        }
        
        if (templateForm.id) {
          ensureSubscriptionSuccess(
            await adminApi.updateSubscriptionTemplate(templateForm.id, data),
            t('admin.subscriptions.saveError')
          )
        } else {
          ensureSubscriptionSuccess(
            await adminApi.createSubscriptionTemplate(selectedGroup.value.id, data),
            t('admin.subscriptions.saveError')
          )
        }
        
        templateSaving.value = false
        showToast(t('admin.subscriptions.templateSaved'), 'success')
        closeTemplateModal()
        loadTemplates(selectedGroup.value.id)
      } catch (error) {
        templateFormError.value = t('admin.subscriptions.saveError')
      } finally {
        templateSaving.value = false
      }
    }
    
    const copyTemplateLink = async (template) => {
      const link = generateNodeLink(template)
      copyToClipboard(link)
    }
    
    const generateNodeLink = (template) => {
      const uuid = 'test-uuid-1234-5678-abcdef'
      
      if (template.type === 'vless') {
        let params = [`encryption=none`]
        
        if (template.tls === 2) {
          params.push('security=reality')
          if (template.reality_public_key) params.push(`pbk=${template.reality_public_key}`)
          if (template.reality_short_id) params.push(`sid=${template.reality_short_id}`)
          if (template.server_name) params.push(`sni=${template.server_name}`)
          if (template.tls_fingerprint) params.push(`fp=${template.tls_fingerprint}`)
        } else if (template.tls === 1) {
          params.push('security=tls')
          if (template.server_name) params.push(`sni=${template.server_name}`)
        } else {
          params.push('security=none')
        }
        
        params.push(`type=${template.transport || 'tcp'}`)
        
        if (template.protocol_settings) {
          try {
            const settings = JSON.parse(template.protocol_settings)
            if (settings.flow) params.push(`flow=${settings.flow}`)
          } catch (e) {}
        }
        
        if (template.transport === 'tcp') {
          params.push('headerType=none')
        }
        
        return `vless://${uuid}@${template.server}:${template.port}?${params.join('&')}#${encodeURIComponent(template.name)}`
      }
      
      if (template.type === 'vmess') {
        const config = {
          v: '2',
          ps: template.name,
          add: template.server,
          port: template.port,
          id: uuid,
          aid: 0,
          scy: 'auto',
          net: template.transport || 'tcp',
          type: 'none',
          tls: template.tls === 1 ? 'tls' : ''
        }
        return `vmess://${btoa(JSON.stringify(config))}`
      }
      
      if (template.type === 'trojan') {
        let params = []
        if (template.server_name) params.push(`sni=${template.server_name}`)
        params.push(`type=${template.transport || 'tcp'}`)
        const paramStr = params.length ? `?${params.join('&')}` : ''
        return `trojan://${uuid}@${template.server}:${template.port}${paramStr}#${encodeURIComponent(template.name)}`
      }
      
      if (template.type === 'hysteria2' || template.type === 'hy2') {
        let params = []
        if (template.server_name) params.push(`sni=${template.server_name}`)
        const paramStr = params.length ? `?${params.join('&')}` : ''
        return `hy2://${uuid}@${template.server}:${template.port}${paramStr}#${encodeURIComponent(template.name)}`
      }
      
      return ''
    }
    
    const getTLSClass = (tls) => {
      return tls === 2 ? 'tls-reality' : tls === 1 ? 'tls-enabled' : 'tls-none'
    }
    
    const getTLSLabel = (tls) => {
      return tls === 2 ? t('admin.subscriptions.tlsReality') : tls === 1 ? t('admin.subscriptions.tlsEnabled') : t('admin.subscriptions.tlsNone')
    }
    
    // Results go to the app toasts: success disappears, errors stay.
    const showToast = (message, type = 'success') => (
      type === 'error' ? toast.error(message) : toast.success(message)
    )
    
    const goToNode = (nodeId) => {
      window.location.hash = `#/admin/nodes?id=${nodeId}`
    }

    // Lifecycle
    onMounted(() => {
      loadGroups()
      loadStats()
      // Auto-refresh stats when page becomes visible
      document.addEventListener('visibilitychange', handleVisibilityChange)
    })

    onUnmounted(() => {
      document.removeEventListener('visibilitychange', handleVisibilityChange)
    })

    function handleVisibilityChange() {
      if (!document.hidden) {
        loadGroups()
        loadStats()
      }
    }
    
    return {
      groups,
      groupStats,
      templates,
      protocols,
      selectedGroup,
      showPreviewModal,
      showSubscriptionModal,
      showCreateGroupModal,
      showEditGroupModal,
      showCreateTemplateModal,
      showEditTemplateModal,
      showManageProtocolsModal,
      availableProtocols,
      selectedProtocolIds,
      isAllSelected,
      groupForm,
      templateForm,
      wsPath,
      vlessFlow,
      previewFormat,
      previewContent,
      subscriptionFormats,
      protocolsSaving,
      protocolsError,
      groupSaving,
      groupFormError,
      templateSaving,
      templateFormError,
      Copy,
      Download,
      totalUsers,
      totalTemplates,
      totalTraffic,
      sortedGroupStats,
      loadGroups,
      loadStats,
      selectGroup,
      selectGroupById,
      copyGroupSubscription,
      getSubscriptionUrl,
      copyToClipboard,
      previewSubscription,
      loadPreview,
      copyPreviewContent,
      downloadPreview,
      editGroup,
      deleteGroup,
      closeGroupModal,
      saveGroup,
      editTemplate,
      deleteTemplate,
      toggleTemplate,
      closeTemplateModal,
      saveTemplate,
      copyTemplateLink,
      copyGroupCombined,
      getTLSClass,
      getTLSLabel,
      goToNode,
      openManageProtocolsModal,
      toggleProtocolSelection,
      toggleAllAvailable,
      saveGroupProtocols,
      formatBytes,
      formatTotalTraffic
    }
  }
}
</script>

<style scoped>
.subscriptions-page { padding: 24px; }

.page-header { margin-bottom: 24px; }
.page-header h1 { margin: 0; font-size: 24px; }
.subtitle { color: var(--text-secondary); margin-top: 8px; }

/* Overview stats */
.overview-stats {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 16px;
  margin-bottom: 24px;
}

.overview-stats .stat-card {
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  padding: 20px;
  text-align: center;
}

.overview-stats .stat-value {
  font-size: 28px;
  font-weight: 700;
  color: var(--text-color);
}

.overview-stats .stat-label {
  font-size: 13px;
  color: var(--text-secondary);
  margin-top: 6px;
}

/* Stats table */
.stats-table-section {
  margin-bottom: 32px;
}

.stats-table-section h2 {
  font-size: 18px;
  margin: 0 0 16px;
}

.stats-table-wrapper {
  overflow-x: auto;
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
}

.stats-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 14px;
}

.stats-table thead {
  background: var(--bg-color);
  border-bottom: 1px solid var(--border-color);
}

.stats-table th {
  padding: 12px 16px;
  text-align: left;
  font-weight: 600;
  color: var(--text-secondary);
  font-size: 12px;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  white-space: nowrap;
}

.stats-table td {
  padding: 12px 16px;
  border-top: 1px solid var(--border-color);
  white-space: nowrap;
}

.stats-table .clickable-row {
  cursor: pointer;
  transition: background 0.15s;
}

.stats-table .clickable-row:hover {
  background: var(--bg-color);
}

.stats-table .group-name {
  font-weight: 500;
  color: var(--text-color);
}

.stats-table .node-count {
  display: inline-block;
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 12px;
  font-weight: 600;
}

.stats-table .node-count.online {
  background: rgba(34, 197, 94, 0.12);
  color: var(--success-color);
}

.stats-table .node-count.offline {
  background: rgba(255, 255, 255, 0.04);
  color: var(--text-secondary);
}

.stats-empty {
  text-align: center;
  padding: 32px;
  color: var(--text-secondary);
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
}

.section-header { display:flex; justify-content:space-between; align-items:center; margin-bottom:16px; }
.section-header h2 { margin:0; font-size:18px; }
.header-actions { display:flex; gap:8px; }

.groups-grid { display:grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap:16px; margin-bottom:32px; }

.group-card {
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  padding: 16px;
  cursor: pointer;
  transition: var(--transition);
}
.group-card:hover { border-color: var(--primary-color); box-shadow: var(--shadow-sm); }
.group-card.active { border-color: var(--primary-color); background: color-mix(in srgb, var(--surface-color) 80%, var(--primary-color) 8%); }

.group-header { display:flex; justify-content:space-between; align-items:center; margin-bottom:8px; }
.group-header h3 { margin:0; font-size:16px; color: var(--text-color); }

.badge { padding:2px 8px; border-radius:4px; font-size:12px; }
.badge-success { background: rgba(34,197,94,0.12); color: var(--success-color); }
.badge-secondary { background: rgba(255,255,255,0.02); color: var(--text-secondary); }

.group-desc { color: var(--text-secondary); font-size:14px; margin-bottom:8px; }
.group-meta { display:flex; gap:16px; font-size:12px; color: var(--text-secondary); margin-bottom:12px; }
.group-actions { display:flex; gap:8px; }

.templates-section { background: var(--surface-color); border:1px solid var(--border-color); border-radius: var(--radius-md); padding:16px; margin-bottom: 24px; }
.info-badge { font-size: 12px; color: var(--text-secondary); background: rgba(255,255,255,0.04); padding: 4px 12px; border-radius: 20px; }
.status-badge { padding: 2px 8px; border-radius: 4px; font-size: 12px; }
.status-online { background: rgba(34,197,94,0.1); color: var(--success-color); }
.status-offline { background: rgba(244,63,94,0.1); color: var(--error-color); }
.status-disabled { background: rgba(255,255,255,0.05); color: var(--text-secondary); }
.templates-table { overflow-x:auto; }
.templates-table table { width:100%; border-collapse:collapse; }
.templates-table th, .templates-table td { padding:12px; text-align:left; border-bottom:1px solid var(--border-color); }
.templates-table th { background: rgba(255,255,255,0.02); font-weight:500; color: var(--text-secondary); }

.protocol-badge { padding:2px 8px; border-radius:4px; font-size:12px; font-weight:500; }
.protocol-vless { background: rgba(59,130,246,0.08); color: var(--primary-color); }
.protocol-vmess { background: rgba(124,58,237,0.06); color: #7b1fa2; }
.protocol-trojan { background: rgba(245,158,11,0.06); color: #e65100; }
.protocol-shadowsocks { background: rgba(34,197,94,0.06); color: var(--success-color); }
.protocol-hysteria2 { background: rgba(194,24,91,0.06); color: #c2185b; }
.protocol-tuic { background: rgba(0,131,143,0.06); color: #00838f; }

.tls-badge { padding:2px 8px; border-radius:4px; font-size:12px; }
.tls-reality { background: rgba(34,197,94,0.06); color: var(--success-color); }
.tls-enabled { background: rgba(59,130,246,0.06); color: var(--primary-color); }
.tls-none { background: rgba(244,63,94,0.06); color: #c62828; }

.switch { position:relative; display:inline-block; width:40px; height:20px; }
.switch input { opacity:0; width:0; height:0; }
.slider { position:absolute; cursor:pointer; top:0; left:0; right:0; bottom:0; background-color: rgba(255,255,255,0.06); transition:0.4s; border-radius:20px; }
.slider:before { position:absolute; content:""; height:16px; width:16px; left:2px; bottom:2px; background-color: var(--surface-color); transition:0.4s; border-radius:50%; }
input:checked + .slider { background-color: var(--primary-color); }
input:checked + .slider:before { transform: translateX(20px); }

/* Dialogs */
.dialog-error { margin: 0; color: var(--danger); }
.dialog-fields { display: grid; gap: var(--space-4); }
.dialog-fields .form-group { margin-bottom: 0; }
.dialog-fields .form-row { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: var(--space-4); }

.form-group { margin-bottom:16px; }
.form-group label { display:block; margin-bottom:4px; font-weight:500; color: var(--text-color); }
.form-group input, .form-group select, .form-group textarea { width:100%; padding:8px 12px; background:var(--surface-color); border:1px solid var(--border-color); border-radius:4px; font-size:14px; color:var(--text-color); }
.form-row { display:flex; gap:16px; }
.form-row .form-group { flex:1; }

.preview-format-selector { margin-bottom:16px; display:flex; align-items:center; gap:8px; }
.preview-format-selector select { padding:8px 12px; border:1px solid var(--border-color); border-radius:4px; background:var(--surface-color); color:var(--text-color); }

.preview-content { background: rgba(255,255,255,0.02); border:1px solid var(--border-color); border-radius:4px; padding:16px; max-height:400px; overflow:auto; color: var(--text-color); }
.preview-content pre { margin:0; white-space:pre-wrap; word-break:break-all; font-size:12px; }

.subscription-links { display:flex; flex-direction:column; gap:12px; }

/* Buttons */
.btn { padding:8px 16px; border:none; border-radius:4px; cursor:pointer; font-size:14px; display:inline-flex; align-items:center; gap:4px; }
.btn-primary { background:var(--accent-fill); color:var(--on-accent); }
.btn-secondary { background: rgba(255,255,255,0.02); color: var(--text-color); border:1px solid var(--border-color); }
.btn-outline {
  background: rgba(255,255,255,0.02);
  border: 1px solid rgba(255,255,255,0.04);
  color: var(--text-color);
}
.btn-outline:hover { background: rgba(255,255,255,0.04); }

/* Ensure icon visibility and proper sizing for small icon-only buttons */
.btn-sm { padding:4px 8px; font-size:12px; min-width:36px; height:34px; display:inline-flex; align-items:center; justify-content:center; }
.btn-sm i { font-size:14px; color: var(--text-secondary); }
.btn-sm:hover i { color: var(--text-color); }
/* svg icon color */
.icon { color: var(--text-secondary); display:inline-block; vertical-align:middle; }
.btn:hover .icon { color: var(--text-color); }
.btn-danger { color: var(--error-color); }
.btn-sm { padding:4px 8px; font-size:12px; }

.ml-2 { margin-left: 8px; }
.clickable-row { cursor: pointer; transition: background 0.2s; }
.clickable-row:hover { background: rgba(255,255,255,0.02); }

.protocol-pool-table { border: 1px solid var(--border-color); border-radius: 4px; overflow: hidden; }
.protocol-pool-table table { width: 100%; border-collapse: collapse; }
.protocol-pool-table th, .protocol-pool-table td { padding: 12px; text-align: left; border-bottom: 1px solid var(--border-color); }
.protocol-pool-table th { background: rgba(255,255,255,0.02); font-weight: 500; font-size: 13px; }

.group-badges { display: flex; flex-wrap: wrap; gap: 4px; }
.mini-badge { padding: 1px 6px; background: rgba(59,130,246,0.08); color: var(--primary-color); border-radius: 4px; font-size: 11px; }
</style>
