// Plugin models matching the backend contract
// See: control-center/workers/src/types.ts

import 'package:anixops_mobile/core/models/api_response.dart';

/// Plugin entity
class Plugin {
  final int id;
  final String name;
  final String? displayName;
  final String? version;
  final String? description;
  final String? author;
  final String? type;
  final bool enabled;
  final String? config;
  final String? permissions;
  final String installedAt;
  final String? updatedAt;

  Plugin({
    required this.id,
    required this.name,
    this.displayName,
    this.version,
    this.description,
    this.author,
    this.type,
    required this.enabled,
    this.config,
    this.permissions,
    required this.installedAt,
    this.updatedAt,
  });

  factory Plugin.fromJson(Map<String, dynamic> json) {
    return Plugin(
      id: json['id'] as int,
      name: json['name'] as String,
      displayName: json['display_name'] as String?,
      version: json['version'] as String?,
      description: json['description'] as String?,
      author: json['author'] as String?,
      type: json['type'] as String?,
      enabled: json['enabled'] as bool,
      config: json['config'] as String?,
      permissions: json['permissions'] as String?,
      installedAt: json['installed_at'] as String,
      updatedAt: json['updated_at'] as String?,
    );
  }
}

/// Plugin list response data
class PluginListResponseData {
  final List<Plugin> items;
  final int total;

  PluginListResponseData({
    required this.items,
    required this.total,
  });

  factory PluginListResponseData.fromJson(Map<String, dynamic> json) {
    return PluginListResponseData(
      items: (json['items'] as List)
          .map((e) => Plugin.fromJson(e as Map<String, dynamic>))
          .toList(),
      total: json['total'] as int,
    );
  }
}

/// Plugin status
class PluginStatus {
  final String name;
  final bool running;
  final String? error;
  final String? lastStarted;
  final String? lastStopped;

  PluginStatus({
    required this.name,
    required this.running,
    this.error,
    this.lastStarted,
    this.lastStopped,
  });

  factory PluginStatus.fromJson(Map<String, dynamic> json) {
    return PluginStatus(
      name: json['name'] as String,
      running: json['running'] as bool,
      error: json['error'] as String?,
      lastStarted: json['last_started'] as String?,
      lastStopped: json['last_stopped'] as String?,
    );
  }
}

/// Plugin execute result
class PluginExecuteResult {
  final String action;
  final bool success;
  final dynamic result;
  final String? error;

  PluginExecuteResult({
    required this.action,
    required this.success,
    this.result,
    this.error,
  });

  factory PluginExecuteResult.fromJson(Map<String, dynamic> json) {
    return PluginExecuteResult(
      action: json['action'] as String,
      success: json['success'] as bool,
      result: json['result'],
      error: json['error'] as String?,
    );
  }
}

/// Response types
class PluginListResponse extends ApiSuccessResponse<PluginListResponseData> {
  PluginListResponse({required super.data});

  factory PluginListResponse.fromJson(Map<String, dynamic> json) {
    return PluginListResponse(
      data: PluginListResponseData.fromJson(
        json['data'] as Map<String, dynamic>,
      ),
    );
  }
}

class PluginDetailResponse extends ApiSuccessResponse<Plugin> {
  PluginDetailResponse({required super.data});

  factory PluginDetailResponse.fromJson(Map<String, dynamic> json) {
    return PluginDetailResponse(
      data: Plugin.fromJson(json['data'] as Map<String, dynamic>),
    );
  }
}

class PluginStatusResponse extends ApiSuccessResponse<PluginStatus> {
  PluginStatusResponse({required super.data});

  factory PluginStatusResponse.fromJson(Map<String, dynamic> json) {
    return PluginStatusResponse(
      data: PluginStatus.fromJson(json['data'] as Map<String, dynamic>),
    );
  }
}

class PluginExecuteResponse extends ApiSuccessResponse<PluginExecuteResult> {
  PluginExecuteResponse({required super.data});

  factory PluginExecuteResponse.fromJson(Map<String, dynamic> json) {
    return PluginExecuteResponse(
      data: PluginExecuteResult.fromJson(json['data'] as Map<String, dynamic>),
    );
  }
}