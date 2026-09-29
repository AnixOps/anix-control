import { describe, it, expect, vi, beforeEach } from 'vitest'
import {
  AI_MODELS,
  generateText,
  generateEmbedding,
  analyzeLog,
  naturalLanguageToQuery,
  generateOpsAdvice,
  chatAssistant,
} from './ai'
import type { Env } from '../types'

function createEnv() {
  return {
    AI: {
      run: vi.fn(),
    },
  } as unknown as Env & { AI: { run: ReturnType<typeof vi.fn> } }
}

describe('ai service', () => {
  let env: ReturnType<typeof createEnv>

  beforeEach(() => {
    env = createEnv()
  })

  it('exports expected model ids', () => {
    expect(AI_MODELS.textGeneration).toContain('@cf/')
    expect(AI_MODELS.textEmbeddings).toContain('@cf/')
  })

  it('generateText sends messages with system prompt', async () => {
    env.AI.run.mockResolvedValue({ response: 'ok' })

    const result = await generateText(env, 'hello world', {
      systemPrompt: 'You are helpful.',
      maxTokens: 123,
      temperature: 0.2,
    })

    expect(result).toEqual({ success: true, data: { response: 'ok' } })
    expect(env.AI.run).toHaveBeenCalledWith(AI_MODELS.textGeneration, {
      messages: [
        { role: 'system', content: 'You are helpful.' },
        { role: 'user', content: 'hello world' },
      ],
      max_tokens: 123,
      temperature: 0.2,
    })
  })

  it('generateText returns failure when AI binding throws', async () => {
    env.AI.run.mockRejectedValue(new Error('binding unavailable'))

    const result = await generateText(env, 'hello')

    expect(result.success).toBe(false)
    expect(result.error).toContain('binding unavailable')
  })

  it('generateEmbedding uses embeddings model', async () => {
    env.AI.run.mockResolvedValue({ data: [[0.1, 0.2, 0.3]] })

    const result = await generateEmbedding(env, 'log line')

    expect(result.success).toBe(true)
    expect(env.AI.run).toHaveBeenCalledWith(AI_MODELS.textEmbeddings, {
      text: 'log line',
    })
  })

  it('analyzeLog uses fast text model with devops prompt', async () => {
    env.AI.run.mockResolvedValue({ response: '{"severity":3}' })

    await analyzeLog(env, 'ERROR database connection failed')

    expect(env.AI.run).toHaveBeenCalledWith(
      AI_MODELS.textGenerationFast,
      expect.objectContaining({
        max_tokens: 256,
        temperature: 0.3,
        messages: expect.arrayContaining([
          expect.objectContaining({ role: 'system' }),
          expect.objectContaining({ role: 'user', content: 'ERROR database connection failed' }),
        ]),
      })
    )
  })

  it('naturalLanguageToQuery injects schema into system prompt', async () => {
    env.AI.run.mockResolvedValue({ response: '{"resource":"tasks"}' })

    await naturalLanguageToQuery(env, 'show failed tasks', 'tasks(status, created_at)')

    const [, payload] = env.AI.run.mock.calls[0]
    expect(payload.messages[0].content).toContain('tasks(status, created_at)')
    expect(payload.messages[1].content).toBe('show failed tasks')
  })

  it('generateOpsAdvice builds context-rich prompt', async () => {
    env.AI.run.mockResolvedValue({ response: '1. Restart node' })

    await generateOpsAdvice(env, {
      nodeStatus: '1 offline',
      taskHistory: '3 failed deploys',
      alerts: 'cpu high',
      metrics: 'load 8.2',
    })

    const [, payload] = env.AI.run.mock.calls[0]
    expect(payload.messages[0].content).toContain('1 offline')
    expect(payload.messages[0].content).toContain('3 failed deploys')
    expect(payload.messages[0].content).toContain('cpu high')
    expect(payload.messages[0].content).toContain('load 8.2')
  })

  it('chatAssistant preserves conversation history', async () => {
    env.AI.run.mockResolvedValue({ response: 'Node is healthy.' })

    await chatAssistant(env, 'Show node status', [
      { role: 'user', content: 'Hello' },
      { role: 'assistant', content: 'Hi there' },
    ])

    expect(env.AI.run).toHaveBeenCalledWith(
      AI_MODELS.textGeneration,
      expect.objectContaining({
        messages: [
          expect.objectContaining({ role: 'system' }),
          { role: 'user', content: 'Hello' },
          { role: 'assistant', content: 'Hi there' },
          { role: 'user', content: 'Show node status' },
        ],
      })
    )
  })
})
