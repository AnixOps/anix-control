import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:shared_preferences/shared_preferences.dart';

/// Theme mode provider
final themeModeProvider = NotifierProvider<ThemeModeNotifier, ThemeMode>(ThemeModeNotifier.new);

class ThemeModeNotifier extends Notifier<ThemeMode> {
  @override
  ThemeMode build() {
    _loadThemeMode();
    return ThemeMode.system;
  }

  Future<void> _loadThemeMode() async {
    final prefs = await SharedPreferences.getInstance();
    final mode = prefs.getString('themeMode');
    switch (mode) {
      case 'light':
        state = ThemeMode.light;
        break;
      case 'dark':
        state = ThemeMode.dark;
        break;
      default:
        state = ThemeMode.system;
    }
  }

  Future<void> setThemeMode(ThemeMode mode) async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.setString('themeMode', mode.name);
    state = mode;
  }
}

/// Available locales for the app
const availableLocales = [
  Locale('en'),
  Locale('zh'),
  Locale('ja', 'JP'),
  Locale('zh', 'TW'),
  Locale('ar', 'SA'),
];

/// Locale names for display
const localeNames = {
  'en': 'English',
  'zh': '简体中文',
  'ja_JP': '日本語',
  'zh_TW': '繁體中文',
  'ar_SA': 'العربية',
};

/// Locale provider
final localeProvider = NotifierProvider<LocaleNotifier, Locale?>(LocaleNotifier.new);

class LocaleNotifier extends Notifier<Locale?> {
  @override
  Locale? build() {
    _loadLocale();
    return null;
  }

  Future<void> _loadLocale() async {
    final prefs = await SharedPreferences.getInstance();
    final localeCode = prefs.getString('locale');

    if (localeCode != null) {
      final parts = localeCode.split('_');
      if (parts.length == 2) {
        state = Locale(parts[0], parts[1]);
      } else {
        state = Locale(parts[0]);
      }
    }
  }

  Future<void> setLocale(Locale locale) async {
    final prefs = await SharedPreferences.getInstance();
    final localeCode = locale.countryCode != null
        ? '${locale.languageCode}_${locale.countryCode}'
        : locale.languageCode;

    await prefs.setString('locale', localeCode);
    state = locale;
  }

  Future<void> clearLocale() async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.remove('locale');
    state = null;
  }
}

/// Helper to check if locale is RTL
bool isRTL(Locale locale) {
  const rtlLocales = ['ar', 'he', 'fa', 'ur'];
  return rtlLocales.contains(locale.languageCode);
}

/// Helper to get locale display name
String getLocaleName(Locale locale) {
  final key = locale.countryCode != null
      ? '${locale.languageCode}_${locale.countryCode}'
      : locale.languageCode;
  return localeNames[key] ?? locale.languageCode;
}