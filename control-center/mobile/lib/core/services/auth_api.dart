import 'package:dio/dio.dart';
import 'package:anixops_mobile/core/models/auth_models.dart';
import 'package:anixops_mobile/core/models/api_response.dart';

/// Authentication API endpoints
class AuthApi {
  final Dio _dio;

  AuthApi(this._dio);

  /// Login with email and password
  Future<AuthLoginResponse> login(String email, String password) async {
    final response = await _dio.post('/auth/login', data: {
      'email': email,
      'password': password,
    });
    return AuthLoginResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Register new user
  Future<AuthRegisterResponse> register(
    String email,
    String password, {
    String role = 'viewer',
  }) async {
    final response = await _dio.post('/auth/register', data: {
      'email': email,
      'password': password,
      'role': role,
    });
    return AuthRegisterResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Logout current user
  Future<AuthLogoutResponse> logout() async {
    final response = await _dio.post('/auth/logout');
    return AuthLogoutResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Refresh authentication token
  Future<AuthRefreshResponse> refresh(String refreshToken) async {
    final response = await _dio.post('/auth/refresh', data: {
      'refresh_token': refreshToken,
    });
    return AuthRefreshResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Get current user info
  Future<AuthMeResponse> me() async {
    final response = await _dio.get('/users/me');
    return AuthMeResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Update password
  Future<ApiMessageResponse> updatePassword({
    required String currentPassword,
    required String newPassword,
  }) async {
    final response = await _dio.put('/auth/password', data: {
      'current_password': currentPassword,
      'new_password': newPassword,
    });
    return ApiMessageResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Update current user profile
  Future<AuthMeResponse> updateProfile({
    String? name,
    String? email,
  }) async {
    final response = await _dio.put('/users/me', data: {
      if (name != null) 'name': name,
      if (email != null) 'email': email,
    });
    return AuthMeResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Request password reset email
  Future<ApiMessageResponse> forgotPassword(String email) async {
    final response = await _dio.post('/auth/forgot-password', data: {
      'email': email,
    });
    return ApiMessageResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Reset password with token
  Future<ApiMessageResponse> resetPassword({
    required String token,
    required String password,
  }) async {
    final response = await _dio.post('/auth/reset-password', data: {
      'token': token,
      'password': password,
    });
    return ApiMessageResponse.fromJson(response.data as Map<String, dynamic>);
  }
}

/// API Tokens API endpoints
class TokensApi {
  final Dio _dio;

  TokensApi(this._dio);

  /// List API tokens
  Future<Response> list() async {
    return _dio.get('/users/me/tokens');
  }

  /// Create API token
  Future<Response> create(String name, {int? expiresInDays}) async {
    return _dio.post('/users/me/tokens', data: {
      'name': name,
      if (expiresInDays != null) 'expires_in_days': expiresInDays,
    });
  }

  /// Delete API token
  Future<ApiMessageResponse> delete(String id) async {
    final response = await _dio.delete('/users/me/tokens/$id');
    return ApiMessageResponse.fromJson(response.data as Map<String, dynamic>);
  }
}

/// Sessions API endpoints
class SessionsApi {
  final Dio _dio;

  SessionsApi(this._dio);

  /// List active sessions
  Future<Response> list() async {
    return _dio.get('/users/me/sessions');
  }

  /// Delete other sessions
  Future<ApiMessageResponse> deleteOthers() async {
    final response = await _dio.delete('/users/me/sessions/others');
    return ApiMessageResponse.fromJson(response.data as Map<String, dynamic>);
  }
}