// Authentication models matching the backend contract
// See: control-center/workers/src/types.ts

import 'package:anixops_mobile/core/models/api_response.dart';

/// User role type
enum UserRole { admin, operator, viewer }

/// Auth provider type
enum AuthProvider { local, github, google, cloudflare }

/// User summary returned in auth responses
class AuthUserSummary {
  final int id;
  final String email;
  final UserRole role;

  AuthUserSummary({
    required this.id,
    required this.email,
    required this.role,
  });

  factory AuthUserSummary.fromJson(Map<String, dynamic> json) {
    return AuthUserSummary(
      id: json['id'] as int,
      email: json['email'] as String,
      role: UserRole.values.firstWhere(
        (e) => e.name == json['role'],
        orElse: () => UserRole.viewer,
      ),
    );
  }
}

/// Login response data
class AuthLoginData {
  final String accessToken;
  final String refreshToken;
  final String tokenType;
  final int expiresIn;
  final AuthUserSummary user;

  AuthLoginData({
    required this.accessToken,
    required this.refreshToken,
    required this.tokenType,
    required this.expiresIn,
    required this.user,
  });

  factory AuthLoginData.fromJson(Map<String, dynamic> json) {
    return AuthLoginData(
      accessToken: json['access_token'] as String,
      refreshToken: json['refresh_token'] as String,
      tokenType: json['token_type'] as String,
      expiresIn: json['expires_in'] as int,
      user: AuthUserSummary.fromJson(json['user'] as Map<String, dynamic>),
    );
  }
}

/// Refresh token response data
class AuthRefreshData {
  final String accessToken;
  final String tokenType;
  final int expiresIn;

  AuthRefreshData({
    required this.accessToken,
    required this.tokenType,
    required this.expiresIn,
  });

  factory AuthRefreshData.fromJson(Map<String, dynamic> json) {
    return AuthRefreshData(
      accessToken: json['access_token'] as String,
      tokenType: json['token_type'] as String,
      expiresIn: json['expires_in'] as int,
    );
  }
}

/// Register response data
class AuthRegisterData {
  final int id;
  final String email;
  final UserRole role;
  final String createdAt;

  AuthRegisterData({
    required this.id,
    required this.email,
    required this.role,
    required this.createdAt,
  });

  factory AuthRegisterData.fromJson(Map<String, dynamic> json) {
    return AuthRegisterData(
      id: json['id'] as int,
      email: json['email'] as String,
      role: UserRole.values.firstWhere(
        (e) => e.name == json['role'],
        orElse: () => UserRole.viewer,
      ),
      createdAt: json['created_at'] as String,
    );
  }
}

/// Current user data (me endpoint)
class AuthMeData {
  final int id;
  final String email;
  final UserRole role;
  final AuthProvider authProvider;
  final String? lastLoginAt;
  final String createdAt;

  AuthMeData({
    required this.id,
    required this.email,
    required this.role,
    required this.authProvider,
    this.lastLoginAt,
    required this.createdAt,
  });

  factory AuthMeData.fromJson(Map<String, dynamic> json) {
    return AuthMeData(
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
      lastLoginAt: json['last_login_at'] as String?,
      createdAt: json['created_at'] as String,
    );
  }
}

/// Lockout details for failed login
class AuthLockoutDetails {
  final String? lockedUntil;
  final int? retryAfter;

  AuthLockoutDetails({this.lockedUntil, this.retryAfter});

  factory AuthLockoutDetails.fromJson(Map<String, dynamic> json) {
    return AuthLockoutDetails(
      lockedUntil: json['locked_until'] as String?,
      retryAfter: json['retry_after'] as int?,
    );
  }
}

/// Failed login details with remaining attempts
class AuthFailedLoginDetails {
  final String? lockedUntil;
  final int? retryAfter;
  final int? remainingAttempts;
  final bool? accountLocked;

  AuthFailedLoginDetails({
    this.lockedUntil,
    this.retryAfter,
    this.remainingAttempts,
    this.accountLocked,
  });

  factory AuthFailedLoginDetails.fromJson(Map<String, dynamic> json) {
    return AuthFailedLoginDetails(
      lockedUntil: json['locked_until'] as String?,
      retryAfter: json['retry_after'] as int?,
      remainingAttempts: json['remaining_attempts'] as int?,
      accountLocked: json['account_locked'] as bool?,
    );
  }
}

/// Login response type
class AuthLoginResponse extends ApiSuccessResponse<AuthLoginData> {
  AuthLoginResponse({required super.data});

  factory AuthLoginResponse.fromJson(Map<String, dynamic> json) {
    return AuthLoginResponse(
      data: AuthLoginData.fromJson(json['data'] as Map<String, dynamic>),
    );
  }
}

/// Refresh response type
class AuthRefreshResponse extends ApiSuccessResponse<AuthRefreshData> {
  AuthRefreshResponse({required super.data});

  factory AuthRefreshResponse.fromJson(Map<String, dynamic> json) {
    return AuthRefreshResponse(
      data: AuthRefreshData.fromJson(json['data'] as Map<String, dynamic>),
    );
  }
}

/// Register response type
class AuthRegisterResponse extends ApiSuccessResponse<AuthRegisterData> {
  AuthRegisterResponse({required super.data});

  factory AuthRegisterResponse.fromJson(Map<String, dynamic> json) {
    return AuthRegisterResponse(
      data: AuthRegisterData.fromJson(json['data'] as Map<String, dynamic>),
    );
  }
}

/// Me response type
class AuthMeResponse extends ApiSuccessResponse<AuthMeData> {
  AuthMeResponse({required super.data});

  factory AuthMeResponse.fromJson(Map<String, dynamic> json) {
    return AuthMeResponse(
      data: AuthMeData.fromJson(json['data'] as Map<String, dynamic>),
    );
  }
}

/// Logout response type
class AuthLogoutResponse extends ApiMessageResponse {
  AuthLogoutResponse({required super.message});

  factory AuthLogoutResponse.fromJson(Map<String, dynamic> json) {
    return AuthLogoutResponse(message: json['message'] as String);
  }
}

/// Lockout response with error
class AuthLockoutResponse extends ApiErrorResponse {
  final String? lockedUntil;
  final int? retryAfter;

  AuthLockoutResponse({
    required super.error,
    this.lockedUntil,
    this.retryAfter,
  });

  factory AuthLockoutResponse.fromJson(Map<String, dynamic> json) {
    return AuthLockoutResponse(
      error: json['error'] as String,
      lockedUntil: json['locked_until'] as String?,
      retryAfter: json['retry_after'] as int?,
    );
  }
}

/// Invalid credentials response with remaining attempts
class AuthInvalidCredentialsResponse extends ApiErrorResponse {
  final String? lockedUntil;
  final int? retryAfter;
  final int? remainingAttempts;
  final bool? accountLocked;

  AuthInvalidCredentialsResponse({
    required super.error,
    this.lockedUntil,
    this.retryAfter,
    this.remainingAttempts,
    this.accountLocked,
  });

  factory AuthInvalidCredentialsResponse.fromJson(Map<String, dynamic> json) {
    return AuthInvalidCredentialsResponse(
      error: json['error'] as String,
      lockedUntil: json['locked_until'] as String?,
      retryAfter: json['retry_after'] as int?,
      remainingAttempts: json['remaining_attempts'] as int?,
      accountLocked: json['account_locked'] as bool?,
    );
  }
}