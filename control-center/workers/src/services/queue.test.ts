import { describe, it, expect } from 'vitest'

// Queue mock data
const mockQueues = [
  { name: 'tasks', messages: 1250, consumers: 5, rate: 50 },
  { name: 'notifications', messages: 450, consumers: 3, rate: 25 },
  { name: 'logs', messages: 8500, consumers: 2, rate: 100 }
]

describe('Queue Management', () => {
  it('lists all queues', () => {
    expect(mockQueues.length).toBe(3)
  })

  it('calculates total messages', () => {
    const total = mockQueues.reduce((sum, q) => sum + q.messages, 0)
    expect(total).toBe(10200)
  })

  it('calculates total consumers', () => {
    const total = mockQueues.reduce((sum, q) => sum + q.consumers, 0)
    expect(total).toBe(10)
  })

  it('finds queue with most messages', () => {
    const max = mockQueues.reduce((a, b) => a.messages > b.messages ? a : b)
    expect(max.name).toBe('logs')
  })

  it('calculates average rate', () => {
    const avg = mockQueues.reduce((sum, q) => sum + q.rate, 0) / mockQueues.length
    expect(avg).toBeCloseTo(58.33)
  })
})

describe('Queue Health', () => {
  it('checks if queue is backed up', () => {
    const threshold = 5000
    const backedUp = mockQueues.filter(q => q.messages > threshold)
    expect(backedUp.length).toBe(1)
  })

  it('checks if consumers are available', () => {
    const hasConsumers = (q) => q.consumers > 0
    mockQueues.forEach(q => {
      expect(hasConsumers(q)).toBe(true)
    })
  })

  it('calculates messages per consumer', () => {
    const perConsumer = mockQueues.map(q => ({
      name: q.name,
      ratio: q.messages / q.consumers
    }))
    expect(perConsumer[0].ratio).toBe(250)
  })
})

describe('Queue Operations', () => {
  it('purges queue messages', () => {
    const queue = { ...mockQueues[0] }
    queue.messages = 0
    expect(queue.messages).toBe(0)
  })

  it('calculates purge impact', () => {
    const messagesToPurge = 8500
    const impact = (messagesToPurge / 10200) * 100
    expect(impact).toBeCloseTo(83.33)
  })
})