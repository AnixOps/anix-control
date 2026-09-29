import { describe, it, expect, vi, beforeEach } from 'vitest'
import {
  vectorSearchHandler,
  vectorInsertHandler,
  vectorDeleteHandler,
} from './vectorize'

function createContext({ body, params, env }: {
  body?: unknown
  params?: Record<string, string>
  env?: { VECTORIZE?: Record<string, unknown> }
}) {
  return {
    env: {
      VECTORIZE: {
        upsert: vi.fn(),
        query: vi.fn(),
        deleteByIds: vi.fn(),
        getByIds: vi.fn(),
        ...(env?.VECTORIZE || {}),
      },
    },
    req: {
      json: async () => body,
      param: (name: string) => params?.[name],
    },
    json: (data: unknown, status = 200) =>
      new Response(JSON.stringify(data), {
        status,
        headers: { 'Content-Type': 'application/json' },
      }),
  }
}

describe('vectorize handlers', () => {
  let query: ReturnType<typeof vi.fn>
  let upsert: ReturnType<typeof vi.fn>
  let deleteByIds: ReturnType<typeof vi.fn>

  beforeEach(() => {
    query = vi.fn()
    upsert = vi.fn()
    deleteByIds = vi.fn()
  })

  it('vectorSearchHandler validates embedding', async () => {
    const response = await vectorSearchHandler(createContext({ body: {} }) as never)
    expect(response.status).toBe(400)
    expect(await response.json()).toEqual({ success: false, error: 'Embedding is required' })
  })

  it('vectorSearchHandler applies type filter', async () => {
    query.mockResolvedValue({
      matches: [{ id: 'log-1', score: 0.9, metadata: { type: 'log', timestamp: '2026-03-23T00:00:00Z' } }],
    })

    const response = await vectorSearchHandler(createContext({
      body: {
        embedding: [0.1, 0.2],
        topK: 3,
        type: 'log',
        filter: { level: 'error' },
      },
      env: { VECTORIZE: { query } },
    }) as never)

    expect(query).toHaveBeenCalledWith([0.1, 0.2], {
      topK: 3,
      filter: { type: 'log', level: 'error' },
      returnMetadata: true,
    })
    expect(response.status).toBe(200)
  })

  it('vectorInsertHandler validates vectors payload', async () => {
    const response = await vectorInsertHandler(createContext({ body: {} }) as never)
    expect(response.status).toBe(400)
    expect(await response.json()).toEqual({ success: false, error: 'Vectors are required' })
  })

  it('vectorInsertHandler inserts vectors', async () => {
    upsert.mockResolvedValue(undefined)

    const response = await vectorInsertHandler(createContext({
      body: {
        vectors: [
          {
            id: 'task-1',
            embedding: [0.3, 0.4],
            metadata: { id: 'task-1', type: 'task', timestamp: '2026-03-23T00:00:00Z', status: 'failed' },
          },
        ],
      },
      env: { VECTORIZE: { upsert } },
    }) as never)

    expect(upsert).toHaveBeenCalledWith([
      {
        id: 'task-1',
        values: [0.3, 0.4],
        metadata: { id: 'task-1', type: 'task', timestamp: '2026-03-23T00:00:00Z', status: 'failed' },
      },
    ])
    expect(response.status).toBe(200)
    expect(await response.json()).toEqual({ success: true, message: 'Inserted 1 vectors' })
  })

  it('vectorDeleteHandler validates id', async () => {
    const response = await vectorDeleteHandler(createContext({ body: {} }) as never)
    expect(response.status).toBe(400)
    expect(await response.json()).toEqual({ success: false, error: 'ID is required' })
  })

  it('vectorDeleteHandler deletes vector', async () => {
    deleteByIds.mockResolvedValue(undefined)

    const response = await vectorDeleteHandler(createContext({
      body: { id: 'log-1' },
      env: { VECTORIZE: { deleteByIds } },
    }) as never)

    expect(deleteByIds).toHaveBeenCalledWith(['log-1'])
    expect(response.status).toBe(200)
    expect(await response.json()).toEqual({ success: true, message: 'Vector deleted' })
  })
})
