import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:anixops_mobile/core/theme/app_theme.dart';
import 'package:anixops_mobile/features/auth/presentation/providers/auth_provider.dart';

/// Desktop sidebar navigation
class DesktopSidebar extends ConsumerWidget {
  final int selectedIndex;
  final ValueChanged<int>? onDestinationSelected;

  const DesktopSidebar({
    super.key,
    required this.selectedIndex,
    this.onDestinationSelected,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final authState = ref.watch(authStateProvider);
    final userEmail = authState.email ?? 'user@example.com';
    final userRole = authState.role ?? 'viewer';
    final userName = userEmail.split('@').first;

    return Container(
      width: 260,
      decoration: BoxDecoration(
        color: AppTheme.darkSurface,
        border: Border(
          right: BorderSide(color: AppTheme.darkBorder),
        ),
      ),
      child: Column(
        children: [
          // Logo
          _buildLogo(context),

          // Navigation items
          Expanded(
            child: _buildNavItems(context, ref),
          ),

          // User profile section
          _buildUserSection(context, ref, userName, userEmail, userRole),
        ],
      ),
    );
  }

  Widget _buildLogo(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 20, vertical: 24),
      child: Row(
        children: [
          Container(
            width: 40,
            height: 40,
            decoration: BoxDecoration(
              color: AppTheme.primaryColor,
              borderRadius: BorderRadius.circular(10),
            ),
            child: const Icon(
              Icons.dashboard_rounded,
              color: Colors.white,
              size: 24,
            ),
          ),
          const SizedBox(width: 12),
          Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                'AnixOps',
                style: Theme.of(context).textTheme.titleLarge?.copyWith(
                      fontWeight: FontWeight.bold,
                      color: AppTheme.darkText,
                    ),
              ),
              Text(
                'Control Center',
                style: Theme.of(context).textTheme.bodySmall?.copyWith(
                      color: AppTheme.darkTextSecondary,
                    ),
              ),
            ],
          ),
        ],
      ),
    );
  }

  Widget _buildNavItems(BuildContext context, WidgetRef ref) {
    final authState = ref.watch(authStateProvider);
    final userRole = authState.role ?? 'viewer';
    final items = _getNavItems(userRole);

    return ListView.builder(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
      itemCount: items.length,
      itemBuilder: (context, index) {
        final item = items[index];
        final isSelected = index == selectedIndex;

        return _NavItem(
          icon: item.icon,
          label: item.label,
          isSelected: isSelected,
          onTap: () {
            onDestinationSelected?.call(index);
            context.go(item.route);
          },
        );
      },
    );
  }

  Widget _buildUserSection(BuildContext context, WidgetRef ref, String userName, String userEmail, String userRole) {
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        border: Border(
          top: BorderSide(color: AppTheme.darkBorder),
        ),
      ),
      child: Row(
        children: [
          CircleAvatar(
            radius: 18,
            backgroundColor: AppTheme.primaryColor,
            child: Text(
              userName.isNotEmpty ? userName[0].toUpperCase() : 'U',
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
                    Text(
                      userName,
                      style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                            color: AppTheme.darkText,
                            fontWeight: FontWeight.w500,
                          ),
                    ),
                    const SizedBox(width: 8),
                    Container(
                      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
                      decoration: BoxDecoration(
                        color: userRole == 'admin'
                            ? Colors.red.withValues(alpha: 0.2)
                            : userRole == 'operator'
                                ? Colors.orange.withValues(alpha: 0.2)
                                : Colors.blue.withValues(alpha: 0.2),
                        borderRadius: BorderRadius.circular(4),
                      ),
                      child: Text(
                        userRole.toUpperCase(),
                        style: TextStyle(
                          fontSize: 10,
                          fontWeight: FontWeight.bold,
                          color: userRole == 'admin'
                              ? Colors.red
                              : userRole == 'operator'
                                  ? Colors.orange
                                  : Colors.blue,
                        ),
                      ),
                    ),
                  ],
                ),
                Text(
                  userEmail,
                  style: Theme.of(context).textTheme.bodySmall?.copyWith(
                        color: AppTheme.darkTextSecondary,
                      ),
                  overflow: TextOverflow.ellipsis,
                ),
              ],
            ),
          ),
          IconButton(
            icon: const Icon(Icons.logout_rounded, color: AppTheme.darkTextSecondary),
            onPressed: () async {
              // Show confirmation dialog
              final confirm = await showDialog<bool>(
                context: context,
                builder: (context) => AlertDialog(
                  title: const Text('Sign Out'),
                  content: const Text('Are you sure you want to sign out?'),
                  actions: [
                    TextButton(
                      onPressed: () => Navigator.pop(context, false),
                      child: const Text('Cancel'),
                    ),
                    ElevatedButton(
                      onPressed: () => Navigator.pop(context, true),
                      style: ElevatedButton.styleFrom(
                        backgroundColor: Colors.red,
                      ),
                      child: const Text('Sign Out'),
                    ),
                  ],
                ),
              );

              if (confirm == true && context.mounted) {
                await ref.read(authStateProvider.notifier).logout();
                if (context.mounted) {
                  context.go('/login');
                }
              }
            },
            tooltip: 'Logout',
          ),
        ],
      ),
    );
  }

  List<_NavItemData> _getNavItems(String userRole) {
    // Base items available to all authenticated users
    final items = <_NavItemData>[
      _NavItemData(Icons.dashboard_rounded, 'Dashboard', '/dashboard'),
      _NavItemData(Icons.dns_rounded, 'Nodes', '/nodes'),
      _NavItemData(Icons.play_circle_rounded, 'Playbooks', '/playbooks'),
      _NavItemData(Icons.task_rounded, 'Tasks', '/tasks'),
      _NavItemData(Icons.schedule_rounded, 'Schedules', '/schedules'),
      _NavItemData(Icons.extension_rounded, 'Plugins', '/plugins'),
    ];

    // Users management - admin only
    if (userRole == 'admin') {
      items.add(_NavItemData(Icons.people_rounded, 'Users', '/users'));
    }

    // Items available to all users
    items.addAll([
      _NavItemData(Icons.description_rounded, 'Logs', '/logs'),
      _NavItemData(Icons.notifications_rounded, 'Notifications', '/notifications'),
      _NavItemData(Icons.settings_rounded, 'Settings', '/settings'),
    ]);

    return items;
  }
}

class _NavItemData {
  final IconData icon;
  final String label;
  final String route;

  _NavItemData(this.icon, this.label, this.route);
}

class _NavItem extends StatelessWidget {
  final IconData icon;
  final String label;
  final bool isSelected;
  final VoidCallback? onTap;

  const _NavItem({
    required this.icon,
    required this.label,
    this.isSelected = false,
    this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    return Material(
      color: Colors.transparent,
      child: InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(8),
        child: AnimatedContainer(
          duration: const Duration(milliseconds: 200),
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
          decoration: BoxDecoration(
            color: isSelected ? AppTheme.primaryColor.withValues(alpha: 0.15) : Colors.transparent,
            borderRadius: BorderRadius.circular(8),
          ),
          child: Row(
            children: [
              Icon(
                icon,
                size: 22,
                color: isSelected ? AppTheme.primaryColor : AppTheme.darkTextSecondary,
              ),
              const SizedBox(width: 14),
              Text(
                label,
                style: TextStyle(
                  fontSize: 14,
                  fontWeight: isSelected ? FontWeight.w600 : FontWeight.w400,
                  color: isSelected ? AppTheme.primaryColor : AppTheme.darkText,
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}