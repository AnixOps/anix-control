import { describe, it, expect, vi, beforeEach } from 'vitest'
import {
  aiChatHandler,
  aiAnalyzeLogHandler,
  aiEmbeddingHandler,
  aiQueryHandler,
} from './ai'

function createContext(body: unknown, runImpl = vi.fn()) {
  return {
    env: {
      AI: { run: runImpl },
    },
    req: {
      json: async () => body,
    },
    json: (data: unknown, status = 200) =>
      new Response(JSON.stringify(data), {
        status,
        headers: { 'Content-Type': 'application/json' },
      }),
  }
}

describe('AI handlers', () => {
  let runImpl: ReturnType<typeof vi.fn>

  beforeEach(() => {
    runImpl = vi.fn()
  })

  it('aiChatHandler validates message', async () => {
    const response = await aiChatHandler(createContext({}) as never)
    expect(response.status).toBe(400)
    expect(await response.json()).toEqual({ success: false, error: 'Message is required' })
  })

  it('aiChatHandler returns AI response payload', async () => {
    runImpl.mockResolvedValue({ response: 'hello from ai' })

    const response = await aiChatHandler(createContext({
      message: 'hello',
      history: [{ role: 'user', content: 'previous' }],
    }, runImpl) as never)

    expect(response.status).toBe(200)
    expect(await response.json()).toEqual({
      success: true,
      data: { response: 'hello from ai' },
    })
  })

  it('aiAnalyzeLogHandler validates log content', async () => {
    const response = await aiAnalyzeLogHandler(createContext({}) as never)
    expect(response.status).toBe(400)
    expect(await response.json()).toEqual({ success: false, error: 'Log content is required' })
  })

  it('aiAnalyzeLogHandler returns AI analysis', async () => {
    runImpl.mockResolvedValue({ response: '{"severity":4}' })

    const response = await aiAnalyzeLogHandler(createContext({
      log: 'ERROR timeout',
    }, runImpl) as never)

    expect(response.status).toBe(200)
    expect(await response.json()).toEqual({
      success: true,
      data: { response: '{"severity":4}' },
    })
  })

  it('aiEmbeddingHandler validates text', async () => {
    const response = await aiEmbeddingHandler(createContext({}) as never)
    expect(response.status).toBe(400)
    expect(await response.json()).toEqual({ success: false, error: 'Text is required' })
  })

  it('aiEmbeddingHandler returns embedding result', async () => {
    runImpl.mockResolvedValue({ data: [[0.1, 0.2]] })

    const response = await aiEmbeddingHandler(createContext({ text: 'hello' }, runImpl) as never)

    expect(response.status).toBe(200)
    expect(await response.json()).toEqual({
      success: true,
      data: { data: [[0.1, 0.2]] },
    })
  })

  it('aiQueryHandler validates query', async () => {
    const response = await aiQueryHandler(createContext({}) as never)
    expect(response.status).toBe(400)
    expect(await response.json()).toEqual({ success: false, error: 'Query is required' })
  })

  it('aiQueryHandler returns translated query', async () => {
    runImpl.mockResolvedValue({ response: '{"resource":"tasks"}' })

    const response = await aiQueryHandler(createContext({
      query: 'show failed tasks',
      schema: 'tasks(status)',
    }, runImpl) as never)

    expect(response.status).toBe(200)
    expect(await response.json()).toEqual({
      success: true,
      data: { response: '{"resource":"tasks"}' },
    })
  })
})
