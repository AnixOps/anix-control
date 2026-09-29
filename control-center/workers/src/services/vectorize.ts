/**
 * Vectorize Service
 * 向量数据库服务，用于语义搜索和相似性匹配
 */

import type { Context } from 'hono'
import type { ApiErrorResponse, ApiMessageResponse, ApiSuccessResponse, Env } from '../types'

// 索引名称
const INDEX_NAME = 'anixops-vectors'

// 向量元数据接口
interface VectorMetadata {
  id: string
  type: 'log' | 'task' | 'node' | 'playbook' | 'document'
  timestamp: string
  [key: string]: string | number | boolean
}

/**
 * 插入向量
 */
export async function insertVector(
  env: Env,
  id: string,
  embedding: number[],
  metadata: VectorMetadata
): Promise<{ success: boolean; error?: string }> {
  try {
    if (!env.VECTORIZE) {
      return { success: false, error: 'Vectorize index not configured' }
    }
    await env.VECTORIZE.upsert([
      {
        id,
        values: embedding,
        metadata,
      },
    ])

    return { success: true }
  } catch (error) {
    return { success: false, error: String(error) }
  }
}

/**
 * 批量插入向量
 */
export async function insertVectors(
  env: Env,
  vectors: Array<{ id: string; values: number[]; metadata: VectorMetadata }>
): Promise<{ success: boolean; error?: string }> {
  try {
    if (!env.VECTORIZE) {
      return { success: false, error: 'Vectorize index not configured' }
    }
    await env.VECTORIZE.upsert(vectors)
    return { success: true }
  } catch (error) {
    return { success: false, error: String(error) }
  }
}

/**
 * 语义搜索
 */
export async function searchVectors(
  env: Env,
  queryEmbedding: number[],
  options?: {
    topK?: number
    filter?: Record<string, string | number | boolean>
    returnMetadata?: boolean
  }
): Promise<{
  success: boolean
  data?: Array<{ id: string; score: number; metadata?: VectorMetadata }>
  error?: string
}> {
  try {
    if (!env.VECTORIZE) {
      return { success: false, error: 'Vectorize index not configured' }
    }
    const result = await env.VECTORIZE.query(queryEmbedding, {
      topK: options?.topK || 10,
      filter: options?.filter,
      returnMetadata: options?.returnMetadata ?? true,
    })

    return {
      success: true,
      data: result.matches.map((m) => ({
        id: m.id,
        score: m.score,
        metadata: m.metadata as VectorMetadata,
      })),
    }
  } catch (error) {
    return { success: false, error: String(error) }
  }
}

/**
 * 删除向量
 */
export async function deleteVector(
  env: Env,
  id: string
): Promise<{ success: boolean; error?: string }> {
  try {
    if (!env.VECTORIZE) {
      return { success: false, error: 'Vectorize index not configured' }
    }
    await env.VECTORIZE.deleteByIds([id])
    return { success: true }
  } catch (error) {
    return { success: false, error: String(error) }
  }
}

/**
 * 获取向量信息
 */
export async function getVector(
  env: Env,
  id: string
): Promise<{
  success: boolean
  data?: { id: string; values: number[]; metadata?: VectorMetadata }
  error?: string
}> {
  try {
    if (!env.VECTORIZE) {
      return { success: false, error: 'Vectorize index not configured' }
    }
    const result = await env.VECTORIZE.getByIds([id])

    if (result.length === 0) {
      return { success: false, error: 'Vector not found' }
    }

    return {
      success: true,
      data: {
        id: result[0].id,
        values: Array.from(result[0].values),
        metadata: result[0].metadata as VectorMetadata,
      },
    }
  } catch (error) {
    return { success: false, error: String(error) }
  }
}

// ==================== 高级功能 ====================

/**
 * 智能日志搜索
 */
export async function searchLogs(
  env: Env,
  queryEmbedding: number[],
  options?: {
    nodeId?: number
    taskId?: string
    level?: string
    startTime?: string
    endTime?: string
    topK?: number
  }
): Promise<{
  success: boolean
  data?: Array<{ id: string; score: number; metadata?: VectorMetadata }>
  error?: string
}> {
  const filter: Record<string, string | number | boolean> = {
    type: 'log',
  }

  if (options?.nodeId) filter.nodeId = options.nodeId
  if (options?.taskId) filter.taskId = options.taskId
  if (options?.level) filter.level = options.level

  return searchVectors(env, queryEmbedding, {
    topK: options?.topK || 20,
    filter,
    returnMetadata: true,
  })
}

