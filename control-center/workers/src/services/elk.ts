import { Hono } from 'hono'
import type { ApiErrorResponse, ApiSuccessResponse } from '../types'

const elk = new Hono()

type IndexTemplateConfig = {
  index_patterns: string[]
  template: {
    settings: Record<string, unknown>
    mappings: Record<string, unknown>
  }
}

type IlmPolicyConfig = {
  policy: {
    phases: Record<string, unknown>
  }
}

type LogstashPipelineConfig = {
  description: string
  pipeline: Record<string, unknown>
}

// Elasticsearch index templates
const indexTemplates: Record<string, IndexTemplateConfig> = {
  'logs-app': {
    index_patterns: ['logs-app-*'],
    template: {
      settings: {
        number_of_shards: 3,
        number_of_replicas: 1,
        'index.lifecycle.name': 'logs-policy',
        'index.lifecycle.rollover_alias': 'logs-app'
      },
      mappings: {
        properties: {
          '@timestamp': { type: 'date' },
          message: { type: 'text' },
          level: { type: 'keyword' },
          service: { type: 'keyword' },
          trace_id: { type: 'keyword' },
          span_id: { type: 'keyword' },
          host: { type: 'keyword' },
          metadata: { type: 'object', enabled: true }
        }
      }
    }
  },
  'metrics-app': {
    index_patterns: ['metrics-app-*'],
    template: {
      settings: {
        number_of_shards: 2,
        number_of_replicas: 1
      },
      mappings: {
        properties: {
          '@timestamp': { type: 'date' },
          metric_name: { type: 'keyword' },
          value: { type: 'double' },
          tags: { type: 'keyword' },
          service: { type: 'keyword' }
        }
      }
    }
  }
}

// Index Lifecycle Management policies
const ilmPolicies: Record<string, IlmPolicyConfig> = {
  'logs-policy': {
    policy: {
      phases: {
        hot: {
          min_age: '0ms',
          actions: {
            rollover: { max_size: '50gb', max_age: '1d' },
            set_priority: { priority: 100 }
          }
        },
        warm: {
          min_age: '7d',
          actions: {
            shrink: { number_of_shards: 1 },
            forcemerge: { max_num_segments: 1 },
            set_priority: { priority: 50 }
          }
        },
        cold: {
          min_age: '30d',
          actions: {
            freeze: {},
            set_priority: { priority: 0 }
          }
        },
        delete: {
          min_age: '90d',
          actions: {
            delete: {}
          }
        }
      }
    }
  }
}

// Logstash pipeline configurations
const logstashPipelines: Record<string, LogstashPipelineConfig> = {
  'logs-pipeline': {
    description: 'Process application logs',
    pipeline: {
      input: {
        beats: {
          port: 5044,
          ssl: true
        }
      },
      filter: [
        {
          grok: {
            match: { message: '%{TIMESTAMP_ISO8601:timestamp} %{LOGLEVEL:level} %{GREEDYDATA:message}' }
          }
        },
        {
          date: {
            match: ['timestamp', 'ISO8601']
          }
        },
        {
          mutate: {
            add_field: { '[@metadata][index]': 'logs-app-%{+YYYY.MM.dd}' }
          }
        }
      ],
      output: {
        elasticsearch: {
          hosts: ['http://elasticsearch:9200'],
          index: '%{[@metadata][index]}',
          pipeline: 'timestamp-pipeline'
        }
      }
    }
  }
}

// Kibana saved objects
const kibanaDashboards = {
  'logs-overview': {
    type: 'dashboard',
    attributes: {
      title: 'Application Logs Overview',
      description: 'Overview of application logs across all services',
      panelsJSON: JSON.stringify([
        {
          id: 'log-volume',
          type: 'visualization',
          gridData: { x: 0, y: 0, w: 12, h: 6 }
        },
        {
          id: 'logs-by-level',
          type: 'visualization',
          gridData: { x: 0, y: 6, w: 6, h: 6 }
        },
        {
          id: 'logs-by-service',
          type: 'visualization',
          gridData: { x: 6, y: 6, w: 6, h: 6 }
        }
      ])
    }
  }
}

// Get index templates
elk.get('/indices/templates', (c) => {
  return c.json({
    templates: Object.entries(indexTemplates).map(([name, config]) => ({
      name,
      index_patterns: config.index_patterns,
      settings: config.template.settings
    }))
  })
})

// Create index template
elk.post('/indices/templates', async (c) => {
  const body = await c.req.json()
  const { name, index_patterns, settings, mappings } = body

  if (!name || !index_patterns) {
    return c.json({ success: false, error: 'Name and index_patterns are required' } as ApiErrorResponse, 400)
  }

  indexTemplates[name] = {
    index_patterns,
    template: { settings, mappings }
  }

  return c.json({ success: true, name })
})

