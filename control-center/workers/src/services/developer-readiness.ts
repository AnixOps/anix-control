import type {
  DeveloperReadinessEndpointSummary,
  DeveloperReadinessSummary,
  EndpointExecutionMode,
  EndpointManifestEntry,
  EndpointReadinessTier,
} from '../types'

export const DEVELOPER_READINESS_MANIFEST: EndpointManifestEntry[] = [
  {
    id: 'GET /health',
    family: 'platform',
    subgroup: 'health',
    name: 'health',
    method: 'GET',
    path: '/health',
    auth: 'public',
    readiness: 'verified',
    execution_mode: 'automated',
    expected_status: 200,
  },
  {
    id: 'GET /health/detailed',
    family: 'platform',
    subgroup: 'health',
    name: 'health detailed',
    method: 'GET',
    path: '/health/detailed',
    auth: 'public',
    readiness: 'verified',
    execution_mode: 'automated',
    expected_status: 200,
  },
  {
    id: 'GET /readiness',
    family: 'platform',
    subgroup: 'health',
    name: 'readiness',
    method: 'GET',
    path: '/readiness',
    auth: 'public',
    readiness: 'verified',
    execution_mode: 'automated',
    expected_status: 200,
  },
  {
    id: 'GET /liveness',
    family: 'platform',
    subgroup: 'health',
    name: 'liveness',
    method: 'GET',
    path: '/liveness',
    auth: 'public',
    readiness: 'verified',
    execution_mode: 'automated',
    expected_status: 200,
  },
  {
    id: 'GET /metrics',
    family: 'platform',
    subgroup: 'health',
    name: 'metrics',
    method: 'GET',
    path: '/metrics',
    auth: 'public',
    readiness: 'verified',
    execution_mode: 'automated',
    expected_status: 200,
  },
  {
    id: 'GET /api/v1/dashboard',
    family: 'platform',
    subgroup: 'dashboard',
    name: 'dashboard overview',
    method: 'GET',
    path: '/api/v1/dashboard',
    auth: 'user',
    readiness: 'verified',
    execution_mode: 'fixture-backed',
    expected_status: 200,
    fixture_keys: ['principals'],
  },
  {
    id: 'GET /api/v1/dashboard/stats',
    family: 'platform',
    subgroup: 'dashboard',
    name: 'dashboard stats',
    method: 'GET',
    path: '/api/v1/dashboard/stats',
    auth: 'user',
    readiness: 'verified',
    execution_mode: 'fixture-backed',
    expected_status: 200,
    fixture_keys: ['principals'],
  },
  {
    id: 'POST /api/v1/auth/register',
    family: 'auth',
    subgroup: 'public',
    name: 'register',
    method: 'POST',
    path: '/api/v1/auth/register',
    auth: 'public',
    readiness: 'inventory',
    execution_mode: 'inventory',
    expected_status: 201,
    manual_notes: 'Bootstrap users are preferred for smoke coverage; registration stays inventory-only here.',
  },
  {
    id: 'POST /api/v1/auth/logout',
    family: 'auth',
    subgroup: 'session',
    name: 'logout',
    method: 'POST',
    path: '/api/v1/auth/logout',
    auth: 'user',
    readiness: 'manual',
    execution_mode: 'manual',
    expected_status: 200,
    fixture_keys: ['principals'],
    manual_notes: 'Validate token invalidation and session teardown in targeted auth regression tests.',
  },
  {
    id: 'GET /api/v1/internal/dev/status',
    family: 'platform',
    subgroup: 'developer-mode',
    name: 'developer mode status',
    method: 'GET',
    path: '/api/v1/internal/dev/status',
    auth: 'admin',
    readiness: 'diagnostic',
    execution_mode: 'diagnostic',
    expected_status: 200,
    fixture_keys: ['principals'],
  },
  {
    id: 'GET /api/v1/internal/dev/diagnostics',
    family: 'platform',
    subgroup: 'developer-mode',
    name: 'developer diagnostics',
    method: 'GET',
    path: '/api/v1/internal/dev/diagnostics',
    auth: 'admin',
    readiness: 'diagnostic',
    execution_mode: 'diagnostic',
    expected_status: 200,
    fixture_keys: ['principals'],
  },
  {
    id: 'GET /api/v1/internal/dev/fixtures',
    family: 'platform',
    subgroup: 'developer-mode',
    name: 'developer fixtures',
    method: 'GET',
    path: '/api/v1/internal/dev/fixtures',
    auth: 'admin',
    readiness: 'diagnostic',
    execution_mode: 'diagnostic',
    expected_status: 200,
    fixture_keys: ['principals'],
  },
  {
    id: 'GET /api/v1/internal/dev/readiness-summary',
    family: 'platform',
    subgroup: 'developer-mode',
    name: 'developer readiness summary',
    method: 'GET',
    path: '/api/v1/internal/dev/readiness-summary',
    auth: 'admin',
    readiness: 'diagnostic',
    execution_mode: 'diagnostic',
    expected_status: 200,
    fixture_keys: ['principals'],
    manual_notes: 'Surface for operator-facing readiness and coverage diagnostics.',
  },
]

