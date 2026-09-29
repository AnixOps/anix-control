import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:dio/dio.dart';
import 'package:anixops_mobile/core/services/api_client.dart';
import 'package:anixops_mobile/core/services/sse_service.dart';
import 'package:anixops_mobile/core/providers/api_providers.dart';
import 'package:anixops_mobile/core/models/auth_models.dart';

class AuthState {
  final bool isAuthenticated;
  final bool isLoading;
  final String? token;
  final String? refreshToken;
  final int? userId;
  final String? email;
  final String? role;
  final String? error;

  const AuthState({
    this.isAuthenticated = false,
    this.isLoading = false,
    this.token,
    this.refreshToken,
    this.userId,
    this.email,
    this.role,
    this.error,
  });

  AuthState copyWith({
    bool? isAuthenticated,
    bool? isLoading,
    String? token,
    String? refreshToken,
    int? userId,
    String? email,
    String? role,
    String? error,
  }) {
    return AuthState(
      isAuthenticated: isAuthenticated ?? this.isAuthenticated,
      isLoading: isLoading ?? this.isLoading,
      token: token ?? this.token,
      refreshToken: refreshToken ?? this.refreshToken,
      userId: userId ?? this.userId,
      email: email ?? this.email,
      role: role ?? this.role,
      error: error,
    );
  }
}

// Provider for SharedPreferences
final sharedPreferencesProvider = Provider<SharedPreferences>((ref) {
  throw UnimplementedError('SharedPreferences must be overridden');
});

// Auth state provider
final authStateProvider = NotifierProvider<AuthNotifier, AuthState>(AuthNotifier.new);

class AuthNotifier extends Notifier<AuthState> {
  late final ApiClient _apiClient;
  late final SharedPreferences _prefs;
  late final SSEService _sseService;

  static const String _tokenKey = 'auth_token';
  static const String _refreshTokenKey = 'refresh_token';
  static const String _userIdKey = 'user_id';
  static const String _emailKey = 'user_email';
  static const String _roleKey = 'user_role';

  @override
  AuthState build() {
    _apiClient = ref.read(apiClientProvider);
    _prefs = ref.read(sharedPreferencesProvider);
    _sseService = ref.read(sseServiceProvider);
    _loadStoredAuth();
    return const AuthState();
  }

  Future<void> _loadStoredAuth() async {
    final token = _prefs.getString(_tokenKey);
    if (token != null) {
      _apiClient.setAuthToken(token);
      state = AuthState(
        isAuthenticated: true,
        token: token,
        refreshToken: _prefs.getString(_refreshTokenKey),
        userId: _prefs.getInt(_userIdKey),
        email: _prefs.getString(_emailKey),
        role: _prefs.getString(_roleKey),
      );
      _connectSSE(token);
    }
  }

  Future<bool> login(String email, String password) async {
    state = state.copyWith(isLoading: true, error: null);

    try {
      final response = await _apiClient.auth.login(email, password);

      final token = response.data.accessToken;
      final refreshToken = response.data.refreshToken;
      final user = response.data.user;

      await _prefs.setString(_tokenKey, token);
      await _prefs.setString(_refreshTokenKey, refreshToken);
      await _prefs.setInt(_userIdKey, user.id);
      await _prefs.setString(_emailKey, user.email);
      await _prefs.setString(_roleKey, user.role.name);

      _apiClient.setAuthToken(token);
      _connectSSE(token);

      state = AuthState(
        isAuthenticated: true,
        isLoading: false,
        token: token,
        refreshToken: refreshToken,
        userId: user.id,
        email: user.email,
        role: user.role.name,
      );

      return true;
    } on DioException catch (e) {
      final error = e.response?.data?['error'] ?? 'Network error';
      state = state.copyWith(isLoading: false, error: error);
      return false;
    } catch (e) {
      state = state.copyWith(isLoading: false, error: e.toString());
      return false;
    }
  }

  void _connectSSE(String token) {
    _sseService.connect(SSEConfig.defaultUrl, token: token).then((_) {
      for (final channel in SSEConfig.defaultChannels) {
        _sseService.subscribe(channel);
      }
    });
  }

  Future<void> logout() async {
    _sseService.disconnect();

    try {
      await _apiClient.auth.logout();
    } catch (_) {}

    await _prefs.remove(_tokenKey);
    await _prefs.remove(_refreshTokenKey);
    await _prefs.remove(_userIdKey);
    await _prefs.remove(_emailKey);
    await _prefs.remove(_roleKey);

    _apiClient.clearAuthToken();
    state = const AuthState();
  }

  Future<bool> refreshAuthToken() async {
    final storedRefreshToken = _prefs.getString(_refreshTokenKey);
    if (storedRefreshToken == null) return false;

    try {
      final response = await _apiClient.auth.refresh(storedRefreshToken);
      final token = response.data.accessToken;

      await _prefs.setString(_tokenKey, token);
      _apiClient.setAuthToken(token);

      state = state.copyWith(token: token);
      return true;
    } catch (_) {}

    await logout();
    return false;
  }

  Future<bool> checkAuth() async {
    if (!state.isAuthenticated) return false;

    try {
      final response = await _apiClient.auth.me();
      return response.success;
    } catch (_) {
      return false;
    }
  }

  Future<bool> register(String email, String password) async {
    state = state.copyWith(isLoading: true, error: null);

    try {
      await _apiClient.auth.register(email, password);
      return await login(email, password);
    } on DioException catch (e) {
      final error = e.response?.data?['error'] ?? 'Network error';
      state = state.copyWith(isLoading: false, error: error);
      return false;
    } catch (e) {
      state = state.copyWith(isLoading: false, error: e.toString());
      return false;
    }
  }
}