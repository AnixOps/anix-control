import 'package:flutter_test/flutter_test.dart';
import 'package:anixops_mobile/core/models/mfa_models.dart';
import 'package:anixops_mobile/features/settings/presentation/providers/mfa_provider.dart';

void main() {
  group('MFAStatus model', () {
    test('is created correctly from JSON', () {
      final json = {
        'enabled': true,
        'has_recovery_codes': true,
        'enabled_at': '2026-03-20T10:00:00Z',
        'method': 'totp',
      };

      final status = MFAStatus.fromJson(json);

      expect(status.enabled, true);
      expect(status.hasRecoveryCodes, true);
      expect(status.enabledAt, isNotNull);
      expect(status.method, 'totp');
    });

    test('handles missing optional fields with defaults', () {
      final json = <String, dynamic>{};

      final status = MFAStatus.fromJson(json);

      expect(status.enabled, false);
      expect(status.hasRecoveryCodes, false);
      expect(status.enabledAt, isNull);
      expect(status.method, isNull);
    });

    test('handles disabled status', () {
      final json = {
        'enabled': false,
        'has_recovery_codes': false,
      };

      final status = MFAStatus.fromJson(json);

      expect(status.enabled, false);
      expect(status.hasRecoveryCodes, false);
    });
  });

  group('MFASetupResult model', () {
    test('is created correctly from JSON', () {
      final json = {
        'secret': 'JBSWY3DPEHPK3PXP',
        'otpauth_url': 'otpauth://totp/AnixOps:test@example.com?secret=JBSWY3DPEHPK3PXP',
        'recovery_codes': ['code1', 'code2', 'code3'],
      };

      final result = MFASetupResult.fromJson(json);

      expect(result.secret, 'JBSWY3DPEHPK3PXP');
      expect(result.otpauthUrl, contains('otpauth://'));
      expect(result.recoveryCodes.length, 3);
      expect(result.recoveryCodes, ['code1', 'code2', 'code3']);
    });

    test('handles missing fields with defaults', () {
      final json = <String, dynamic>{};

      final result = MFASetupResult.fromJson(json);

      expect(result.secret, '');
      expect(result.otpauthUrl, '');
      expect(result.recoveryCodes, isEmpty);
    });
  });

  group('MFAState', () {
    test('initial state is correct', () {
      const state = MFAState();

      expect(state.status, isNull);
      expect(state.setupResult, isNull);
      expect(state.isLoading, false);
      expect(state.error, isNull);
    });

    test('copyWith works correctly', () {
      const state = MFAState();

      final newStatus = MFAStatus(enabled: true);
      final newState = state.copyWith(
        status: newStatus,
        isLoading: true,
      );

      expect(newState.status?.enabled, true);
      expect(newState.isLoading, true);
      expect(newState.setupResult, isNull);
    });

    test('stores setup result correctly', () {
      final setupResult = MFASetupResult(
        secret: 'test-secret',
        otpauthUrl: 'otpauth://test',
        recoveryCodes: ['code1', 'code2'],
      );

      final state = MFAState(setupResult: setupResult);

      expect(state.setupResult?.secret, 'test-secret');
      expect(state.setupResult?.recoveryCodes.length, 2);
    });

    test('handles loading state', () {
      const state1 = MFAState(isLoading: true);
      const state2 = MFAState(isLoading: false);

      expect(state1.isLoading, true);
      expect(state2.isLoading, false);
    });

    test('handles error state', () {
      const state = MFAState(error: 'Invalid verification code');

      expect(state.error, 'Invalid verification code');
    });
  });
}