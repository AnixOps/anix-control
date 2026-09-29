import 'package:dio/dio.dart';
import 'package:anixops_mobile/core/models/node_models.dart';

/// Nodes API endpoints
class NodesApi {
  final Dio _dio;

  NodesApi(this._dio);

  /// List all nodes with optional filters
  Future<NodeSummaryResponse> list({
    String? search,
    String? status,
    String? type,
    int page = 1,
    int limit = 20,
  }) async {
    final response = await _dio.get('/nodes', queryParameters: {
      if (search != null) 'search': search,
      if (status != null) 'status': status,
      if (type != null) 'type': type,
      'page': page,
      'limit': limit,
    });
    return NodeSummaryResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Get a single node by ID
  Future<NodeGetResponse> get(String id) async {
    final response = await _dio.get('/nodes/$id');
    return NodeGetResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Create a new node
  Future<NodeGetResponse> create(Map<String, dynamic> data) async {
    final response = await _dio.post('/nodes', data: data);
    return NodeGetResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Update a node
  Future<NodeGetResponse> update(String id, Map<String, dynamic> data) async {
    final response = await _dio.put('/nodes/$id', data: data);
    return NodeGetResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Delete a node
  Future<void> delete(String id) async {
    await _dio.delete('/nodes/$id');
  }

  /// Start a node
  Future<void> start(String id) async {
    await _dio.post('/nodes/$id/start');
  }

  /// Stop a node
  Future<void> stop(String id) async {
    await _dio.post('/nodes/$id/stop');
  }

  /// Restart a node
  Future<void> restart(String id) async {
    await _dio.post('/nodes/$id/restart');
  }

  /// Get node statistics
  Future<NodeStatsResponse> stats(String id) async {
    final response = await _dio.get('/nodes/$id/stats');
    return NodeStatsResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Get node logs
  Future<NodeLogsResponse> logs(String id, {int limit = 100, String? level}) async {
    final response = await _dio.get('/nodes/$id/logs', queryParameters: {
      'limit': limit,
      if (level != null) 'level': level,
    });
    return NodeLogsResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Test node connection
  Future<void> testConnection(String id) async {
    await _dio.post('/nodes/$id/test');
  }

  /// Sync node configuration
  Future<void> sync(String id) async {
    await _dio.post('/nodes/$id/sync');
  }

  /// Bulk operations on nodes
  Future<NodeBulkActionResponse> bulkAction({
    required List<String> nodeIds,
    required String action, // start, stop, restart, delete
  }) async {
    final response = await _dio.post('/nodes/bulk', data: {
      'node_ids': nodeIds,
      'action': action,
    });
    return NodeBulkActionResponse.fromJson(response.data as Map<String, dynamic>);
  }
}