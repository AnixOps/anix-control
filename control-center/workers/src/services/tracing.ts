/**
 * Tracing Service
 *
 * Provides distributed tracing with OpenTelemetry compatibility
 */

import type { Env } from '../types'

// Span context
export interface SpanContext {
  traceId: string
  spanId: string
  traceFlags: number
  traceState?: string
}

// Span kind
export type SpanKind = 'unspecified' | 'internal' | 'server' | 'client' | 'producer' | 'consumer'

// Span status
export type SpanStatusCode = 'ok' | 'error' | 'unset'

// Span attribute value
export type AttributeValue = string | number | boolean | Array<string | number | boolean>

// Span
export interface Span {
  traceId: string
  spanId: string
  parentSpanId?: string
  name: string
  kind: SpanKind
  startTime: string
  endTime?: string
  duration?: number
  attributes: Record<string, AttributeValue>
  events: SpanEvent[]
  status: { code: SpanStatusCode; message?: string }
  resource: Record<string, AttributeValue>
  links: SpanLink[]
}

// Span event
export interface SpanEvent {
  name: string
  timestamp: string
  attributes?: Record<string, AttributeValue>
}

// Span link
export interface SpanLink {
  traceId: string
  spanId: string
  attributes?: Record<string, AttributeValue>
}

// Trace
export interface Trace {
  traceId: string
  spans: Span[]
  rootSpan?: Span
  duration: number
  serviceCount: number
  spanCount: number
  status: SpanStatusCode
}

// Trace query
export interface TraceQuery {
  service?: string
  operation?: string
  minDuration?: number
  maxDuration?: number
  status?: SpanStatusCode
  startTime?: string
  endTime?: string
  limit?: number
  offset?: number
}

// Trace statistics
export interface TraceStats {
  totalTraces: number
  totalSpans: number
  averageDuration: number
  errorRate: number
  services: Array<{ name: string; spanCount: number }>
  operations: Array<{ name: string; count: number }>
}

// Generate trace ID
export function generateTraceId(): string {
  const bytes = new Uint8Array(16)
  crypto.getRandomValues(bytes)
  return Array.from(bytes).map(b => b.toString(16).padStart(2, '0')).join('')
}

// Generate span ID
export function generateSpanId(): string {
  const bytes = new Uint8Array(8)
  crypto.getRandomValues(bytes)
  return Array.from(bytes).map(b => b.toString(16).padStart(2, '0')).join('')
}

/**
 * Start a new span
 */
export async function startSpan(
  env: Env,
  params: {
    name: string
    kind: SpanKind
    parentContext?: SpanContext
    attributes?: Record<string, AttributeValue>
    resource?: Record<string, AttributeValue>
  }
): Promise<{ span: Span; context: SpanContext }> {
  const traceId = params.parentContext?.traceId || generateTraceId()
  const spanId = generateSpanId()

  const span: Span = {
    traceId,
    spanId,
    parentSpanId: params.parentContext?.spanId,
    name: params.name,
    kind: params.kind,
    startTime: new Date().toISOString(),
    attributes: params.attributes || {},
    events: [],
    status: { code: 'unset' },
    resource: params.resource || {},
    links: [],
  }

  // Store span
  await env.KV.put(
    `trace:span:${spanId}`,
    JSON.stringify(span),
    { expirationTtl: 86400 } // 24 hours
  )

  // Add to trace index
  await addToTraceIndex(env, traceId, spanId)

  const context: SpanContext = {
    traceId,
    spanId,
    traceFlags: 1, // sampled
  }

  return { span, context }
}

/**
 * End a span
 */
export async function endSpan(
  env: Env,
  spanId: string,
  status?: { code: SpanStatusCode; message?: string }
): Promise<{ success: boolean }> {
  try {
    const spanData = await env.KV.get(`trace:span:${spanId}`, 'json')
    if (!spanData) {
      return { success: false }
    }

    const span = spanData as Span
    const endTime = new Date().toISOString()
    const startTime = new Date(span.startTime).getTime()
    const duration = new Date(endTime).getTime() - startTime

    const updatedSpan: Span = {
      ...span,
      endTime,
      duration,
      status: status || span.status,
    }

    await env.KV.put(
      `trace:span:${spanId}`,
      JSON.stringify(updatedSpan),
      { expirationTtl: 86400 }
    )

    return { success: true }
  } catch (err) {
    console.error('Failed to end span:', err)
    return { success: false }
  }
}

/**
 * Add event to span
 */
export async function addSpanEvent(
  env: Env,
  spanId: string,
  event: Omit<SpanEvent, 'timestamp'>
): Promise<{ success: boolean }> {
  try {
    const spanData = await env.KV.get(`trace:span:${spanId}`, 'json')
    if (!spanData) {
      return { success: false }
    }

    const span = spanData as Span
    span.events.push({
      ...event,
      timestamp: new Date().toISOString(),
    })

    await env.KV.put(
      `trace:span:${spanId}`,
      JSON.stringify(span),
      { expirationTtl: 86400 }
    )

    return { success: true }
  } catch {
    return { success: false }
  }
}

/**
 * Set span attributes
 */
export async function setSpanAttributes(
  env: Env,
  spanId: string,
  attributes: Record<string, AttributeValue>
): Promise<{ success: boolean }> {
  try {
    const spanData = await env.KV.get(`trace:span:${spanId}`, 'json')
    if (!spanData) {
      return { success: false }
    }

    const span = spanData as Span
    span.attributes = { ...span.attributes, ...attributes }

    await env.KV.put(
      `trace:span:${spanId}`,
      JSON.stringify(span),
      { expirationTtl: 86400 }
    )

    return { success: true }
  } catch {
    return { success: false }
  }
}

