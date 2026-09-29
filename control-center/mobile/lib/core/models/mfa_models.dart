// MFA models matching the backend contract
// See: control-center/workers/src/types.ts

import 'package:anixops_mobile/core/models/api_response.dart';

/// MFA status
class MFAStatus {
  final bool enabled;
  final bool hasRecoveryCodes;
  final String? enabledAt;
  final String? method;

  MFAStatus({
    required this.enabled,
    this.hasRecoveryCodes = false,
    this.enabledAt,
    this.method,
  });

  factory MFAStatus.fromJson(Map<String, dynamic> json) {
    return MFAStatus(
      enabled: json['enabled'] as bool? ?? false,
      hasRecoveryCodes: json['has_recovery_codes'] as bool? ?? false,
      enabledAt: json['enabled_at'] as String?,
      method: json['method'] as String?,
    );
  }
}

/// MFA setup result
class MFASetupResult {
  final String secret;
  final String otpauthUrl;
  final List<String> recoveryCodes;

  MFASetupResult({
    required this.secret,
    required this.otpauthUrl,
    required this.recoveryCodes,
  });

  factory MFASetupResult.fromJson(Map<String, dynamic> json) {
    return MFASetupResult(
      secret: json['secret'] as String? ?? '',
      otpauthUrl: json['otpauth_url'] as String? ?? '',
      recoveryCodes: (json['recovery_codes'] as List<dynamic>?)
              ?.map((e) => e as String)
              .toList() ??
          [],
    );
  }
}

/// MFA verify result
class MFAVerifyResult {
  final bool success;
  final String? message;

  MFAVerifyResult({required this.success, this.message});

  factory MFAVerifyResult.fromJson(Map<String, dynamic> json) {
    return MFAVerifyResult(
      success: json['success'] as bool? ?? false,
      message: json['message'] as String?,
    );
  }
}

/// MFA recovery codes result
class MFARecoveryCodesResult {
  final List<String> recoveryCodes;

  MFARecoveryCodesResult({required this.recoveryCodes});

  factory MFARecoveryCodesResult.fromJson(Map<String, dynamic> json) {
    return MFARecoveryCodesResult(
      recoveryCodes: (json['recovery_codes'] as List<dynamic>?)
              ?.map((e) => e as String)
              .toList() ??
          [],
    );
  }
}

/// Response types
class MFAStatusResponse extends ApiSuccessResponse<MFAStatus> {
  MFAStatusResponse({required super.data});

  factory MFAStatusResponse.fromJson(Map<String, dynamic> json) {
    return MFAStatusResponse(
      data: MFAStatus.fromJson(json['data'] as Map<String, dynamic>),
    );
  }
}

class MFASetupResponse extends ApiSuccessResponse<MFASetupResult> {
  MFASetupResponse({required super.data});

  factory MFASetupResponse.fromJson(Map<String, dynamic> json) {
    return MFASetupResponse(
      data: MFASetupResult.fromJson(json['data'] as Map<String, dynamic>),
    );
  }
}

class MFAVerifyResponse extends ApiSuccessResponse<MFAVerifyResult> {
  MFAVerifyResponse({required super.data});

  factory MFAVerifyResponse.fromJson(Map<String, dynamic> json) {
    return MFAVerifyResponse(
      data: MFAVerifyResult.fromJson(json['data'] as Map<String, dynamic>),
    );
  }
}

class MFARecoveryCodesResponse extends ApiSuccessResponse<MFARecoveryCodesResult> {
  MFARecoveryCodesResponse({required super.data});

  factory MFARecoveryCodesResponse.fromJson(Map<String, dynamic> json) {
    return MFARecoveryCodesResponse(
      data: MFARecoveryCodesResult.fromJson(json['data'] as Map<String, dynamic>),
    );
  }
}