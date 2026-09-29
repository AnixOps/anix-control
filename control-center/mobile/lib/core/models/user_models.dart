// User models matching the backend contract
// See: control-center/workers/src/types.ts

import 'package:anixops_mobile/core/models/api_response.dart';
import 'auth_models.dart';

/// User entity
class User {
  final int id;
  final String email;
  final UserRole role;
  final AuthProvider authProvider;
  final bool enabled;
  final String? lastLoginAt;
  final String createdAt;
  final String updatedAt;

  User({
    required this.id,
    required this.email,
    required this.role,
    required this.authProvider,
    required this.enabled,
    this.lastLoginAt,
    required this.createdAt,
    required this.updatedAt,
  });

  factory User.fromJson(Map<String, dynamic> json) {
    return User(
      id: json['id'] as int,
      email: json['email'] as String,
      role: UserRole.values.firstWhere(
        (e) => e.name == json['role'],
        orElse: () => UserRole.viewer,
      ),
      authProvider: AuthProvider.values.firstWhere(
        (e) => e.name == json['auth_provider'],
        orElse: () => AuthProvider.local,
      ),
      enabled: json['enabled'] as bool,
      lastLoginAt: json['last_login_at'] as String?,
      createdAt: json['created_at'] as String,
      updatedAt: json['updated_at'] as String,
    );
  }
}

/// API token
class ApiToken {
  final int id;
  final int userId;
  final String name;
  final String token;
  final String? expiresAt;
  final String? lastUsed;
  final String createdAt;

  ApiToken({
    required this.id,
    required this.userId,
    required this.name,
    required this.token,
    this.expiresAt,
    this.lastUsed,
    required this.createdAt,
  });

  factory ApiToken.fromJson(Map<String, dynamic> json) {
    return ApiToken(
      id: json['id'] as int,
      userId: json['user_id'] as int,
      name: json['name'] as String,
      token: json['token'] as String,
      expiresAt: json['expires_at'] as String?,
      lastUsed: json['last_used'] as String?,
      createdAt: json['created_at'] as String,
    );
  }
}

/// User session
class UserSession {
  final String id;
  final int userId;
  final String ipAddress;
  final String userAgent;
  final String createdAt;
  final String? lastAccessedAt;

  UserSession({
    required this.id,
    required this.userId,
    required this.ipAddress,
    required this.userAgent,
    required this.createdAt,
    this.lastAccessedAt,
  });

  factory UserSession.fromJson(Map<String, dynamic> json) {
    return UserSession(
      id: json['id'] as String,
      userId: json['user_id'] as int,
      ipAddress: json['ip_address'] as String,
      userAgent: json['user_agent'] as String,
      createdAt: json['created_at'] as String,
      lastAccessedAt: json['last_accessed_at'] as String?,
    );
  }
}

/// User list response data
class UserListResponseData {
  final List<User> items;
  final int total;
  final int page;
  final int perPage;
  final int totalPages;

  UserListResponseData({
    required this.items,
    required this.total,
    required this.page,
    required this.perPage,
    required this.totalPages,
  });

  factory UserListResponseData.fromJson(Map<String, dynamic> json) {
    return UserListResponseData(
      items: (json['items'] as List)
          .map((e) => User.fromJson(e as Map<String, dynamic>))
          .toList(),
      total: json['total'] as int,
      page: json['page'] as int,
      perPage: json['per_page'] as int,
      totalPages: json['total_pages'] as int,
    );
  }
}

/// Response types
class UserListResponse extends ApiSuccessResponse<UserListResponseData> {
  UserListResponse({required super.data});

  factory UserListResponse.fromJson(Map<String, dynamic> json) {
    return UserListResponse(
      data: UserListResponseData.fromJson(
        json['data'] as Map<String, dynamic>,
      ),
    );
  }
}

class UserDetailResponse extends ApiSuccessResponse<User> {
  UserDetailResponse({required super.data});

  factory UserDetailResponse.fromJson(Map<String, dynamic> json) {
    return UserDetailResponse(
      data: User.fromJson(json['data'] as Map<String, dynamic>),
    );
  }
}

class ApiTokenListResponse extends ApiSuccessResponse<List<ApiToken>> {
  ApiTokenListResponse({required super.data});

  factory ApiTokenListResponse.fromJson(Map<String, dynamic> json) {
    return ApiTokenListResponse(
      data: (json['data'] as List)
          .map((e) => ApiToken.fromJson(e as Map<String, dynamic>))
          .toList(),
    );
  }
}

class SessionListResponse extends ApiSuccessResponse<List<UserSession>> {
  SessionListResponse({required super.data});

  factory SessionListResponse.fromJson(Map<String, dynamic> json) {
    return SessionListResponse(
      data: (json['data'] as List)
          .map((e) => UserSession.fromJson(e as Map<String, dynamic>))
          .toList(),
    );
  }
}