<template>
  <div class="knowledge-page">
    <div class="page-header">
      <h1>{{ t('user.knowledge.title') }}</h1>
      <p class="text-secondary">{{ t('user.knowledge.subtitle') }}</p>
    </div>

    <div class="category-tabs">
      <button
        v-for="category in categories"
        :key="category"
        :class="['tab-item', { active: currentCategory === category }]"
        @click="currentCategory = category"
      >
        {{ category }}
      </button>
    </div>

    <div class="content-container">
      <div v-if="loading" class="loading-state">
        <div class="spinner"></div>
        <p>{{ t('user.knowledge.loading') }}</p>
      </div>

      <div v-else-if="filteredArticles.length === 0" class="empty-state">
        <div class="empty-icon">K</div>
        <p>{{ t('user.knowledge.empty') }}</p>
      </div>

      <div v-else class="articles-grid">
        <div
          v-for="article in filteredArticles"
          :key="article.id"
          class="article-card"
          @click="viewDetail(article)"
        >
          <div class="article-meta">
            <span class="category-tag">{{ article.category }}</span>
            <span class="date">{{ formatDate(article.updated_at) }}</span>
          </div>
          <h3 class="article-title">{{ article.title }}</h3>
          <p class="article-excerpt">{{ truncate(article.body, 120) }}</p>
          <div class="article-footer">
            <span class="read-more">{{ t('user.knowledge.readMore') }}</span>
          </div>
        </div>
      </div>
    </div>

    <div v-if="showDetail" class="modal-overlay" @click.self="showDetail = false">
      <div class="modal modal-lg article-detail-modal">
        <div class="modal-header">
          <div class="header-info">
            <span class="category-tag">{{ detailArticle.category }}</span>
            <span class="date text-secondary">{{ formatDate(detailArticle.updated_at) }} {{ t('user.knowledge.updated') }}</span>
          </div>
          <h3>{{ detailArticle.title }}</h3>
          <button class="close-btn" @click="showDetail = false">×</button>
        </div>
        <div class="modal-body markdown-body">
          <pre style="white-space: pre-wrap; font-family: inherit;">{{ detailArticle.body }}</pre>
        </div>
        <div class="modal-footer">
          <button class="btn-primary" @click="showDetail = false">{{ t('user.knowledge.closeAction') }}</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { getKnowledgeList } from '@/api/user'
import { useAppI18n } from '@/composables/useAppI18n'

const { t, formatDate } = useAppI18n()
const articles = ref([])
const loading = ref(true)
const currentCategory = ref('')
const showDetail = ref(false)
const detailArticle = ref({})

const allCategoryLabel = computed(() => t('user.knowledge.all'))

const categories = computed(() => {
  const result = [allCategoryLabel.value]
  for (const article of articles.value) {
    if (article.category && !result.includes(article.category)) {
      result.push(article.category)
    }
  }
  return result
})

const filteredArticles = computed(() => {
  if (currentCategory.value === allCategoryLabel.value) {
    return articles.value
  }
  return articles.value.filter((article) => article.category === currentCategory.value)
})

async function fetchArticles() {
  loading.value = true
  try {
    const res = await getKnowledgeList()
    articles.value = res.data || []
    if (!currentCategory.value) {
      currentCategory.value = allCategoryLabel.value
    }
  } catch (err) {
    console.error('Failed to load knowledge articles:', err)
  } finally {
    loading.value = false
  }
}

function truncate(text, length) {
  if (!text) return ''
  return text.length > length ? `${text.slice(0, length)}...` : text
}

function viewDetail(article) {
  detailArticle.value = article
  showDetail.value = true
}

onMounted(() => {
  currentCategory.value = allCategoryLabel.value
  fetchArticles()
})
</script>

<style scoped>
.knowledge-page {
  max-width: 1200px;
  margin: 0 auto;
}

.page-header {
  margin-bottom: 32px;
}

.page-header h1 {
  font-size: 28px;
  margin-bottom: 8px;
}

.category-tabs {
  display: flex;
  gap: 12px;
  margin-bottom: 24px;
  padding-bottom: 12px;
  overflow-x: auto;
  scrollbar-width: none;
}

.category-tabs::-webkit-scrollbar {
  display: none;
}

.tab-item {
  padding: 8px 20px;
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  border-radius: 20px;
  font-size: 14px;
  color: var(--text-secondary);
  cursor: pointer;
  white-space: nowrap;
  transition: var(--transition);
}

.tab-item:hover {
  border-color: var(--primary-color);
  color: var(--text-color);
}

.tab-item.active {
  background: var(--primary-color);
  border-color: var(--primary-color);
  color: white;
}

.content-container {
  min-height: 400px;
}

.articles-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(340px, 1fr));
  gap: 20px;
}

@media (max-width: 640px) {
  .articles-grid {
    grid-template-columns: 1fr;
  }
}

.article-card {
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
  padding: 24px;
  cursor: pointer;
  transition: var(--transition);
  display: flex;
  flex-direction: column;
}

.article-card:hover {
  transform: translateY(-4px);
  border-color: var(--primary-color);
  box-shadow: var(--shadow-lg);
}

.article-meta {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
}

.category-tag {
  font-size: 11px;
  font-weight: 600;
  padding: 2px 8px;
  background: rgba(59, 130, 246, 0.1);
  color: var(--primary-color);
  border-radius: 4px;
  text-transform: uppercase;
}

.date {
  font-size: 12px;
  color: var(--text-secondary);
}

.article-title {
  font-size: 18px;
  font-weight: 600;
  margin-bottom: 12px;
  line-height: 1.4;
}

.article-excerpt {
  font-size: 14px;
  color: var(--text-secondary);
  line-height: 1.6;
  margin-bottom: 20px;
  flex: 1;
}

.article-footer {
  margin-top: auto;
}

.read-more {
  font-size: 14px;
  font-weight: 500;
  color: var(--primary-color);
}

.article-detail-modal .modal-header {
  display: block;
}

.header-info {
  display: flex;
  gap: 12px;
  align-items: center;
  margin-bottom: 8px;
}

.article-detail-modal h3 {
  font-size: 24px;
  margin-top: 4px;
}

.markdown-body {
  font-size: 15px;
  line-height: 1.8;
  color: var(--text-color);
}

.loading-state,
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 80px 20px;
  color: var(--text-secondary);
}

.spinner {
  width: 40px;
  height: 40px;
  border: 3px solid rgba(59, 130, 246, 0.1);
  border-top-color: var(--primary-color);
  border-radius: 50%;
  animation: spin 1s linear infinite;
  margin-bottom: 16px;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

.empty-icon {
  font-size: 48px;
  margin-bottom: 16px;
}
</style>
