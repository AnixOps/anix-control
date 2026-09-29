// Schedule models matching the backend contract
// See: control-center/workers/src/types.ts

import 'package:anixops_mobile/core/models/api_response.dart';

/// Schedule entity
class Schedule {
  final int id;
  final String name;
  final int playbookId;
  final String playbookName;
  final String cron;
  final String? timezone;
  final String? targetNodes;
  final String? variables;
  final bool enabled;
  final String? lastRun;
  final String? nextRun;
  final String? lastTaskId;
  final int? createdBy;
  final String createdAt;
  final String updatedAt;

  Schedule({
    required this.id,
    required this.name,
    required this.playbookId,
    required this.playbookName,
    required this.cron,
    this.timezone,
    this.targetNodes,
    this.variables,
    required this.enabled,
    this.lastRun,
    this.nextRun,
    this.lastTaskId,
    this.createdBy,
    required this.createdAt,
    required this.updatedAt,
  });

  factory Schedule.fromJson(Map<String, dynamic> json) {
    return Schedule(
      id: json['id'] as int,
      name: json['name'] as String,
      playbookId: json['playbook_id'] as int,
      playbookName: json['playbook_name'] as String,
      cron: json['cron'] as String,
      timezone: json['timezone'] as String?,
      targetNodes: json['target_nodes'] as String?,
      variables: json['variables'] as String?,
      enabled: json['enabled'] as bool,
      lastRun: json['last_run'] as String?,
      nextRun: json['next_run'] as String?,
      lastTaskId: json['last_task_id'] as String?,
      createdBy: json['created_by'] as int?,
      createdAt: json['created_at'] as String,
      updatedAt: json['updated_at'] as String,
    );
  }
}

/// Schedule list response data
class ScheduleListResponseData {
  final List<Schedule> items;
  final int total;
  final int page;
  final int perPage;
  final int totalPages;

  ScheduleListResponseData({
    required this.items,
    required this.total,
    required this.page,
    required this.perPage,
    required this.totalPages,
  });

  factory ScheduleListResponseData.fromJson(Map<String, dynamic> json) {
    return ScheduleListResponseData(
      items: (json['items'] as List)
          .map((e) => Schedule.fromJson(e as Map<String, dynamic>))
          .toList(),
      total: json['total'] as int,
      page: json['page'] as int,
      perPage: json['per_page'] as int,
      totalPages: json['total_pages'] as int,
    );
  }
}

/// Schedule create/update request
class ScheduleRequest {
  final String name;
  final int playbookId;
  final String cron;
  final String? timezone;
  final String? targetNodes;
  final Map<String, dynamic>? variables;
  final bool enabled;

  ScheduleRequest({
    required this.name,
    required this.playbookId,
    required this.cron,
    this.timezone,
    this.targetNodes,
    this.variables,
    this.enabled = true,
  });

  Map<String, dynamic> toJson() => {
        'name': name,
        'playbook_id': playbookId,
        'cron': cron,
        if (timezone != null) 'timezone': timezone,
        if (targetNodes != null) 'target_nodes': targetNodes,
        if (variables != null) 'variables': variables,
        'enabled': enabled,
      };
}

/// Response types
class ScheduleListResponse extends ApiSuccessResponse<ScheduleListResponseData> {
  ScheduleListResponse({required super.data});

  factory ScheduleListResponse.fromJson(Map<String, dynamic> json) {
    return ScheduleListResponse(
      data: ScheduleListResponseData.fromJson(
        json['data'] as Map<String, dynamic>,
      ),
    );
  }
}

class ScheduleDetailResponse extends ApiSuccessResponse<Schedule> {
  ScheduleDetailResponse({required super.data});

  factory ScheduleDetailResponse.fromJson(Map<String, dynamic> json) {
    return ScheduleDetailResponse(
      data: Schedule.fromJson(json['data'] as Map<String, dynamic>),
    );
  }
}