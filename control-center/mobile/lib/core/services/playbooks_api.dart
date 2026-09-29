import 'package:dio/dio.dart';
import 'package:anixops_mobile/core/models/playbook_models.dart';

/// Playbooks API endpoints
class PlaybooksApi {
  final Dio _dio;

  PlaybooksApi(this._dio);

  /// Get all playbooks
  Future<PlaybookListResponse> list({String? category, String? source}) async {
    final queryParams = <String, String>{};
    if (category != null) queryParams['category'] = category;
    if (source != null) queryParams['source'] = source;

    final response = await _dio.get('/playbooks', queryParameters: queryParams);
    return PlaybookListResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Get single playbook
  Future<PlaybookDetailResponse> get(String name) async {
    final response = await _dio.get('/playbooks/$name');
    return PlaybookDetailResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Create playbook
  Future<PlaybookDetailResponse> create({
    required String name,
    required String storageKey,
    String? description,
    String? category,
    String? source,
    Map<String, dynamic>? variables,
  }) async {
    final response = await _dio.post('/playbooks', data: {
      'name': name,
      'storage_key': storageKey,
      if (description != null) 'description': description,
      if (category != null) 'category': category,
      if (source != null) 'source': source,
      if (variables != null) 'variables': variables,
    });
    return PlaybookDetailResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Delete playbook
  Future<void> delete(String name) async {
    await _dio.delete('/playbooks/$name');
  }

  /// Run playbook
  Future<Map<String, dynamic>> run(PlaybookRunRequest request) async {
    final response = await _dio.post('/playbooks/run', data: request.toJson());
    return response.data as Map<String, dynamic>;
  }
}