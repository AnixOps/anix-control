import 'package:flutter_test/flutter_test.dart';
import 'package:anixops_mobile/features/auth/presentation/providers/auth_provider.dart';

void main() {
  group('AuthState', () {
    test('initial state is correct', () {
      const state = AuthState();

      expect(state.isAuthenticated, false);
      expect(state.isLoading, false);
      expect(state.token, isNull);
      expect(state.refreshToken, isNull);
      expect(state.userId, isNull);
      expect(state.email, isNull);
      expect(state.role, isNull);
      expect(state.error, isNull);
    });

    test('copyWith works correctly', () {
      const state = AuthState();

      final newState = state.copyWith(
        isAuthenticated: true,
        token: 'test-token',
        userId: 1,
        email: 'test@example.com',
        role: 'admin',
      );

      expect(newState.isAuthenticated, true);
      expect(newState.token, 'test-token');
      expect(newState.userId, 1);
      expect(newState.email, 'test@example.com');
      expect(newState.role, 'admin');
      expect(newState.isLoading, false);
    });

    test('copyWith can clear error with null', () {
      const state = AuthState(error: 'Previous error');

      final newState = state.copyWith(isLoading: true);

      expect(newState.error, isNull);
      expect(newState.isLoading, true);
    });

    test('isAuthenticated reflects auth status', () {
      const unauthenticatedState = AuthState();
      const authenticatedState = AuthState(
        isAuthenticated: true,
        token: 'valid-token',
      );

      expect(unauthenticatedState.isAuthenticated, false);
      expect(authenticatedState.isAuthenticated, true);
    });

    test('stores user info correctly', () {
      final state = AuthState(
        isAuthenticated: true,
        token: 'access-token',
        refreshToken: 'refresh-token',
        userId: 123,
        email: 'admin@anixops.com',
        role: 'admin',
      );

      expect(state.token, 'access-token');
      expect(state.refreshToken, 'refresh-token');
      expect(state.userId, 123);
      expect(state.email, 'admin@anixops.com');
      expect(state.role, 'admin');
    });

    test('handles different roles', () {
      const adminState = AuthState(role: 'admin');
      const operatorState = AuthState(role: 'operator');
      const viewerState = AuthState(role: 'viewer');

      expect(adminState.role, 'admin');
      expect(operatorState.role, 'operator');
      expect(viewerState.role, 'viewer');
    });

    test('copyWith preserves existing values', () {
      const state = AuthState(
        token: 'existing-token',
        email: 'existing@test.com',
      );

      final newState = state.copyWith(isLoading: true);

      expect(newState.token, 'existing-token');
      expect(newState.email, 'existing@test.com');
      expect(newState.isLoading, true);
    });
  });
}