/**
 * Tests for Tracing Service
 */

import { describe, it, expect } from 'vitest'
import { createMockKV, createMockD1 } from './setup'
import {
  startSpan,
  endSpan,
  addSpanEvent,
  getTrace,
  searchTraces,
  getTraceStats,
  type Span,
  type SpanContext,
  type Trace,
} from '../src/services/tracing'

const createMockEnv = () => ({
  KV: createMockKV(),
  DB: createMockD1(),
  AI: {} as any,
  VECTORIZE: {} as any,
  ASSETS: {} as any,
})

describe('Tracing Service', () => {
  describe('Span Types', () => {
    it('should have correct span kinds', () => {
      type SpanKind = 'unspecified' | 'internal' | 'server' | 'client' | 'producer' | 'consumer'
      const kinds: SpanKind[] = ['unspecified', 'internal', 'server', 'client', 'producer', 'consumer']
      expect(kinds).toHaveLength(6)
    })

    it('should have correct span status codes', () => {
      type SpanStatusCode = 'ok' | 'error' | 'unset'
      const codes: SpanStatusCode[] = ['ok', 'error', 'unset']
      expect(codes).toHaveLength(3)
    })
  })

  describe('SpanContext', () => {
    it('should have trace context', () => {
      const context: SpanContext = {
        traceId: '0af7651916cd43dd8448eb211c80319c',
        spanId: 'b7ad6b7169203331',
        traceFlags: 1,
        traceState: 'congo=t61rcWkgMzE',
      }

      expect(context.traceId).toHaveLength(32)
      expect(context.spanId).toHaveLength(16)
      expect(context.traceFlags).toBe(1)
    })
  })

  describe('startSpan', () => {
    it('should start a span', async () => {
      const env = createMockEnv()

      const result = await startSpan(env, {
        name: 'test-operation',
        kind: 'server',
        attributes: { 'http.method': 'GET' },
      })

      expect(result).toBeDefined()
      expect(result.span.name).toBe('test-operation')
      expect(result.span.spanId).toBeDefined()
      expect(result.span.traceId).toBeDefined()
      expect(result.context.spanId).toBe(result.span.spanId)
    })
  })

  describe('endSpan', () => {
    it('should end a span', async () => {
      const env = createMockEnv()

      const { span } = await startSpan(env, {
        name: 'test-operation',
        kind: 'server',
        attributes: {},
      })

      const result = await endSpan(env, span.spanId)
      expect(result.success).toBe(true)
    })
  })

  describe('addSpanEvent', () => {
    it('should add event to span', async () => {
      const env = createMockEnv()

      const { span } = await startSpan(env, {
        name: 'test-operation',
        kind: 'server',
        attributes: {},
      })

      const result = await addSpanEvent(env, span.spanId, {
        name: 'retry-attempt',
        attributes: { 'retry.count': 1 },
      })

      expect(result.success).toBe(true)
    })
  })

  describe('getTrace', () => {
    it('should return null for nonexistent trace', async () => {
      const env = createMockEnv()
      const trace = await getTrace(env, 'nonexistent-trace-id')
      expect(trace).toBeNull()
    })
  })

  describe('queryTraces', () => {
    it('should return empty array when no traces', async () => {
      const env = createMockEnv()
      const traces = await searchTraces(env, {
        service: 'nonexistent-service',
      })
      expect(traces).toEqual([])
    })
  })

  describe('getServiceStats', () => {
    it('should return service stats', async () => {
      const env = createMockEnv()
      const stats = await getTraceStats(env)

      expect(stats).toHaveProperty('totalSpans')
      expect(stats).toHaveProperty('errorRate')
      expect(stats).toHaveProperty('averageDuration')
    })
  })
})