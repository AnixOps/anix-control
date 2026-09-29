import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../../../../core/models/schedule_models.dart';
import '../../../../core/providers/api_providers.dart';

/// Schedules state
class SchedulesState {
  final List<Schedule> schedules;
  final bool isLoading;
  final String? error;

  const SchedulesState({
    this.schedules = const [],
    this.isLoading = false,
    this.error,
  });

  SchedulesState copyWith({
    List<Schedule>? schedules,
    bool? isLoading,
    String? error,
  }) {
    return SchedulesState(
      schedules: schedules ?? this.schedules,
      isLoading: isLoading ?? this.isLoading,
      error: error ?? this.error,
    );
  }

  List<Schedule> get enabledSchedules => schedules.where((s) => s.enabled).toList();
  List<Schedule> get disabledSchedules => schedules.where((s) => !s.enabled).toList();
}

/// Provider for SchedulesState
final schedulesProvider = NotifierProvider<SchedulesNotifier, SchedulesState>(SchedulesNotifier.new);

/// Schedules notifier
class SchedulesNotifier extends Notifier<SchedulesState> {
  @override
  SchedulesState build() {
    Future.microtask(() => loadSchedules());
    return const SchedulesState();
  }

  Future<void> loadSchedules() async {
    state = state.copyWith(isLoading: true, error: null);

    try {
      final client = ref.read(apiClientProvider);
      final response = await client.schedules.list();
      state = state.copyWith(
        schedules: response.data.items,
        isLoading: false,
      );
    } catch (e) {
      state = state.copyWith(
        isLoading: false,
        error: e.toString(),
      );
    }
  }

  Future<bool> createSchedule(ScheduleRequest request) async {
    try {
      final client = ref.read(apiClientProvider);
      await client.schedules.create(request);
      await loadSchedules();
      return true;
    } catch (e) {
      state = state.copyWith(error: e.toString());
      return false;
    }
  }

  Future<bool> updateSchedule(int id, ScheduleRequest request) async {
    try {
      final client = ref.read(apiClientProvider);
      await client.schedules.update(id, request);
      await loadSchedules();
      return true;
    } catch (e) {
      state = state.copyWith(error: e.toString());
      return false;
    }
  }

  Future<bool> deleteSchedule(int id) async {
    try {
      final client = ref.read(apiClientProvider);
      await client.schedules.delete(id);
      state = state.copyWith(
        schedules: state.schedules.where((s) => s.id != id).toList(),
      );
      return true;
    } catch (e) {
      state = state.copyWith(error: e.toString());
      return false;
    }
  }

  Future<bool> toggleSchedule(int id) async {
    try {
      final client = ref.read(apiClientProvider);
      final newEnabled = await client.schedules.toggle(id);
      state = state.copyWith(
        schedules: state.schedules.map((s) {
          if (s.id == id) {
            return Schedule(
              id: s.id,
              name: s.name,
              playbookId: s.playbookId,
              playbookName: s.playbookName,
              cron: s.cron,
              timezone: s.timezone,
              targetNodes: s.targetNodes,
              variables: s.variables,
              enabled: newEnabled,
              lastRun: s.lastRun,
              nextRun: s.nextRun,
              lastTaskId: s.lastTaskId,
              createdBy: s.createdBy,
              createdAt: s.createdAt,
              updatedAt: s.updatedAt,
            );
          }
          return s;
        }).toList(),
      );
      return true;
    } catch (e) {
      state = state.copyWith(error: e.toString());
      return false;
    }
  }

  Future<String?> runScheduleNow(int id) async {
    try {
      final client = ref.read(apiClientProvider);
      final taskId = await client.schedules.runNow(id);
      await loadSchedules();
      return taskId;
    } catch (e) {
      state = state.copyWith(error: e.toString());
      return null;
    }
  }

  void clearError() {
    state = state.copyWith(error: null);
  }
}