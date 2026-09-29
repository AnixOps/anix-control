// Dashboard models matching the backend contract
// See: control-center/workers/src/types.ts

import 'package:anixops_mobile/core/models/api_response.dart';

/// Health response
class HealthResponse {
  final String status;
  final String version;
  final String buildSha;
  final String timestamp;
  final String environment;

  HealthResponse({
    required this.status,
    required this.version,
    required this.buildSha,
    required this.timestamp,
    required this.environment,
  });

  factory HealthResponse.fromJson(Map<String, dynamic> json) {
    return HealthResponse(
      status: json['status'] as String,
      version: json['version'] as String,
      buildSha: json['build_sha'] as String,
      timestamp: json['timestamp'] as String,
      environment: json['environment'] as String,
    );
  }
}

/// Runtime service health check
class RuntimeServiceHealthCheck {
  final String name;
  final String status;
  final int latency;
  final String? message;
  final String lastCheck;

  RuntimeServiceHealthCheck({
    required this.name,
    required this.status,
    required this.latency,
    this.message,
    required this.lastCheck,
  });

  factory RuntimeServiceHealthCheck.fromJson(Map<String, dynamic> json) {
    return RuntimeServiceHealthCheck(
      name: json['name'] as String,
      status: json['status'] as String,
      latency: json['latency'] as int,
      message: json['message'] as String?,
      lastCheck: json['lastCheck'] as String,
    );
  }
}

/// Runtime service checks
class RuntimeServiceChecks {
  final RuntimeServiceHealthCheck database;
  final RuntimeServiceHealthCheck kv;
  final RuntimeServiceHealthCheck r2;

  RuntimeServiceChecks({
    required this.database,
    required this.kv,
    required this.r2,
  });

  factory RuntimeServiceChecks.fromJson(Map<String, dynamic> json) {
    return RuntimeServiceChecks(
      database: RuntimeServiceHealthCheck.fromJson(
        json['database'] as Map<String, dynamic>,
      ),
      kv: RuntimeServiceHealthCheck.fromJson(
        json['kv'] as Map<String, dynamic>,
      ),
      r2: RuntimeServiceHealthCheck.fromJson(
        json['r2'] as Map<String, dynamic>,
      ),
    );
  }
}

/// Readiness response
class ReadinessResponse {
  final String status;
  final String version;
  final String buildSha;
  final RuntimeServiceChecks checks;
  final String timestamp;

  ReadinessResponse({
    required this.status,
    required this.version,
    required this.buildSha,
    required this.checks,
    required this.timestamp,
  });

  factory ReadinessResponse.fromJson(Map<String, dynamic> json) {
    return ReadinessResponse(
      status: json['status'] as String,
      version: json['version'] as String,
      buildSha: json['build_sha'] as String,
      checks: RuntimeServiceChecks.fromJson(
        json['checks'] as Map<String, dynamic>,
      ),
      timestamp: json['timestamp'] as String,
    );
  }
}

/// Dashboard overview node summary
class DashboardNodeSummary {
  final int total;
  final int online;
  final int offline;
  final int maintenance;

  DashboardNodeSummary({
    required this.total,
    required this.online,
    required this.offline,
    required this.maintenance,
  });

  factory DashboardNodeSummary.fromJson(Map<String, dynamic> json) {
    return DashboardNodeSummary(
      total: json['total'] as int,
      online: json['online'] as int,
      offline: json['offline'] as int,
      maintenance: json['maintenance'] as int,
    );
  }
}

/// Dashboard overview task summary
class DashboardTaskSummary {
  final int total;
  final int pending;
  final int running;
  final int completed;
  final int failed;

  DashboardTaskSummary({
    required this.total,
    required this.pending,
    required this.running,
    required this.completed,
    required this.failed,
  });

