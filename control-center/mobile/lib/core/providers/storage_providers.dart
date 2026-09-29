import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:shared_preferences/shared_preferences.dart';
import '../services/storage_service.dart';

/// Provider for StorageService
final storageServiceProvider = FutureProvider<StorageService>((ref) async {
  return await StorageService.instance;
});

/// Provider for auth token
final authTokenProvider = NotifierProvider<AuthTokenNotifier, String?>(AuthTokenNotifier.new);

class AuthTokenNotifier extends Notifier<String?> {
  @override
  String? build() => null;

  Future<void> load() async {
    final storage = await StorageService.instance;
    state = storage.getString(StorageKeys.token);
  }

  Future<void> set(String? token) async {
    final storage = await StorageService.instance;
    if (token != null) {
      await storage.setString(StorageKeys.token, token);
    } else {
      await storage.remove(StorageKeys.token);
    }
    state = token;
  }

  Future<void> clear() async {
    final storage = await StorageService.instance;
    await storage.remove(StorageKeys.token);
    state = null;
  }
}

/// Provider for theme mode
final themeModeStorageProvider = NotifierProvider<ThemeModeNotifier, String>(ThemeModeNotifier.new);

class ThemeModeNotifier extends Notifier<String> {
  @override
  String build() => 'system';

  Future<void> load() async {
    final storage = await StorageService.instance;
    state = storage.getString(StorageKeys.theme) ?? 'system';
  }

  Future<void> set(String mode) async {
    final storage = await StorageService.instance;
    await storage.setString(StorageKeys.theme, mode);
    state = mode;
  }
}

/// Provider for API URL
final apiUrlProvider = NotifierProvider<ApiUrlNotifier, String>(ApiUrlNotifier.new);

class ApiUrlNotifier extends Notifier<String> {
  @override
  String build() => 'http://localhost:8080/api/v1';

  Future<void> load() async {
    final storage = await StorageService.instance;
    state = storage.getString(StorageKeys.apiUrl) ?? 'http://localhost:8080/api/v1';
  }

  Future<void> set(String url) async {
    final storage = await StorageService.instance;
    await storage.setString(StorageKeys.apiUrl, url);
    state = url;
  }
}

/// Provider for notifications enabled
final notificationsEnabledProvider = NotifierProvider<NotificationsNotifier, bool>(NotificationsNotifier.new);

class NotificationsNotifier extends Notifier<bool> {
  @override
  bool build() => true;

  Future<void> load() async {
    final storage = await StorageService.instance;
    state = storage.getBool(StorageKeys.notifications) ?? true;
  }

  Future<void> set(bool enabled) async {
    final storage = await StorageService.instance;
    await storage.setBool(StorageKeys.notifications, enabled);
    state = enabled;
  }
}

/// Provider for user preferences
final userPreferencesProvider = FutureProvider<UserPreferences>((ref) async {
  final storage = await StorageService.instance;

  return UserPreferences(
    theme: storage.getString(StorageKeys.theme) ?? 'system',
    language: storage.getString(StorageKeys.language) ?? 'en',
    notificationsEnabled: storage.getBool(StorageKeys.notifications) ?? true,
    biometricEnabled: storage.getBool(StorageKeys.biometricEnabled) ?? false,
    apiUrl: storage.getString(StorageKeys.apiUrl) ?? 'http://localhost:8080/api/v1',
  );
});

class UserPreferences {
  final String theme;
  final String language;
  final bool notificationsEnabled;
  final bool biometricEnabled;
  final String apiUrl;

  const UserPreferences({
    required this.theme,
    required this.language,
    required this.notificationsEnabled,
    required this.biometricEnabled,
    required this.apiUrl,
  });

  UserPreferences copyWith({
    String? theme,
    String? language,
    bool? notificationsEnabled,
    bool? biometricEnabled,
    String? apiUrl,
  }) {
    return UserPreferences(
      theme: theme ?? this.theme,
      language: language ?? this.language,
      notificationsEnabled: notificationsEnabled ?? this.notificationsEnabled,
      biometricEnabled: biometricEnabled ?? this.biometricEnabled,
      apiUrl: apiUrl ?? this.apiUrl,
    );
  }
}