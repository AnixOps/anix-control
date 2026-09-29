import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:anixops_mobile/core/providers/api_providers.dart';

/// User model
class User {
  final String id;
  final String email;
  final String? name;
  final String role;
  final String status;
  final int? trafficUsed;
  final int? trafficLimit;
  final DateTime? expireAt;
  final DateTime? createdAt;
  final DateTime? lastLoginAt;
  final List<String>? nodeIds;

  const User({
    required this.id,
    required this.email,
    this.name,
    this.role = 'user',
    this.status = 'active',
    this.trafficUsed,
    this.trafficLimit,
    this.expireAt,
    this.createdAt,
    this.lastLoginAt,
    this.nodeIds,
  });

  factory User.fromJson(Map<String, dynamic> json) {
    return User(
      id: json['id']?.toString() ?? '',
      email: json['email'] ?? '',
      name: json['name'],
      role: json['role'] ?? 'user',
      status: json['status'] ?? 'active',
      trafficUsed: json['traffic_used'],
      trafficLimit: json['traffic_limit'],
      expireAt: json['expire_at'] != null
          ? DateTime.tryParse(json['expire_at'])
          : null,
      createdAt: json['created_at'] != null
          ? DateTime.tryParse(json['created_at'])
          : null,
      lastLoginAt: json['last_login_at'] != null
          ? DateTime.tryParse(json['last_login_at'])
          : null,
      nodeIds: json['node_ids']?.cast<String>(),
    );
  }

  Map<String, dynamic> toJson() => {
        'id': id,
        'email': email,
        'name': name,
        'role': role,
        'status': status,
        'traffic_used': trafficUsed,
        'traffic_limit': trafficLimit,
        'expire_at': expireAt?.toIso8601String(),
        'created_at': createdAt?.toIso8601String(),
        'last_login_at': lastLoginAt?.toIso8601String(),
        'node_ids': nodeIds,
      };

  bool get isAdmin => role == 'admin';
  bool get isActive => status == 'active';
  bool get isBanned => status == 'banned';
  double? get trafficUsagePercent =>
      trafficLimit != null && trafficLimit! > 0
          ? (trafficUsed ?? 0) / trafficLimit! * 100
          : null;
}

/// Users state
class UsersState {
  final List<User> users;
  final bool loading;
  final String? error;
  final String search;
  final String roleFilter;
  final String statusFilter;
  final int page;
  final int total;

  const UsersState({
    this.users = const [],
    this.loading = false,
    this.error,
    this.search = '',
    this.roleFilter = '',
    this.statusFilter = '',
    this.page = 1,
    this.total = 0,
  });

  UsersState copyWith({
    List<User>? users,
    bool? loading,
    String? error,
    String? search,
    String? roleFilter,
    String? statusFilter,
    int? page,
    int? total,
  }) {
    return UsersState(
      users: users ?? this.users,
      loading: loading ?? this.loading,
      error: error,
      search: search ?? this.search,
      roleFilter: roleFilter ?? this.roleFilter,
      statusFilter: statusFilter ?? this.statusFilter,
      page: page ?? this.page,
      total: total ?? this.total,
    );
  }

  List<User> get filteredUsers {
    var result = users;
    if (search.isNotEmpty) {
      result = result
          .where((u) =>
              u.email.toLowerCase().contains(search.toLowerCase()) ||
              (u.name?.toLowerCase().contains(search.toLowerCase()) ?? false))
          .toList();
    }
    if (roleFilter.isNotEmpty) {
      result = result.where((u) => u.role == roleFilter).toList();
    }
    if (statusFilter.isNotEmpty) {
      result = result.where((u) => u.status == statusFilter).toList();
    }
    return result;
  }

  int get activeCount => users.where((u) => u.status == 'active').length;
  int get bannedCount => users.where((u) => u.status == 'banned').length;
  int get adminCount => users.where((u) => u.role == 'admin').length;
}

/// Provider for users state
final usersProvider = NotifierProvider<UsersNotifier, UsersState>(UsersNotifier.new);

/// Provider for a single user by ID
final userProvider = Provider.family<User?, String>((ref, id) {
  final state = ref.watch(usersProvider);
  return state.users.where((u) => u.id == id).firstOrNull;
});

/// Users notifier
class UsersNotifier extends Notifier<UsersState> {
  @override
  UsersState build() => const UsersState();

