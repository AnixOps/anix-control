import 'dart:async';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../../../../core/models/task_models.dart';
import '../../../../core/providers/api_providers.dart';

/// Tasks state
class TasksState {
  final List<TaskListItem> tasks;
  final TaskDetailResponseData? selectedTask;
  final List<TaskLog> taskLogs;
  final bool isLoading;
  final bool isLoadingLogs;
  final String? error;
  final String? statusFilter;
  final int currentPage;
  final int totalPages;
  final int total;

  const TasksState({
    this.tasks = const [],
    this.selectedTask,
    this.taskLogs = const [],
    this.isLoading = false,
    this.isLoadingLogs = false,
    this.error,
    this.statusFilter,
    this.currentPage = 1,
    this.totalPages = 1,
    this.total = 0,
  });

  TasksState copyWith({
    List<TaskListItem>? tasks,
    TaskDetailResponseData? selectedTask,
    List<TaskLog>? taskLogs,
    bool? isLoading,
    bool? isLoadingLogs,
    String? error,
    String? statusFilter,
    int? currentPage,
    int? totalPages,
    int? total,
  }) {
    return TasksState(
      tasks: tasks ?? this.tasks,
      selectedTask: selectedTask ?? this.selectedTask,
      taskLogs: taskLogs ?? this.taskLogs,
      isLoading: isLoading ?? this.isLoading,
      isLoadingLogs: isLoadingLogs ?? this.isLoadingLogs,
      error: error ?? this.error,
      statusFilter: statusFilter ?? this.statusFilter,
      currentPage: currentPage ?? this.currentPage,
      totalPages: totalPages ?? this.totalPages,
      total: total ?? this.total,
    );
  }

  List<TaskListItem> get filteredTasks {
    if (statusFilter == null || statusFilter == 'all') {
      return tasks;
    }
    return tasks.where((t) => t.status.name == statusFilter).toList();
  }
}

/// Provider for TasksState
final tasksProvider = NotifierProvider<TasksNotifier, TasksState>(TasksNotifier.new);

/// Tasks notifier
class TasksNotifier extends Notifier<TasksState> {
  StreamSubscription<bool>? _connectionSubscription;

  @override
  TasksState build() {
    _bindRealtimeUpdates();
    Future.microtask(() => loadTasks());
    return const TasksState();
  }

  void _bindRealtimeUpdates() {
    final sse = ref.read(sseServiceProvider);

    sse.on('task_update', _handleTaskUpdate);
    sse.on('log', _handleTaskLog);
    _connectionSubscription = sse.connectionState.listen((connected) {
      if (!connected) return;
      if (!sse.subscribedChannels.contains('tasks')) {
        unawaited(sse.subscribe('tasks'));
      }
      if (!sse.subscribedChannels.contains('logs')) {
        unawaited(sse.subscribe('logs'));
      }
    });
  }

  void _handleTaskUpdate(dynamic payload) {
    if (payload is! Map) return;

    final taskId = payload['task_id']?.toString();
    if (taskId == null || taskId.isEmpty) return;

    final updatedTasks = state.tasks.map((task) {
      if (task.taskId != taskId) return task;

      return TaskListItem(
        id: task.id,
        taskId: task.taskId,
        playbookId: task.playbookId,
        playbookName: task.playbookName,
        status: payload['status'] != null
            ? TaskStatus.values.firstWhere((e) => e.name == payload['status'],
                orElse: () => task.status)
            : task.status,
        triggerType: task.triggerType,
        triggeredBy: task.triggeredBy,
        targetNodes: task.targetNodes,
        variables: task.variables,
        result: payload['result']?.toString() ?? task.result,
        error: payload['error']?.toString() ?? task.error,
        startedAt: task.startedAt,
        completedAt: task.completedAt,
        createdAt: task.createdAt,
        category: task.category,
        triggeredByEmail: task.triggeredByEmail,
      );
    }).toList();

    TaskDetailResponseData? updatedSelectedTask = state.selectedTask;
    if (updatedSelectedTask?.taskId == taskId) {
      final updated = updatedTasks.firstWhere(
        (task) => task.taskId == taskId,
        orElse: () => updatedTasks.first,
      );
      if (updated != null) {
        updatedSelectedTask = TaskDetailResponseData(
          id: updated.id,
          taskId: updated.taskId,
          playbookId: updated.playbookId,
          playbookName: updated.playbookName,
          status: updated.status,
          triggerType: updated.triggerType,
          triggeredBy: updated.triggeredBy,
          targetNodes: updated.targetNodes,
          variables: updated.variables,
          result: updated.result,
          error: updated.error,
          startedAt: updated.startedAt,
          completedAt: updated.completedAt,
          createdAt: updated.createdAt,
          category: updated.category,
          triggeredByEmail: updated.triggeredByEmail,
        );
      }
    }

    state = state.copyWith(tasks: updatedTasks, selectedTask: updatedSelectedTask);
  }

