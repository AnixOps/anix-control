import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:anixops_mobile/core/providers/api_providers.dart';

/// Plugin model
class Plugin {
  final String name;
  final String displayName;
  final String version;
  final String status;
  final String? description;
  final String? author;
  final bool enabled;
  final Map<String, dynamic>? config;
  final DateTime? lastStarted;

  const Plugin({
    required this.name,
    required this.displayName,
    required this.version,
    this.status = 'stopped',
    this.description,
    this.author,
    this.enabled = false,
    this.config,
    this.lastStarted,
  });

  factory Plugin.fromJson(Map<String, dynamic> json) {
    return Plugin(
      name: json['name'] ?? '',
      displayName: json['display_name'] ?? json['name'] ?? '',
      version: json['version'] ?? '0.0.0',
      status: json['status'] ?? 'stopped',
      description: json['description'],
      author: json['author'],
      enabled: json['enabled'] ?? false,
      config: json['config'],
      lastStarted: json['last_started'] != null
          ? DateTime.tryParse(json['last_started'])
          : null,
    );
  }

  Map<String, dynamic> toJson() => {
        'name': name,
        'display_name': displayName,
        'version': version,
        'status': status,
        'description': description,
        'author': author,
        'enabled': enabled,
        'config': config,
        'last_started': lastStarted?.toIso8601String(),
      };

  bool get isRunning => status == 'running';
  bool get isStopped => status == 'stopped';
  bool get isError => status == 'error';
}

/// Plugins state
class PluginsState {
  final List<Plugin> plugins;
  final bool loading;
  final String? error;
  final String search;
  final String statusFilter;
  final bool executing;

  const PluginsState({
    this.plugins = const [],
    this.loading = false,
    this.error,
    this.search = '',
    this.statusFilter = '',
    this.executing = false,
  });

  PluginsState copyWith({
    List<Plugin>? plugins,
    bool? loading,
    String? error,
    String? search,
    String? statusFilter,
    bool? executing,
  }) {
    return PluginsState(
      plugins: plugins ?? this.plugins,
      loading: loading ?? this.loading,
      error: error,
      search: search ?? this.search,
      statusFilter: statusFilter ?? this.statusFilter,
      executing: executing ?? this.executing,
    );
  }

  List<Plugin> get filteredPlugins {
    var result = plugins;
    if (search.isNotEmpty) {
      result = result
          .where((p) =>
              p.name.toLowerCase().contains(search.toLowerCase()) ||
              p.displayName.toLowerCase().contains(search.toLowerCase()))
          .toList();
    }
    if (statusFilter.isNotEmpty) {
      result = result.where((p) => p.status == statusFilter).toList();
    }
    return result;
  }

  int get runningCount => plugins.where((p) => p.status == 'running').length;
  int get stoppedCount => plugins.where((p) => p.status == 'stopped').length;
  int get enabledCount => plugins.where((p) => p.enabled).length;
}

/// Provider for plugins state
final pluginsProvider = NotifierProvider<PluginsNotifier, PluginsState>(PluginsNotifier.new);

/// Provider for a single plugin by name
final pluginProvider = Provider.family<Plugin?, String>((ref, name) {
  final state = ref.watch(pluginsProvider);
  return state.plugins.where((p) => p.name == name).firstOrNull;
});

/// Plugins notifier
class PluginsNotifier extends Notifier<PluginsState> {
  @override
  PluginsState build() => const PluginsState();

  Future<void> fetchPlugins() async {
    if (state.loading) return;

    state = state.copyWith(loading: true, error: null);

    try {
      final api = ref.read(apiClientProvider);
      final response = await api.plugins.list();

      // Convert API Plugin model to local Plugin model
      final List<Plugin> plugins = response.data.items.map((apiPlugin) {
        return Plugin(
          name: apiPlugin.name,
          displayName: apiPlugin.displayName ?? apiPlugin.name,
          version: apiPlugin.version ?? '0.0.0',
          status: apiPlugin.enabled ? 'running' : 'stopped',
          description: apiPlugin.description,
          author: apiPlugin.author,
          enabled: apiPlugin.enabled,
          config: apiPlugin.config != null
              ? Map<String, dynamic>.from({}) // Parse config string if needed
              : null,
          lastStarted: null,
        );
      }).toList();

      state = state.copyWith(
        plugins: plugins,
        loading: false,
      );
    } catch (e) {
      state = state.copyWith(
        error: e.toString(),
        loading: false,
      );
    }
  }

