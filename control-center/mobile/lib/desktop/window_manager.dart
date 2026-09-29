import 'dart:io';
import 'package:flutter/material.dart';
import 'package:window_manager/window_manager.dart';

/// Desktop window configuration
class DesktopWindow {
  static const double minWidth = 1200.0;
  static const double minHeight = 700.0;
  static const double defaultWidth = 1400.0;
  static const double defaultHeight = 900.0;

  static Future<void> initialize() async {
    if (!Platform.isWindows && !Platform.isMacOS && !Platform.isLinux) {
      return;
    }

    await windowManager.ensureInitialized();

    const windowOptions = WindowOptions(
      size: Size(defaultWidth, defaultHeight),
      minimumSize: Size(minWidth, minHeight),
      center: true,
      backgroundColor: Colors.transparent,
      skipTaskbar: false,
      titleBarStyle: TitleBarStyle.hidden,
      title: 'AnixOps Control Center',
    );

    await windowManager.waitUntilReadyToShow(windowOptions, () async {
      await windowManager.show();
      await windowManager.focus();
    });
  }

  static Future<void> setTitle(String title) async {
    await windowManager.setTitle(title);
  }

  static Future<void> minimize() async {
    await windowManager.minimize();
  }

  static Future<void> maximize() async {
    await windowManager.maximize();
  }

  static Future<void> unmaximize() async {
    await windowManager.unmaximize();
  }

  static Future<void> close() async {
    await windowManager.close();
  }

  static Future<bool> isMaximized() async {
    return await windowManager.isMaximized();
  }
}