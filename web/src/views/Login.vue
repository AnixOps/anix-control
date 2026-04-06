<template>
  <div class="login-page">
    <div class="login-container">
      <div class="login-brand">
        <div class="brand-icon">🚀</div>
        <h1>V2Board</h1>
        <p>高性能代理服务管理面板</p>
      </div>
      
      <div class="login-box">
        <h2>{{ isRegisterMode ? '创建账户' : '欢迎回来' }}</h2>
        <p class="login-subtitle">{{ isRegisterMode ? '请填写以下信息注册' : '请登录您的账户' }}</p>
        
        <form @submit.prevent="isRegisterMode ? handleRegister() : handleLogin()" class="login-form">
          <div class="form-group">
            <label for="email">邮箱</label>
            <input 
              id="email"
              v-model="email" 
              type="email" 
              placeholder="请输入邮箱地址"
              autocomplete="email"
            />
          </div>
          
          <div class="form-group">
            <label for="password">密码</label>
            <input 
              id="password"
              v-model="password" 
              type="password" 
              :placeholder="isRegisterMode ? '请输入密码（至少6位）' : '请输入密码'"
              autocomplete="current-password"
            />
          </div>

          <div class="form-group" v-if="isRegisterMode">
            <label for="confirmPassword">确认密码</label>
            <input 
              id="confirmPassword"
              v-model="confirmPassword" 
              type="password" 
              placeholder="请再次输入密码"
              autocomplete="new-password"
            />
          </div>
          
          <div v-if="errorMsg" class="error-msg">
            <span class="error-icon">⚠️</span>
            {{ errorMsg }}
          </div>

          <div v-if="successMsg" class="success-msg">
            <span class="success-icon">✅</span>
            {{ successMsg }}
          </div>
          
          <button type="submit" class="login-btn" :disabled="loading">
            <span v-if="loading" class="spinner"></span>
            {{ loading ? (isRegisterMode ? '注册中...' : '登录中...') : (isRegisterMode ? '注册' : '登录') }}
          </button>
        </form>
        
        <div class="login-footer">
          <a href="#" v-if="!isRegisterMode">忘记密码?</a>
          <a href="#" @click.prevent="toggleMode">
            {{ isRegisterMode ? '已有账户？去登录' : '没有账户？去注册' }}
          </a>
        </div>
        
        <!-- 开发模式快速登录 -->
        <div class="dev-actions" v-if="enableMockLogin && !isRegisterMode">
          <div class="dev-divider">
            <span>开发模式</span>
          </div>
          <div class="dev-buttons">
            <button type="button" class="btn-ghost" @click="mockLogin('user')">
              👤 模拟用户
            </button>
            <button type="button" class="btn-ghost" @click="mockLogin('admin')">
              👑 模拟管理员
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import { useUserStore } from '@/stores/user'
import { login, register } from '@/api/auth'

const email = ref('')
const password = ref('')
const confirmPassword = ref('')
const router = useRouter()
const userStore = useUserStore()
const loading = ref(false)
const errorMsg = ref('')
const successMsg = ref('')
const isRegisterMode = ref(false)

// 检测是否为开发环境
const enableMockLogin = computed(() => {
  const flag = String(import.meta.env.VITE_ENABLE_MOCK_LOGIN || '').trim().toLowerCase()
  return flag === 'true' || flag === '1' || flag === 'yes'
})

const toggleMode = () => {
  isRegisterMode.value = !isRegisterMode.value
  errorMsg.value = ''
  successMsg.value = ''
  password.value = ''
  confirmPassword.value = ''
}

const handleRegister = async () => {
  if (!email.value || !password.value) {
    errorMsg.value = '请输入邮箱和密码'
    return
  }
  
  if (password.value.length < 6) {
    errorMsg.value = '密码长度至少6位'
    return
  }

  if (password.value !== confirmPassword.value) {
    errorMsg.value = '两次输入的密码不一致'
    return
  }
  
  loading.value = true
  errorMsg.value = ''
  successMsg.value = ''
  
  try {
    const res = await register({
      email: email.value,
      password: password.value
    })
    
    if (res.data?.token) {
      const { token, is_admin, user_id, email: userEmail } = res.data
      userStore.login(token, {
        id: user_id,
        email: userEmail,
        is_admin: is_admin
      })
      
      successMsg.value = '注册成功！正在跳转...'
      setTimeout(() => {
        if (is_admin) {
          router.push('/admin/dashboard')
        } else {
          router.push('/user/dashboard')
        }
      }, 1000)
    } else {
      throw new Error('register response missing token')
    }
  } catch (err) {
    errorMsg.value = err.response?.data?.message || '注册失败，请稍后重试'
  } finally {
    loading.value = false
  }
}

