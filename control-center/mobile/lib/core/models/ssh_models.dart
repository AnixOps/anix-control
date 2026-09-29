// SSH models matching the backend contract
// See: control-center/workers/src/types.ts

import 'package:anixops_mobile/core/models/api_response.dart';

/// SSH connection test result
class SshTestResult {
  final bool success;
  final String host;
  final int port;
  final String? error;
  final String? serverType;
  final String? os;

  SshTestResult({
    required this.success,
    required this.host,
    required this.port,
    this.error,
    this.serverType,
    this.os,
  });

  factory SshTestResult.fromJson(Map<String, dynamic> json) {
    return SshTestResult(
      success: json['success'] as bool? ?? false,
      host: json['host'] as String,
      port: json['port'] as int,
      error: json['error'] as String?,
      serverType: json['server_type'] as String?,
      os: json['os'] as String?,
    );
  }
}

/// SSH server import result
class SshImportResult {
  final int nodeId;
  final String name;
  final String host;
  final bool success;
  final String? error;

  SshImportResult({
    required this.nodeId,
    required this.name,
    required this.host,
    required this.success,
    this.error,
  });

  factory SshImportResult.fromJson(Map<String, dynamic> json) {
    return SshImportResult(
      nodeId: json['node_id'] as int,
      name: json['name'] as String,
      host: json['host'] as String,
      success: json['success'] as bool? ?? false,
      error: json['error'] as String?,
    );
  }
}

/// SSH server detection result
class SshDetectResult {
  final String host;
  final String? serverType;
  final String? os;
  final String? version;
  final Map<String, dynamic>? services;

  SshDetectResult({
    required this.host,
    this.serverType,
    this.os,
    this.version,
    this.services,
  });

  factory SshDetectResult.fromJson(Map<String, dynamic> json) {
    return SshDetectResult(
      host: json['host'] as String,
      serverType: json['server_type'] as String?,
      os: json['os'] as String?,
      version: json['version'] as String?,
      services: json['services'] as Map<String, dynamic>?,
    );
  }
}

/// SSH command execution result
class SshExecuteResult {
  final int exitCode;
  final String stdout;
  final String stderr;
  final int duration;

  SshExecuteResult({
    required this.exitCode,
    required this.stdout,
    required this.stderr,
    required this.duration,
  });

  factory SshExecuteResult.fromJson(Map<String, dynamic> json) {
    return SshExecuteResult(
      exitCode: json['exit_code'] as int? ?? 0,
      stdout: json['stdout'] as String? ?? '',
      stderr: json['stderr'] as String? ?? '',
      duration: json['duration'] as int? ?? 0,
    );
  }
}

/// Response types
class SshTestResponse extends ApiSuccessResponse<SshTestResult> {
  SshTestResponse({required super.data});

  factory SshTestResponse.fromJson(Map<String, dynamic> json) {
    return SshTestResponse(
      data: SshTestResult.fromJson(json['data'] as Map<String, dynamic>),
    );
  }
}

class SshImportResponse extends ApiSuccessResponse<SshImportResult> {
  SshImportResponse({required super.data});

  factory SshImportResponse.fromJson(Map<String, dynamic> json) {
    return SshImportResponse(
      data: SshImportResult.fromJson(json['data'] as Map<String, dynamic>),
    );
  }
}

class SshDetectResponse extends ApiSuccessResponse<SshDetectResult> {
  SshDetectResponse({required super.data});

  factory SshDetectResponse.fromJson(Map<String, dynamic> json) {
    return SshDetectResponse(
      data: SshDetectResult.fromJson(json['data'] as Map<String, dynamic>),
    );
  }
}

class SshExecuteResponse extends ApiSuccessResponse<SshExecuteResult> {
  SshExecuteResponse({required super.data});

  factory SshExecuteResponse.fromJson(Map<String, dynamic> json) {
    return SshExecuteResponse(
      data: SshExecuteResult.fromJson(json['data'] as Map<String, dynamic>),
    );
  }
}