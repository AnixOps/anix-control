<template>
  <div class="page-shell knowledge-page">
    <div class="page-toolbar">
      <div>
        <h1>{{ t('adminKnowledge.title') }}</h1>
        <p>{{ t('adminKnowledge.subtitle') }}</p>
      </div>
      <button class="btn btn-primary" @click="openCreate">
        {{ t('adminKnowledge.actions.createArticle') }}
      </button>
    </div>

    <div class="articles-grid">
      <div v-for="article in articles" :key="article.id" class="article-card">
        <div class="article-header">
          <span class="category-badge">{{ categoryLabel(article.category) }}</span>
          <span :class="['visibility-badge', article.show ? 'visible' : 'hidden']">
            {{ article.show ? t('adminKnowledge.visibility.visible') : t('adminKnowledge.visibility.hidden') }}
          </span>
        </div>
        <h3 class="article-title">{{ article.title }}</h3>
        <p class="article-preview">{{ truncate(article.body, 100) }}</p>
        <div class="article-footer">
          <span class="article-date">{{ formatArticleDate(article.updated_at) }}</span>
          <div class="action-buttons">
            <button
              class="btn btn-sm"
              :title="t('common.actions.edit')"
              :aria-label="t('common.actions.edit')"
              @click="editArticle(article)"
            >
              {{ t('common.actions.edit') }}
            </button>
            <button
              class="btn btn-sm btn-danger"
              :title="t('common.actions.delete')"
              :aria-label="t('common.actions.delete')"
              @click="removeArticle(article)"
            >
              {{ t('common.actions.delete') }}
            </button>
          </div>
        </div>
      </div>
      <div v-if="articles.length === 0" class="empty-card">
        {{ t('adminKnowledge.empty.noData') }}
      </div>
    </div>

    <UiDialog v-model:open="showModal" size="lg" :title="isEdit ? t('adminKnowledge.modal.editTitle') : t('adminKnowledge.modal.createTitle')">
      <div class="dialog-fields">
        <div class="form-row">
          <div class="form-group flex-2">
            <label for="knowledge-title">{{ t('adminKnowledge.fields.title') }} <span class="required">*</span></label>
            <input id="knowledge-title" v-model="form.title" type="text" data-test="knowledge-title" :placeholder="t('adminKnowledge.placeholders.title')" :aria-invalid="formError && !form.title.trim() ? 'true' : undefined" />
          </div>
          <div class="form-group">
            <label for="knowledge-category">{{ t('adminKnowledge.fields.category') }}</label>
            <select id="knowledge-category" v-model="form.category">
              <option v-for="category in categoryOptions" :key="category.value" :value="category.value">
                {{ category.label }}
              </option>
            </select>
          </div>
        </div>
        <div class="form-group">
          <label for="knowledge-body">{{ t('adminKnowledge.fields.content') }} <span class="required">*</span></label>
          <textarea
            id="knowledge-body"
            v-model="form.body"
            rows="12"
            data-test="knowledge-body"
            :placeholder="t('adminKnowledge.placeholders.body')"
            :aria-invalid="formError && !form.body.trim() ? 'true' : undefined"
          ></textarea>
        </div>
        <div class="form-row">
          <div class="form-group">
            <label for="knowledge-sort">{{ t('adminKnowledge.fields.sort') }}</label>
            <input id="knowledge-sort" v-model.number="form.sort" type="number" min="0" />
          </div>
          <div class="form-group">
            <label for="knowledge-show">{{ t('adminKnowledge.fields.visibility') }}</label>
            <select id="knowledge-show" v-model="form.show">
              <option :value="1">{{ t('adminKnowledge.visibility.visible') }}</option>
              <option :value="0">{{ t('adminKnowledge.visibility.hidden') }}</option>
            </select>
          </div>
        </div>
        <p v-if="formError" class="form-error" role="alert" data-test="knowledge-error">{{ formError }}</p>
      </div>
      <template #footer="{ close }">
        <UiButton :disabled="saving" @click="close">{{ t('common.actions.cancel') }}</UiButton>
        <UiButton variant="primary" data-test="knowledge-save" :loading="saving" @click="saveArticle">{{ isEdit ? t('common.actions.save') : t('adminKnowledge.actions.publish') }}</UiButton>
      </template>
    </UiDialog>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import adminApi from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'
import { UiButton, UiDialog, useConfirm, useToast } from '@/ui'

const { t, formatDate } = useAppI18n()
const toast = useToast()
const confirm = useConfirm()
const saving = ref(false)
const formError = ref('')

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

const load = async () => {
  try {
    const res = await adminApi.getKnowledgeList()
    articles.value = (res.data || []).map((article) => ({
      ...article,
      category: normalizeCategory(article.category)
    }))
  } catch (error) {
    console.error(t('adminKnowledge.messages.fetchFailed'), error)
  }
}

onMounted(() => {
  load()
})

const formatArticleDate = (ts) => {
  if (!ts) return '-'
  return formatDate(ts)
}

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

const truncate = (text, length) => {
  if (!text) return ''
  return text.length > length ? `${text.substring(0, length)}...` : text
}

const resetForm = () => {
  form.id = null
  form.title = ''
  form.category = defaultCategory()
  form.body = ''
  form.sort = 0
  form.show = 1
  formError.value = ''
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
</script>

<style scoped>
.articles-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 16px;
}

.article-card {
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
  padding: 20px;
  transition: var(--transition);
  box-shadow: var(--shadow-sm);
}

.article-card:hover {
  border-color: var(--border-strong);
  box-shadow: var(--shadow-md);
}

.article-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
}

.category-badge {
  background: var(--primary-soft);
  color: var(--primary-color);
  padding: 4px 12px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 700;
}

.visibility-badge {
  font-size: 12px;
  padding: 3px 10px;
  border-radius: 999px;
  font-weight: 700;
}

.visibility-badge.visible {
  background: rgba(22, 163, 74, 0.08);
  color: var(--success-color);
}

.visibility-badge.hidden {
  background: var(--surface-muted);
  color: var(--text-secondary);
}

.article-title {
  font-size: 16px;
  font-weight: 600;
  margin-bottom: 8px;
}

.article-preview {
  font-size: 14px;
  color: var(--text-secondary);
  line-height: 1.5;
  margin-bottom: 16px;
  min-height: 42px;
}

.article-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding-top: 12px;
  border-top: 1px solid var(--border-color);
}

.article-date {
  font-size: 12px;
  color: var(--text-secondary);
}

.action-buttons {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}

.empty-card {
  grid-column: 1 / -1;
  text-align: center;
  color: var(--text-secondary);
  padding: 60px 20px;
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
}

.form-row .form-group.flex-2 {
  flex: 2;
}

.dialog-fields {
  display: grid;
  gap: var(--space-4);
}

.dialog-fields .form-group,
.dialog-fields .form-row {
  margin-bottom: 0;
}

.form-error {
  margin: 0;
  color: var(--danger);
}

textarea {
  resize: vertical;
  font-family: 'Consolas', 'Monaco', monospace;
}
</style>

