import 'package:dio/dio.dart';
import 'package:anixops_mobile/core/models/user_models.dart';
import 'package:anixops_mobile/core/models/api_response.dart';

/// Users API endpoints
class UsersApi {
  final Dio _dio;

  UsersApi(this._dio);

  /// List all users with optional filters
  Future<UserListResponse> list({
    String? search,
    String? role,
    String? status,
    int page = 1,
    int limit = 20,
  }) async {
    final response = await _dio.get('/users', queryParameters: {
      if (search != null) 'search': search,
      if (role != null) 'role': role,
      if (status != null) 'status': status,
      'page': page,
      'limit': limit,
    });
    return UserListResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Get a single user by ID
  Future<UserDetailResponse> get(int id) async {
    final response = await _dio.get('/users/$id');
    return UserDetailResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Create a new user
  Future<UserDetailResponse> create({
    required String email,
    required String password,
    String role = 'viewer',
  }) async {
    final response = await _dio.post('/users', data: {
      'email': email,
      'password': password,
      'role': role,
    });
    return UserDetailResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Update a user
  Future<UserDetailResponse> update(int id, {
    String? email,
    String? role,
    bool? enabled,
  }) async {
    final response = await _dio.put('/users/$id', data: {
      if (email != null) 'email': email,
      if (role != null) 'role': role,
      if (enabled != null) 'enabled': enabled,
    });
    return UserDetailResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Delete a user
  Future<ApiMessageResponse> delete(int id) async {
    final response = await _dio.delete('/users/$id');
    return ApiMessageResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Ban a user
  Future<ApiMessageResponse> ban(int id) async {
    final response = await _dio.post('/users/$id/ban');
    return ApiMessageResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Unban a user
  Future<ApiMessageResponse> unban(int id) async {
    final response = await _dio.post('/users/$id/unban');
    return ApiMessageResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Reset user password
  Future<ApiMessageResponse> resetPassword(int id) async {
    final response = await _dio.post('/users/$id/reset-password');
    return ApiMessageResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Update user role
  Future<UserDetailResponse> updateRole(int id, String role) async {
    final response = await _dio.put('/users/$id/role', data: {'role': role});
    return UserDetailResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// List API tokens for current user
  Future<ApiTokenListResponse> listTokens() async {
    final response = await _dio.get('/users/me/tokens');
    return ApiTokenListResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Create API token
  Future<ApiToken> createToken(String name, {int? expiresInDays}) async {
    final response = await _dio.post('/users/me/tokens', data: {
      'name': name,
      if (expiresInDays != null) 'expires_in_days': expiresInDays,
    });
    return ApiToken.fromJson(response.data['data'] as Map<String, dynamic>);
  }

  /// Delete API token
  Future<ApiMessageResponse> deleteToken(int id) async {
    final response = await _dio.delete('/users/me/tokens/$id');
    return ApiMessageResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// List active sessions
  Future<SessionListResponse> listSessions() async {
    final response = await _dio.get('/users/me/sessions');
    return SessionListResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Delete other sessions
  Future<ApiMessageResponse> deleteOtherSessions() async {
    final response = await _dio.delete('/users/me/sessions/others');
    return ApiMessageResponse.fromJson(response.data as Map<String, dynamic>);
  }
}