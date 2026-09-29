import 'dart:io';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../features/auth/presentation/pages/login_page.dart';
import '../../features/auth/presentation/pages/register_page.dart';
import '../../features/dashboard/presentation/pages/dashboard_page.dart';
import '../../features/nodes/presentation/pages/nodes_page.dart';
import '../../features/nodes/presentation/pages/node_detail_page.dart';
import '../../features/plugins/presentation/pages/plugins_page.dart';
import '../../features/users/presentation/pages/users_page.dart';
import '../../features/logs/presentation/pages/logs_page.dart';
import '../../features/settings/presentation/pages/settings_page.dart';
import '../../features/notifications/presentation/pages/notifications_page.dart';
import '../../features/playbooks/presentation/pages/playbooks_page.dart';
import '../../features/playbooks/presentation/pages/playbook_detail_page.dart';
import '../../features/tasks/presentation/pages/tasks_page.dart';
import '../../features/tasks/presentation/pages/task_detail_page.dart';
import '../../features/schedules/presentation/pages/schedules_page.dart';

import '../../features/auth/presentation/providers/auth_provider.dart';
import '../../shared/presentation/pages/main_shell.dart';
import '../../desktop/desktop_shell.dart';

final routerProvider = Provider<GoRouter>((ref) {
  final authState = ref.watch(authStateProvider);

  return GoRouter(
    initialLocation: '/dashboard',
    redirect: (context, state) {
      final isAuthenticated = authState.isAuthenticated;
      final isAuthRoute = state.matchedLocation == '/login' || state.matchedLocation == '/register';

      if (!isAuthenticated && !isAuthRoute) {
        return '/login';
      }

      if (isAuthenticated && isAuthRoute) {
        return '/dashboard';
      }

      return null;
    },
    routes: [
      // Auth routes
      GoRoute(
        path: '/login',
        name: 'login',
        builder: (context, state) => _buildLoginShell(const LoginPage()),
      ),
      GoRoute(
        path: '/register',
        name: 'register',
        builder: (context, state) => _buildLoginShell(const RegisterPage()),
      ),

      // Main app shell - different layout for desktop vs mobile
      ShellRoute(
        builder: (context, state, child) => _buildMainShell(child),
        routes: [
          GoRoute(
            path: '/dashboard',
            name: 'dashboard',
            builder: (context, state) => const DashboardPage(),
          ),
          GoRoute(
            path: '/nodes',
            name: 'nodes',
            builder: (context, state) => const NodesPage(),
          ),
          GoRoute(
            path: '/nodes/:id',
            name: 'node-detail',
            builder: (context, state) {
              final id = state.pathParameters['id']!;
              return NodeDetailPage(nodeId: id);
            },
          ),
          GoRoute(
            path: '/playbooks',
            name: 'playbooks',
            builder: (context, state) => const PlaybooksPage(),
          ),
          GoRoute(
            path: '/playbooks/:name',
            name: 'playbook-detail',
            builder: (context, state) {
              final name = state.pathParameters['name']!;
              return PlaybookDetailPage(playbookName: name);
            },
          ),
          GoRoute(
            path: '/tasks',
            name: 'tasks',
            builder: (context, state) => const TasksPage(),
          ),
          GoRoute(
            path: '/tasks/:id',
            name: 'task-detail',
            builder: (context, state) {
              final id = state.pathParameters['id']!;
              return TaskDetailPage(taskId: id);
            },
          ),
          GoRoute(
            path: '/schedules',
            name: 'schedules',
            builder: (context, state) => const SchedulesPage(),
          ),
          GoRoute(
            path: '/plugins',
            name: 'plugins',
            builder: (context, state) => const PluginsPage(),
          ),
          GoRoute(
            path: '/users',
            name: 'users',
            builder: (context, state) => const UsersPage(),
          ),
          GoRoute(
            path: '/logs',
            name: 'logs',
            builder: (context, state) => const LogsPage(),
          ),
          GoRoute(
            path: '/settings',
            name: 'settings',
            builder: (context, state) => const SettingsPage(),
          ),
          GoRoute(
            path: '/notifications',
            name: 'notifications',
            builder: (context, state) => const NotificationsPage(),
          ),
        ],
      ),
    ],
    errorBuilder: (context, state) => const ErrorPage(),
  );
});

/// Build appropriate main shell based on platform
Widget _buildMainShell(Widget child) {
  if (Platform.isWindows || Platform.isMacOS || Platform.isLinux) {
    return DesktopShell(child: child);
  }
  return MainShell(child: child);
}

/// Build login shell with appropriate layout
Widget _buildLoginShell(Widget child) {
  if (Platform.isWindows || Platform.isMacOS || Platform.isLinux) {
    return DesktopLoginShell(child: child);
  }
  return child;
}

class ErrorPage extends StatelessWidget {
  const ErrorPage({super.key});

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: Center(
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            const Icon(Icons.error_outline, size: 64, color: Colors.grey),
            const SizedBox(height: 16),
            Text(
              'Page not found',
              style: Theme.of(context).textTheme.headlineMedium,
            ),
            const SizedBox(height: 8),
            ElevatedButton(
              onPressed: () => context.go('/dashboard'),
              child: const Text('Go Home'),
            ),
          ],
        ),
      ),
    );
  }
}

/// Placeholder for PlaybooksPage - now imported from features/playbooks