import 'dart:io';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:window_manager/window_manager.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'core/theme/app_theme.dart';
import 'core/router/app_router.dart';
import 'l10n/app_localizations.dart';
import 'core/providers/locale_provider.dart';
import 'features/auth/presentation/providers/auth_provider.dart';
import 'desktop/window_manager.dart' as desktop;

void main() async {
  WidgetsFlutterBinding.ensureInitialized();

  // Initialize desktop window if on desktop platform
  if (Platform.isWindows || Platform.isMacOS || Platform.isLinux) {
    await desktop.DesktopWindow.initialize();
  }

  // Initialize SharedPreferences
  final sharedPreferences = await SharedPreferences.getInstance();

  runApp(
    ProviderScope(
      overrides: [
        sharedPreferencesProvider.overrideWithValue(sharedPreferences),
      ],
      child: const AnixOpsApp(),
    ),
  );
}

class AnixOpsApp extends ConsumerWidget {
  const AnixOpsApp({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final router = ref.watch(routerProvider);
    final themeMode = ref.watch(themeModeProvider);
    final locale = ref.watch(localeProvider);

    return MaterialApp.router(
      title: 'AnixOps Control Center',
      debugShowCheckedModeBanner: false,

      // Theme
      theme: AppTheme.light,
      darkTheme: AppTheme.dark,
      themeMode: themeMode,

      // Localization
      localizationsDelegates: AppLocalizations.localizationsDelegates,
      supportedLocales: AppLocalizations.supportedLocales,
      locale: locale,

      // Router
      routerConfig: router,
    );
  }
}

/// Check if running on desktop
bool get isDesktop {
  return Platform.isWindows || Platform.isMacOS || Platform.isLinux;
}

/// Check if running on mobile
bool get isMobile {
  return Platform.isAndroid || Platform.isIOS;
}