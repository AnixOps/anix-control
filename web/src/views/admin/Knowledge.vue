<template>
  <div class="list-page">
    <UiPageHeader :title="t('adminKnowledge.title')" :description="t('adminKnowledge.subtitle')">
      <template #actions>
        <UiButton variant="primary" :icon="Plus" data-test="knowledge-create" @click="openCreate">{{ t('adminKnowledge.actions.createArticle') }}</UiButton>
      </template>
    </UiPageHeader>

    <UiDataTable
      :columns="columns"
      :rows="visibleArticles"
      :label="t('adminKnowledge.table.label')"
      :row-label="article => article.title"
      storage-key="admin.knowledge"
      :page-size="20"
      :sort="{ key: 'sort', direction: 'asc' }"
      :loading="loading"
      :error="loadError"
      :error-title="t('adminKnowledge.messages.fetchFailed')"
      :filtered="Boolean(search || categoryFilter)"
      :empty-icon="BookOpen"
      :empty-title="t('adminKnowledge.empty.title')"
      :empty-description="t('adminKnowledge.empty.description')"
      activatable
      :row-actions="articleActions"
      @row-activate="editArticle"
      @retry="load"
      @clear-filters="clearFilters"
    >
      <template #toolbar>
        <UiSearchField v-model="search" class="list-page__search" :label="t('adminKnowledge.filters.search')" data-test="knowledge-search" />
        <UiFilterChips v-model="categoryFilter" :label="t('adminKnowledge.filters.label')" :options="categoryChips" />
      </template>
      <template #empty-actions>
        <UiButton variant="primary" :icon="Plus" @click="openCreate">{{ t('adminKnowledge.actions.createArticle') }}</UiButton>
      </template>
      <template #cell-title="{ row }">
        <span class="article-cell">
          <span class="article-cell__title">{{ row.title }}</span>
          <span class="article-cell__excerpt">{{ articleExcerpt(row.body, 90) }}</span>
        </span>
      </template>
      <template #cell-show="{ row }">
        <UiBadge :tone="row.show ? 'success' : 'neutral'" :label="row.show ? t('adminKnowledge.visibility.visible') : t('adminKnowledge.visibility.hidden')" />
      </template>
    </UiDataTable>

    <UiDialog
      v-model:open="showModal"
      size="lg"
      class="knowledge-editor"
      :title="isEdit ? t('adminKnowledge.modal.editTitle') : t('adminKnowledge.modal.createTitle')"
      :description="t('adminKnowledge.modal.description')"
      :dismissible="!saving"
    >
      <div class="form-grid editor-meta">
        <UiTextField
          v-model="form.title"
          class="form-grid__full"
          required
          :label="t('adminKnowledge.fields.title')"
          :placeholder="t('adminKnowledge.placeholders.title')"
          :error="formError && !form.title.trim() ? t('adminKnowledge.messages.titleRequired') : ''"
          data-test="knowledge-title"
        />
        <UiSelect v-model="form.category" size="md" :label="t('adminKnowledge.fields.category')" :options="categoryOptions" />
        <UiTextField v-model.number="form.sort" size="md" type="number" min="0" :label="t('adminKnowledge.fields.sort')" :help="t('adminKnowledge.fields.sortHelp')" />
        <UiSwitch v-model="visible" class="form-grid__full" :label="t('adminKnowledge.fields.visibility')" :description="t('adminKnowledge.fields.visibilityHelp')" />
      </div>

      <div v-if="narrow" class="editor-switch">
        <UiSegmentedControl
          v-model="editorView"
          :aria-label="t('adminKnowledge.editor.view')"
          :options="editorViews"
          block
        />
      </div>
      <div class="editor" :class="{ 'is-narrow': narrow }">
        <div v-show="!narrow || editorView === 'write'" class="editor__pane">
          <UiTextarea
            v-model="form.body"
            class="editor__source"
            required
            :rows="16"
            :label="t('adminKnowledge.fields.content')"
            :help="t('adminKnowledge.editor.syntax')"
            :placeholder="t('adminKnowledge.placeholders.body')"
            :error="formError && !form.body.trim() ? t('adminKnowledge.messages.bodyRequired') : ''"
            data-test="knowledge-body"
          />
        </div>
        <section v-show="!narrow || editorView === 'preview'" class="editor__pane editor__preview" aria-labelledby="knowledge-preview-label" data-test="knowledge-preview">
          <span id="knowledge-preview-label" class="editor__preview-label">{{ t('adminKnowledge.editor.preview') }}</span>
          <div class="editor__preview-body" tabindex="0" :aria-labelledby="'knowledge-preview-label'">
            <p v-if="form.title.trim()" class="editor__preview-title">{{ form.title }}</p>
            <ArticleBody v-if="previewBlocks.length" :blocks="previewBlocks" />
            <p v-else class="editor__preview-empty">{{ t('adminKnowledge.editor.previewEmpty') }}</p>
          </div>
        </section>
      </div>
      <p v-if="formError" class="form-error" role="alert" data-test="knowledge-error">{{ formError }}</p>
      <template #footer="{ close }">
        <UiButton :disabled="saving" @click="close">{{ t('common.actions.cancel') }}</UiButton>
        <UiButton variant="primary" data-test="knowledge-save" :loading="saving" @click="saveArticle">{{ isEdit ? t('common.actions.save') : t('adminKnowledge.actions.publish') }}</UiButton>
      </template>
    </UiDialog>
  </div>
