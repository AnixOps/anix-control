import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:anixops_mobile/core/providers/api_providers.dart';

/// Dashboard stats model
class DashboardStats {
  final NodeStats nodes;
  final UserStats users;
  final AgentStats agents;
  final TrafficStats traffic;
  final PluginStats plugins;

  const DashboardStats({
    this.nodes = const NodeStats(),
    this.users = const UserStats(),
    this.agents = const AgentStats(),
    this.traffic = const TrafficStats(),
    this.plugins = const PluginStats(),
  });

  factory DashboardStats.fromJson(Map<String, dynamic> json) {
    return DashboardStats(
      nodes: NodeStats.fromJson(json['nodes'] ?? {}),
      users: UserStats.fromJson(json['users'] ?? {}),
      agents: AgentStats.fromJson(json['agents'] ?? {}),
      traffic: TrafficStats.fromJson(json['traffic'] ?? {}),
      plugins: PluginStats.fromJson(json['plugins'] ?? {}),
    );
  }
}

class NodeStats {
  final int total;
  final int online;
  final int offline;

  const NodeStats({this.total = 0, this.online = 0, this.offline = 0});

  factory NodeStats.fromJson(Map<String, dynamic> json) {
    return NodeStats(
      total: json['total'] ?? 0,
      online: json['online'] ?? 0,
      offline: json['offline'] ?? 0,
    );
  }
}

class UserStats {
  final int total;
  final int active;

  const UserStats({this.total = 0, this.active = 0});

  factory UserStats.fromJson(Map<String, dynamic> json) {
    return UserStats(
      total: json['total'] ?? 0,
      active: json['active'] ?? 0,
    );
  }
}

class AgentStats {
  final int total;
  final int online;

  const AgentStats({this.total = 0, this.online = 0});

  factory AgentStats.fromJson(Map<String, dynamic> json) {
    return AgentStats(
      total: json['total'] ?? 0,
      online: json['online'] ?? 0,
    );
  }
}

class TrafficStats {
  final int today;
  final int month;

  const TrafficStats({this.today = 0, this.month = 0});

  factory TrafficStats.fromJson(Map<String, dynamic> json) {
    return TrafficStats(
      today: json['today'] ?? 0,
      month: json['month'] ?? 0,
    );
  }
}

class PluginStats {
  final int total;
  final int active;

  const PluginStats({this.total = 0, this.active = 0});

  factory PluginStats.fromJson(Map<String, dynamic> json) {
    return PluginStats(
      total: json['total'] ?? 0,
      active: json['active'] ?? 0,
    );
  }
}

/// Activity model
class Activity {
  final String id;
  final String type;
  final String message;
  final String? userId;
  final String? userName;
  final DateTime timestamp;
  final Map<String, dynamic>? metadata;

  const Activity({
    required this.id,
    required this.type,
    required this.message,
    this.userId,
    this.userName,
    required this.timestamp,
    this.metadata,
  });

  factory Activity.fromJson(Map<String, dynamic> json) {
    return Activity(
      id: json['id']?.toString() ?? '',
      type: json['type'] ?? '',
      message: json['message'] ?? '',
      userId: json['user_id']?.toString(),
      userName: json['user_name'],
      timestamp: json['timestamp'] != null
          ? DateTime.tryParse(json['timestamp']) ?? DateTime.now()
          : DateTime.now(),
      metadata: json['metadata'],
    );
  }
}

/// Alert model
class Alert {
  final String id;
  final String level;
  final String title;
  final String message;
  final DateTime timestamp;
  final bool acknowledged;

  const Alert({
    required this.id,
    required this.level,
    required this.title,
    required this.message,
    required this.timestamp,
    this.acknowledged = false,
  });

