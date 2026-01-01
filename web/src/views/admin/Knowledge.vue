<template>
  <div class="knowledge-page">
    <div class="page-header">
      <h1>知识库管理</h1>
      <p class="text-secondary">管理使用教程和公告</p>
    </div>

    <!-- 筛选栏 -->
    <div class="filter-bar">
      <button @click="openCreate">➕ 新建文章</button>
    </div>

    <!-- 文章列表 -->
    <div class="articles-grid">
      <div class="article-card" v-for="a in articles" :key="a.id">
        <div class="article-header">
          <span class="category-badge">{{ a.category }}</span>
          <span :class="['visibility-badge', a.show ? 'visible' : 'hidden']">
            {{ a.show ? '可见' : '隐藏' }}
          </span>
        </div>
        <h3 class="article-title">{{ a.title }}</h3>
        <p class="article-preview">{{ truncate(a.body, 100) }}</p>
        <div class="article-footer">
          <span class="article-date">{{ formatDate(a.updated_at) }}</span>
          <div class="action-buttons">
            <button class="btn-sm btn-ghost" @click="edit(a)" title="编辑">✏️</button>
            <button class="btn-sm btn-ghost" @click="remove(a)" title="删除">🗑️</button>
          </div>
        </div>
      </div>
      <div v-if="articles.length === 0" class="empty-card">
        暂无文章
      </div>
    </div>

    <!-- 创建/编辑弹窗 -->
    <div v-if="showModal" class="modal-overlay" @click.self="closeModal">
      <div class="modal modal-lg">
        <div class="modal-header">
          <h3>{{ isEdit ? '编辑文章' : '新建文章' }}</h3>
          <button class="close-btn" @click="closeModal">✕</button>
        </div>
        <div class="modal-body">
          <div class="form-row">
            <div class="form-group flex-2">
              <label>标题 <span class="required">*</span></label>
              <input v-model="form.title" type="text" placeholder="文章标题">
            </div>
            <div class="form-group">
              <label>分类</label>
              <select v-model="form.category">
                <option value="公告">公告</option>
                <option value="教程">教程</option>
                <option value="常见问题">常见问题</option>
                <option value="其他">其他</option>
              </select>
            </div>
          </div>
          <div class="form-group">
            <label>内容 <span class="required">*</span></label>
            <textarea v-model="form.body" rows="12" placeholder="支持 Markdown 格式..."></textarea>
          </div>
          <div class="form-row">
            <div class="form-group">
              <label>排序（数字越小越靠前）</label>
              <input v-model.number="form.sort" type="number" min="0">
            </div>
            <div class="form-group">
              <label>是否显示</label>
              <select v-model="form.show">
                <option :value="1">显示</option>
                <option :value="0">隐藏</option>
              </select>
            </div>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn-secondary" @click="closeModal">取消</button>
          <button @click="save">{{ isEdit ? '保存' : '发布' }}</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import adminApi from '@/api/admin'

const articles = ref([])
const showModal = ref(false)
const isEdit = ref(false)
const form = reactive({
  id: null,
  title: '',
  category: '公告',
  body: '',
  sort: 0,
  show: 1
})

const load = async () => {
  try {
    const res = await adminApi.getKnowledgeList()
    articles.value = res.data || []
  } catch (e) {
    console.error('加载失败:', e)
  }
}

onMounted(() => { load() })

const formatDate = (ts) => {
  if (!ts) return '-'
  return new Date(ts * 1000).toLocaleDateString('zh-CN')
}

const truncate = (text, length) => {
  if (!text) return ''
  return text.length > length ? text.substring(0, length) + '...' : text
}

const resetForm = () => {
  form.id = null
  form.title = ''
  form.category = '公告'
  form.body = ''
  form.sort = 0
  form.show = 1
}

const openCreate = () => {
  resetForm()
  isEdit.value = false
  showModal.value = true
}

const edit = (a) => {
  form.id = a.id
  form.title = a.title
  form.category = a.category
  form.body = a.body
  form.sort = a.sort
  form.show = a.show
  isEdit.value = true
  showModal.value = true
}

const closeModal = () => {
  showModal.value = false
}

const save = async () => {
  if (!form.title.trim() || !form.body.trim()) {
    alert('请填写标题和内容')
    return
  }
  try {
    const payload = {
      title: form.title.trim(),
      category: form.category,
      body: form.body,
      sort: form.sort,
      show: form.show
    }
    if (isEdit.value) {
      await adminApi.updateKnowledge(form.id, payload)
      alert('保存成功')
    } else {
      await adminApi.createKnowledge(payload)
      alert('发布成功')
    }
    closeModal()
    await load()
  } catch (e) {
    alert(e.message || '操作失败')
  }
}

const remove = async (a) => {
  if (!confirm(`确定删除文章 "${a.title}"？`)) return
  try {
    await adminApi.deleteKnowledge(a.id)
    await load()
  } catch (e) {
    alert(e.message || '删除失败')
  }
}
</script>

<style scoped>
.knowledge-page {
  max-width: 1400px;
}

.page-header {
  margin-bottom: 24px;
}

.page-header h1 {
  font-size: 24px;
  margin-bottom: 4px;
}

.filter-bar {
  display: flex;
  gap: 12px;
  margin-bottom: 16px;
  flex-wrap: wrap;
}

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
}

.article-card:hover {
  border-color: var(--text-secondary);
}

.article-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
}

.category-badge {
  background: var(--primary-color);
  color: white;
  padding: 4px 12px;
  border-radius: 20px;
  font-size: 12px;
  font-weight: 500;
}

.visibility-badge {
  font-size: 11px;
  padding: 3px 8px;
  border-radius: 10px;
}

.visibility-badge.visible {
  background: rgba(34, 197, 94, 0.15);
  color: var(--success-color);
}

.visibility-badge.hidden {
  background: rgba(161, 161, 170, 0.15);
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
  gap: 4px;
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

/* 弹窗样式 */
.modal-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.7);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  padding: 20px;
}

.modal {
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
  width: 100%;
  max-width: 500px;
  max-height: 90vh;
  overflow-y: auto;
}

.modal.modal-lg {
  max-width: 700px;
}

.modal-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 20px;
  border-bottom: 1px solid var(--border-color);
}

.modal-header h3 {
  font-size: 18px;
  font-weight: 600;
}

.close-btn {
  background: transparent;
  border: none;
  font-size: 18px;
  color: var(--text-secondary);
  cursor: pointer;
  padding: 4px;
}

.modal-body {
  padding: 20px;
}

.form-row {
  display: flex;
  gap: 16px;
}

@media (max-width: 640px) {
  .form-row {
    flex-direction: column;
    gap: 0;
  }
}

.form-row .form-group {
  flex: 1;
}

.form-row .form-group.flex-2 {
  flex: 2;
}

.modal-body .form-group {
  margin-bottom: 16px;
}

.modal-body label {
  display: block;
  margin-bottom: 6px;
  font-size: 14px;
  font-weight: 500;
}

.modal-body .required {
  color: var(--error-color);
}

.modal-body textarea {
  resize: vertical;
  font-family: 'Consolas', 'Monaco', monospace;
}

.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  padding: 16px 20px;
  border-top: 1px solid var(--border-color);
}

.modal-footer button {
  min-width: 80px;
}
</style>