</template>

<script setup>
// 帮助中心内容 (plan §8.2): the articles as a list (search, category chips,
// order, visibility) and an editor with the Markdown source and a live
// preview side by side (stacked behind a switch on narrow screens). The
// preview uses the help center's own renderer (utils/articleMarkup.js →
// ArticleBody): elements, never HTML from the text. Endpoints unchanged:
// GET/POST /admin/knowledge, PUT/DELETE /admin/knowledge/:id.
import { computed, onMounted, reactive, ref } from 'vue'
import { BookOpen, Eye, EyeOff, Pencil, Plus, Trash2 } from '@lucide/vue'
import adminApi from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'
import { NARROW_QUERY, useMediaQuery } from '@/composables/useMediaQuery'
import { articleExcerpt, parseArticle } from '@/utils/articleMarkup'
import ArticleBody from '@/views/user/ArticleBody.vue'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiDialog from '@/ui/UiDialog.vue'
import UiFilterChips from '@/ui/UiFilterChips.vue'
import UiPageHeader from '@/ui/UiPageHeader.vue'
import UiSearchField from '@/ui/UiSearchField.vue'
import UiSegmentedControl from '@/ui/UiSegmentedControl.vue'
import UiSelect from '@/ui/UiSelect.vue'
import UiSwitch from '@/ui/UiSwitch.vue'
import UiTextField from '@/ui/UiTextField.vue'
import UiTextarea from '@/ui/UiTextarea.vue'
import { useConfirm } from '@/ui/composables/useConfirm'
import { useFormat } from '@/ui/composables/useFormat'
import { useToast } from '@/ui/composables/useToast'

const { t } = useAppI18n()
const format = useFormat()
const toast = useToast()
const confirm = useConfirm()
const narrow = useMediaQuery(NARROW_QUERY)
const saving = ref(false)
const formError = ref('')
const loading = ref(false)
const loadError = ref(null)
const search = ref('')
const categoryFilter = ref('')
const editorView = ref('write')

const KNOWLEDGE_CATEGORY_VALUES = Object.freeze({
  announcement: 'announcement',
  tutorial: 'tutorial',
  faq: 'faq',
  other: 'other'
})

const LEGACY_CATEGORY_ALIASES = Object.freeze({
  '公告': KNOWLEDGE_CATEGORY_VALUES.announcement,
  '教程': KNOWLEDGE_CATEGORY_VALUES.tutorial,
  '常见问题': KNOWLEDGE_CATEGORY_VALUES.faq,
  '其他': KNOWLEDGE_CATEGORY_VALUES.other
})

const CATEGORY_STORAGE_VALUES = Object.freeze({
  [KNOWLEDGE_CATEGORY_VALUES.announcement]: '公告',
  [KNOWLEDGE_CATEGORY_VALUES.tutorial]: '教程',
  [KNOWLEDGE_CATEGORY_VALUES.faq]: '常见问题',
  [KNOWLEDGE_CATEGORY_VALUES.other]: '其他'
})

const normalizeCategory = (category) => LEGACY_CATEGORY_ALIASES[category] || category || KNOWLEDGE_CATEGORY_VALUES.other
const toStorageCategory = (category) => CATEGORY_STORAGE_VALUES[normalizeCategory(category)] || category
const defaultCategory = () => KNOWLEDGE_CATEGORY_VALUES.announcement
const categoryOptions = computed(() => ([
  { value: KNOWLEDGE_CATEGORY_VALUES.announcement, label: t('adminKnowledge.categories.announcement') },
  { value: KNOWLEDGE_CATEGORY_VALUES.tutorial, label: t('adminKnowledge.categories.tutorial') },
  { value: KNOWLEDGE_CATEGORY_VALUES.faq, label: t('adminKnowledge.categories.faq') },
  { value: KNOWLEDGE_CATEGORY_VALUES.other, label: t('adminKnowledge.categories.other') }
]))