const handleLogin = async () => {
  if (!email.value || !password.value) {
    errorMsg.value = '请输入邮箱和密码'
    return
  }
  
  loading.value = true
  errorMsg.value = ''
  
  try {
    const res = await login({
      email: email.value,
      password: password.value
    })
    
    if (res.data?.token) {
      const { token, is_admin, user_id, email: userEmail } = res.data
      userStore.login(token, {
        id: user_id,
        email: userEmail,
        is_admin: is_admin
      })
      
      if (is_admin) {
        router.push('/admin/dashboard')
      } else {
        router.push('/user/dashboard')
      }
    } else {
      throw new Error('login response missing token')
    }
  } catch (err) {
    errorMsg.value = err.response?.data?.message || '登录失败，请检查邮箱和密码'
  } finally {
    loading.value = false
  }
}

const mockLogin = (role) => {
  if (!enableMockLogin.value) {
    return
  }
  const mockUser = {
    id: 1,
    email: role === 'admin' ? 'admin@example.com' : 'user@example.com',
    is_admin: role === 'admin'
  }
  userStore.login('mock-token-' + role, mockUser)
  
  if (role === 'admin') {
    router.push('/admin/dashboard')
  } else {
    router.push('/user/dashboard')
  }
}
</script>

<style scoped>
.login-page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 20px;
  background: linear-gradient(135deg, #0a0a0a 0%, #1a1a2e 100%);
}

.login-container {
  width: 100%;
  max-width: 420px;
}

.login-brand {
  text-align: center;
  margin-bottom: 32px;
}

.brand-icon {
  font-size: 48px;
  margin-bottom: 16px;
}

.login-brand h1 {
  font-size: 28px;
  font-weight: 700;
  margin-bottom: 8px;
  background: linear-gradient(135deg, #3b82f6, #8b5cf6);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

.login-brand p {
  color: var(--text-secondary);
  font-size: 14px;
}

.login-box {
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
  padding: 32px;
  box-shadow: var(--shadow-lg);
}

.login-box h2 {
  font-size: 24px;
  font-weight: 600;
  margin-bottom: 8px;
}

.login-subtitle {
  color: var(--text-secondary);
  margin-bottom: 24px;
  font-size: 14px;
}

.login-form {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.form-group {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.form-group label {
  font-size: 14px;
  font-weight: 500;
  color: var(--text-color);
}

.error-msg {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 16px;
  background: rgba(239, 68, 68, 0.1);
  border: 1px solid rgba(239, 68, 68, 0.3);
  border-radius: var(--radius-md);
  color: var(--error-color);
  font-size: 14px;
}

.success-msg {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 16px;
  background: rgba(34, 197, 94, 0.1);
  border: 1px solid rgba(34, 197, 94, 0.3);
  border-radius: var(--radius-md);
  color: var(--success-color);
  font-size: 14px;
}

.login-btn {
  width: 100%;
  padding: 14px 24px;
  font-size: 16px;
  font-weight: 600;
  margin-top: 8px;
}

.spinner {
  width: 16px;
  height: 16px;
  border: 2px solid rgba(255, 255, 255, 0.3);
  border-top-color: white;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.login-footer {
  text-align: center;
  margin-top: 20px;
  display: flex;
  justify-content: center;
  gap: 16px;
  flex-wrap: wrap;
}

.login-footer a {
  color: var(--primary-color);
  text-decoration: none;
  font-size: 14px;
}

.login-footer a:hover {
  text-decoration: underline;
}

.dev-actions {
  margin-top: 24px;
}

.dev-divider {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-bottom: 16px;
}

.dev-divider::before,
.dev-divider::after {
  content: '';
  flex: 1;
  height: 1px;
  background: var(--border-color);
}

.dev-divider span {
  font-size: 12px;
  color: var(--text-secondary);
  text-transform: uppercase;
  letter-spacing: 1px;
}

.dev-buttons {
  display: flex;
  gap: 12px;
}

.dev-buttons .btn-ghost {
  flex: 1;
  padding: 10px 16px;
  font-size: 13px;
}

/* 移动端适配 */
@media (max-width: 480px) {
  .login-page {
    padding: 16px;
    align-items: flex-start;
    padding-top: 60px;
  }
  
  .login-box {
    padding: 24px;
  }
  
  .login-brand h1 {
    font-size: 24px;
  }
  
  .dev-buttons {
    flex-direction: column;
  }
}

/* 平板适配 */
@media (min-width: 768px) {
  .login-container {
    max-width: 440px;
  }
  
  .login-box {
    padding: 40px;
  }
}
</style>
