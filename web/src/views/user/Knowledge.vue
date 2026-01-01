<template>
  <div class="knowledge-page">
    <div class="page-header">
      <h1>📚 使用教程</h1>
      <p class="text-secondary">获取最新的使用说明、公告和常见问题解答</p>
    </div>

    <!-- 分类筛选 -->
    <div class="category-tabs">
      <button 
        v-for="cat in categories" 
        :key="cat"
        :class="['tab-item', { active: currentCategory === cat }]"
        @click="currentCategory = cat"
      >
        {{ cat }}
      </button>
    </div>

    <!-- 文章列表 -->
    <div class="content-container">
      <div v-if="loading" class="loading-state">
        <div class="spinner"></div>
        <p>加载中...</p>
      </div>

      <div v-else-if="filteredArticles.length === 0" class="empty-state">
        <div class="empty-icon">📭</div>
        <p>暂无相关教程</p>
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
            <span class="read-more">阅读全文 →</span>
          </div>
        </div>
      </div>
    </div>

    <!-- 文章详情弹窗 -->
    <div v-if="showDetail" class="modal-overlay" @click.self="showDetail = false">
      <div class="modal modal-lg article-detail-modal">
        <div class="modal-header">
          <div class="header-info">
            <span class="category-tag">{{ detailArticle.category }}</span>
            <span class="date text-secondary">{{ formatDate(detailArticle.updated_at) }} 更新</span>
          </div>
          <h3>{{ detailArticle.title }}</h3>
          <button class="close-btn" @click="showDetail = false">✕</button>
        </div>
        <div class="modal-body markdown-body">
          <pre style="white-space: pre-wrap; font-family: inherit;">{{ detailArticle.body }}</pre>
        </div>
        <div class="modal-footer">
          <button class="btn-primary" @click="showDetail = false">我知道了</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { getKnowledgeList } from '@/api/user'

const articles = ref([])
const loading = ref(true)
const currentCategory = ref('全部')
const showDetail = ref(false)
const detailArticle = ref({})

const categories = computed(() => {
  const cats = ['全部']
  articles.value.forEach(a => {
    if (!cats.includes(a.category)) {
      cats.push(a.category)
    }
  })
  return cats
})

const filteredArticles = computed(() => {
  if (currentCategory.value === '全部') return articles.value
  return articles.value.filter(a => a.category === currentCategory.value)
})

const fetchArticles = async () => {
  loading.value = true
  try {
    const res = await getKnowledgeList()
    articles.value = res.data || []
  } catch (err) {
    console.error('获取知识库失败:', err)
  } finally {
    loading.value = false
  }
}

const formatDate = (ts) => {
  if (!ts) return '-'
  return new Date(ts * 1000).toLocaleDateString('zh-CN')
}

const truncate = (text, length) => {
  if (!text) return ''
  return text.length > length ? text.substring(0, length) + '...' : text
}

const viewDetail = (article) => {
  detailArticle.value = article
  showDetail.value = true
}

onMounted(() => {
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

/* 详情弹窗增强 */
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

/* 状态样式 */
.loading-state, .empty-state {
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
  to { transform: rotate(360deg); }
}

.empty-icon {
  font-size: 48px;
  margin-bottom: 16px;
}
</style>
