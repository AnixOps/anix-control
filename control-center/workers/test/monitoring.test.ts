/**
 * Tests for Monitoring Service
 */

import { describe, it, expect } from 'vitest'
import { createMockKV, createMockD1 } from './setup'
import {
  buildDeveloperReadinessSummary,
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
  AI: {} as any,
  VECTORIZE: {} as any,
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

  describe('Developer Readiness Summary', () => {
    it('should build the developer readiness summary contract', async () => {
      const summary = await buildDeveloperReadinessSummary()

      expect(summary.manifest_total).toBeGreaterThan(0)
      expect(summary.readiness_counts.verified).toBeGreaterThan(0)
      expect(summary.readiness_counts.diagnostic).toBeGreaterThan(0)
      expect(summary.execution_mode_counts.automated).toBeGreaterThan(0)
      expect(summary.execution_mode_counts['fixture-backed']).toBeGreaterThan(0)
      expect(summary.ready_endpoints.length).toBeGreaterThan(0)
      expect(summary.manual_endpoints.length).toBeGreaterThan(0)
      expect(summary.ready_endpoints.every(entry => entry.readiness === 'verified')).toBe(true)
      expect(summary.manual_endpoints.every(entry => entry.readiness === 'manual' || entry.readiness === 'inventory')).toBe(true)
      expect(summary.fixture_coverage.total_endpoints).toBeGreaterThan(0)
      expect(summary.fixture_coverage.fixture_key_counts.principals).toBeGreaterThan(0)
      expect(summary.fixture_coverage.endpoints.every(entry => entry.fixture_keys.length > 0)).toBe(true)
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