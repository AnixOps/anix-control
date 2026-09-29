/**
 * Workers AI Service
 * 使用 Cloudflare Workers AI 进行智能分析
 */

import type { Context } from 'hono'
import type { ApiErrorResponse, ApiSuccessResponse, Env } from '../types'

// 支持的模型
export const AI_MODELS = {
  // 文本生成
  textGeneration: '@cf/meta/llama-3.1-8b-instruct',
  textGenerationFast: '@cf/meta/llama-3-8b-instruct',

  // 文本嵌入 (用于向量搜索)
  textEmbeddings: '@cf/baai/bge-base-en-v1.5',

  // 图像分类
  imageClassification: '@cf/microsoft/resnet-50',

  // 目标检测
  objectDetection: '@cf/facebook/detr-resnet-50',

  // 翻译
  translation: '@cf/meta/m2m100-1.2b',

  // 总结
  summarization: '@cf/facebook/bart-large-cnn',
}

type AIResponse = ApiSuccessResponse<unknown> | ApiErrorResponse

/**
 * 文本生成 (LLM 推理)
 */
export async function generateText(
  env: Env,
  prompt: string,
  options?: {
    model?: string
    maxTokens?: number
    temperature?: number
    systemPrompt?: string
  }
): Promise<AIResponse> {
  try {
    const model = options?.model || AI_MODELS.textGeneration

    const messages = []
    if (options?.systemPrompt) {
      messages.push({ role: 'system', content: options.systemPrompt })
    }
    messages.push({ role: 'user', content: prompt })

    const response = await env.AI.run(model as any, {
      messages,
      max_tokens: options?.maxTokens || 512,
      temperature: options?.temperature || 0.7,
    })

    return { success: true, data: response }
  } catch (error) {
    return { success: false, error: String(error) }
  }
}


/**
 * 生成文本嵌入向量 (用于语义搜索)
 */
export async function generateEmbedding(
  env: Env,
  text: string
): Promise<AIResponse> {
  try {
    const response = await env.AI.run(AI_MODELS.textEmbeddings as any, {
      text,
    })

    return { success: true, data: response }
  } catch (error) {
    return { success: false, error: String(error) }
  }
}

/**
 * 智能日志分析
 */
export async function analyzeLog(
  env: Env,
  logContent: string
): Promise<AIResponse> {
  const systemPrompt = `You are an expert DevOps engineer. Analyze the following log content and provide:
1. Error classification (error/warning/info)
2. Root cause analysis
3. Recommended actions
4. Severity level (1-5)

Respond in JSON format.`

  return generateText(env, logContent, {
    model: AI_MODELS.textGenerationFast,
    maxTokens: 256,
    temperature: 0.3,
    systemPrompt,
  })
}

/**
 * 自然语言查询转换为结构化查询
 */
export async function naturalLanguageToQuery(
  env: Env,
  query: string,
  schema: string
): Promise<AIResponse> {
  const systemPrompt = `You are a query translator. Convert natural language queries to structured queries based on the provided schema.
Schema: ${schema}

Respond only with the structured query in JSON format.`

  return generateText(env, query, {
    model: AI_MODELS.textGenerationFast,
    maxTokens: 128,
    temperature: 0.1,
    systemPrompt,
  })
}

/**
 * 运维建议生成
 */
export async function generateOpsAdvice(
  env: Env,
  context: {
    nodeStatus?: string
    taskHistory?: string
    alerts?: string
    metrics?: string
  }
): Promise<AIResponse> {
  const prompt = `Based on the following operational context, provide actionable recommendations:

Node Status: ${context.nodeStatus || 'N/A'}
Task History: ${context.taskHistory || 'N/A'}
Active Alerts: ${context.alerts || 'N/A'}
Current Metrics: ${context.metrics || 'N/A'}

Provide 3-5 specific, actionable recommendations.`

  return generateText(env, prompt, {
    model: AI_MODELS.textGeneration,
    maxTokens: 512,
    temperature: 0.5,
  })
}

