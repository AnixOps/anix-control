// Task models matching the backend contract
// See: control-center/workers/src/types.ts

import 'package:anixops_mobile/core/models/api_response.dart';

/// Task status type
enum TaskStatus { pending, running, success, failed, cancelled }

/// Task trigger type
enum TaskTriggerType { manual, scheduled, webhook, api }

/// Task log level
enum TaskLogLevel { debug, info, warning, error }

/// Task entity
class Task {
  final int id;
  final String taskId;
  final int playbookId;
  final String playbookName;
  final TaskStatus status;
  final TaskTriggerType triggerType;
  final int? triggeredBy;
  final String? targetNodes;
  final String? variables;
  final String? result;
  final String? error;
  final String? startedAt;
  final String? completedAt;
  final String createdAt;

  Task({
    required this.id,
    required this.taskId,
    required this.playbookId,
    required this.playbookName,
    required this.status,
    required this.triggerType,
    this.triggeredBy,
    this.targetNodes,
    this.variables,
    this.result,
    this.error,
    this.startedAt,
    this.completedAt,
    required this.createdAt,
  });

  factory Task.fromJson(Map<String, dynamic> json) {
    return Task(
      id: json['id'] as int,
      taskId: json['task_id'] as String,
      playbookId: json['playbook_id'] as int,
      playbookName: json['playbook_name'] as String,
      status: TaskStatus.values.firstWhere(
        (e) => e.name == json['status'],
        orElse: () => TaskStatus.pending,
      ),
      triggerType: TaskTriggerType.values.firstWhere(
        (e) => e.name == json['trigger_type'],
        orElse: () => TaskTriggerType.manual,
      ),
      triggeredBy: json['triggered_by'] as int?,
      targetNodes: json['target_nodes'] as String?,
      variables: json['variables'] as String?,
      result: json['result'] as String?,
      error: json['error'] as String?,
      startedAt: json['started_at'] as String?,
      completedAt: json['completed_at'] as String?,
      createdAt: json['created_at'] as String,
    );
  }
}

/// Task log entry
class TaskLog {
  final int id;
  final String taskId;
  final int? nodeId;
  final String? nodeName;
  final TaskLogLevel level;
  final String message;
  final String? metadata;
  final String createdAt;

  TaskLog({
    required this.id,
    required this.taskId,
    this.nodeId,
    this.nodeName,
    required this.level,
    required this.message,
    this.metadata,
    required this.createdAt,
  });

  factory TaskLog.fromJson(Map<String, dynamic> json) {
    return TaskLog(
      id: json['id'] as int,
      taskId: json['task_id'] as String,
      nodeId: json['node_id'] as int?,
      nodeName: json['node_name'] as String?,
      level: TaskLogLevel.values.firstWhere(
        (e) => e.name == json['level'],
        orElse: () => TaskLogLevel.info,
      ),
      message: json['message'] as String,
      metadata: json['metadata'] as String?,
      createdAt: json['created_at'] as String,
    );
  }
}

/// Task list item with additional fields
class TaskListItem extends Task {
  final String? category;
  final String? triggeredByEmail;

  TaskListItem({
    required super.id,
    required super.taskId,
    required super.playbookId,
    required super.playbookName,
    required super.status,
    required super.triggerType,
    super.triggeredBy,
    super.targetNodes,
    super.variables,
    super.result,
    super.error,
    super.startedAt,
    super.completedAt,
    required super.createdAt,
    this.category,
    this.triggeredByEmail,
  });

  factory TaskListItem.fromJson(Map<String, dynamic> json) {
    return TaskListItem(
      id: json['id'] as int,
      taskId: json['task_id'] as String,
      playbookId: json['playbook_id'] as int,
      playbookName: json['playbook_name'] as String,
      status: TaskStatus.values.firstWhere(
        (e) => e.name == json['status'],
        orElse: () => TaskStatus.pending,
      ),
      triggerType: TaskTriggerType.values.firstWhere(
        (e) => e.name == json['trigger_type'],
        orElse: () => TaskTriggerType.manual,
      ),
      triggeredBy: json['triggered_by'] as int?,
      targetNodes: json['target_nodes'] as String?,
      variables: json['variables'] as String?,
      result: json['result'] as String?,
      error: json['error'] as String?,
      startedAt: json['started_at'] as String?,
      completedAt: json['completed_at'] as String?,
      createdAt: json['created_at'] as String,
      category: json['category'] as String?,
      triggeredByEmail: json['triggered_by_email'] as String?,
    );
  }
}

/// Task list response data
class TaskListResponseData {
  final List<TaskListItem> items;
  final int total;
  final int page;
  final int perPage;
  final int totalPages;

  TaskListResponseData({
    required this.items,
    required this.total,
    required this.page,
    required this.perPage,
    required this.totalPages,
  });