  Future<void> fetchUsers({int? page, bool refresh = false}) async {
    if (state.loading) return;

    state = state.copyWith(loading: true, error: null);

    try {
      final api = ref.read(apiClientProvider);
      final response = await api.users.list(
        search: state.search.isNotEmpty ? state.search : null,
        role: state.roleFilter.isNotEmpty ? state.roleFilter : null,
        status: state.statusFilter.isNotEmpty ? state.statusFilter : null,
        page: page ?? state.page,
      );

      // Convert API users to local User format
      final List<User> users = response.data.items.map((apiUser) {
        return User(
          id: apiUser.id.toString(),
          email: apiUser.email,
          name: null,
          role: apiUser.role.name,
          status: apiUser.enabled ? 'active' : 'disabled',
          trafficUsed: null,
          trafficLimit: null,
          expireAt: null,
          createdAt: apiUser.createdAt != null
              ? DateTime.tryParse(apiUser.createdAt)
              : null,
          lastLoginAt: apiUser.lastLoginAt != null
              ? DateTime.tryParse(apiUser.lastLoginAt!)
              : null,
          nodeIds: null,
        );
      }).toList();

      state = state.copyWith(
        users: refresh ? users : [...state.users, ...users],
        loading: false,
        page: page ?? state.page,
        total: response.data.total,
      );
    } catch (e) {
      state = state.copyWith(
        error: e.toString(),
        loading: false,
      );
    }
  }

  Future<void> refresh() => fetchUsers(page: 1, refresh: true);

  Future<void> loadMore() => fetchUsers(page: state.page + 1);

  void setSearch(String search) {
    state = state.copyWith(search: search);
    fetchUsers(page: 1, refresh: true);
  }

  void setRoleFilter(String filter) {
    state = state.copyWith(roleFilter: filter);
    fetchUsers(page: 1, refresh: true);
  }

  void setStatusFilter(String filter) {
    state = state.copyWith(statusFilter: filter);
    fetchUsers(page: 1, refresh: true);
  }

  Future<void> banUser(String id) async {
    try {
      final api = ref.read(apiClientProvider);
      await api.users.ban(int.parse(id));
      updateUserStatus(id, 'banned');
    } catch (e) {
      state = state.copyWith(error: e.toString());
      rethrow;
    }
  }

  Future<void> unbanUser(String id) async {
    try {
      final api = ref.read(apiClientProvider);
      await api.users.unban(int.parse(id));
      updateUserStatus(id, 'active');
    } catch (e) {
      state = state.copyWith(error: e.toString());
      rethrow;
    }
  }

  Future<void> deleteUser(String id) async {
    try {
      final api = ref.read(apiClientProvider);
      await api.users.delete(int.parse(id));
      state = state.copyWith(
        users: state.users.where((u) => u.id != id).toList(),
      );
    } catch (e) {
      state = state.copyWith(error: e.toString());
      rethrow;
    }
  }

  Future<void> updateRole(String id, String role) async {
    try {
      final api = ref.read(apiClientProvider);
      await api.users.updateRole(int.parse(id), role);
      final users = state.users.map((u) {
        if (u.id == id) {
          return User(
            id: u.id,
            email: u.email,
            name: u.name,
            role: role,
            status: u.status,
            trafficUsed: u.trafficUsed,
            trafficLimit: u.trafficLimit,
            expireAt: u.expireAt,
            createdAt: u.createdAt,
            lastLoginAt: u.lastLoginAt,
            nodeIds: u.nodeIds,
          );
        }
        return u;
      }).toList();
      state = state.copyWith(users: users);
    } catch (e) {
      state = state.copyWith(error: e.toString());
      rethrow;
    }
  }

  void updateUserStatus(String id, String status) {
    final users = state.users.map((u) {
      if (u.id == id) {
        return User(
          id: u.id,
          email: u.email,
          name: u.name,
          role: u.role,
          status: status,
          trafficUsed: u.trafficUsed,
          trafficLimit: u.trafficLimit,
          expireAt: u.expireAt,
          createdAt: u.createdAt,
          lastLoginAt: u.lastLoginAt,
          nodeIds: u.nodeIds,
        );
      }
      return u;
    }).toList();
    state = state.copyWith(users: users);
  }
}