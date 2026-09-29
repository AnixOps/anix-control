import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../../../../core/models/task_models.dart';
import '../providers/tasks_provider.dart';
import 'task_detail_page.dart';

class TasksPage extends ConsumerWidget {
  const TasksPage({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(tasksProvider);
    final theme = Theme.of(context);

    return Scaffold(
      appBar: AppBar(
        title: const Text('Tasks'),
        actions: [
          PopupMenuButton<String>(
            icon: const Icon(Icons.filter_list),
            onSelected: (value) {
              ref.read(tasksProvider.notifier).setStatusFilter(value == 'all' ? null : value);
            },
            itemBuilder: (context) => [
              const PopupMenuItem(value: 'all', child: Text('All')),
              const PopupMenuItem(value: 'pending', child: Text('Pending')),
              const PopupMenuItem(value: 'running', child: Text('Running')),
              const PopupMenuItem(value: 'success', child: Text('Success')),
              const PopupMenuItem(value: 'failed', child: Text('Failed')),
            ],
          ),
        ],
      ),
      body: state.isLoading
          ? const Center(child: CircularProgressIndicator())
          : state.error != null
              ? _buildError(context, state.error!, ref)
              : state.tasks.isEmpty
                  ? _buildEmptyState(context)
                  : _buildTasksList(context, ref, state, theme),
    );
  }

  Widget _buildError(BuildContext context, String error, WidgetRef ref) {
    return Center(
      child: Column(
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          const Icon(Icons.error_outline, size: 48, color: Colors.red),
          const SizedBox(height: 16),
          Text('Error: $error'),
          const SizedBox(height: 16),
          ElevatedButton(
            onPressed: () => ref.read(tasksProvider.notifier).loadTasks(),
            child: const Text('Retry'),
          ),
        ],
      ),
    );
  }

  Widget _buildEmptyState(BuildContext context) {
    return Center(
      child: Column(
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          Icon(Icons.task_alt, size: 64, color: Colors.grey[600]),
          const SizedBox(height: 16),
          Text(
            'No tasks yet',
            style: Theme.of(context).textTheme.titleLarge,
          ),
          const SizedBox(height: 8),
          const Text('Execute a playbook to create a task'),
        ],
      ),
    );
  }

  Widget _buildTasksList(
    BuildContext context,
    WidgetRef ref,
    TasksState state,
    ThemeData theme,
  ) {
    return RefreshIndicator(
      onRefresh: () => ref.read(tasksProvider.notifier).loadTasks(),
      child: ListView.builder(
        padding: const EdgeInsets.all(16),
        itemCount: state.tasks.length,
        itemBuilder: (context, index) {
          final task = state.tasks[index];
          return _TaskCard(
            task: task,
            onTap: () => _openTaskDetail(context, task),
            onCancel: task.status == TaskStatus.pending || task.status == TaskStatus.running
                ? () => _cancelTask(context, ref, task.taskId)
                : null,
            onRetry: task.status == TaskStatus.failed || task.status == TaskStatus.cancelled
                ? () => _retryTask(context, ref, task.taskId)
                : null,
          );
        },
      ),
    );
  }

  void _openTaskDetail(BuildContext context, TaskListItem task) {
    Navigator.push(
      context,
      MaterialPageRoute(
        builder: (context) => TaskDetailPage(taskId: task.taskId),
      ),
    );
  }

  Future<void> _cancelTask(BuildContext context, WidgetRef ref, String taskId) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Cancel Task'),
        content: const Text('Are you sure you want to cancel this task?'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('No'),
          ),
          ElevatedButton(
            onPressed: () => Navigator.pop(context, true),
            style: ElevatedButton.styleFrom(backgroundColor: Colors.orange),
            child: const Text('Yes, Cancel'),
          ),
        ],
      ),
    );

    if (confirmed == true && context.mounted) {
      final success = await ref.read(tasksProvider.notifier).cancelTask(taskId);
      if (context.mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text(success ? 'Task cancelled' : 'Cancel failed')),
        );
      }
    }
  }

  Future<void> _retryTask(BuildContext context, WidgetRef ref, String taskId) async {
    final success = await ref.read(tasksProvider.notifier).retryTask(taskId);
    if (context.mounted) {
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(success ? 'Task retry created' : 'Retry failed'),
        ),
      );
    }
  }
}

class _TaskCard extends StatelessWidget {
  final TaskListItem task;
  final VoidCallback onTap;
  final VoidCallback? onCancel;
  final VoidCallback? onRetry;

