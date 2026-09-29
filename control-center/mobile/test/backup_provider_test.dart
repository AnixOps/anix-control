import 'package:flutter_test/flutter_test.dart';
import 'package:anixops_mobile/core/models/backup_models.dart';

void main() {
  group('Backup model', () {
    test('is created correctly from JSON', () {
      final json = {
        'id': 123,
        'name': 'Daily Backup',
        'status': 'completed',
        'size': 1048576,
        'description': 'System backup',
        'created_at': '2026-03-20T10:00:00Z',
        'completed_at': '2026-03-20T10:05:00Z',
      };

      final backup = Backup.fromJson(json);

      expect(backup.id, 123);
      expect(backup.name, 'Daily Backup');
      expect(backup.status, BackupStatusType.completed);
      expect(backup.size, 1048576);
      expect(backup.description, 'System backup');
    });

    test('handles missing optional fields', () {
      final json = <String, dynamic>{
        'id': 2,
        'name': 'Test Backup',
        'status': 'pending',
        'created_at': '2026-03-20T10:00:00Z',
      };

      final backup = Backup.fromJson(json);

      expect(backup.id, 2);
      expect(backup.name, 'Test Backup');
      expect(backup.status, BackupStatusType.pending);
      expect(backup.size, 0);
      expect(backup.description, isNull);
      expect(backup.completedAt, isNull);
      expect(backup.error, isNull);
    });

    test('formattedSize formats bytes correctly', () {
      expect(Backup(id: 1, name: 'test', status: BackupStatusType.pending, size: 500, createdAt: '').formattedSize, '500 B');
      expect(Backup(id: 1, name: 'test', status: BackupStatusType.pending, size: 1024, createdAt: '').formattedSize, '1.0 KB');
      expect(Backup(id: 1, name: 'test', status: BackupStatusType.pending, size: 1048576, createdAt: '').formattedSize, '1.0 MB');
      expect(Backup(id: 1, name: 'test', status: BackupStatusType.pending, size: 1073741824, createdAt: '').formattedSize, '1.0 GB');
    });

    test('status enum works correctly', () {
      expect(BackupStatusType.values.contains(BackupStatusType.completed), true);
      expect(BackupStatusType.values.contains(BackupStatusType.pending), true);
      expect(BackupStatusType.values.contains(BackupStatusType.failed), true);
      expect(BackupStatusType.values.contains(BackupStatusType.running), true);
    });
  });

  group('BackupSystemStatus model', () {
    test('is created correctly from JSON', () {
      final json = {
        'is_running': true,
        'last_backup_at': '2026-03-20T10:00:00Z',
        'total_backups': 10,
        'total_size': 10737418240,
      };

      final status = BackupSystemStatus.fromJson(json);

      expect(status.isRunning, true);
      expect(status.lastBackupAt, isNotNull);
      expect(status.totalBackups, 10);
      expect(status.totalSize, 10737418240);
    });

    test('handles missing fields with defaults', () {
      final json = <String, dynamic>{};

      final status = BackupSystemStatus.fromJson(json);

      expect(status.isRunning, false);
      expect(status.lastBackupAt, isNull);
      expect(status.totalBackups, 0);
      expect(status.totalSize, 0);
    });
  });
}