  factory TaskListResponseData.fromJson(Map<String, dynamic> json) {
    return TaskListResponseData(
      items: (json['items'] as List)
          .map((e) => TaskListItem.fromJson(e as Map<String, dynamic>))
          .toList(),
      total: json['total'] as int,
      page: json['page'] as int,
      perPage: json['per_page'] as int,
      totalPages: json['total_pages'] as int,
    );
  }
}

/// Task detail response data
class TaskDetailResponseData extends TaskListItem {
  final String? playbookVariables;

  TaskDetailResponseData({
    required super.id,
    required super.taskId,
    required super.playbookId,
    required super.playbookName,
    required super.status,
    required super.triggerType,
    super.triggeredBy,
    super.targetNodes,
    super.variables,
    super.result,
    super.error,
    super.startedAt,
    super.completedAt,
    required super.createdAt,
    super.category,
    super.triggeredByEmail,
    this.playbookVariables,
  });

  factory TaskDetailResponseData.fromJson(Map<String, dynamic> json) {
    return TaskDetailResponseData(
      id: json['id'] as int,
      taskId: json['task_id'] as String,
      playbookId: json['playbook_id'] as int,
      playbookName: json['playbook_name'] as String,
      status: TaskStatus.values.firstWhere(
        (e) => e.name == json['status'],
        orElse: () => TaskStatus.pending,
      ),
      triggerType: TaskTriggerType.values.firstWhere(
        (e) => e.name == json['trigger_type'],
        orElse: () => TaskTriggerType.manual,
      ),
      triggeredBy: json['triggered_by'] as int?,
      targetNodes: json['target_nodes'] as String?,
      variables: json['variables'] as String?,
      result: json['result'] as String?,
      error: json['error'] as String?,
      startedAt: json['started_at'] as String?,
      completedAt: json['completed_at'] as String?,
      createdAt: json['created_at'] as String,
      category: json['category'] as String?,
      triggeredByEmail: json['triggered_by_email'] as String?,
      playbookVariables: json['playbook_variables'] as String?,
    );
  }
}

/// Task create response data
class TaskCreateResponseData {
  final String taskId;
  final TaskStatus status;
  final String message;

  TaskCreateResponseData({
    required this.taskId,
    required this.status,
    required this.message,
  });

  factory TaskCreateResponseData.fromJson(Map<String, dynamic> json) {
    return TaskCreateResponseData(
      taskId: json['task_id'] as String,
      status: TaskStatus.pending,
      message: json['message'] as String,
    );
  }
}

/// Task retry response data
class TaskRetryResponseData {
  final String taskId;
  final TaskStatus status;
  final String message;

  TaskRetryResponseData({
    required this.taskId,
    required this.status,
    required this.message,
  });

  factory TaskRetryResponseData.fromJson(Map<String, dynamic> json) {
    return TaskRetryResponseData(
      taskId: json['task_id'] as String,
      status: TaskStatus.pending,
      message: json['message'] as String,
    );
  }
}

/// Response types
class TaskListResponse extends ApiSuccessResponse<TaskListResponseData> {
  TaskListResponse({required super.data});

  factory TaskListResponse.fromJson(Map<String, dynamic> json) {
    return TaskListResponse(
      data: TaskListResponseData.fromJson(
        json['data'] as Map<String, dynamic>,
      ),
    );
  }
}

class TaskDetailResponse extends ApiSuccessResponse<TaskDetailResponseData> {
  TaskDetailResponse({required super.data});

  factory TaskDetailResponse.fromJson(Map<String, dynamic> json) {
    return TaskDetailResponse(
      data: TaskDetailResponseData.fromJson(
        json['data'] as Map<String, dynamic>,
      ),
    );
  }
}

class TaskCreateResponse extends ApiSuccessResponse<TaskCreateResponseData> {
  TaskCreateResponse({required super.data});

  factory TaskCreateResponse.fromJson(Map<String, dynamic> json) {
    return TaskCreateResponse(
      data: TaskCreateResponseData.fromJson(
        json['data'] as Map<String, dynamic>,
      ),
    );
  }
}

class TaskRetryResponse extends ApiSuccessResponse<TaskRetryResponseData> {
  TaskRetryResponse({required super.data});

  factory TaskRetryResponse.fromJson(Map<String, dynamic> json) {
    return TaskRetryResponse(
      data: TaskRetryResponseData.fromJson(
        json['data'] as Map<String, dynamic>,
      ),
    );
  }
}

class TaskLogsResponse extends ApiSuccessResponse<List<TaskLog>> {
  TaskLogsResponse({required super.data});

  factory TaskLogsResponse.fromJson(Map<String, dynamic> json) {
    return TaskLogsResponse(
      data: (json['data'] as List)
          .map((e) => TaskLog.fromJson(e as Map<String, dynamic>))
          .toList(),
    );
  }
}