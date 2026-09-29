import 'package:dio/dio.dart';
import 'package:anixops_mobile/core/models/mfa_models.dart';

/// MFA API endpoints
class MFAApi {
  final Dio _dio;

  MFAApi(this._dio);

  /// Get MFA status
  Future<MFAStatusResponse> status() async {
    final response = await _dio.get('/mfa/status');
    return MFAStatusResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Setup MFA (generate secret and recovery codes)
  Future<MFASetupResponse> setup() async {
    final response = await _dio.post('/mfa/setup');
    return MFASetupResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Enable MFA (verify setup and enable)
  Future<MFAVerifyResponse> enable(String code) async {
    final response = await _dio.post('/mfa/enable', data: {'code': code});
    return MFAVerifyResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Disable MFA
  Future<MFAVerifyResponse> disable(String code) async {
    final response = await _dio.post('/mfa/disable', data: {'code': code});
    return MFAVerifyResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Verify MFA code
  Future<MFAVerifyResponse> verify(String code) async {
    final response = await _dio.post('/mfa/verify', data: {'code': code});
    return MFAVerifyResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Regenerate recovery codes
  Future<MFARecoveryCodesResponse> regenerateRecoveryCodes(String code) async {
    final response = await _dio.post('/mfa/recovery-codes/regenerate', data: {'code': code});
    return MFARecoveryCodesResponse.fromJson(response.data as Map<String, dynamic>);
  }
}