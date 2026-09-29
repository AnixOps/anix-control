import 'package:dio/dio.dart';
import 'package:anixops_mobile/core/models/schedule_models.dart';

/// Schedules API endpoints
class SchedulesApi {
  final Dio _dio;

  SchedulesApi(this._dio);

  /// Get all schedules
  Future<ScheduleListResponse> list() async {
    final response = await _dio.get('/schedules');
    return ScheduleListResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Get single schedule
  Future<ScheduleDetailResponse> get(int id) async {
    final response = await _dio.get('/schedules/$id');
    return ScheduleDetailResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Create schedule
  Future<ScheduleDetailResponse> create(ScheduleRequest request) async {
    final response = await _dio.post('/schedules', data: request.toJson());
    return ScheduleDetailResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Update schedule
  Future<ScheduleDetailResponse> update(int id, ScheduleRequest request) async {
    final response = await _dio.put('/schedules/$id', data: request.toJson());
    return ScheduleDetailResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Delete schedule
  Future<void> delete(int id) async {
    await _dio.delete('/schedules/$id');
  }

  /// Toggle schedule enabled
  Future<bool> toggle(int id) async {
    final response = await _dio.post('/schedules/$id/toggle');
    return (response.data['data'] as Map<String, dynamic>)['enabled'] as bool;
  }

  /// Run schedule now
  Future<String?> runNow(int id) async {
    final response = await _dio.post('/schedules/$id/run');
    return (response.data['data'] as Map<String, dynamic>)['task_id'] as String?;
  }
}