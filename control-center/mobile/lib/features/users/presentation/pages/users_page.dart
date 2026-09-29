import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:anixops_mobile/core/theme/app_theme.dart';

import '../providers/users_provider.dart';

// Users Page
class UsersPage extends ConsumerStatefulWidget {
  const UsersPage({super.key});

  @override
  ConsumerState<UsersPage> createState() => _UsersPageState();
}

class _UsersPageState extends ConsumerState<UsersPage> {
  @override
  void initState() {
    super.initState();
    Future.microtask(() => ref.read(usersProvider.notifier).fetchUsers());
  }

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(usersProvider);
    final theme = Theme.of(context);

    return Scaffold(
      body: RefreshIndicator(
        onRefresh: () => ref.read(usersProvider.notifier).refresh(),
        child: CustomScrollView(
          slivers: [
            SliverAppBar(
              floating: true,
              snap: true,
              title: const Text('Users'),
              actions: [
                IconButton(
                  icon: const Icon(Icons.person_add),
                  onPressed: () {
                    // TODO: Add user
                  },
                ),
              ],
              bottom: PreferredSize(
                preferredSize: const Size.fromHeight(120),
                child: Column(
                  children: [
                    // Search Bar
                    Padding(
                      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
                      child: TextField(
                        onChanged: (value) => ref.read(usersProvider.notifier).setSearch(value),
                        decoration: InputDecoration(
                          hintText: 'Search users...',
                          prefixIcon: const Icon(Icons.search),
                          filled: true,
                          fillColor: theme.colorScheme.surface,
                          border: OutlineInputBorder(
                            borderRadius: BorderRadius.circular(12),
                            borderSide: BorderSide.none,
                          ),
                          contentPadding: EdgeInsets.zero,
                        ),
                      ),
                    ),
                    // Stats
                    Padding(
                      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
                      child: Row(
                        children: [
                          _buildStatCard('Total', state.users.length.toString(), Colors.blue, theme),
                          const SizedBox(width: 8),
                          _buildStatCard('Active', state.activeCount.toString(), Colors.green, theme),
                          const SizedBox(width: 8),
                          _buildStatCard('Banned', state.bannedCount.toString(), Colors.red, theme),
                        ],
                      ),
                    ),
                  ],
                ),
              ),
            ),

            // Filter Chips
            SliverToBoxAdapter(
              child: Padding(
                padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
                child: SingleChildScrollView(
                  scrollDirection: Axis.horizontal,
                  child: Row(
                    children: [
                      FilterChip(
                        label: const Text('All'),
                        selected: state.statusFilter.isEmpty,
                        onSelected: (_) => ref.read(usersProvider.notifier).setStatusFilter(''),
                      ),
                      const SizedBox(width: 8),
                      FilterChip(
                        label: const Text('Active'),
                        selected: state.statusFilter == 'active',
                        onSelected: (_) => ref.read(usersProvider.notifier).setStatusFilter('active'),
                      ),
                      const SizedBox(width: 8),
                      FilterChip(
                        label: const Text('Banned'),
                        selected: state.statusFilter == 'banned',
                        onSelected: (_) => ref.read(usersProvider.notifier).setStatusFilter('banned'),
                      ),
                    ],
                  ),
                ),
              ),
            ),

            // Users List
            state.loading
                ? const SliverFillRemaining(
                    child: Center(child: CircularProgressIndicator()),
                  )
                : state.filteredUsers.isEmpty
                    ? SliverFillRemaining(
                        child: Center(
                          child: Column(
                            mainAxisAlignment: MainAxisAlignment.center,
                            children: [
                              Icon(
                                Icons.people_outline,
                                size: 64,
                                color: theme.colorScheme.onSurfaceVariant,
                              ),
                              const SizedBox(height: 16),
                              Text(
                                'No users found',
                                style: theme.textTheme.bodyLarge?.copyWith(
                                  color: theme.colorScheme.onSurfaceVariant,
                                ),
                              ),
                            ],
                          ),
                        ),
                      )
                    : SliverPadding(
                        padding: const EdgeInsets.all(16),
                        sliver: SliverList(
                          delegate: SliverChildBuilderDelegate(
                            (context, index) {
                              final user = state.filteredUsers[index];
                              return _UserCard(
                                user: user,
                                onBan: () => ref.read(usersProvider.notifier).banUser(user.id),
                                onUnban: () => ref.read(usersProvider.notifier).unbanUser(user.id),
                              );
                            },
                            childCount: state.filteredUsers.length,
                          ),
                        ),
                      ),
          ],
        ),
      ),
    );
  }

  Widget _buildStatCard(String label, String value, Color color, ThemeData theme) {
    return Expanded(
      child: Container(
        padding: const EdgeInsets.all(12),
        decoration: BoxDecoration(
          color: color.withOpacity(0.1),
          borderRadius: BorderRadius.circular(12),
          border: Border.all(color: color.withOpacity(0.3)),
        ),
        child: Column(
          children: [
            Text(
              value,
              style: theme.textTheme.headlineSmall?.copyWith(
                fontWeight: FontWeight.bold,
                color: color,
              ),
            ),
            Text(
              label,
              style: theme.textTheme.bodySmall?.copyWith(
                color: theme.colorScheme.onSurfaceVariant,
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _UserCard extends StatelessWidget {
  final User user;
  final VoidCallback? onBan;
  final VoidCallback? onUnban;

  const _UserCard({
    required this.user,
    this.onBan,
    this.onUnban,
  });

  String _formatBytes(int bytes) {
    if (bytes == 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    final i = (bytes.bitLength - 1) ~/ 10;
    return '${(bytes / (1 << (i * 10))).toStringAsFixed(1)} ${sizes[i]}';
  }

  String _getInitials(String email) {
    return email.substring(0, 2).toUpperCase();
  }

  Color _getAvatarColor(String email) {
    final colors = [
      Colors.red, Colors.orange, Colors.amber, Colors.yellow,
      Colors.lime, Colors.green, Colors.teal, Colors.cyan,
      Colors.lightBlue, Colors.blue, Colors.indigo, Colors.purple,
    ];
    final hash = email.codeUnits.fold(0, (a, b) => a + b);
    return colors[hash % colors.length];
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);

    return Card(
      margin: const EdgeInsets.only(bottom: 12),
      child: InkWell(
        onTap: () {
          // TODO: Navigate to user detail
        },
        borderRadius: BorderRadius.circular(16),
        child: Padding(
          padding: const EdgeInsets.all(16),
          child: Row(
            children: [
              CircleAvatar(
                radius: 24,
                backgroundColor: _getAvatarColor(user.email),
                child: Text(
                  _getInitials(user.email),
                  style: const TextStyle(
                    color: Colors.white,
                    fontWeight: FontWeight.bold,
                  ),
                ),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Row(
                      children: [
                        Expanded(
                          child: Text(
                            user.email,
                            style: theme.textTheme.titleSmall?.copyWith(
                              fontWeight: FontWeight.bold,
                            ),
                            overflow: TextOverflow.ellipsis,
                          ),
                        ),
                        Container(
                          padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
                          decoration: BoxDecoration(
                            color: user.isActive
                                ? Colors.green.withOpacity(0.1)
                                : Colors.red.withOpacity(0.1),
                            borderRadius: BorderRadius.circular(12),
                          ),
                          child: Text(
                            user.status,
                            style: TextStyle(
                              fontSize: 12,
                              color: user.isActive ? Colors.green : Colors.red,
                            ),
                          ),
                        ),
                      ],
                    ),
                    const SizedBox(height: 4),
                    Row(
                      children: [
                        Container(
                          padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
                          decoration: BoxDecoration(
                            color: theme.colorScheme.surface,
                            borderRadius: BorderRadius.circular(4),
                          ),
                          child: Text(
                            user.role.toUpperCase(),
                            style: theme.textTheme.labelSmall,
                          ),
                        ),
                        if (user.trafficUsed != null && user.trafficLimit != null) ...[
                          const SizedBox(width: 8),
                          Icon(
                            Icons.swap_vert,
                            size: 14,
                            color: theme.colorScheme.onSurfaceVariant,
                          ),
                          const SizedBox(width: 4),
                          Text(
                            '${_formatBytes(user.trafficUsed!)} / ${_formatBytes(user.trafficLimit!)}',
                            style: theme.textTheme.bodySmall?.copyWith(
                              color: theme.colorScheme.onSurfaceVariant,
                            ),
                          ),
                        ],
                      ],
                    ),
                  ],
                ),
              ),
              IconButton(
                icon: Icon(
                  user.isActive ? Icons.block : Icons.check_circle,
                  color: user.isActive ? Colors.red : Colors.green,
                ),
                onPressed: user.isActive ? onBan : onUnban,
              ),
            ],
          ),
        ),
      ),
    );
  }
}