const articles = ref([])
const showModal = ref(false)
const isEdit = ref(false)
const form = reactive({
  id: null,
  title: '',
  category: defaultCategory(),
  body: '',
  sort: 0,
  show: 1
})

const visible = computed({
  get: () => Boolean(form.show),
  set: (value) => { form.show = value ? 1 : 0 }
})
const previewBlocks = computed(() => parseArticle(form.body))
const editorViews = computed(() => [
  { value: 'write', label: t('adminKnowledge.editor.write') },
  { value: 'preview', label: t('adminKnowledge.editor.preview') }
])

const columns = computed(() => [
  { key: 'title', label: t('adminKnowledge.fields.title'), primary: true, sortable: true },
  { key: 'category', label: t('adminKnowledge.fields.category'), secondary: true, sortable: true, format: value => categoryLabel(value), sortValue: row => categoryLabel(row.category) },
  { key: 'show', label: t('adminKnowledge.fields.visibility'), sortable: true, sortValue: row => (row.show ? 0 : 1) },
  { key: 'sort', label: t('adminKnowledge.fields.sort'), sortable: true, numeric: true, align: 'end', value: row => Number(row.sort || 0) },
  { key: 'updated_at', label: t('adminKnowledge.table.updatedAt'), sortable: true, firstDirection: 'desc', nowrap: true, format: value => format.date(value), sortValue: row => Number(row.updated_at || 0) }
])
const categoryChips = computed(() => categoryOptions.value.map(option => ({
  ...option,
  count: articles.value.filter(article => article.category === option.value).length
})))
const visibleArticles = computed(() => {
  const needle = search.value.trim().toLowerCase()
  return articles.value
    .filter(article => !categoryFilter.value || article.category === categoryFilter.value)
    .filter(article => !needle || String(article.title || '').toLowerCase().includes(needle) || String(article.body || '').toLowerCase().includes(needle))
})

const articleActions = article => [
  { key: 'edit', label: t('common.actions.edit'), icon: Pencil, onSelect: () => editArticle(article) },
  article.show
    ? { key: 'hide', label: t('adminKnowledge.actions.hide'), icon: EyeOff, onSelect: () => setVisible(article, false) }
    : { key: 'show', label: t('adminKnowledge.actions.show'), icon: Eye, onSelect: () => setVisible(article, true) },
  { key: 'delete', label: t('adminKnowledge.actions.delete'), icon: Trash2, danger: true, separatorBefore: true, onSelect: () => removeArticle(article) }
]

const readList = (res) => {
  if (res && typeof res.code === 'number' && res.code !== 0) throw new Error(res.msg || t('adminKnowledge.messages.fetchFailed'))
  return Array.isArray(res?.data) ? res.data : []
}

const load = async () => {
  loading.value = true
  try {
    articles.value = readList(await adminApi.getKnowledgeList()).map((article) => ({
      ...article,
      category: normalizeCategory(article.category)
    }))
    loadError.value = null
  } catch (error) {
    loadError.value = error
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  load()
})

const categoryLabel = (category) => {
  switch (normalizeCategory(category)) {
    case KNOWLEDGE_CATEGORY_VALUES.announcement:
      return t('adminKnowledge.categories.announcement')
    case KNOWLEDGE_CATEGORY_VALUES.tutorial:
      return t('adminKnowledge.categories.tutorial')
    case KNOWLEDGE_CATEGORY_VALUES.faq:
      return t('adminKnowledge.categories.faq')
    case KNOWLEDGE_CATEGORY_VALUES.other:
      return t('adminKnowledge.categories.other')
    default:
      return category || t('adminKnowledge.categories.other')
  }
}

const clearFilters = () => {
  search.value = ''
  categoryFilter.value = ''
}

const resetForm = () => {
  form.id = null
  form.title = ''
  form.category = defaultCategory()
  form.body = ''
  form.sort = 0
  form.show = 1
  formError.value = ''
  editorView.value = 'write'
}

const openCreate = () => {
  resetForm()
  isEdit.value = false
  showModal.value = true
}

const editArticle = (article) => {
  form.id = article.id
  form.title = article.title
  form.category = normalizeCategory(article.category)
  form.body = article.body
  form.sort = article.sort
  form.show = article.show
  formError.value = ''
  editorView.value = 'write'
  isEdit.value = true
  showModal.value = true
}