/**
 * 相似任务搜索
 */
export async function searchSimilarTasks(
  env: Env,
  taskEmbedding: number[],
  topK: number = 10
): Promise<{
  success: boolean
  data?: Array<{ id: string; score: number; metadata?: VectorMetadata }>
  error?: string
}> {
  return searchVectors(env, taskEmbedding, {
    topK,
    filter: { type: 'task' },
    returnMetadata: true,
  })
}

/**
 * 异常检测
 */
export async function detectAnomalies(
  env: Env,
  currentEmbedding: number[],
  baseline: number[]
): Promise<{
  success: boolean
  data?: { isAnomaly: boolean; distance: number; threshold: number }
  error?: string
}> {
  try {
    // 计算余弦相似度
    const similarity = cosineSimilarity(currentEmbedding, baseline)
    const distance = 1 - similarity
    const threshold = 0.3 // 可调整的阈值

    return {
      success: true,
      data: {
        isAnomaly: distance > threshold,
        distance,
        threshold,
      },
    }
  } catch (error) {
    return { success: false, error: String(error) }
  }
}

/**
 * 计算余弦相似度
 */
function cosineSimilarity(a: number[], b: number[]): number {
  if (a.length !== b.length) return 0

  let dotProduct = 0
  let normA = 0
  let normB = 0

  for (let i = 0; i < a.length; i++) {
    dotProduct += a[i] * b[i]
    normA += a[i] * a[i]
    normB += b[i] * b[i]
  }

  return dotProduct / (Math.sqrt(normA) * Math.sqrt(normB))
}

// ==================== HTTP Handlers ====================

/**
 * 向量搜索处理
 */
export async function vectorSearchHandler(c: Context<{ Bindings: Env }>) {
  const body = await c.req.json<{
    embedding: number[]
    topK?: number
    type?: string
    filter?: Record<string, string | number | boolean>
  }>()

  if (!body.embedding || !Array.isArray(body.embedding)) {
    return c.json({ success: false, error: 'Embedding is required' } as ApiErrorResponse, 400)
  }

  const filter = body.type ? { type: body.type, ...body.filter } : body.filter

  const result = await searchVectors(c.env, body.embedding, {
    topK: body.topK,
    filter,
  })

  if (result.success) {
    return c.json({ success: true, data: result.data } as ApiSuccessResponse<unknown>)
  }

  return c.json({ success: false, error: result.error } as ApiErrorResponse, 500)
}

/**
 * 向量插入处理
 */
export async function vectorInsertHandler(c: Context<{ Bindings: Env }>) {
  const body = await c.req.json<{
    vectors: Array<{
      id: string
      embedding: number[]
      metadata: VectorMetadata
    }>
  }>()

  if (!body.vectors || !Array.isArray(body.vectors)) {
    return c.json({ success: false, error: 'Vectors are required' } as ApiErrorResponse, 400)
  }

  const vectors = body.vectors.map((v) => ({
    id: v.id,
    values: v.embedding,
    metadata: v.metadata,
  }))

  const result = await insertVectors(c.env, vectors)

  if (result.success) {
    return c.json({ success: true, message: `Inserted ${vectors.length} vectors` } as ApiMessageResponse)
  }

  return c.json({ success: false, error: result.error } as ApiErrorResponse, 500)
}

/**
 * 向量删除处理
 */
export async function vectorDeleteHandler(c: Context<{ Bindings: Env }>) {
  const body = await c.req.json<{ id: string }>()

  if (!body.id) {
    return c.json({ success: false, error: 'ID is required' } as ApiErrorResponse, 400)
  }

  const result = await deleteVector(c.env, body.id)

  if (result.success) {
    return c.json({ success: true, message: 'Vector deleted' } as ApiMessageResponse)
  }

  return c.json({ success: false, error: result.error } as ApiErrorResponse, 500)
}