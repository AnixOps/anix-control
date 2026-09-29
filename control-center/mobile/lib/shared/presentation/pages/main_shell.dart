import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/theme/app_theme.dart';
import '../../../features/auth/presentation/providers/auth_provider.dart';

class MainShell extends ConsumerWidget {
  final Widget child;

  const MainShell({
    super.key,
    required this.child,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final currentPath = GoRouterState.of(context).matchedLocation;
    final authState = ref.watch(authStateProvider);
    final userRole = authState.role ?? 'viewer';

    return Scaffold(
      body: child,
      bottomNavigationBar: NavigationBar(
        selectedIndex: _calculateSelectedIndex(currentPath),
        onDestinationSelected: (index) => _onItemTapped(index, context),
        destinations: _getDestinations(userRole),
      ),
    );
  }

  List<NavigationDestination> _getDestinations(String userRole) {
    return [
      const NavigationDestination(
        icon: Icon(Icons.dashboard_outlined),
        selectedIcon: Icon(Icons.dashboard),
        label: 'Dashboard',
      ),
      const NavigationDestination(
        icon: Icon(Icons.dns_outlined),
        selectedIcon: Icon(Icons.dns),
        label: 'Nodes',
      ),
      const NavigationDestination(
        icon: Icon(Icons.play_circle_outlined),
        selectedIcon: Icon(Icons.play_circle),
        label: 'Playbooks',
      ),
      const NavigationDestination(
        icon: Icon(Icons.task_outlined),
        selectedIcon: Icon(Icons.task),
        label: 'Tasks',
      ),
      const NavigationDestination(
        icon: Icon(Icons.more_horiz),
        selectedIcon: Icon(Icons.more_horiz),
        label: 'More',
      ),
    ];
  }

  int _calculateSelectedIndex(String path) {
    if (path.startsWith('/dashboard')) return 0;
    if (path.startsWith('/nodes')) return 1;
    if (path.startsWith('/playbooks')) return 2;
    if (path.startsWith('/tasks')) return 3;
    if (path.startsWith('/schedules') ||
        path.startsWith('/plugins') ||
        path.startsWith('/users') ||
        path.startsWith('/logs') ||
        path.startsWith('/settings') ||
        path.startsWith('/notifications')) return 4;
    return 0;
  }

  void _onItemTapped(int index, BuildContext context) {
    final routes = ['/dashboard', '/nodes', '/playbooks', '/tasks'];
    if (index < routes.length) {
      context.go(routes[index]);
    } else {
      // Show more menu as bottom sheet
      _showMoreMenu(context);
    }
  }

  void _showMoreMenu(BuildContext context) {
    showModalBottomSheet(
      context: context,
      builder: (context) => const _MoreMenuSheet(),
    );
  }
}

class _MoreMenuSheet extends ConsumerWidget {
  const _MoreMenuSheet();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final authState = ref.watch(authStateProvider);
    final userRole = authState.role ?? 'viewer';

    return SafeArea(
      child: Padding(
        padding: const EdgeInsets.symmetric(vertical: 16),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Container(
              width: 40,
              height: 4,
              margin: const EdgeInsets.only(bottom: 16),
              decoration: BoxDecoration(
                color: Colors.grey[300],
                borderRadius: BorderRadius.circular(2),
              ),
            ),
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 16),
              child: GridView.count(
                shrinkWrap: true,
                crossAxisCount: 4,
                mainAxisSpacing: 16,
                crossAxisSpacing: 16,
                children: [
                  _MoreMenuItem(
                    icon: Icons.schedule,
                    label: 'Schedules',
                    onTap: () {
                      Navigator.pop(context);
                      context.go('/schedules');
                    },
                  ),
                  _MoreMenuItem(
                    icon: Icons.extension,
                    label: 'Plugins',
                    onTap: () {
                      Navigator.pop(context);
                      context.go('/plugins');
                    },
                  ),
                  if (userRole == 'admin')
                    _MoreMenuItem(
                      icon: Icons.people,
                      label: 'Users',
                      onTap: () {
                        Navigator.pop(context);
                        context.go('/users');
                      },
                    ),
                  _MoreMenuItem(
                    icon: Icons.description,
                    label: 'Logs',
                    onTap: () {
                      Navigator.pop(context);
                      context.go('/logs');
                    },
                  ),
                  _MoreMenuItem(
                    icon: Icons.notifications,
                    label: 'Alerts',
                    onTap: () {
                      Navigator.pop(context);
                      context.go('/notifications');
                    },
                  ),
                  _MoreMenuItem(
                    icon: Icons.settings,
                    label: 'Settings',
                    onTap: () {
                      Navigator.pop(context);
                      context.go('/settings');
                    },
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _MoreMenuItem extends StatelessWidget {
  final IconData icon;
  final String label;
  final VoidCallback onTap;

  const _MoreMenuItem({
    required this.icon,
    required this.label,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    return InkWell(
      onTap: onTap,
      borderRadius: BorderRadius.circular(12),
      child: Column(
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          Container(
            width: 48,
            height: 48,
            decoration: BoxDecoration(
              color: AppTheme.primaryColor.withValues(alpha: 0.1),
              borderRadius: BorderRadius.circular(12),
            ),
            child: Icon(icon, color: AppTheme.primaryColor),
          ),
          const SizedBox(height: 8),
          Text(
            label,
            style: Theme.of(context).textTheme.bodySmall,
            textAlign: TextAlign.center,
          ),
        ],
      ),
    );
  }
}