const closeModal = () => {
  showModal.value = false
}

const saveArticle = async () => {
  if (saving.value) return
  formError.value = ''
  if (!form.title.trim() || !form.body.trim()) {
    formError.value = t('adminKnowledge.messages.requiredFields')
    if (narrow.value && !form.body.trim()) editorView.value = 'write'
    return
  }

  saving.value = true
  try {
    const payload = {
      title: form.title.trim(),
      category: toStorageCategory(form.category),
      body: form.body,
      sort: form.sort,
      show: form.show
    }

    if (isEdit.value) {
      await adminApi.updateKnowledge(form.id, payload)
      toast.success(t('adminKnowledge.messages.saveSuccess'))
    } else {
      await adminApi.createKnowledge(payload)
      toast.success(t('adminKnowledge.messages.publishSuccess'))
    }

    closeModal()
    await load()
  } catch (error) {
    formError.value = error.message || t('adminKnowledge.messages.actionFailed')
  } finally {
    saving.value = false
  }
}

// Show / hide from the row menu: the same update as the editor, with the
// article's other fields unchanged. Showing and hiding undo each other, so
// the toast offers 撤销 instead of a confirmation.
const setVisible = async (article, show) => {
  const payload = {
    title: article.title,
    category: toStorageCategory(article.category),
    body: article.body,
    sort: article.sort,
    show: show ? 1 : 0
  }
  try {
    await adminApi.updateKnowledge(article.id, payload)
  } catch (error) {
    toast.error(error.message || t('adminKnowledge.messages.actionFailed'))
    return
  }
  await load()
  toast.success(show ? t('adminKnowledge.messages.shown', { title: article.title }) : t('adminKnowledge.messages.hidden', { title: article.title }), {
    undo: async () => {
      await adminApi.updateKnowledge(article.id, { ...payload, show: show ? 0 : 1 })
      await load()
    }
  })
}

const removeArticle = async (article) => {
  const confirmed = await confirm({
    title: t('adminKnowledge.confirm.deleteTitle', { title: article.title }),
    message: t('adminKnowledge.confirm.deleteMessage'),
    confirmLabel: t('adminKnowledge.confirm.deleteAction'),
    tone: 'danger',
    onConfirm: async () => {
      try {
        await adminApi.deleteKnowledge(article.id)
      } catch (error) {
        throw new Error(error.message || t('adminKnowledge.messages.deleteFailed'))
      }
    }
  })
  if (!confirmed) return
  toast.success(t('adminKnowledge.messages.deleted', { title: article.title }))
  await load()
}

defineExpose({ load, editArticle, openCreate, saveArticle })
</script>

<style scoped>
.article-cell {
  display: flex;
  flex-direction: column;
  gap: var(--space-0-5);
  min-width: 0;
}

.article-cell__title {
  font-weight: var(--weight-medium);
}

.article-cell__excerpt {
  display: -webkit-box;
  overflow: hidden;
  color: var(--label-2);
  font-size: var(--type-callout-size);
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 1;
}

/* The editor is wider than a large dialog: source and preview side by side. */
:global(.ui-dialog.ui-dialog--lg.knowledge-editor) {
  --ui-dialog-width: 1040px;
}

.editor-meta {
  margin-bottom: var(--space-5);
}

.editor-meta,
.editor-switch,
.editor {
  flex: none;
}

.editor-switch {
  margin-bottom: var(--space-4);
}

.editor {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  gap: var(--space-5);
}

.editor.is-narrow {
  grid-template-columns: minmax(0, 1fr);
}

.editor__pane {
  min-width: 0;
}

.editor__source :deep(textarea) {
  min-height: 360px;
  font-family: var(--font-mono);
  font-size: var(--type-callout-size);
  line-height: var(--type-body-line);
}

.editor__preview {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.editor__preview-label {
  color: var(--label-1);
  font-size: var(--type-callout-size);
  font-weight: var(--weight-medium);
}

.editor__preview-body {
  flex: 1;
  min-height: 360px;
  max-height: 520px;
  padding: var(--space-5);
  overflow-y: auto;
  border-radius: var(--radius-sm);
  background: var(--bg-grouped);
}

.editor__preview-body:focus-visible {
  outline: var(--focus-ring);
  outline-offset: var(--focus-ring-offset);
}

.editor__preview-title {
  margin-bottom: var(--space-4);
  font-size: var(--type-title-2-size);
  font-weight: var(--type-title-2-weight);
  line-height: var(--type-title-2-line);
}

.editor__preview-empty {
  color: var(--label-2);
}
</style>
