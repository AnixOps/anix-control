import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';
import 'package:anixops_mobile/core/theme/app_theme.dart';
import 'package:anixops_mobile/desktop/sidebar.dart';
import 'package:anixops_mobile/desktop/title_bar.dart';

/// Desktop-specific layout with sidebar navigation
class DesktopShell extends StatefulWidget {
  final Widget child;

  const DesktopShell({
    super.key,
    required this.child,
  });

  @override
  State<DesktopShell> createState() => _DesktopShellState();
}

class _DesktopShellState extends State<DesktopShell> {
  int _selectedIndex = 0;

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    _updateSelectedIndex();
  }

  void _updateSelectedIndex() {
    final location = GoRouterState.of(context).uri.toString();

    if (location.startsWith('/dashboard')) {
      _selectedIndex = 0;
    } else if (location.startsWith('/nodes')) {
      _selectedIndex = 1;
    } else if (location.startsWith('/playbooks')) {
      _selectedIndex = 2;
    } else if (location.startsWith('/tasks')) {
      _selectedIndex = 3;
    } else if (location.startsWith('/schedules')) {
      _selectedIndex = 4;
    } else if (location.startsWith('/plugins')) {
      _selectedIndex = 5;
    } else if (location.startsWith('/users')) {
      _selectedIndex = 6;
    } else if (location.startsWith('/logs')) {
      _selectedIndex = 7;
    } else if (location.startsWith('/settings')) {
      _selectedIndex = 8;
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: AppTheme.darkBackground,
      body: Column(
        children: [
          // Custom title bar
          const WindowTitleBar(),

          // Main content area
          Expanded(
            child: Row(
              children: [
                // Sidebar navigation
                DesktopSidebar(
                  selectedIndex: _selectedIndex,
                  onDestinationSelected: (index) {
                    setState(() {
                      _selectedIndex = index;
                    });
                  },
                ),

                // Main content
                Expanded(
                  child: Container(
                    decoration: BoxDecoration(
                      color: AppTheme.darkBackground,
                    ),
                    child: widget.child,
                  ),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

/// Desktop login page layout
class DesktopLoginShell extends StatelessWidget {
  final Widget child;

  const DesktopLoginShell({
    super.key,
    required this.child,
  });

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: AppTheme.darkBackground,
      body: Column(
        children: [
          // Minimal title bar for login
          WindowTitleBar(
            leading: Container(),
            title: '',
          ),

          // Login form centered
          Expanded(
            child: Center(
              child: Container(
                constraints: const BoxConstraints(maxWidth: 440),
                child: child,
              ),
            ),
          ),
        ],
      ),
    );
  }
}