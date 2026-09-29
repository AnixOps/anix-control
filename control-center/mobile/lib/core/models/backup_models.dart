// Backup models matching the backend contract
// See: control-center/workers/src/types.ts

import 'package:anixops_mobile/core/models/api_response.dart';

/// Backup status enum
enum BackupStatusType { pending, running, completed, failed }

/// Backup entity
class Backup {
  final int id;
  final String name;
  final BackupStatusType status;
  final int size;
  final String? description;
  final String? error;
  final String createdAt;
  final String? completedAt;

  Backup({
    required this.id,
    required this.name,
    required this.status,
    this.size = 0,
    this.description,
    this.error,
    required this.createdAt,
    this.completedAt,
  });

  factory Backup.fromJson(Map<String, dynamic> json) {
    return Backup(
      id: json['id'] as int,
      name: json['name'] as String,
      status: BackupStatusType.values.firstWhere(
        (e) => e.name == json['status'],
        orElse: () => BackupStatusType.pending,
      ),
      size: json['size'] as int? ?? 0,
      description: json['description'] as String?,
      error: json['error'] as String?,
      createdAt: json['created_at'] as String,
      completedAt: json['completed_at'] as String?,
    );
  }

  String get formattedSize {
    if (size < 1024) return '$size B';
    if (size < 1024 * 1024) return '${(size / 1024).toStringAsFixed(1)} KB';
    if (size < 1024 * 1024 * 1024) {
      return '${(size / (1024 * 1024)).toStringAsFixed(1)} MB';
    }
    return '${(size / (1024 * 1024 * 1024)).toStringAsFixed(1)} GB';
  }
}

/// Backup list response data
class BackupListResponseData {
  final List<Backup> items;
  final int total;

  BackupListResponseData({
    required this.items,
    required this.total,
  });

  factory BackupListResponseData.fromJson(Map<String, dynamic> json) {
    return BackupListResponseData(
      items: (json['items'] as List)
          .map((e) => Backup.fromJson(e as Map<String, dynamic>))
          .toList(),
      total: json['total'] as int,
    );
  }
}

/// Backup system status
class BackupSystemStatus {
  final bool isRunning;
  final String? lastBackupAt;
  final int totalBackups;
  final int totalSize;

  BackupSystemStatus({
    required this.isRunning,
    this.lastBackupAt,
    required this.totalBackups,
    required this.totalSize,
  });

  factory BackupSystemStatus.fromJson(Map<String, dynamic> json) {
    return BackupSystemStatus(
      isRunning: json['is_running'] as bool? ?? false,
      lastBackupAt: json['last_backup_at'] as String?,
      totalBackups: json['total_backups'] as int? ?? 0,
      totalSize: json['total_size'] as int? ?? 0,
    );
  }
}

/// Backup cleanup result
class BackupCleanupResult {
  final int deletedCount;

  BackupCleanupResult({required this.deletedCount});

  factory BackupCleanupResult.fromJson(Map<String, dynamic> json) {
    return BackupCleanupResult(deletedCount: json['deleted_count'] as int? ?? 0);
  }
}

/// Response types
class BackupListResponse extends ApiSuccessResponse<BackupListResponseData> {
  BackupListResponse({required super.data});

  factory BackupListResponse.fromJson(Map<String, dynamic> json) {
    return BackupListResponse(
      data: BackupListResponseData.fromJson(
        json['data'] as Map<String, dynamic>,
      ),
    );
  }
}

class BackupDetailResponse extends ApiSuccessResponse<Backup> {
  BackupDetailResponse({required super.data});

  factory BackupDetailResponse.fromJson(Map<String, dynamic> json) {
    return BackupDetailResponse(
      data: Backup.fromJson(json['data'] as Map<String, dynamic>),
    );
  }
}

class BackupStatusResponse extends ApiSuccessResponse<BackupSystemStatus> {
  BackupStatusResponse({required super.data});

  factory BackupStatusResponse.fromJson(Map<String, dynamic> json) {
    return BackupStatusResponse(
      data: BackupSystemStatus.fromJson(json['data'] as Map<String, dynamic>),
    );
  }
}

class BackupCleanupResponse extends ApiSuccessResponse<BackupCleanupResult> {
  BackupCleanupResponse({required super.data});

  factory BackupCleanupResponse.fromJson(Map<String, dynamic> json) {
    return BackupCleanupResponse(
      data: BackupCleanupResult.fromJson(json['data'] as Map<String, dynamic>),
    );
  }
}