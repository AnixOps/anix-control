import 'package:dio/dio.dart';
import 'package:anixops_mobile/core/models/task_models.dart';

/// Tasks API endpoints
class TasksApi {
  final Dio _dio;

  TasksApi(this._dio);

  /// Get all tasks
  Future<TaskListResponse> list({
    String? status,
    int? playbookId,
    int page = 1,
    int perPage = 20,
  }) async {
    final queryParams = <String, dynamic>{
      'page': page,
      'per_page': perPage,
    };
    if (status != null) queryParams['status'] = status;
    if (playbookId != null) queryParams['playbook_id'] = playbookId;

    final response = await _dio.get('/tasks', queryParameters: queryParams);
    return TaskListResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Get single task
  Future<TaskDetailResponse> get(String taskId) async {
    final response = await _dio.get('/tasks/$taskId');
    return TaskDetailResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Create task (run playbook)
  Future<TaskCreateResponse> create({
    required int playbookId,
    required List<int> targetNodeIds,
    Map<String, dynamic>? variables,
  }) async {
    final response = await _dio.post('/tasks', data: {
      'playbook_id': playbookId,
      'target_node_ids': targetNodeIds,
      if (variables != null) 'variables': variables,
    });
    return TaskCreateResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Cancel task
  Future<void> cancel(String taskId) async {
    await _dio.post('/tasks/$taskId/cancel');
  }

  /// Retry task
  Future<TaskRetryResponse> retry(String taskId) async {
    final response = await _dio.post('/tasks/$taskId/retry');
    return TaskRetryResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Get task logs
  Future<TaskLogsResponse> logs(
    String taskId, {
    String level = 'all',
    int limit = 1000,
    int offset = 0,
  }) async {
    final response = await _dio.get('/tasks/$taskId/logs', queryParameters: {
      'level': level,
      'limit': limit,
      'offset': offset,
    });
    return TaskLogsResponse.fromJson(response.data as Map<String, dynamic>);
  }
}