import { Hono } from 'hono'
import type { ApiErrorResponse, Env } from './types'
import { createApp } from './app/create-app'

// 创建应用 (所有路由在 src/app/register-*.ts 中注册)
const app = createApp(new Hono<{ Bindings: Env }>())

// ==================== 错误处理 ====================

app.notFound((c) => {
  return c.json({ success: false, error: 'Not Found' } as ApiErrorResponse, 404)
})

app.onError((err, c) => {
  console.error('Error:', err)

  // 开发环境返回详细错误
  if (c.env.ENVIRONMENT === 'development') {
    return c.json({
      success: false,
      error: err.message,
      stack: err.stack,
    }, 500)
  }

  return c.json({ success: false, error: 'Internal Server Error' } as ApiErrorResponse, 500)
})

// Export
export default app
