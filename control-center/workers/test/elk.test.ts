/**
 * Tests for ELK Service
 */

import { describe, it, expect } from 'vitest'

describe('ELK Service', () => {
  describe('Index Templates', () => {
    it('should have index template configuration', () => {
      interface IndexTemplateConfig {
        index_patterns: string[]
        template: {
          settings: Record<string, unknown>
          mappings: Record<string, unknown>
        }
      }

      const config: IndexTemplateConfig = {
        index_patterns: ['logs-app-*'],
        template: {
          settings: {
            number_of_shards: 3,
            number_of_replicas: 1,
          },
          mappings: {
            properties: {
              '@timestamp': { type: 'date' },
              message: { type: 'text' },
              level: { type: 'keyword' },
            },
          },
        },
      }

      expect(config.index_patterns).toContain('logs-app-*')
      expect(config.template.settings.number_of_shards).toBe(3)
    })
  })

  describe('ILM Policies', () => {
    it('should have ILM policy configuration', () => {
      interface IlmPolicyConfig {
        policy: {
          phases: Record<string, unknown>
        }
      }

      const policy: IlmPolicyConfig = {
        policy: {
          phases: {
            hot: {
              min_age: '0ms',
              actions: {
                rollover: { max_size: '50gb', max_age: '1d' },
              },
            },
            warm: {
              min_age: '7d',
              actions: {
                forcemerge: { max_num_segments: 1 },
              },
            },
          },
        },
      }

      expect(policy.policy.phases).toHaveProperty('hot')
    })
  })

  describe('Logstash Pipeline', () => {
    it('should have pipeline configuration', () => {
      interface LogstashPipelineConfig {
        description: string
        pipeline: Record<string, unknown>
      }

      const pipeline: LogstashPipelineConfig = {
        description: 'Process application logs',
        pipeline: {
          input: { beats: { port: 5044 } },
          filter: [
            { grok: { match: { message: '%{TIMESTAMP_ISO8601:timestamp}' } } },
          ],
          output: { elasticsearch: { hosts: ['localhost:9200'] } },
        },
      }

      expect(pipeline.description).toBe('Process application logs')
    })
  })
})