  const _TaskCard({
    required this.task,
    required this.onTap,
    this.onCancel,
    this.onRetry,
  });

  Color _getStatusColor() {
    switch (task.status) {
      case TaskStatus.pending:
        return Colors.grey;
      case TaskStatus.running:
        return Colors.blue;
      case TaskStatus.success:
        return Colors.green;
      case TaskStatus.failed:
        return Colors.red;
      case TaskStatus.cancelled:
        return Colors.orange;
    }
  }

  IconData _getStatusIcon() {
    switch (task.status) {
      case TaskStatus.pending:
        return Icons.schedule;
      case TaskStatus.running:
        return Icons.play_circle;
      case TaskStatus.success:
        return Icons.check_circle;
      case TaskStatus.failed:
        return Icons.error;
      case TaskStatus.cancelled:
        return Icons.cancel;
    }
  }

  String _formatDuration() {
    if (task.startedAt == null) return 'Not started';

    final start = DateTime.tryParse(task.startedAt!);
    if (start == null) return 'Not started';

    final endStr = task.completedAt;
    final end = endStr != null ? DateTime.tryParse(endStr) : DateTime.now();
    if (end == null) return 'Not started';

    final duration = end.difference(start);

    if (duration.inHours > 0) {
      return '${duration.inHours}h ${duration.inMinutes.remainder(60)}m';
    } else if (duration.inMinutes > 0) {
      return '${duration.inMinutes}m ${duration.inSeconds.remainder(60)}s';
    } else {
      return '${duration.inSeconds}s';
    }
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);

    return Card(
      margin: const EdgeInsets.only(bottom: 12),
      child: InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(12),
        child: Padding(
          padding: const EdgeInsets.all(16),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                children: [
                  Container(
                    width: 40,
                    height: 40,
                    decoration: BoxDecoration(
                      color: _getStatusColor().withValues(alpha: 0.1),
                      borderRadius: BorderRadius.circular(8),
                    ),
                    child: Icon(
                      _getStatusIcon(),
                      color: _getStatusColor(),
                    ),
                  ),
                  const SizedBox(width: 12),
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          task.playbookName,
                          style: theme.textTheme.titleMedium?.copyWith(
                            fontWeight: FontWeight.bold,
                          ),
                        ),
                        Text(
                          task.playbookName,
                          style: theme.textTheme.bodySmall,
                        ),
                      ],
                    ),
                  ),
                  Container(
                    padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
                    decoration: BoxDecoration(
                      color: _getStatusColor().withValues(alpha: 0.1),
                      borderRadius: BorderRadius.circular(4),
                    ),
                    child: Text(
                      task.status.name.toUpperCase(),
                      style: TextStyle(
                        fontSize: 10,
                        fontWeight: FontWeight.bold,
                        color: _getStatusColor(),
                      ),
                    ),
                  ),
                ],
              ),
              const SizedBox(height: 12),
              Row(
                children: [
                  Icon(Icons.devices, size: 14, color: Colors.grey[600]),
                  const SizedBox(width: 4),
                  Text(
                    '${task.targetNodes?.split(',').where((s) => s.trim().isNotEmpty).length ?? 0} nodes',
                    style: theme.textTheme.bodySmall,
                  ),
                  const SizedBox(width: 16),
                  Icon(Icons.timer_outlined, size: 14, color: Colors.grey[600]),
                  const SizedBox(width: 4),
                  Text(
                    _formatDuration(),
                    style: theme.textTheme.bodySmall,
                  ),
                  if (task.createdAt.isNotEmpty) ...[
                    const SizedBox(width: 16),
                    Icon(Icons.calendar_today, size: 14, color: Colors.grey[600]),
                    const SizedBox(width: 4),
                    Text(
                      task.createdAt.split('T').first.split('-').reversed.take(2).join('/'),
                      style: theme.textTheme.bodySmall,
                    ),
                  ],
                ],
              ),
              if (onCancel != null || onRetry != null) ...[
                const SizedBox(height: 12),
                Row(
                  mainAxisAlignment: MainAxisAlignment.end,
                  children: [
                    if (onRetry != null)
                      TextButton(
                        onPressed: onRetry,
                        child: const Text('Retry'),
                      ),
                    if (onCancel != null)
                      TextButton(
                        onPressed: onCancel,
                        style: TextButton.styleFrom(foregroundColor: Colors.orange),
                        child: const Text('Cancel'),
                      ),
                  ],
                ),
              ],
            ],
          ),
        ),
      ),
    );
  }
}