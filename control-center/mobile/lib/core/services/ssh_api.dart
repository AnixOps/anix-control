import 'package:dio/dio.dart';
import 'package:anixops_mobile/core/models/ssh_models.dart';

/// SSH API endpoints for server import
class SshApi {
  final Dio _dio;

  SshApi(this._dio);

  /// Test SSH connection
  Future<SshTestResponse> test({
    required String host,
    required int port,
    required String username,
    required String authType,
    String? password,
    String? privateKey,
    String? passphrase,
  }) async {
    final response = await _dio.post('/ssh/test', data: {
      'host': host,
      'port': port,
      'username': username,
      'auth_type': authType,
      if (password != null) 'password': password,
      if (privateKey != null) 'private_key': privateKey,
      if (passphrase != null) 'passphrase': passphrase,
    });
    return SshTestResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Test SSH connection with password (convenience method)
  Future<SshTestResponse> testWithPassword({
    required String host,
    required int port,
    required String username,
    required String password,
  }) async {
    return test(
      host: host,
      port: port,
      username: username,
      authType: 'password',
      password: password,
    );
  }

  /// Test SSH connection with private key (convenience method)
  Future<SshTestResponse> testWithKey({
    required String host,
    required int port,
    required String username,
    required String privateKey,
    String? passphrase,
  }) async {
    return test(
      host: host,
      port: port,
      username: username,
      authType: 'key',
      privateKey: privateKey,
      passphrase: passphrase,
    );
  }

  /// Import server via SSH
  Future<SshImportResponse> import({
    required String host,
    required int port,
    required String username,
    required String authType,
    String? password,
    String? privateKey,
    String? passphrase,
    String? name,
    String? group,
    List<String>? tags,
  }) async {
    final response = await _dio.post('/ssh/import', data: {
      'host': host,
      'port': port,
      'username': username,
      'auth_type': authType,
      if (password != null) 'password': password,
      if (privateKey != null) 'private_key': privateKey,
      if (passphrase != null) 'passphrase': passphrase,
      if (name != null) 'name': name,
      if (group != null) 'group': group,
      if (tags != null) 'tags': tags,
    });
    return SshImportResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Detect server type via SSH
  Future<SshDetectResponse> detect({
    required String host,
    required int port,
    required String username,
    required String authType,
    String? password,
    String? privateKey,
    String? passphrase,
  }) async {
    final response = await _dio.post('/ssh/detect', data: {
      'host': host,
      'port': port,
      'username': username,
      'auth_type': authType,
      if (password != null) 'password': password,
      if (privateKey != null) 'private_key': privateKey,
      if (passphrase != null) 'passphrase': passphrase,
    });
    return SshDetectResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Execute command via SSH
  Future<SshExecuteResponse> execute({
    required String nodeId,
    required String command,
  }) async {
    final response = await _dio.post('/nodes/$nodeId/execute', data: {
      'command': command,
    });
    return SshExecuteResponse.fromJson(response.data as Map<String, dynamic>);
  }
}