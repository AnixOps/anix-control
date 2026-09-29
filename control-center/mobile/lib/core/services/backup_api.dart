import 'package:dio/dio.dart';
import 'package:anixops_mobile/core/models/backup_models.dart';
import 'package:anixops_mobile/core/models/api_response.dart';

/// Backup API service
class BackupApi {
  final Dio _dio;

  BackupApi(this._dio);

  /// List backups
  Future<BackupListResponse> list({
    int limit = 50,
    int page = 1,
  }) async {
    final response = await _dio.get('/backups', queryParameters: {
      'limit': limit,
      'page': page,
    });
    return BackupListResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Get backup status
  Future<BackupStatusResponse> status() async {
    final response = await _dio.get('/backups/status');
    return BackupStatusResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Create backup
  Future<BackupDetailResponse> create({
    String? name,
    String? description,
  }) async {
    final response = await _dio.post('/backups', data: {
      if (name != null) 'name': name,
      if (description != null) 'description': description,
    });
    return BackupDetailResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Get backup details
  Future<BackupDetailResponse> get(int id) async {
    final response = await _dio.get('/backups/$id');
    return BackupDetailResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Restore from backup
  Future<ApiMessageResponse> restore(int id) async {
    final response = await _dio.post('/backups/$id/restore');
    return ApiMessageResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Delete backup
  Future<ApiMessageResponse> delete(int id) async {
    final response = await _dio.delete('/backups/$id');
    return ApiMessageResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Cleanup old backups
  Future<BackupCleanupResponse> cleanup({int keepLast = 10}) async {
    final response = await _dio.post('/backups/cleanup', data: {
      'keep_last': keepLast,
    });
    return BackupCleanupResponse.fromJson(response.data as Map<String, dynamic>);
  }
}