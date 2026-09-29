import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:anixops_mobile/core/models/schedule_models.dart';

void main() {
  group('Schedule model', () {
    test('is created correctly from JSON', () {
      final json = {
        'id': 1,
        'name': 'Daily Backup',
        'playbook_id': 5,
        'playbook_name': 'backup-system',
        'cron': '0 2 * * *',
        'timezone': 'UTC',
        'target_nodes': 'node-1,node-2',
        'variables': '{"backup_type": "full"}',
        'enabled': true,
        'next_run': '2026-03-21T02:00:00Z',
        'last_run': '2026-03-20T02:00:00Z',
        'last_task_id': 'task-123',
        'created_by': 1,
        'created_at': '2026-03-15T10:00:00Z',
        'updated_at': '2026-03-20T08:00:00Z',
      };

      final schedule = Schedule.fromJson(json);

      expect(schedule.id, 1);
      expect(schedule.name, 'Daily Backup');
      expect(schedule.playbookId, 5);
      expect(schedule.playbookName, 'backup-system');
      expect(schedule.cron, '0 2 * * *');
      expect(schedule.timezone, 'UTC');
      expect(schedule.targetNodes, 'node-1,node-2');
      expect(schedule.variables, '{"backup_type": "full"}');
      expect(schedule.enabled, true);
      expect(schedule.nextRun, isNotNull);
      expect(schedule.lastRun, isNotNull);
      expect(schedule.lastTaskId, 'task-123');
      expect(schedule.createdBy, 1);
    });

    test('handles missing optional fields', () {
      final json = {
        'id': 2,
        'name': 'Test Schedule',
        'playbook_id': 1,
        'playbook_name': 'test-playbook',
        'cron': '0 * * * *',
        'enabled': true,
        'created_at': '2026-03-15T10:00:00Z',
        'updated_at': '2026-03-20T08:00:00Z',
      };

      final schedule = Schedule.fromJson(json);

      expect(schedule.id, 2);
      expect(schedule.name, 'Test Schedule');
      expect(schedule.playbookId, 1);
      expect(schedule.playbookName, 'test-playbook');
      expect(schedule.timezone, isNull);
      expect(schedule.targetNodes, isNull);
      expect(schedule.variables, isNull);
      expect(schedule.enabled, true);
      expect(schedule.nextRun, isNull);
      expect(schedule.lastRun, isNull);
    });

    test('handles disabled schedule', () {
      final json = {
        'id': 1,
        'name': 'Disabled Schedule',
        'playbook_id': 1,
        'playbook_name': 'test',
        'cron': '0 * * * *',
        'enabled': false,
        'created_at': '2026-03-15T10:00:00Z',
        'updated_at': '2026-03-20T08:00:00Z',
      };

      final schedule = Schedule.fromJson(json);

      expect(schedule.enabled, false);
    });
  });

  group('ScheduleRequest', () {
    test('toJson creates correct map', () {
      final request = ScheduleRequest(
        name: 'Test Schedule',
        playbookId: 1,
        cron: '0 2 * * *',
        timezone: 'UTC',
        enabled: true,
      );

      final json = request.toJson();

      expect(json['name'], 'Test Schedule');
      expect(json['playbook_id'], 1);
      expect(json['cron'], '0 2 * * *');
      expect(json['timezone'], 'UTC');
      expect(json['enabled'], true);
    });
  });
}