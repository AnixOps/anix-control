import 'package:dio/dio.dart';
import 'package:anixops_mobile/core/models/plugin_models.dart';
import 'package:anixops_mobile/core/models/api_response.dart';

/// Plugins API endpoints
class PluginsApi {
  final Dio _dio;

  PluginsApi(this._dio);

  /// List all plugins
  Future<PluginListResponse> list() async {
    final response = await _dio.get('/plugins');
    return PluginListResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Get a single plugin by name
  Future<PluginDetailResponse> get(String name) async {
    final response = await _dio.get('/plugins/$name');
    return PluginDetailResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Execute a plugin action
  Future<PluginExecuteResponse> execute(String name, String action, {Map<String, dynamic>? params}) async {
    final response = await _dio.post('/plugins/$name/execute', data: {
      'action': action,
      'params': params ?? {},
    });
    return PluginExecuteResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Get plugin status
  Future<PluginStatusResponse> status(String name) async {
    final response = await _dio.get('/plugins/$name/status');
    return PluginStatusResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Get plugin configuration
  Future<Map<String, dynamic>> config(String name) async {
    final response = await _dio.get('/plugins/$name/config');
    return response.data['data'] as Map<String, dynamic>;
  }

  /// Update plugin configuration
  Future<ApiMessageResponse> updateConfig(String name, Map<String, dynamic> config) async {
    final response = await _dio.put('/plugins/$name/config', data: config);
    return ApiMessageResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Enable a plugin
  Future<ApiMessageResponse> enable(String name) async {
    final response = await _dio.post('/plugins/$name/enable');
    return ApiMessageResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Disable a plugin
  Future<ApiMessageResponse> disable(String name) async {
    final response = await _dio.post('/plugins/$name/disable');
    return ApiMessageResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Start a plugin
  Future<ApiMessageResponse> start(String name) async {
    final response = await _dio.post('/plugins/$name/start');
    return ApiMessageResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Stop a plugin
  Future<ApiMessageResponse> stop(String name) async {
    final response = await _dio.post('/plugins/$name/stop');
    return ApiMessageResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Restart a plugin
  Future<ApiMessageResponse> restart(String name) async {
    final response = await _dio.post('/plugins/$name/restart');
    return ApiMessageResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Get plugin logs
  Future<List<String>> logs(String name, {int limit = 100}) async {
    final response = await _dio.get('/plugins/$name/logs', queryParameters: {'limit': limit});
    return (response.data['data'] as List).map((e) => e as String).toList();
  }
}