  factory DashboardTaskSummary.fromJson(Map<String, dynamic> json) {
    return DashboardTaskSummary(
      total: json['total'] as int,
      pending: json['pending'] as int,
      running: json['running'] as int,
      completed: json['completed'] as int,
      failed: json['failed'] as int,
    );
  }
}

/// Dashboard overview schedule summary
class DashboardScheduleSummary {
  final int total;
  final int enabled;
  final int disabled;

  DashboardScheduleSummary({
    required this.total,
    required this.enabled,
    required this.disabled,
  });

  factory DashboardScheduleSummary.fromJson(Map<String, dynamic> json) {
    return DashboardScheduleSummary(
      total: json['total'] as int,
      enabled: json['enabled'] as int,
      disabled: json['disabled'] as int,
    );
  }
}

/// Dashboard overview response data
class DashboardOverviewResponseData {
  final DashboardNodeSummary nodes;
  final DashboardTaskSummary tasks;
  final DashboardScheduleSummary schedules;
  final int users;
  final int playbooks;
  final int auditLogs;

  DashboardOverviewResponseData({
    required this.nodes,
    required this.tasks,
    required this.schedules,
    required this.users,
    required this.playbooks,
    required this.auditLogs,
  });

  factory DashboardOverviewResponseData.fromJson(Map<String, dynamic> json) {
    return DashboardOverviewResponseData(
      nodes: DashboardNodeSummary.fromJson(
        json['nodes'] as Map<String, dynamic>,
      ),
      tasks: DashboardTaskSummary.fromJson(
        json['tasks'] as Map<String, dynamic>,
      ),
      schedules: DashboardScheduleSummary.fromJson(
        json['schedules'] as Map<String, dynamic>,
      ),
      users: json['users'] as int,
      playbooks: json['playbooks'] as int,
      auditLogs: json['audit_logs'] as int,
    );
  }
}

/// Dashboard overview response
class DashboardOverviewResponse
    extends ApiSuccessResponse<DashboardOverviewResponseData> {
  DashboardOverviewResponse({required super.data});

  factory DashboardOverviewResponse.fromJson(Map<String, dynamic> json) {
    return DashboardOverviewResponse(
      data: DashboardOverviewResponseData.fromJson(
        json['data'] as Map<String, dynamic>,
      ),
    );
  }
}

/// Dashboard panel type
enum DashboardPanelType { line, bar, pie, stat, table }

/// Dashboard panel
class DashboardPanel {
  final String id;
  final String title;
  final DashboardPanelType type;
  final List<String> metrics;
  final int width;
  final int height;
  final int x;
  final int y;

  DashboardPanel({
    required this.id,
    required this.title,
    required this.type,
    required this.metrics,
    required this.width,
    required this.height,
    required this.x,
    required this.y,
  });

  factory DashboardPanel.fromJson(Map<String, dynamic> json) {
    return DashboardPanel(
      id: json['id'] as String,
      title: json['title'] as String,
      type: DashboardPanelType.values.firstWhere(
        (e) => e.name == json['type'],
        orElse: () => DashboardPanelType.stat,
      ),
      metrics: (json['metrics'] as List).map((e) => e as String).toList(),
      width: json['width'] as int,
      height: json['height'] as int,
      x: json['x'] as int,
      y: json['y'] as int,
    );
  }
}

/// Dashboard config
class DashboardConfig {
  final String id;
  final String name;
  final List<DashboardPanel> panels;
  final int refreshInterval;
  final String timeRange;

  DashboardConfig({
    required this.id,
    required this.name,
    required this.panels,
    required this.refreshInterval,
    required this.timeRange,
  });

  factory DashboardConfig.fromJson(Map<String, dynamic> json) {
    return DashboardConfig(
      id: json['id'] as String,
      name: json['name'] as String,
      panels: (json['panels'] as List)
          .map((e) => DashboardPanel.fromJson(e as Map<String, dynamic>))
          .toList(),
      refreshInterval: json['refreshInterval'] as int,
      timeRange: json['timeRange'] as String,
    );
  }
}