<template>
  <div class="plans-page">
    <div class="page-header">
      <h1>套餐管理</h1>
      <p class="subtitle">创建、编辑、删除套餐并分配给用户</p>
    </div>

    <div class="section-header">
      <button class="btn btn-primary" @click="showCreate = true">新建套餐</button>
    </div>

    <div class="plans-table">
      <table>
        <thead>
          <tr>
            <th>ID</th>
            <th>名称</th>
            <th>流量(GB)</th>
            <th>月价(分)</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="p in plans" :key="p.id">
            <td>{{ p.id }}</td>
            <td>{{ p.name }}</td>
            <td>{{ p.transfer_enable }}</td>
            <td>{{ p.month_price || '-' }}</td>
            <td>
              <button class="btn btn-sm" @click="edit(p)">编辑</button>
              <button class="btn btn-sm" @click="remove(p)">删除</button>
              <button class="btn btn-sm" @click="openAssign(p)">分配</button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Create/Edit Modal -->
    <div class="modal" v-if="showCreate || showEdit" @click.self="closeModal">
      <div class="modal-content modal-md">
        <div class="modal-header">
          <h3>{{ showEdit ? '编辑套餐' : '新建套餐' }}</h3>
          <button class="close-btn" @click="closeModal">&times;</button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label>名称</label>
            <input v-model="form.name" type="text">
          </div>
          <div class="form-group">
            <label>流量(GB)</label>
            <input v-model.number="form.transfer_enable" type="number">
          </div>
          <div class="form-group">
            <label>月价(分)</label>
            <input v-model.number="form.month_price" type="number">
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn" @click="closeModal">取消</button>
          <button class="btn btn-primary" @click="save">保存</button>
        </div>
      </div>
    </div>

    <!-- Assign Modal -->
    <div class="modal" v-if="showAssign" @click.self="closeAssign">
      <div class="modal-content modal-md">
        <div class="modal-header">
          <h3>将套餐分配给用户</h3>
          <button class="close-btn" @click="closeAssign">&times;</button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label>用户 ID</label>
            <input v-model.number="assignForm.user_id" type="number">
          </div>
          <div class="form-group">
            <label>过期时间 (Unix 秒，留空表示不变)</label>
            <input v-model.number="assignForm.expire_at" type="number">
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn" @click="closeAssign">取消</button>
          <button class="btn btn-primary" @click="assign">分配</button>
        </div>
      </div>
    </div>

  </div>
</template>

<script>
import { ref, reactive, onMounted } from 'vue'
import adminApi from '@/api/admin'

export default {
  name: 'AdminPlans',
  setup() {
    const plans = ref([])
    const showCreate = ref(false)
    const showEdit = ref(false)
    const showAssign = ref(false)
    const form = reactive({ id: null, name: '', transfer_enable: 0, month_price: null })
    const assignForm = reactive({ user_id: null, expire_at: null, plan_id: null })

    const loading = ref(false)
    const error = ref('')

    const load = async () => {
      loading.value = true
      error.value = ''
      try {
        const res = await adminApi.getPlans()
        plans.value = res.data || []
      } catch (e) {
        error.value = e.message || '加载失败'
        alert(error.value)
      } finally {
        loading.value = false
      }
    }

    onMounted(() => { load() })

    const edit = (p) => {
      form.id = p.id
      form.name = p.name
      form.transfer_enable = p.transfer_enable
      form.month_price = p.month_price
      showEdit.value = true
    }

    const remove = async (p) => {
      if (!confirm('确认删除该套餐？')) return
      try {
        await adminApi.deletePlan(p.id)
        await load()
      } catch (e) {
        alert(e.message || '删除失败')
      }
    }

    const closeModal = () => { showCreate.value = false; showEdit.value = false }
    const save = async () => {
      if (!form.name || form.name.trim() === '') { alert('请输入名称'); return }
      const payload = { name: form.name.trim(), transfer_enable: form.transfer_enable, month_price: form.month_price }
      try {
        if (showEdit.value) {
          await adminApi.updatePlan(form.id, payload)
        } else {
          await adminApi.createPlan(payload)
        }
        closeModal(); await load()
      } catch (e) {
        alert(e.message || '保存失败')
      }
    }

    const openAssign = (p) => { assignForm.plan_id = p.id; assignForm.user_id = null; assignForm.expire_at = null; showAssign.value = true }
    const closeAssign = () => { showAssign.value = false }
    const assign = async () => {
      if (!assignForm.user_id) { alert('请输入用户ID'); return }
      try {
        await adminApi.assignPlanToUser(assignForm.plan_id, { user_id: assignForm.user_id, expire_at: assignForm.expire_at })
        closeAssign(); alert('分配成功')
      } catch (e) {
        alert(e.message || '分配失败')
      }
    }

    return { plans, showCreate, showEdit, form, save, edit, remove, showAssign, assignForm, openAssign, closeAssign, assign, loading, error }
  }
}
</script>

<style scoped>
.plans-table table { width: 100%; border-collapse: collapse }
.plans-table th, .plans-table td { padding: 8px; border-bottom: 1px solid #eee }
.modal { position: fixed; inset:0; display:flex; align-items:center; justify-content:center; background: rgba(0,0,0,0.4) }
.modal-content { background:#fff; padding:16px; border-radius:6px; width:600px }
.modal-md { width:600px }
.modal-lg { width:900px }
.modal-header { display:flex; justify-content:space-between; align-items:center }
/* loading state */
.loading { padding: 16px; text-align: center }
</style>