// Get ILM policies
elk.get('/ilm/policies', (c) => {
  return c.json({
    policies: Object.entries(ilmPolicies).map(([name, config]) => ({
      name,
      phases: Object.keys(config.policy.phases)
    }))
  })
})

// Create ILM policy
elk.post('/ilm/policies', async (c) => {
  const body = await c.req.json()
  const { name, phases } = body

  if (!name || !phases) {
    return c.json({ success: false, error: 'Name and phases are required' } as ApiErrorResponse, 400)
  }

  ilmPolicies[name] = { policy: { phases } }
  return c.json({ success: true, name })
})

// Get Logstash pipelines
elk.get('/logstash/pipelines', (c) => {
  return c.json({
    pipelines: Object.entries(logstashPipelines).map(([name, config]) => ({
      name,
      description: config.description
    }))
  })
})

// Get specific pipeline
elk.get('/logstash/pipelines/:name', (c) => {
  const name = c.req.param('name') as string
  const pipeline = logstashPipelines[name]

  if (!pipeline) {
    return c.json({ success: false, error: 'Pipeline not found' } as ApiErrorResponse, 404)
  }

  return c.json({ name, ...pipeline })
})

// Get Kibana dashboards
elk.get('/kibana/dashboards', (c) => {
  return c.json({
    dashboards: Object.entries(kibanaDashboards).map(([id, config]) => ({
      id,
      title: config.attributes.title,
      description: config.attributes.description
    }))
  })
})

// Search logs
elk.post('/logs/search', async (c) => {
  const body = await c.req.json()
  const { query, service, level, from, size } = body

  // Mock search response
  const hits = [
    {
      _index: 'logs-app-2026.03.23',
      _id: 'abc123',
      _score: 1.5,
      _source: {
        '@timestamp': '2026-03-23T10:00:00.000Z',
        message: 'Request processed successfully',
        level: 'INFO',
        service: 'api-gateway',
        trace_id: '0af7651916cd43dd8448eb211c80319c',
        host: 'node-1'
      }
    },
    {
      _index: 'logs-app-2026.03.23',
      _id: 'def456',
      _score: 1.2,
      _source: {
        '@timestamp': '2026-03-23T10:01:00.000Z',
        message: 'Database connection timeout',
        level: 'ERROR',
        service: 'auth-service',
        trace_id: '1bf7651916cd43dd8448eb211c80319d',
        host: 'node-2'
      }
    }
  ]

  return c.json({
    took: 5,
    timed_out: false,
    hits: {
      total: { value: 2, relation: 'eq' },
      max_score: 1.5,
      hits
    }
  })
})

// Get cluster health
elk.get('/cluster/health', (c) => {
  return c.json({
    cluster_name: 'anixops-logs',
    status: 'green',
    timed_out: false,
    number_of_nodes: 3,
    number_of_data_nodes: 3,
    active_primary_shards: 15,
    active_shards: 30,
    relocating_shards: 0,
    initializing_shards: 0,
    unassigned_shards: 0,
    delayed_unassigned_shards: 0,
    number_of_pending_tasks: 0,
    number_of_in_flight_fetch: 0,
    task_max_waiting_in_queue_millis: 0,
    active_shards_percent_as_number: 100.0
  })
})

// Get cluster stats
elk.get('/cluster/stats', (c) => {
  return c.json({
    cluster_name: 'anixops-logs',
    indices: {
      count: 45,
      shards: { total: 90, primaries: 45 },
      docs: { count: 12500000, deleted: 50000 },
      store: { size_in_bytes: 53687091200 },
      fielddata: { memory_size_in_bytes: 1048576 }
    },
    nodes: {
      count: { total: 3, data: 3, master: 3 },
      os: {
        mem: { total_in_bytes: 34359738368 },
        cpu: { percent: 25 }
      },
      jvm: {
        mem: { heap_max_in_bytes: 8589934592 },
        gc: { collectors: { old: { collection_count: 10 } } }
      }
    }
  })
})

// Get index stats
elk.get('/indices/stats', (c) => {
  return c.json({
    indices: {
      'logs-app-2026.03.23': {
        primaries: {
          docs: { count: 500000 },
          store: { size_in_bytes: 1073741824 }
        }
      },
      'logs-app-2026.03.22': {
        primaries: {
          docs: { count: 750000 },
          store: { size_in_bytes: 1610612736 }
        }
      }
    }
  })
})

export default elk