  void _handleTaskLog(dynamic payload) {
    if (payload is! Map) return;

    final taskId = payload['task_id']?.toString();
    if (taskId == null || taskId.isEmpty) return;
    if (state.selectedTask?.taskId != taskId) return;

    final log = TaskLog(
      id: payload['id'] as int? ?? 0,
      taskId: taskId,
      nodeId: payload['node_id'] as int?,
      nodeName: payload['node_name'] as String?,
      level: TaskLogLevel.values.firstWhere((e) => e.name == payload['level'],
          orElse: () => TaskLogLevel.info),
      message: payload['message'] as String? ?? '',
      metadata: payload['metadata'] as String?,
      createdAt: payload['created_at'] as String? ?? '',
    );
    state = state.copyWith(taskLogs: [...state.taskLogs, log]);
  }

  /// Load tasks
  Future<void> loadTasks({int page = 1, String? status}) async {
    state = state.copyWith(isLoading: true, error: null);

    try {
      final client = ref.read(apiClientProvider);
      final response = await client.tasks.list(
        page: page,
        status: status ?? state.statusFilter,
      );

      state = state.copyWith(
        tasks: response.data.items,
        isLoading: false,
        currentPage: page,
        total: response.data.total,
        totalPages: response.data.totalPages,
      );
    } catch (e) {
      state = state.copyWith(
        isLoading: false,
        error: e.toString(),
      );
    }
  }

  /// Set status filter
  void setStatusFilter(String? status) {
    state = state.copyWith(statusFilter: status);
    loadTasks(page: 1, status: status);
  }

  /// Load task detail
  Future<void> loadTask(String taskId) async {
    state = state.copyWith(isLoading: true, error: null);

    try {
      final client = ref.read(apiClientProvider);
      final response = await client.tasks.get(taskId);
      state = state.copyWith(
        selectedTask: response.data,
        isLoading: false,
      );
    } catch (e) {
      state = state.copyWith(
        isLoading: false,
        error: e.toString(),
      );
    }
  }

  /// Load task logs
  Future<void> loadTaskLogs(String taskId) async {
    state = state.copyWith(isLoadingLogs: true);

    try {
      final client = ref.read(apiClientProvider);
      final response = await client.tasks.logs(taskId);
      state = state.copyWith(
        taskLogs: response.data,
        isLoadingLogs: false,
      );
    } catch (e) {
      state = state.copyWith(isLoadingLogs: false);
    }
  }

  /// Create task
  Future<bool> createTask({
    required int playbookId,
    required List<int> targetNodeIds,
    Map<String, dynamic>? variables,
  }) async {
    try {
      final client = ref.read(apiClientProvider);
      await client.tasks.create(
        playbookId: playbookId,
        targetNodeIds: targetNodeIds,
        variables: variables,
      );
      await loadTasks();
      return true;
    } catch (e) {
      state = state.copyWith(error: e.toString());
      return false;
    }
  }

  /// Cancel task
  Future<bool> cancelTask(String taskId) async {
    try {
      final client = ref.read(apiClientProvider);
      await client.tasks.cancel(taskId);
      await loadTasks();
      if (state.selectedTask?.taskId == taskId) {
        await loadTask(taskId);
      }
      return true;
    } catch (e) {
      state = state.copyWith(error: e.toString());
      return false;
    }
  }

  /// Retry task
  Future<bool> retryTask(String taskId) async {
    try {
      final client = ref.read(apiClientProvider);
      await client.tasks.retry(taskId);
      await loadTasks();
      return true;
    } catch (e) {
      state = state.copyWith(error: e.toString());
      return false;
    }
  }

  /// Clear selected task
  void clearSelectedTask() {
    state = state.copyWith(
      selectedTask: null,
      taskLogs: const [],
    );
  }
}