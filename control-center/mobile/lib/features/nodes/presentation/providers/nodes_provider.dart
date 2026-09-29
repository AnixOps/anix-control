import 'dart:async';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:anixops_mobile/core/providers/api_providers.dart';
import 'package:anixops_mobile/core/services/sse_service.dart';

/// Node model
class Node {
  final String id;
  final String name;
  final String host;
  final int port;
  final String status;
  final String type;
  final int users;
  final int traffic;
  final DateTime? lastSeen;
  final double? cpuUsage;
  final double? memoryUsage;
  final String? version;

  const Node({
    required this.id,
    required this.name,
    required this.host,
    this.port = 443,
    this.status = 'offline',
    this.type = 'v2ray',
    this.users = 0,
    this.traffic = 0,
    this.lastSeen,
    this.cpuUsage,
    this.memoryUsage,
    this.version,
  });

  factory Node.fromJson(Map<String, dynamic> json) {
    return Node(
      id: json['id']?.toString() ?? '',
      name: json['name'] ?? '',
      host: json['host'] ?? '',
      port: json['port'] ?? 443,
      status: json['status'] ?? 'offline',
      type: json['type'] ?? 'v2ray',
      users: json['users'] ?? 0,
      traffic: json['traffic'] ?? 0,
      lastSeen: json['last_seen'] != null
          ? DateTime.tryParse(json['last_seen'])
          : null,
      cpuUsage: json['cpu_usage']?.toDouble(),
      memoryUsage: json['memory_usage']?.toDouble(),
      version: json['version'],
    );
  }

  Map<String, dynamic> toJson() => {
        'id': id,
        'name': name,
        'host': host,
        'port': port,
        'status': status,
        'type': type,
        'users': users,
        'traffic': traffic,
        'last_seen': lastSeen?.toIso8601String(),
        'cpu_usage': cpuUsage,
        'memory_usage': memoryUsage,
        'version': version,
      };
}

/// Nodes state
class NodesState {
  final List<Node> nodes;
  final bool loading;
  final String? error;
  final String search;
  final String statusFilter;
  final String typeFilter;
  final int page;
  final int total;

  const NodesState({
    this.nodes = const [],
    this.loading = false,
    this.error,
    this.search = '',
    this.statusFilter = '',
    this.typeFilter = '',
    this.page = 1,
    this.total = 0,
  });

  NodesState copyWith({
    List<Node>? nodes,
    bool? loading,
    String? error,
    String? search,
    String? statusFilter,
    String? typeFilter,
    int? page,
    int? total,
  }) {
    return NodesState(
      nodes: nodes ?? this.nodes,
      loading: loading ?? this.loading,
      error: error,
      search: search ?? this.search,
      statusFilter: statusFilter ?? this.statusFilter,
      typeFilter: typeFilter ?? this.typeFilter,
      page: page ?? this.page,
      total: total ?? this.total,
    );
  }

  List<Node> get filteredNodes {
    var result = nodes;
    if (search.isNotEmpty) {
      result = result
          .where((n) =>
              n.name.toLowerCase().contains(search.toLowerCase()) ||
              n.host.contains(search))
          .toList();
    }
    if (statusFilter.isNotEmpty) {
      result = result.where((n) => n.status == statusFilter).toList();
    }
    if (typeFilter.isNotEmpty) {
      result = result.where((n) => n.type == typeFilter).toList();
    }
    return result;
  }

  int get onlineCount => nodes.where((n) => n.status == 'online').length;
  int get offlineCount => nodes.where((n) => n.status == 'offline').length;
}

/// Provider for nodes state
final nodesProvider = NotifierProvider<NodesNotifier, NodesState>(NodesNotifier.new);

/// Provider for a single node by ID
final nodeProvider = Provider.family<Node?, String>((ref, id) {
  final state = ref.watch(nodesProvider);
  return state.nodes.where((n) => n.id == id).firstOrNull;
});

/// Nodes notifier
class NodesNotifier extends Notifier<NodesState> {
  StreamSubscription<bool>? _connectionSubscription;

  @override
  NodesState build() {
    _bindRealtimeUpdates();
    return const NodesState();
  }

  void _bindRealtimeUpdates() {
    final sse = ref.read(sseServiceProvider);

    sse.on('node_update', _handleNodeUpdate);
    _connectionSubscription = sse.connectionState.listen((connected) {
      if (connected && !sse.subscribedChannels.contains('nodes')) {
        unawaited(sse.subscribe('nodes'));
      }
    });
  }

