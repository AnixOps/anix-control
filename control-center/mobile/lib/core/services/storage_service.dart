import 'dart:convert';
import 'package:shared_preferences/shared_preferences.dart';

/// Local storage service using SharedPreferences
class StorageService {
  static StorageService? _instance;
  static SharedPreferences? _prefs;

  StorageService._();

  /// Get singleton instance
  static Future<StorageService> get instance async {
    if (_instance == null) {
      _instance = StorageService._();
      _prefs = await SharedPreferences.getInstance();
    }
    return _instance!;
  }

  /// Store a string value
  Future<bool> setString(String key, String value) async {
    return _prefs!.setString(key, value);
  }

  /// Get a string value
  String? getString(String key) {
    return _prefs!.getString(key);
  }

  /// Store an integer value
  Future<bool> setInt(String key, int value) async {
    return _prefs!.setInt(key, value);
  }

  /// Get an integer value
  int? getInt(String key) {
    return _prefs!.getInt(key);
  }

  /// Store a double value
  Future<bool> setDouble(String key, double value) async {
    return _prefs!.setDouble(key, value);
  }

  /// Get a double value
  double? getDouble(String key) {
    return _prefs!.getDouble(key);
  }

  /// Store a boolean value
  Future<bool> setBool(String key, bool value) async {
    return _prefs!.setBool(key, value);
  }

  /// Get a boolean value
  bool? getBool(String key) {
    return _prefs!.getBool(key);
  }

  /// Store a string list
  Future<bool> setStringList(String key, List<String> value) async {
    return _prefs!.setStringList(key, value);
  }

  /// Get a string list
  List<String>? getStringList(String key) {
    return _prefs!.getStringList(key);
  }

  /// Store a JSON object
  Future<bool> setJson(String key, Map<String, dynamic> value) async {
    return _prefs!.setString(key, jsonEncode(value));
  }

  /// Get a JSON object
  Map<String, dynamic>? getJson(String key) {
    final value = _prefs!.getString(key);
    if (value == null) return null;
    try {
      return jsonDecode(value) as Map<String, dynamic>;
    } catch (e) {
      return null;
    }
  }

  /// Store an object with toJson
  Future<bool> setObject<T>(String key, T object, Map<String, dynamic> Function(T) toJson) async {
    return setJson(key, toJson(object));
  }

  /// Get an object fromJson
  T? getObject<T>(String key, T Function(Map<String, dynamic>) fromJson) {
    final json = getJson(key);
    if (json == null) return null;
    try {
      return fromJson(json);
    } catch (e) {
      return null;
    }
  }

  /// Check if key exists
  bool containsKey(String key) {
    return _prefs!.containsKey(key);
  }

  /// Remove a value
  Future<bool> remove(String key) async {
    return _prefs!.remove(key);
  }

  /// Clear all values
  Future<bool> clear() async {
    return _prefs!.clear();
  }

  /// Get all keys
  Set<String> get keys => _prefs!.getKeys();

  /// Re-read data from disk
  Future<void> reload() async {
    await _prefs!.reload();
  }
}

/// Storage keys constants
class StorageKeys {
  static const String token = 'auth_token';
  static const String refreshToken = 'refresh_token';
  static const String userId = 'user_id';
  static const String userEmail = 'user_email';
  static const String userRole = 'user_role';
  static const String theme = 'theme_mode';
  static const String language = 'language';
  static const String apiUrl = 'api_url';
  static const String notifications = 'notifications_enabled';
  static const String biometricEnabled = 'biometric_enabled';
  static const String lastSync = 'last_sync_time';
  static const String cacheData = 'cache_data';
}