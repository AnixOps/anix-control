import 'package:flutter/material.dart';
import 'package:window_manager/window_manager.dart';
import 'package:anixops_mobile/core/theme/app_theme.dart';

/// Custom window title bar with minimize, maximize, close buttons
class WindowTitleBar extends StatefulWidget {
  final String title;
  final Widget? leading;

  const WindowTitleBar({
    super.key,
    this.title = 'AnixOps Control Center',
    this.leading,
  });

  @override
  State<WindowTitleBar> createState() => _WindowTitleBarState();
}

class _WindowTitleBarState extends State<WindowTitleBar> with WindowListener {
  bool _isMaximized = false;

  @override
  void initState() {
    super.initState();
    windowManager.addListener(this);
    _checkMaximized();
  }

  @override
  void dispose() {
    windowManager.removeListener(this);
    super.dispose();
  }

  @override
  void onWindowMaximize() {
    setState(() => _isMaximized = true);
  }

  @override
  void onWindowUnmaximize() {
    setState(() => _isMaximized = false);
  }

  Future<void> _checkMaximized() async {
    final isMax = await windowManager.isMaximized();
    if (mounted) {
      setState(() => _isMaximized = isMax);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Container(
      height: 40,
      decoration: BoxDecoration(
        color: AppTheme.darkSurface,
        border: Border(
          bottom: BorderSide(color: AppTheme.darkBorder),
        ),
      ),
      child: Row(
        children: [
          // Leading widget (hamburger menu or back button)
          if (widget.leading != null) widget.leading!,

          // Window drag area with title
          Expanded(
            child: GestureDetector(
              behavior: HitTestBehavior.translucent,
              onPanStart: (_) {
                windowManager.startDragging();
              },
              onDoubleTap: () {
                _toggleMaximize();
              },
              child: Container(
                alignment: Alignment.centerLeft,
                padding: const EdgeInsets.symmetric(horizontal: 16),
                child: Row(
                  children: [
                    Text(
                      widget.title,
                      style: TextStyle(
                        fontSize: 13,
                        color: AppTheme.darkText,
                        fontWeight: FontWeight.w500,
                      ),
                    ),
                  ],
                ),
              ),
            ),
          ),

          // Window control buttons
          _buildWindowControls(),
        ],
      ),
    );
  }

  Widget _buildWindowControls() {
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        _WindowButton(
          icon: Icons.remove_rounded,
          onPressed: _minimize,
          tooltip: 'Minimize',
        ),
        _WindowButton(
          icon: _isMaximized ? Icons.filter_none_rounded : Icons.crop_square_rounded,
          onPressed: _toggleMaximize,
          tooltip: _isMaximized ? 'Restore' : 'Maximize',
        ),
        _WindowButton(
          icon: Icons.close_rounded,
          onPressed: _close,
          tooltip: 'Close',
          isClose: true,
        ),
      ],
    );
  }

  void _minimize() {
    windowManager.minimize();
  }

  Future<void> _toggleMaximize() async {
    if (_isMaximized) {
      await windowManager.unmaximize();
    } else {
      await windowManager.maximize();
    }
  }

  void _close() {
    windowManager.close();
  }
}

class _WindowButton extends StatelessWidget {
  final IconData icon;
  final VoidCallback? onPressed;
  final String tooltip;
  final bool isClose;

  const _WindowButton({
    required this.icon,
    this.onPressed,
    required this.tooltip,
    this.isClose = false,
  });

  @override
  Widget build(BuildContext context) {
    return Tooltip(
      message: tooltip,
      waitDuration: const Duration(milliseconds: 500),
      child: Material(
        color: Colors.transparent,
        child: InkWell(
          onTap: onPressed,
          child: Container(
            width: 46,
            height: 40,
            alignment: Alignment.center,
            child: Icon(
              icon,
              size: 18,
              color: isClose ? null : AppTheme.darkTextSecondary,
            ),
          ),
        ),
      ),
    );
  }
}