  factory Alert.fromJson(Map<String, dynamic> json) {
    return Alert(
      id: json['id']?.toString() ?? '',
      level: json['level'] ?? 'info',
      title: json['title'] ?? '',
      message: json['message'] ?? '',
      timestamp: json['timestamp'] != null
          ? DateTime.tryParse(json['timestamp']) ?? DateTime.now()
          : DateTime.now(),
      acknowledged: json['acknowledged'] ?? false,
    );
  }
}

/// Dashboard state
class DashboardState {
  final DashboardStats stats;
  final List<Activity> activities;
  final List<Alert> alerts;
  final bool loading;
  final String? error;
  final DateTime? lastUpdate;

  const DashboardState({
    this.stats = const DashboardStats(),
    this.activities = const [],
    this.alerts = const [],
    this.loading = false,
    this.error,
    this.lastUpdate,
  });

  DashboardState copyWith({
    DashboardStats? stats,
    List<Activity>? activities,
    List<Alert>? alerts,
    bool? loading,
    String? error,
    DateTime? lastUpdate,
  }) {
    return DashboardState(
      stats: stats ?? this.stats,
      activities: activities ?? this.activities,
      alerts: alerts ?? this.alerts,
      loading: loading ?? this.loading,
      error: error,
      lastUpdate: lastUpdate ?? this.lastUpdate,
    );
  }

  bool get hasAlerts => alerts.isNotEmpty;
  List<Alert> get criticalAlerts => alerts.where((a) => a.level == 'critical').toList();
  List<Alert> get warningAlerts => alerts.where((a) => a.level == 'warning').toList();
}

/// Dashboard notifier
class DashboardNotifier extends Notifier<DashboardState> {
  @override
  DashboardState build() => const DashboardState();

  Future<void> fetchStats() async {
    try {
      final api = ref.read(apiClientProvider);
      final response = await api.dio.get('/dashboard/stats');
      final stats = DashboardStats.fromJson(response.data);
      state = state.copyWith(stats: stats, lastUpdate: DateTime.now());
    } catch (e) {
      state = state.copyWith(error: e.toString());
    }
  }

  Future<void> fetchActivities({int limit = 10}) async {
    try {
      final api = ref.read(apiClientProvider);
      final response = await api.dio.get('/dashboard/activities', queryParameters: {'limit': limit});
      final data = response.data;
      final List<Activity> activities = (data['data'] ?? data)
          .map<Activity>((json) => Activity.fromJson(json))
          .toList();
      state = state.copyWith(activities: activities);
    } catch (e) {
      state = state.copyWith(error: e.toString());
    }
  }

  Future<void> fetchAlerts() async {
    try {
      final api = ref.read(apiClientProvider);
      final response = await api.dio.get('/dashboard/alerts');
      final data = response.data;
      final List<Alert> alerts = (data['data'] ?? data)
          .map<Alert>((json) => Alert.fromJson(json))
          .toList();
      state = state.copyWith(alerts: alerts);
    } catch (e) {
      state = state.copyWith(error: e.toString());
    }
  }

  Future<void> fetchAll() async {
    state = state.copyWith(loading: true, error: null);
    try {
      await Future.wait([
        fetchStats(),
        fetchActivities(),
        fetchAlerts(),
      ]);
    } catch (e) {
      state = state.copyWith(error: e.toString());
    } finally {
      state = state.copyWith(loading: false);
    }
  }

  void addActivity(Activity activity) {
    final activities = [activity, ...state.activities];
    if (activities.length > 50) {
      activities.removeRange(50, activities.length);
    }
    state = state.copyWith(activities: activities);
  }

  void addAlert(Alert alert) {
    state = state.copyWith(alerts: [alert, ...state.alerts]);
  }

  void removeAlert(String id) {
    state = state.copyWith(
      alerts: state.alerts.where((a) => a.id != id).toList(),
    );
  }

  void updateStats(DashboardStats stats) {
    state = state.copyWith(stats: stats, lastUpdate: DateTime.now());
  }
}

/// Provider for dashboard state
final dashboardProvider = NotifierProvider<DashboardNotifier, DashboardState>(DashboardNotifier.new);