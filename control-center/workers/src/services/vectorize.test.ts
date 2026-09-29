import { describe, it, expect, vi, beforeEach } from 'vitest'
import {
  insertVector,
  insertVectors,
  searchVectors,
  deleteVector,
  getVector,
  searchLogs,
  searchSimilarTasks,
  detectAnomalies,
} from './vectorize'
import type { Env } from '../types'

function createEnv() {
  return {
    VECTORIZE: {
      upsert: vi.fn(),
      query: vi.fn(),
      deleteByIds: vi.fn(),
      getByIds: vi.fn(),
    },
  } as unknown as Env & {
    VECTORIZE: {
      upsert: ReturnType<typeof vi.fn>
      query: ReturnType<typeof vi.fn>
      deleteByIds: ReturnType<typeof vi.fn>
      getByIds: ReturnType<typeof vi.fn>
    }
  }
}

describe('vectorize service', () => {
  let env: ReturnType<typeof createEnv>

  beforeEach(() => {
    env = createEnv()
  })

  it('insertVector upserts a single vector', async () => {
    env.VECTORIZE.upsert.mockResolvedValue(undefined)

    const result = await insertVector(env, 'log-1', [0.1, 0.2], {
      id: 'log-1',
      type: 'log',
      timestamp: '2026-03-23T00:00:00Z',
      level: 'error',
    })

    expect(result).toEqual({ success: true })
    expect(env.VECTORIZE.upsert).toHaveBeenCalledWith([
      {
        id: 'log-1',
        values: [0.1, 0.2],
        metadata: {
          id: 'log-1',
          type: 'log',
          timestamp: '2026-03-23T00:00:00Z',
          level: 'error',
        },
      },
    ])
  })

  it('insertVectors returns failure when binding throws', async () => {
    env.VECTORIZE.upsert.mockRejectedValue(new Error('vectorize unavailable'))

    const result = await insertVectors(env, [
      {
        id: 'task-1',
        values: [0.3, 0.4],
        metadata: {
          id: 'task-1',
          type: 'task',
          timestamp: '2026-03-23T00:00:00Z',
          status: 'failed',
        },
      },
    ])

    expect(result.success).toBe(false)
    expect(result.error).toContain('vectorize unavailable')
  })

  it('searchVectors passes topK, filter and metadata options', async () => {
    env.VECTORIZE.query.mockResolvedValue({
      matches: [
        {
          id: 'log-1',
          score: 0.91,
          metadata: { id: 'log-1', type: 'log', timestamp: '2026-03-23T00:00:00Z' },
        },
      ],
    })

    const result = await searchVectors(env, [0.1, 0.2], {
      topK: 5,
      filter: { type: 'log', level: 'error' },
      returnMetadata: true,
    })

    expect(env.VECTORIZE.query).toHaveBeenCalledWith([0.1, 0.2], {
      topK: 5,
      filter: { type: 'log', level: 'error' },
      returnMetadata: true,
    })
    expect(result).toEqual({
      success: true,
      data: [
        {
          id: 'log-1',
          score: 0.91,
          metadata: { id: 'log-1', type: 'log', timestamp: '2026-03-23T00:00:00Z' },
        },
      ],
    })
  })

  it('searchLogs enforces log filter and passes node/task constraints', async () => {
    env.VECTORIZE.query.mockResolvedValue({ matches: [] })

    await searchLogs(env, [0.5, 0.6], {
      nodeId: 7,
      taskId: 'task-9',
      level: 'error',
      topK: 20,
    })

    expect(env.VECTORIZE.query).toHaveBeenCalledWith([0.5, 0.6], {
      topK: 20,
      filter: { type: 'log', nodeId: 7, taskId: 'task-9', level: 'error' },
      returnMetadata: true,
    })
  })

  it('searchSimilarTasks searches only task vectors', async () => {
    env.VECTORIZE.query.mockResolvedValue({ matches: [] })

    await searchSimilarTasks(env, [0.7, 0.8], 3)

    expect(env.VECTORIZE.query).toHaveBeenCalledWith([0.7, 0.8], {
      topK: 3,
      filter: { type: 'task' },
      returnMetadata: true,
    })
  })

  it('getVector returns not found when no result exists', async () => {
    env.VECTORIZE.getByIds.mockResolvedValue([])

    const result = await getVector(env, 'missing-id')

    expect(result).toEqual({ success: false, error: 'Vector not found' })
  })

  it('deleteVector deletes by id', async () => {
    env.VECTORIZE.deleteByIds.mockResolvedValue(undefined)

    const result = await deleteVector(env, 'task-1')

    expect(result).toEqual({ success: true })
    expect(env.VECTORIZE.deleteByIds).toHaveBeenCalledWith(['task-1'])
  })

  it('detectAnomalies flags distant vectors', async () => {
    const result = await detectAnomalies(env, [1, 0, 0], [0, 1, 0])

    expect(result.success).toBe(true)
    expect(result.data?.isAnomaly).toBe(true)
    expect(result.data?.threshold).toBe(0.3)
  })
})
