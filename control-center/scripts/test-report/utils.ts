/**
 * Utility functions for test report generation
 */

import { execSync } from 'child_process';
import * as fs from 'fs';
import * as path from 'path';
import {
  TestReportMetadata,
  CoverageMetric,
  AggregatedMetrics,
  TestReportSummary,
  CoverageTrend,
  TrendEntry,
} from './types';

export const PROJECT_ROOT = path.resolve(__dirname, '../../..');
export const WORKERS_ROOT = path.resolve(PROJECT_ROOT, 'workers');
export const MOBILE_ROOT = path.resolve(PROJECT_ROOT, 'mobile');
export const WEB_ROOT = path.resolve(PROJECT_ROOT, 'web');
export const REPORT_DIR = path.resolve(PROJECT_ROOT, 'reports/test-reports');
export const TRENDS_FILE = path.resolve(REPORT_DIR, 'trends/coverage-trend.json');

export function getTimestamp(): string {
  return new Date().toISOString();
}

export function getTimestampFolder(): string {
  return new Date().toISOString().replace(/[:.]/g, '-').split('Z')[0];
}

export function getMetadata(): TestReportMetadata {
  const isCI = process.env.CI === 'true';
  let gitBranch = 'unknown';
  let gitCommit = 'unknown';
  let gitRef = 'unknown';

  try {
    gitBranch = execSync('git rev-parse --abbrev-ref HEAD', { encoding: 'utf-8' }).trim();
    gitCommit = execSync('git rev-parse HEAD', { encoding: 'utf-8' }).trim();
    gitRef = execSync('git describe --tags --always 2>/dev/null || echo "unknown"', {
      encoding: 'utf-8',
      shell: '/bin/bash',
    }).trim();
  } catch (e) {
    console.warn('Could not get git info:', e);
  }

  return {
    timestamp: getTimestamp(),
    gitCommit,
    gitBranch,
    gitRef,
    environment: isCI ? 'ci' : 'local',
    hostname: require('os').hostname(),
    platform: process.platform,
    nodeVersion: process.version,
  };
}

export function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`;
  const minutes = Math.floor(ms / 60000);
  const seconds = Math.round((ms % 60000) / 1000);
  return `${minutes}m ${seconds}s`;
}

export function calculatePercentage(covered: number, total: number): number {
  if (total === 0) return 0;
  return Math.round((covered / total) * 10000) / 100;
}

export function aggregateCoverage(metrics: CoverageMetric[]): CoverageMetric {
  const covered = metrics.reduce((sum, m) => sum + m.covered, 0);
  const total = metrics.reduce((sum, m) => sum + m.total, 0);
  return {
    covered,
    total,
    percentage: calculatePercentage(covered, total),
  };
}

export function ensureDirectory(dir: string): void {
  if (!fs.existsSync(dir)) {
    fs.mkdirSync(dir, { recursive: true });
  }
}

export function writeJson(filePath: string, data: unknown): void {
  ensureDirectory(path.dirname(filePath));
  fs.writeFileSync(filePath, JSON.stringify(data, null, 2));
}

export function readJson<T>(filePath: string, defaultValue: T): T {
  if (!fs.existsSync(filePath)) {
    return defaultValue;
  }
  try {
    return JSON.parse(fs.readFileSync(filePath, 'utf-8')) as T;
  } catch {
    return defaultValue;
  }
}

export function updateTrends(summary: TestReportSummary): void {
  const trends: CoverageTrend = readJson(TRENDS_FILE, {
    project: 'anixops-control-center',
    updated: getTimestamp(),
    data: [],
  });

  const entry: TrendEntry = {
    date: summary.metadata.timestamp.split('T')[0],
    timestamp: summary.metadata.timestamp,
    commit: summary.metadata.gitCommit,
    branch: summary.metadata.gitBranch,
    coverage: {
      lines: summary.aggregated.coverage.lines.percentage,
      branches: summary.aggregated.coverage.branches.percentage,
      functions: summary.aggregated.coverage.functions.percentage,
      statements: summary.aggregated.coverage.statements.percentage,
    },
    testCount: {
      total: summary.aggregated.testExecution.total,
      passed: summary.aggregated.testExecution.passed,
      failed: summary.aggregated.testExecution.failed,
    },
    projects: {},
  };

  // Add project-specific data
  for (const [name, project] of Object.entries(summary.projects)) {
    if (project) {
      entry.projects[name] = {
        lines: project.coverage?.lines?.percentage || 0,
        tests: project.testExecution.total,
      };
    }
  }

  trends.data.push(entry);

  // Keep only last 90 days
  if (trends.data.length > 90) {
    trends.data = trends.data.slice(-90);
  }

  trends.updated = getTimestamp();
  writeJson(TRENDS_FILE, trends);
}

export function getPreviousTrend(): TrendEntry | null {
  const trends: CoverageTrend = readJson(TRENDS_FILE, {
    project: 'anixops-control-center',
    updated: getTimestamp(),
    data: [],
  });

  if (trends.data.length < 2) {
    return null;
  }

  return trends.data[trends.data.length - 2];
}

export function parseVitestJson(jsonPath: string): {
  total: number;
  passed: number;
  failed: number;
  skipped: number;
  duration: number;
} {
  if (!fs.existsSync(jsonPath)) {
    return { total: 0, passed: 0, failed: 0, skipped: 0, duration: 0 };
  }

  try {
    const data = JSON.parse(fs.readFileSync(jsonPath, 'utf-8'));
    // Vitest JSON structure
    const results = data.testResults || [];
    let total = 0,
      passed = 0,
      failed = 0,
      skipped = 0,
      duration = 0;

    for (const file of results) {
      for (const test of file.assertionResults || []) {
        total++;
        duration += test.duration || 0;
        if (test.status === 'passed') passed++;
        else if (test.status === 'failed') failed++;
        else if (test.status === 'skipped') skipped++;
      }
    }

    return { total, passed, failed, skipped, duration };
  } catch {
    return { total: 0, passed: 0, failed: 0, skipped: 0, duration: 0 };
  }
}

export function parseCoverageSummary(jsonPath: string): {
  lines: CoverageMetric;
  branches: CoverageMetric;
  functions: CoverageMetric;
  statements: CoverageMetric;
} {
  if (!fs.existsSync(jsonPath)) {
    return {
      lines: { covered: 0, total: 0, percentage: 0 },
      branches: { covered: 0, total: 0, percentage: 0 },
      functions: { covered: 0, total: 0, percentage: 0 },
      statements: { covered: 0, total: 0, percentage: 0 },
    };
  }

  try {
    const data = JSON.parse(fs.readFileSync(jsonPath, 'utf-8'));
    const total = data.total || {};

    return {
      lines: {
        covered: total.lines?.covered || 0,
        total: total.lines?.total || 0,
        percentage: total.lines?.pct || 0,
      },
      branches: {
        covered: total.branches?.covered || 0,
        total: total.branches?.total || 0,
        percentage: total.branches?.pct || 0,
      },
      functions: {
        covered: total.functions?.covered || 0,
        total: total.functions?.total || 0,
        percentage: total.functions?.pct || 0,
      },
      statements: {
        covered: total.statements?.covered || 0,
        total: total.statements?.total || 0,
        percentage: total.statements?.pct || 0,
      },
    };
  } catch {
    return {
      lines: { covered: 0, total: 0, percentage: 0 },
      branches: { covered: 0, total: 0, percentage: 0 },
      functions: { covered: 0, total: 0, percentage: 0 },
      statements: { covered: 0, total: 0, percentage: 0 },
    };
  }
}