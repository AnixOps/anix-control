import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../../../../core/models/backup_models.dart';
import '../../../../core/providers/api_providers.dart';

/// Backup state
class BackupState {
  final List<Backup> backups;
  final BackupSystemStatus? status;
  final bool isLoading;
  final String? error;

  const BackupState({
    this.backups = const [],
    this.status,
    this.isLoading = false,
    this.error,
  });

  BackupState copyWith({
    List<Backup>? backups,
    BackupSystemStatus? status,
    bool? isLoading,
    String? error,
  }) {
    return BackupState(
      backups: backups ?? this.backups,
      status: status ?? this.status,
      isLoading: isLoading ?? this.isLoading,
      error: error,
    );
  }

  List<Backup> get completedBackups =>
      backups.where((b) => b.status == BackupStatusType.completed).toList();

  List<Backup> get pendingBackups =>
      backups.where((b) => b.status == BackupStatusType.pending).toList();

  List<Backup> get failedBackups =>
      backups.where((b) => b.status == BackupStatusType.failed).toList();
}

/// Provider for backup state
final backupProvider =
    NotifierProvider<BackupNotifier, BackupState>(BackupNotifier.new);

/// Provider for backup status
final backupStatusProvider = Provider<BackupSystemStatus?>((ref) {
  return ref.watch(backupProvider).status;
});

/// Backup notifier
class BackupNotifier extends Notifier<BackupState> {
  @override
  BackupState build() => const BackupState();

  Future<void> loadBackups() async {
    state = state.copyWith(isLoading: true, error: null);
    try {
      final client = ref.read(apiClientProvider);
      final listResponse = await client.backup.list();
      final statusResponse = await client.backup.status();
      state = state.copyWith(
        backups: listResponse.data.items,
        status: statusResponse.data,
        isLoading: false,
      );
    } catch (e) {
      state = state.copyWith(
        isLoading: false,
        error: e.toString(),
      );
    }
  }

  Future<Backup?> createBackup({String? name, String? description}) async {
    try {
      final client = ref.read(apiClientProvider);
      final response = await client.backup.create(
        name: name,
        description: description,
      );
      await loadBackups();
      return response.data;
    } catch (e) {
      state = state.copyWith(error: e.toString());
      return null;
    }
  }

  Future<bool> restoreBackup(int id) async {
    try {
      final client = ref.read(apiClientProvider);
      await client.backup.restore(id);
      return true;
    } catch (e) {
      state = state.copyWith(error: e.toString());
      return false;
    }
  }

  Future<bool> deleteBackup(int id) async {
    try {
      final client = ref.read(apiClientProvider);
      await client.backup.delete(id);
      final backups = state.backups.where((b) => b.id != id).toList();
      state = state.copyWith(backups: backups);
      return true;
    } catch (e) {
      state = state.copyWith(error: e.toString());
      return false;
    }
  }

  Future<int> cleanupBackups({int keepLast = 10}) async {
    try {
      final client = ref.read(apiClientProvider);
      final response = await client.backup.cleanup(keepLast: keepLast);
      await loadBackups();
      return response.data.deletedCount;
    } catch (e) {
      state = state.copyWith(error: e.toString());
      return 0;
    }
  }

  void clearError() {
    state = state.copyWith(error: null);
  }
}