  void setSearch(String search) {
    state = state.copyWith(search: search);
  }

  void setStatusFilter(String filter) {
    state = state.copyWith(statusFilter: filter);
  }

  Future<void> startPlugin(String name) async {
    try {
      state = state.copyWith(executing: true);
      final api = ref.read(apiClientProvider);
      await api.plugins.start(name);
      updatePluginStatus(name, 'running');
    } catch (e) {
      state = state.copyWith(error: e.toString());
      rethrow;
    } finally {
      state = state.copyWith(executing: false);
    }
  }

  Future<void> stopPlugin(String name) async {
    try {
      state = state.copyWith(executing: true);
      final api = ref.read(apiClientProvider);
      await api.plugins.stop(name);
      updatePluginStatus(name, 'stopped');
    } catch (e) {
      state = state.copyWith(error: e.toString());
      rethrow;
    } finally {
      state = state.copyWith(executing: false);
    }
  }

  Future<void> restartPlugin(String name) async {
    try {
      state = state.copyWith(executing: true);
      final api = ref.read(apiClientProvider);
      await api.plugins.restart(name);
      updatePluginStatus(name, 'running');
    } catch (e) {
      state = state.copyWith(error: e.toString());
      rethrow;
    } finally {
      state = state.copyWith(executing: false);
    }
  }

  Future<void> enablePlugin(String name) async {
    try {
      final api = ref.read(apiClientProvider);
      await api.plugins.enable(name);
      final plugins = state.plugins.map((p) {
        if (p.name == name) {
          return Plugin(
            name: p.name,
            displayName: p.displayName,
            version: p.version,
            status: p.status,
            description: p.description,
            author: p.author,
            enabled: true,
            config: p.config,
            lastStarted: p.lastStarted,
          );
        }
        return p;
      }).toList();
      state = state.copyWith(plugins: plugins);
    } catch (e) {
      state = state.copyWith(error: e.toString());
      rethrow;
    }
  }

  Future<void> disablePlugin(String name) async {
    try {
      final api = ref.read(apiClientProvider);
      await api.plugins.disable(name);
      final plugins = state.plugins.map((p) {
        if (p.name == name) {
          return Plugin(
            name: p.name,
            displayName: p.displayName,
            version: p.version,
            status: 'stopped',
            description: p.description,
            author: p.author,
            enabled: false,
            config: p.config,
            lastStarted: p.lastStarted,
          );
        }
        return p;
      }).toList();
      state = state.copyWith(plugins: plugins);
    } catch (e) {
      state = state.copyWith(error: e.toString());
      rethrow;
    }
  }

  Future<Map<String, dynamic>?> fetchPluginConfig(String name) async {
    try {
      final api = ref.read(apiClientProvider);
      final config = await api.plugins.config(name);
      return config;
    } catch (e) {
      state = state.copyWith(error: e.toString());
      rethrow;
    }
  }

  Future<void> updatePluginConfig(String name, Map<String, dynamic> config) async {
    try {
      final api = ref.read(apiClientProvider);
      await api.plugins.updateConfig(name, config);
      final plugins = state.plugins.map((p) {
        if (p.name == name) {
          return Plugin(
            name: p.name,
            displayName: p.displayName,
            version: p.version,
            status: p.status,
            description: p.description,
            author: p.author,
            enabled: p.enabled,
            config: config,
            lastStarted: p.lastStarted,
          );
        }
        return p;
      }).toList();
      state = state.copyWith(plugins: plugins);
    } catch (e) {
      state = state.copyWith(error: e.toString());
      rethrow;
    }
  }

  void updatePluginStatus(String name, String status) {
    final plugins = state.plugins.map((p) {
      if (p.name == name) {
        return Plugin(
          name: p.name,
          displayName: p.displayName,
          version: p.version,
          status: status,
          description: p.description,
          author: p.author,
          enabled: p.enabled,
          config: p.config,
          lastStarted: status == 'running' ? DateTime.now() : p.lastStarted,
        );
      }
      return p;
    }).toList();
    state = state.copyWith(plugins: plugins);
  }
}