/**
 * Tests for Monitoring Service
 */

import { describe, it, expect } from 'vitest'
import { createMockKV, createMockD1 } from './setup'
import {
  recordMetric,
  queryMetrics,
  createAlertRule,
  getAlertRules,
  createDashboard,
  getDashboards,
  type MetricPoint,
  type AlertRule,
  type DashboardConfig,
} from '../src/services/monitoring'

const createMockEnv = () => ({
  KV: createMockKV(),
  DB: createMockD1(),
  ASSETS: {} as any,
})

describe('Monitoring Service', () => {
  describe('recordMetric', () => {
    it('should record a metric point', async () => {
      const env = createMockEnv()
      const result = await recordMetric(env, {
        name: 'cpu_usage',
        type: 'gauge',
        value: 75.5,
        labels: { host: 'server-1' },
      })

      expect(result.success).toBe(true)
    })

    it('should record counter metrics', async () => {
      const env = createMockEnv()
      const result = await recordMetric(env, {
        name: 'requests_total',
        type: 'counter',
        value: 1,
        labels: { method: 'GET', status: '200' },
      })

      expect(result.success).toBe(true)
    })

    it('should record histogram metrics', async () => {
      const env = createMockEnv()
      const result = await recordMetric(env, {
        name: 'request_duration',
        type: 'histogram',
        value: 0.125,
        labels: { endpoint: '/api/v1/users' },
      })

      expect(result.success).toBe(true)
    })
  })

  describe('queryMetrics', () => {
    it('should return empty array when no metrics', async () => {
      const env = createMockEnv()
      const result = await queryMetrics(env, {
        name: 'nonexistent_metric',
        startTime: new Date(Date.now() - 3600000).toISOString(),
        endTime: new Date().toISOString(),
      })
      expect(result.success).toBe(true)
      expect(result.data).toEqual([])
    })
  })

  describe('Alert Rules', () => {
    it('should create an alert rule', async () => {
      const env = createMockEnv()

      const rule: Omit<AlertRule, 'id'> = {
        name: 'High CPU Alert',
        metric: 'cpu_usage',
        operator: 'gt',
        threshold: 80,
        duration: 300,
        severity: 'warning',
        enabled: true,
      }

      const result = await createAlertRule(env, rule)
      expect(result.success).toBe(true)
      expect(result.id).toBeDefined()
    })

    it('should get alert rules', async () => {
      const env = createMockEnv()
      const rules = await getAlertRules(env)
      expect(Array.isArray(rules)).toBe(true)
    })
  })

  describe('Dashboards', () => {
    it('should create a dashboard', async () => {
      const env = createMockEnv()

      const dashboard: Omit<DashboardConfig, 'id'> = {
        name: 'System Overview',
        panels: [
          {
            id: 'panel-1',
            title: 'CPU Usage',
            type: 'line',
            metrics: ['cpu_usage'],
            width: 6,
            height: 4,
            x: 0,
            y: 0,
          },
        ],
        refreshInterval: 30,
        timeRange: '1h',
      }

      const result = await createDashboard(env, dashboard)
      expect(result.success).toBe(true)
      expect(result.id).toBeDefined()
    })

    it('should get dashboards', async () => {
      const env = createMockEnv()
      const dashboards = await getDashboards(env)
      expect(Array.isArray(dashboards)).toBe(true)
    })
  })
})