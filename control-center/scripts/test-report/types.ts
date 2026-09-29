/**
 * Test Report Types
 * Defines the schema for test reports across all projects
 */

export interface TestReportMetadata {
  timestamp: string;
  gitCommit: string;
  gitBranch: string;
  gitRef: string;
  environment: 'local' | 'ci';
  hostname: string;
  platform: string;
  nodeVersion?: string;
  goVersion?: string;
  flutterVersion?: string;
}

export interface CoverageMetric {
  covered: number;
  total: number;
  percentage: number;
}

export interface CoverageThresholds {
  lines: number;
  branches: number;
  functions: number;
  statements: number;
}

export interface FailedTest {
  name: string;
  file: string;
  error: string;
  duration: number;
}

export interface AnalysisIssue {
  file: string;
  line: number;
  column: number;
  severity: 'error' | 'warning' | 'info';
  message: string;
  rule: string;
}

export interface TestExecution {
  total: number;
  passed: number;
  failed: number;
  skipped: number;
  duration: number; // milliseconds
  failedTests: FailedTest[];
}

export interface ProjectReport {
  name: string;
  path: string;
  testExecution: TestExecution;
  coverage?: {
    lines: CoverageMetric;
    branches: CoverageMetric;
    functions: CoverageMetric;
    statements: CoverageMetric;
    thresholdMet: boolean;
  };
  staticAnalysis?: {
    tool: string;
    errors: number;
    warnings: number;
    info: number;
    issues: AnalysisIssue[];
  };
  rawOutputPath: string;
}

export interface AggregatedMetrics {
  testExecution: {
    total: number;
    passed: number;
    failed: number;
    skipped: number;
    duration: number;
  };
  coverage: {
    lines: CoverageMetric;
    branches: CoverageMetric;
    functions: CoverageMetric;
    statements: CoverageMetric;
  };
  staticAnalysis: {
    errors: number;
    warnings: number;
    info: number;
  };
}

export interface TrendData {
  coverageChange: number;
  testCountChange: number;
  previousReport: string | null;
}

export interface TestReportSummary {
  metadata: TestReportMetadata;
  projects: {
    'backend-go'?: ProjectReport;
    'backend-workers'?: ProjectReport;
    'mobile-flutter'?: ProjectReport;
    'web-vue'?: ProjectReport;
  };
  aggregated: AggregatedMetrics;
  trends: TrendData;
  thresholds: CoverageThresholds;
  passed: boolean;
}

export interface TrendEntry {
  date: string;
  timestamp: string;
  commit: string;
  branch: string;
  coverage: {
    lines: number;
    branches: number;
    functions: number;
    statements: number;
  };
  testCount: {
    total: number;
    passed: number;
    failed: number;
  };
  projects: Record<string, { lines: number; tests: number }>;
}

export interface CoverageTrend {
  project: string;
  updated: string;
  data: TrendEntry[];
}