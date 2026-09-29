import 'package:flutter_test/flutter_test.dart';
import 'package:anixops_mobile/core/models/task_models.dart';
import 'package:anixops_mobile/features/tasks/presentation/providers/tasks_provider.dart';

void main() {
  group('Task model', () {
    test('is created correctly from JSON', () {
      final json = {
        'id': 1,
        'task_id': 'task-123',
        'playbook_id': 1,
        'playbook_name': 'install-docker',
        'status': 'success',
        'trigger_type': 'manual',
        'triggered_by': 1,
        'target_nodes': 'node-1,node-2',
        'variables': '{"version": "latest"}',
        'result': '{"success": true}',
        'error': null,
        'created_at': '2026-03-20T10:00:00Z',
        'started_at': '2026-03-20T10:01:00Z',
        'completed_at': '2026-03-20T10:05:00Z',
      };

      final task = Task.fromJson(json);

      expect(task.id, 1);
      expect(task.taskId, 'task-123');
      expect(task.playbookId, 1);
      expect(task.playbookName, 'install-docker');
      expect(task.status, TaskStatus.success);
      expect(task.triggerType, TaskTriggerType.manual);
      expect(task.triggeredBy, 1);
      expect(task.targetNodes, 'node-1,node-2');
      expect(task.variables, '{"version": "latest"}');
      expect(task.result, '{"success": true}');
    });

    test('handles missing optional fields', () {
      final json = {
        'id': 2,
        'task_id': 'task-789',
        'playbook_id': 1,
        'playbook_name': 'test',
        'status': 'pending',
        'trigger_type': 'manual',
        'created_at': '2026-03-20T10:00:00Z',
      };

      final task = Task.fromJson(json);

      expect(task.taskId, 'task-789');
      expect(task.status, TaskStatus.pending);
      expect(task.targetNodes, isNull);
      expect(task.variables, isNull);
      expect(task.startedAt, isNull);
      expect(task.completedAt, isNull);
    });
  });

  group('TaskLog model', () {
    test('is created correctly from JSON', () {
      final json = {
        'id': 1,
        'task_id': 'task-123',
        'node_id': 1,
        'node_name': 'node-1',
        'level': 'info',
        'message': 'Task started successfully',
        'metadata': '{"duration": 5000}',
        'created_at': '2026-03-20T10:00:00Z',
      };

      final log = TaskLog.fromJson(json);

      expect(log.id, 1);
      expect(log.taskId, 'task-123');
      expect(log.nodeId, 1);
      expect(log.nodeName, 'node-1');
      expect(log.level, TaskLogLevel.info);
      expect(log.message, 'Task started successfully');
    });

    test('handles missing fields with defaults', () {
      final json = <String, dynamic>{
        'id': 0,
        'task_id': '',
        'message': '',
        'created_at': '',
      };

      final log = TaskLog.fromJson(json);

      expect(log.id, 0);
      expect(log.taskId, '');
      expect(log.nodeId, isNull);
      expect(log.nodeName, isNull);
      expect(log.level, TaskLogLevel.info);
    });
  });

  group('TaskListItem model', () {
    test('is created correctly from JSON with extra fields', () {
      final json = {
        'id': 1,
        'task_id': 'task-123',
        'playbook_id': 1,
        'playbook_name': 'install-docker',
        'status': 'running',
        'trigger_type': 'manual',
        'triggered_by': 1,
        'triggered_by_email': 'admin@test.com',
        'category': 'software',
        'created_at': '2026-03-20T10:00:00Z',
      };

      final task = TaskListItem.fromJson(json);

      expect(task.taskId, 'task-123');
      expect(task.playbookName, 'install-docker');
      expect(task.status, TaskStatus.running);
      expect(task.triggeredByEmail, 'admin@test.com');
      expect(task.category, 'software');
    });
  });

  group('TasksState', () {
    test('filteredTasks returns all when no filter', () {
      final state = TasksState(
        tasks: [
          TaskListItem(id: 1, taskId: '1', playbookId: 1, playbookName: 'p1', status: TaskStatus.success, triggerType: TaskTriggerType.manual, createdAt: ''),
          TaskListItem(id: 2, taskId: '2', playbookId: 2, playbookName: 'p2', status: TaskStatus.running, triggerType: TaskTriggerType.manual, createdAt: ''),
          TaskListItem(id: 3, taskId: '3', playbookId: 3, playbookName: 'p3', status: TaskStatus.pending, triggerType: TaskTriggerType.manual, createdAt: ''),
        ],
      );

      expect(state.filteredTasks.length, 3);
    });

    test('filteredTasks filters by status', () {
      final state = TasksState(
        tasks: [
          TaskListItem(id: 1, taskId: '1', playbookId: 1, playbookName: 'p1', status: TaskStatus.success, triggerType: TaskTriggerType.manual, createdAt: ''),
          TaskListItem(id: 2, taskId: '2', playbookId: 2, playbookName: 'p2', status: TaskStatus.running, triggerType: TaskTriggerType.manual, createdAt: ''),
          TaskListItem(id: 3, taskId: '3', playbookId: 3, playbookName: 'p3', status: TaskStatus.success, triggerType: TaskTriggerType.manual, createdAt: ''),
        ],
        statusFilter: 'success',
      );

      expect(state.filteredTasks.length, 2);
      expect(state.filteredTasks.every((t) => t.status == TaskStatus.success), true);
    });

    test('filteredTasks handles "all" filter', () {
      final state = TasksState(
        tasks: [
          TaskListItem(id: 1, taskId: '1', playbookId: 1, playbookName: 'p1', status: TaskStatus.success, triggerType: TaskTriggerType.manual, createdAt: ''),
          TaskListItem(id: 2, taskId: '2', playbookId: 2, playbookName: 'p2', status: TaskStatus.running, triggerType: TaskTriggerType.manual, createdAt: ''),
        ],
        statusFilter: 'all',
      );

      expect(state.filteredTasks.length, 2);
    });

    test('copyWith works correctly', () {
      const state = TasksState(
        tasks: [],
        isLoading: false,
      );

      final newState = state.copyWith(
        isLoading: true,
        error: 'Test error',
      );

      expect(newState.isLoading, true);
      expect(newState.error, 'Test error');
      expect(newState.tasks, isEmpty);
    });
  });
}