<template>
  <div class="mfa-page">
    <div class="page-header">
      <h1>多因素认证管理</h1>
      <p class="text-secondary">配置全局 MFA 策略</p>
    </div>

    <div class="config-section">
      <h3>全局配置</h3>

      <div class="form-group">
        <label class="checkbox-label">
          <input type="checkbox" v-model="config.enabled" />
          <span>启用多因素认证</span>
        </label>
        <p class="help-text">启用后，用户可选择开启 MFA 保护账户安全</p>
      </div>

      <div class="form-group">
        <label class="checkbox-label">
          <input type="checkbox" v-model="config.required" />
          <span>强制启用 MFA</span>
        </label>
        <p class="help-text">强制所有用户启用 MFA，否则无法使用服务</p>
      </div>

      <div class="form-group">
        <label>支持的认证方式</label>
        <div class="checkbox-group">
          <label class="checkbox-label">
            <input type="checkbox" v-model="config.methods.totp" />
            <span>TOPT (Google Authenticator / Authy)</span>
          </label>
          <label class="checkbox-label">
            <input type="checkbox" v-model="config.methods.sms" />
            <span>短信验证码</span>
          </label>
          <label class="checkbox-label">
            <input type="checkbox" v-model="config.methods.email" />
            <span>邮箱验证码</span>
          </label>
        </div>
      </div>

      <div class="form-row">
        <div class="form-group">
          <label>备用码数量</label>
          <input v-model.number="config.backup_codes_count" type="number" min="1" max="20" />
          <p class="help-text">用户启用 MFA 时生成的备用码数量</p>
        </div>
        <div class="form-group">
          <label>登录尝试限制</label>
          <input v-model.number="config.max_attempts" type="number" min="1" max="10" />
          <p class="help-text">超过限制将临时锁定账户</p>
        </div>
      </div>

      <div class="form-group">
        <label>锁定时长 (分钟)</label>
        <input v-model.number="config.lockout_duration" type="number" min="1" />
        <p class="help-text">登录失败超过限制后的锁定时间</p>
      </div>

      <div class="form-actions">
        <button class="btn-primary" @click="saveConfig">保存配置</button>
      </div>
    </div>

    <div class="info-section">
      <h3>💡 使用说明</h3>
      <div class="info-grid">
        <div class="info-item">
          <h4>TOPT 认证</h4>
          <p>基于时间的一次性密码，用户可使用 Google Authenticator、Authy 等应用扫描二维码绑定。</p>
        </div>
        <div class="info-item">
          <h4>备用码</h4>
          <p>当用户无法使用认证器时，可使用备用码登录。每个备用码只能使用一次。</p>
        </div>
        <div class="info-item">
          <h4>账户锁定</h4>
          <p>连续多次 MFA 验证失败将触发账户锁定，防止暴力破解。</p>
        </div>
        <div class="info-item">
          <h4>用户端操作</h4>
          <p>用户可在「安全设置」页面自行管理 MFA，包括启用、禁用、重新生成备用码。</p>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { getMFAConfig, updateMFAConfig } from '@/api/admin'

const config = ref({
  enabled: false,
  required: false,
  methods: {
    totp: true,
    sms: false,
    email: true
  },
  backup_codes_count: 10,
  max_attempts: 5,
  lockout_duration: 15
})

const fetchConfig = async () => {
  try {
    const res = await getMFAConfig()
    if (res.data) {
      config.value = {
        ...config.value,
        ...res.data,
        methods: { ...config.value.methods, ...res.data.methods }
      }
    }
  } catch (err) {
    console.error('获取配置失败:', err)
  }
}

const saveConfig = async () => {
  try {
    await updateMFAConfig(config.value)
    alert('保存成功')
  } catch (err) {
    alert('保存失败: ' + (err.response?.data?.error || err.message))
  }
}

onMounted(() => {
  fetchConfig()
})
</script>

<style scoped>
.config-section, .info-section {
  background: var(--surface-color);
  padding: 24px;
  border-radius: var(--radius-lg);
  border: 1px solid var(--border-color);
  margin-bottom: 20px;
}

.config-section h3, .info-section h3 {
  margin-bottom: 16px;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--border-color);
}

.checkbox-label {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
}

.checkbox-label input[type="checkbox"] {
  width: 18px;
  height: 18px;
}

.checkbox-group {
  display: flex;
  flex-direction: column;
  gap: 12px;
  margin-top: 8px;
}

.help-text {
  color: var(--text-secondary);
  font-size: 12px;
  margin-top: 4px;
}

.info-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
  gap: 20px;
  margin-top: 16px;
}

.info-item {
  padding: 16px;
  background: var(--background-color);
  border-radius: var(--radius-md);
}

.info-item h4 {
  margin-bottom: 8px;
  color: var(--primary-color);
}

.info-item p {
  color: var(--text-secondary);
  font-size: 13px;
  margin: 0;
}
</style>