export function summarizeDeveloperReadinessEntry(entry: EndpointManifestEntry): DeveloperReadinessEndpointSummary {
  return {
    id: entry.id,
    family: entry.family,
    subgroup: entry.subgroup,
    name: entry.name,
    method: entry.method,
    path: entry.path,
    auth: entry.auth,
    readiness: entry.readiness,
    execution_mode: entry.execution_mode,
    fixture_keys: entry.fixture_keys || [],
  }
}

export function getDeveloperReadinessDiagnosticsRoutes() {
  return {
    public_health_endpoints: DEVELOPER_READINESS_MANIFEST
      .filter(entry => entry.subgroup === 'health')
      .map(entry => entry.path),
    internal_debug_endpoints: DEVELOPER_READINESS_MANIFEST
      .filter(entry => entry.subgroup === 'developer-mode')
      .map(entry => entry.path),
  }
}

export function buildDeveloperReadinessSummary(): Promise<DeveloperReadinessSummary> {
  const readiness_counts: Record<EndpointReadinessTier, number> = {
    verified: 0,
    seeded: 0,
    diagnostic: 0,
    manual: 0,
    inventory: 0,
  }
  const execution_mode_counts: Record<EndpointExecutionMode, number> = {
    automated: 0,
    'fixture-backed': 0,
    diagnostic: 0,
    manual: 0,
    inventory: 0,
  }
  const fixture_key_counts: Record<string, number> = {}
  const ready_endpoints: DeveloperReadinessEndpointSummary[] = []
  const manual_endpoints: DeveloperReadinessEndpointSummary[] = []
  const fixture_backed_endpoints: DeveloperReadinessEndpointSummary[] = []

  for (const entry of DEVELOPER_READINESS_MANIFEST) {
    const summary = summarizeDeveloperReadinessEntry(entry)

    readiness_counts[summary.readiness] += 1
    execution_mode_counts[summary.execution_mode] += 1

    if (summary.readiness === 'verified') {
      ready_endpoints.push(summary)
    }

    if (summary.readiness === 'manual' || summary.readiness === 'inventory') {
      manual_endpoints.push(summary)
    }

    if (summary.fixture_keys.length > 0) {
      fixture_backed_endpoints.push(summary)
      for (const key of summary.fixture_keys) {
        fixture_key_counts[key] = (fixture_key_counts[key] || 0) + 1
      }
    }
  }

  return Promise.resolve({
    manifest_total: DEVELOPER_READINESS_MANIFEST.length,
    readiness_counts,
    execution_mode_counts,
    ready_endpoints,
    manual_endpoints,
    fixture_coverage: {
      total_endpoints: fixture_backed_endpoints.length,
      fixture_key_counts,
      endpoints: fixture_backed_endpoints,
    },
  })
}