/**
 * AI 聊天助手
 */
export async function chatAssistant(
  env: Env,
  message: string,
  conversationHistory: Array<{ role: string; content: string }> = []
): Promise<AIResponse> {
  try {
    const systemPrompt = `You are AnixOps Assistant, an AI-powered DevOps helper. You help users with:
- Infrastructure management
- Task automation
- Troubleshooting
- Best practices
- System monitoring

Be concise, helpful, and technically accurate.`

    const messages = [
      { role: 'system', content: systemPrompt },
      ...conversationHistory,
      { role: 'user', content: message },
    ]

    const response = await env.AI.run(AI_MODELS.textGeneration as any, {
      messages,
      max_tokens: 1024,
      temperature: 0.7,
    })

    return { success: true, data: response }
  } catch (error) {
    return { success: false, error: String(error) }
  }
}

// ==================== HTTP Handlers ====================

/**
 * AI 聊天处理
 */
export async function aiChatHandler(c: Context<{ Bindings: Env }>) {
  const body = await c.req.json<{ message: string; history?: Array<{ role: string; content: string }> }>()

  if (!body.message) {
    return c.json({ success: false, error: 'Message is required' } as ApiErrorResponse, 400)
  }

  const result = await chatAssistant(c.env, body.message, body.history || [])

  if (result.success) {
    return c.json({ success: true, data: result.data } as ApiSuccessResponse<unknown>)
  }

  return c.json({ success: false, error: result.error } as ApiErrorResponse, 500)
}

/**
 * 日志分析处理
 */
export async function aiAnalyzeLogHandler(c: Context<{ Bindings: Env }>) {
  const body = await c.req.json<{ log: string }>()

  if (!body.log) {
    return c.json({ success: false, error: 'Log content is required' } as ApiErrorResponse, 400)
  }

  const result = await analyzeLog(c.env, body.log)

  if (result.success) {
    return c.json({ success: true, data: result.data } as ApiSuccessResponse<unknown>)
  }

  return c.json({ success: false, error: result.error } as ApiErrorResponse, 500)
}

/**
 * 运维建议处理
 */
export async function aiOpsAdviceHandler(c: Context<{ Bindings: Env }>) {
  const body = await c.req.json<{
    nodeStatus?: string
    taskHistory?: string
    alerts?: string
    metrics?: string
  }>()

  const result = await generateOpsAdvice(c.env, body)

  if (result.success) {
    return c.json({ success: true, data: result.data } as ApiSuccessResponse<unknown>)
  }

  return c.json({ success: false, error: result.error } as ApiErrorResponse, 500)
}

/**
 * 文本嵌入处理
 */
export async function aiEmbeddingHandler(c: Context<{ Bindings: Env }>) {
  const body = await c.req.json<{ text: string }>()

  if (!body.text) {
    return c.json({ success: false, error: 'Text is required' } as ApiErrorResponse, 400)
  }

  const result = await generateEmbedding(c.env, body.text)

  if (result.success) {
    return c.json({ success: true, data: result.data } as ApiSuccessResponse<unknown>)
  }

  return c.json({ success: false, error: result.error } as ApiErrorResponse, 500)
}

/**
 * 查询转换处理
 */
export async function aiQueryHandler(c: Context<{ Bindings: Env }>) {
  const body = await c.req.json<{ query: string; schema?: string }>()

  if (!body.query) {
    return c.json({ success: false, error: 'Query is required' } as ApiErrorResponse, 400)
  }

  const schema = body.schema || 'nodes, tasks, schedules, playbooks, logs'

  const result = await naturalLanguageToQuery(c.env, body.query, schema)

  if (result.success) {
    return c.json({ success: true, data: result.data } as ApiSuccessResponse<unknown>)
  }

  return c.json({ success: false, error: result.error } as ApiErrorResponse, 500)
}