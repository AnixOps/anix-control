import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../../../../core/models/playbook_models.dart';
import '../../../../core/providers/api_providers.dart';

/// Playbooks state
class PlaybooksState {
  final List<Playbook> playbooks;
  final List<Playbook> builtInPlaybooks;
  final List<PlaybookCategory> categories;
  final bool isLoading;
  final String? error;
  final String? selectedCategory;

  const PlaybooksState({
    this.playbooks = const [],
    this.builtInPlaybooks = const [],
    this.categories = const [],
    this.isLoading = false,
    this.error,
    this.selectedCategory,
  });

  PlaybooksState copyWith({
    List<Playbook>? playbooks,
    List<Playbook>? builtInPlaybooks,
    List<PlaybookCategory>? categories,
    bool? isLoading,
    String? error,
    String? selectedCategory,
  }) {
    return PlaybooksState(
      playbooks: playbooks ?? this.playbooks,
      builtInPlaybooks: builtInPlaybooks ?? this.builtInPlaybooks,
      categories: categories ?? this.categories,
      isLoading: isLoading ?? this.isLoading,
      error: error ?? this.error,
      selectedCategory: selectedCategory ?? this.selectedCategory,
    );
  }

  List<Playbook> get allPlaybooks => [...builtInPlaybooks, ...playbooks];

  List<Playbook> get filteredPlaybooks {
    if (selectedCategory == null || selectedCategory == 'all') {
      return allPlaybooks;
    }
    return allPlaybooks.where((p) => p.category == selectedCategory).toList();
  }
}

/// Provider for PlaybooksState
final playbooksProvider = NotifierProvider<PlaybooksNotifier, PlaybooksState>(PlaybooksNotifier.new);

/// Playbooks notifier
class PlaybooksNotifier extends Notifier<PlaybooksState> {
  @override
  PlaybooksState build() {
    Future.microtask(() => loadAll());
    return const PlaybooksState();
  }

  Future<void> loadAll() async {
    state = state.copyWith(isLoading: true, error: null);

    try {
      final client = ref.read(apiClientProvider);
      final response = await client.playbooks.list();

      state = state.copyWith(
        playbooks: response.data.items,
        builtInPlaybooks: [], // No built-in playbooks endpoint yet
        categories: [],
        isLoading: false,
      );
    } catch (e) {
      state = state.copyWith(
        isLoading: false,
        error: e.toString(),
      );
    }
  }

  void setCategory(String? category) {
    state = state.copyWith(selectedCategory: category);
  }

  Future<Playbook?> getPlaybook(String name) async {
    try {
      final client = ref.read(apiClientProvider);
      final response = await client.playbooks.get(name);
      return response.data;
    } catch (e) {
      return null;
    }
  }

  Future<bool> uploadPlaybook({
    required String name,
    required String storageKey,
    String? description,
    String? category,
  }) async {
    try {
      final client = ref.read(apiClientProvider);
      final response = await client.playbooks.create(
        name: name,
        storageKey: storageKey,
        description: description,
        category: category ?? 'custom',
      );
      state = state.copyWith(
        playbooks: [...state.playbooks, response.data],
      );
      return true;
    } catch (e) {
      state = state.copyWith(error: e.toString());
      return false;
    }
  }

  Future<bool> deletePlaybook(String name) async {
    try {
      final client = ref.read(apiClientProvider);
      await client.playbooks.delete(name);
      state = state.copyWith(
        playbooks: state.playbooks.where((p) => p.name != name).toList(),
      );
      return true;
    } catch (e) {
      state = state.copyWith(error: e.toString());
      return false;
    }
  }

  void clearError() {
    state = state.copyWith(error: null);
  }
}

/// Playbook category placeholder
class PlaybookCategory {
  final String id;
  final String name;
  final String? icon;
  final String? description;

  const PlaybookCategory({
    required this.id,
    required this.name,
    this.icon,
    this.description,
  });
}