  void _handleNodeUpdate(dynamic payload) {
    if (payload is! Map) return;

    final nodeId = payload['node_id']?.toString();
    if (nodeId == null || nodeId.isEmpty) return;

    if (payload['status'] != null ||
        payload['users'] != null ||
        payload['traffic'] != null ||
        payload['cpu_usage'] != null ||
        payload['memory_usage'] != null ||
        payload['last_seen'] != null ||
        payload['version'] != null) {
      updateNodeStats(nodeId, Map<String, dynamic>.from(payload));
    }
  }

  Future<void> fetchNodes({int? page, bool refresh = false}) async {
    if (state.loading) return;

    state = state.copyWith(loading: true, error: null);

    try {
      final api = ref.read(apiClientProvider);
      final response = await api.nodes.list(
        search: state.search.isNotEmpty ? state.search : null,
        status: state.statusFilter.isNotEmpty ? state.statusFilter : null,
        type: state.typeFilter.isNotEmpty ? state.typeFilter : null,
        page: page ?? state.page,
      );

      final data = response.data;
      final List<Node> nodes = (data.items)
          .map<Node>((json) => Node.fromJson({
            'id': json.id,
            'name': json.name,
            'host': json.host,
            'port': json.port,
            'status': json.status.name,
            'type': 'v2ray',
            'users': 0,
            'traffic': 0,
            'last_seen': json.lastSeen,
            'cpu_usage': null,
            'memory_usage': null,
            'version': json.agentVersion,
          }))
          .toList();

      state = state.copyWith(
        nodes: refresh ? nodes : [...state.nodes, ...nodes],
        loading: false,
        page: page ?? state.page,
        total: data.total,
      );
    } catch (e) {
      state = state.copyWith(
        error: e.toString(),
        loading: false,
      );
    }
  }

  Future<void> refresh() => fetchNodes(page: 1, refresh: true);

  Future<void> loadMore() => fetchNodes(page: state.page + 1);

  void setSearch(String search) {
    state = state.copyWith(search: search);
    fetchNodes(page: 1, refresh: true);
  }

  void setStatusFilter(String filter) {
    state = state.copyWith(statusFilter: filter);
    fetchNodes(page: 1, refresh: true);
  }

  void setTypeFilter(String filter) {
    state = state.copyWith(typeFilter: filter);
    fetchNodes(page: 1, refresh: true);
  }

  Future<void> startNode(String id) async {
    try {
      final api = ref.read(apiClientProvider);
      await api.nodes.start(id);
      updateNodeStatus(id, 'starting');
    } catch (e) {
      state = state.copyWith(error: e.toString());
      rethrow;
    }
  }

  Future<void> stopNode(String id) async {
    try {
      final api = ref.read(apiClientProvider);
      await api.nodes.stop(id);
      updateNodeStatus(id, 'stopping');
    } catch (e) {
      state = state.copyWith(error: e.toString());
      rethrow;
    }
  }

  Future<void> restartNode(String id) async {
    try {
      final api = ref.read(apiClientProvider);
      await api.nodes.restart(id);
      updateNodeStatus(id, 'restarting');
    } catch (e) {
      state = state.copyWith(error: e.toString());
      rethrow;
    }
  }

  Future<void> deleteNode(String id) async {
    try {
      final api = ref.read(apiClientProvider);
      await api.nodes.delete(id);
      state = state.copyWith(
        nodes: state.nodes.where((n) => n.id != id).toList(),
      );
    } catch (e) {
      state = state.copyWith(error: e.toString());
      rethrow;
    }
  }

  void updateNodeStatus(String id, String status) {
    final nodes = state.nodes.map((n) {
      if (n.id == id) {
        return Node(
          id: n.id,
          name: n.name,
          host: n.host,
          port: n.port,
          status: status,
          type: n.type,
          users: n.users,
          traffic: n.traffic,
          lastSeen: n.lastSeen,
          cpuUsage: n.cpuUsage,
          memoryUsage: n.memoryUsage,
          version: n.version,
        );
      }
      return n;
    }).toList();
    state = state.copyWith(nodes: nodes);
  }

  void updateNodeStats(String id, Map<String, dynamic> stats) {
    final nodes = state.nodes.map((n) {
      if (n.id == id) {
        return Node(
          id: n.id,
          name: n.name,
          host: n.host,
          port: n.port,
          status: stats['status'] ?? n.status,
          type: n.type,
          users: stats['users'] ?? n.users,
          traffic: stats['traffic'] ?? n.traffic,
          lastSeen: stats['last_seen'] != null
              ? DateTime.tryParse(stats['last_seen'])
              : n.lastSeen,
          cpuUsage: stats['cpu_usage']?.toDouble() ?? n.cpuUsage,
          memoryUsage: stats['memory_usage']?.toDouble() ?? n.memoryUsage,
          version: stats['version'] ?? n.version,
        );
      }
      return n;
    }).toList();
    state = state.copyWith(nodes: nodes);
  }
}