import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../../../../core/models/mfa_models.dart';
import '../../../../core/providers/api_providers.dart';

/// MFA state
class MFAState {
  final MFAStatus? status;
  final MFASetupResult? setupResult;
  final bool isLoading;
  final String? error;

  const MFAState({
    this.status,
    this.setupResult,
    this.isLoading = false,
    this.error,
  });

  MFAState copyWith({
    MFAStatus? status,
    MFASetupResult? setupResult,
    bool? isLoading,
    String? error,
  }) {
    return MFAState(
      status: status ?? this.status,
      setupResult: setupResult ?? this.setupResult,
      isLoading: isLoading ?? this.isLoading,
      error: error ?? this.error,
    );
  }
}

/// Provider for MFAState
final mfaProvider = NotifierProvider<MFANotifier, MFAState>(MFANotifier.new);

/// MFA notifier
class MFANotifier extends Notifier<MFAState> {
  @override
  MFAState build() {
    Future.microtask(() => loadStatus());
    return const MFAState();
  }

  /// Load MFA status
  Future<void> loadStatus() async {
    state = state.copyWith(isLoading: true, error: null);

    try {
      final client = ref.read(apiClientProvider);
      final response = await client.mfa.status();
      state = state.copyWith(
        status: response.data,
        isLoading: false,
      );
    } catch (e) {
      state = state.copyWith(
        isLoading: false,
        error: e.toString(),
      );
    }
  }

  /// Start MFA setup
  Future<MFASetupResult?> setup() async {
    state = state.copyWith(isLoading: true, error: null);

    try {
      final client = ref.read(apiClientProvider);
      final response = await client.mfa.setup();
      state = state.copyWith(
        setupResult: response.data,
        isLoading: false,
      );
      return response.data;
    } catch (e) {
      state = state.copyWith(
        isLoading: false,
        error: e.toString(),
      );
      return null;
    }
  }

  /// Enable MFA with verification code
  Future<bool> enable(String code) async {
    state = state.copyWith(isLoading: true, error: null);

    try {
      final client = ref.read(apiClientProvider);
      final response = await client.mfa.enable(code);
      if (response.data.success) {
        await loadStatus();
        state = state.copyWith(setupResult: null);
      } else {
        state = state.copyWith(
          isLoading: false,
          error: response.data.message ?? 'Invalid verification code',
        );
      }
      return response.data.success;
    } catch (e) {
      state = state.copyWith(
        isLoading: false,
        error: e.toString(),
      );
      return false;
    }
  }

  /// Disable MFA with verification code
  Future<bool> disable(String code) async {
    state = state.copyWith(isLoading: true, error: null);

    try {
      final client = ref.read(apiClientProvider);
      final response = await client.mfa.disable(code);
      if (response.data.success) {
        await loadStatus();
      } else {
        state = state.copyWith(
          isLoading: false,
          error: response.data.message ?? 'Invalid verification code',
        );
      }
      return response.data.success;
    } catch (e) {
      state = state.copyWith(
        isLoading: false,
        error: e.toString(),
      );
      return false;
    }
  }

  /// Regenerate recovery codes
  Future<List<String>?> regenerateRecoveryCodes(String code) async {
    state = state.copyWith(isLoading: true, error: null);

    try {
      final client = ref.read(apiClientProvider);
      final response = await client.mfa.regenerateRecoveryCodes(code);
      state = state.copyWith(isLoading: false);
      return response.data.recoveryCodes;
    } catch (e) {
      state = state.copyWith(
        isLoading: false,
        error: e.toString(),
      );
      return null;
    }
  }

  /// Clear error
  void clearError() {
    state = state.copyWith(error: null);
  }

  /// Clear setup result
  void clearSetupResult() {
    state = state.copyWith(setupResult: null);
  }
}