/**
 * Get trace by ID
 */
export async function getTrace(
  env: Env,
  traceId: string
): Promise<Trace | null> {
  try {
    const spanIds = await env.KV.get(`trace:index:${traceId}`, 'json') as string[] | null
    if (!spanIds || spanIds.length === 0) {
      return null
    }

    const spans: Span[] = []
    for (const spanId of spanIds) {
      const span = await env.KV.get(`trace:span:${spanId}`, 'json') as Span | null
      if (span) spans.push(span)
    }

    if (spans.length === 0) {
      return null
    }

    // Find root span (no parent)
    const rootSpan = spans.find(s => !s.parentSpanId)

    // Calculate trace duration
    const durations = spans.filter(s => s.duration).map(s => s.duration as number)
    const totalDuration = Math.max(...durations, 0)

    // Determine overall status
    const hasError = spans.some(s => s.status.code === 'error')
    const status: SpanStatusCode = hasError ? 'error' : 'ok'

    // Count unique services
    const services = new Set(spans.map(s => s.resource['service.name'] as string).filter(Boolean))

    return {
      traceId,
      spans,
      rootSpan,
      duration: totalDuration,
      serviceCount: services.size,
      spanCount: spans.length,
      status,
    }
  } catch (err) {
    console.error('Failed to get trace:', err)
    return null
  }
}

/**
 * Search traces
 */
export async function searchTraces(
  env: Env,
  query: TraceQuery
): Promise<Trace[]> {
  try {
    // Get recent trace IDs
    const traceIndexKey = 'trace:recent'
    const traceIds = await env.KV.get(traceIndexKey, 'json') as string[] | null

    if (!traceIds || traceIds.length === 0) {
      return []
    }

    const traces: Trace[] = []
    const limit = query.limit || 50

    for (const traceId of traceIds.slice(query.offset || 0, limit)) {
      const trace = await getTrace(env, traceId)
      if (!trace) continue

      // Apply filters
      if (query.service) {
        const hasService = trace.spans.some(
          s => s.resource['service.name'] === query.service
        )
        if (!hasService) continue
      }

      if (query.minDuration && trace.duration < query.minDuration) {
        continue
      }

      if (query.maxDuration && trace.duration > query.maxDuration) {
        continue
      }

      if (query.status && trace.status !== query.status) {
        continue
      }

      traces.push(trace)
    }

    return traces
  } catch {
    return []
  }
}

/**
 * Get trace statistics
 */
export async function getTraceStats(
  env: Env,
  timeRange?: { start: string; end: string }
): Promise<TraceStats> {
  try {
    const traceIds = await env.KV.get('trace:recent', 'json') as string[] | null

    if (!traceIds || traceIds.length === 0) {
      return {
        totalTraces: 0,
        totalSpans: 0,
        averageDuration: 0,
        errorRate: 0,
        services: [],
        operations: [],
      }
    }

    let totalSpans = 0
    let totalDuration = 0
    let errorCount = 0
    const serviceMap = new Map<string, number>()
    const operationMap = new Map<string, number>()

    for (const traceId of traceIds.slice(0, 100)) {
      const trace = await getTrace(env, traceId)
      if (!trace) continue

      totalSpans += trace.spanCount
      totalDuration += trace.duration
      if (trace.status === 'error') errorCount++

      for (const span of trace.spans) {
        const serviceName = span.resource['service.name'] as string
        if (serviceName) {
          serviceMap.set(serviceName, (serviceMap.get(serviceName) || 0) + 1)
        }
        const operation = span.name
        operationMap.set(operation, (operationMap.get(operation) || 0) + 1)
      }
    }

    const services = Array.from(serviceMap.entries())
      .map(([name, spanCount]) => ({ name, spanCount }))
      .sort((a, b) => b.spanCount - a.spanCount)
      .slice(0, 10)

    const operations = Array.from(operationMap.entries())
      .map(([name, count]) => ({ name, count }))
      .sort((a, b) => b.count - a.count)
      .slice(0, 10)

    return {
      totalTraces: traceIds.length,
      totalSpans,
      averageDuration: traceIds.length > 0 ? totalDuration / traceIds.length : 0,
      errorRate: traceIds.length > 0 ? errorCount / traceIds.length : 0,
      services,
      operations,
    }
  } catch {
    return {
      totalTraces: 0,
      totalSpans: 0,
      averageDuration: 0,
      errorRate: 0,
      services: [],
      operations: [],
    }
  }
}

// Helper functions
async function addToTraceIndex(env: Env, traceId: string, spanId: string): Promise<void> {
  // Add span to trace's span list
  const indexKey = `trace:index:${traceId}`
  const existing = await env.KV.get(indexKey, 'json') as string[] | null
  const spanIds = existing || []
  spanIds.push(spanId)
  await env.KV.put(indexKey, JSON.stringify(spanIds), { expirationTtl: 86400 })

  // Add trace to recent traces list
  const recentKey = 'trace:recent'
  const recent = await env.KV.get(recentKey, 'json') as string[] | null
  const recentTraces = recent || []

  // Move to front if exists, or add if new
  const existingIndex = recentTraces.indexOf(traceId)
  if (existingIndex >= 0) {
    recentTraces.splice(existingIndex, 1)
  }
  recentTraces.unshift(traceId)

  // Keep only last 1000 traces
  if (recentTraces.length > 1000) {
    recentTraces.pop()
  }

  await env.KV.put(recentKey, JSON.stringify(recentTraces), { expirationTtl: 86400 })
}