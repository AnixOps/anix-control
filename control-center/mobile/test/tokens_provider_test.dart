import 'package:flutter_test/flutter_test.dart';
import 'package:anixops_mobile/core/services/tokens_api.dart';

void main() {
  group('ApiToken model', () {
    test('is created correctly from JSON', () {
      final json = {
        'id': 'token-123',
        'name': 'CI Token',
        'token': 'secret-token-abc',
        'created_at': '2026-03-20T10:00:00Z',
        'last_used_at': '2026-03-21T15:30:00Z',
        'expires_at': '2027-03-20T10:00:00Z',
      };

      final token = ApiToken.fromJson(json);

      expect(token.id, 'token-123');
      expect(token.name, 'CI Token');
      expect(token.token, 'secret-token-abc');
      expect(token.createdAt, isNotNull);
      expect(token.lastUsedAt, isNotNull);
      expect(token.expiresAt, isNotNull);
    });

    test('handles missing optional fields', () {
      final json = <String, dynamic>{
        'id': '2',
        'name': 'Test Token',
      };

      final token = ApiToken.fromJson(json);

      expect(token.id, '2');
      expect(token.name, 'Test Token');
      expect(token.token, isNull);
      expect(token.lastUsedAt, isNull);
      expect(token.expiresAt, isNull);
    });

    test('isExpired returns false when no expiry', () {
      final token = ApiToken(
        id: '1',
        name: 'No Expiry',
        createdAt: DateTime.now(),
      );

      expect(token.isExpired, false);
    });

    test('isExpired returns false when not expired', () {
      final token = ApiToken(
        id: '1',
        name: 'Future Expiry',
        createdAt: DateTime.now(),
        expiresAt: DateTime.now().add(const Duration(days: 30)),
      );

      expect(token.isExpired, false);
    });

    test('isExpired returns true when expired', () {
      final token = ApiToken(
        id: '1',
        name: 'Expired',
        createdAt: DateTime.now().subtract(const Duration(days: 60)),
        expiresAt: DateTime.now().subtract(const Duration(days: 30)),
      );

      expect(token.isExpired, true);
    });

    test('toJson serializes correctly', () {
      final token = ApiToken(
        id: '1',
        name: 'Test',
        token: 'secret',
        createdAt: DateTime(2026, 3, 20, 10, 0),
        lastUsedAt: DateTime(2026, 3, 21, 15, 30),
        expiresAt: DateTime(2027, 3, 20, 10, 0),
      );

      final json = token.toJson();

      expect(json['id'], '1');
      expect(json['name'], 'Test');
      expect(json['token'], 'secret');
      expect(json['created_at'], contains('2026-03-20'));
      expect(json['last_used_at'], contains('2026-03-21'));
      expect(json['expires_at'], contains('2027-03-20'));
    });
  });

  group('Session model', () {
    test('is created correctly from JSON', () {
      final json = {
        'id': 'session-123',
        'device': 'Chrome on Windows',
        'ip_address': '192.168.1.1',
        'location': 'New York, US',
        'created_at': '2026-03-20T10:00:00Z',
        'last_active_at': '2026-03-21T15:30:00Z',
        'is_current': true,
      };

      final session = Session.fromJson(json);

      expect(session.id, 'session-123');
      expect(session.device, 'Chrome on Windows');
      expect(session.ipAddress, '192.168.1.1');
      expect(session.location, 'New York, US');
      expect(session.isCurrent, true);
    });

    test('handles user_agent as device fallback', () {
      final json = {
        'id': '2',
        'user_agent': 'Mozilla/5.0',
        'created_at': '2026-03-20T10:00:00Z',
        'last_active_at': '2026-03-20T10:00:00Z',
      };

      final session = Session.fromJson(json);

      expect(session.device, 'Mozilla/5.0');
    });

    test('handles missing fields with defaults', () {
      final json = <String, dynamic>{
        'id': '3',
      };

      final session = Session.fromJson(json);

      expect(session.id, '3');
      expect(session.device, 'Unknown Device');
      expect(session.ipAddress, isNull);
      expect(session.location, isNull);
      expect(session.isCurrent, false);
    });

    test('toJson serializes correctly', () {
      final session = Session(
        id: '1',
        device: 'Chrome',
        ipAddress: '10.0.0.1',
        location: 'Tokyo',
        createdAt: DateTime(2026, 3, 20),
        lastActiveAt: DateTime(2026, 3, 21),
        isCurrent: true,
      );

      final json = session.toJson();

      expect(json['id'], '1');
      expect(json['device'], 'Chrome');
      expect(json['ip_address'], '10.0.0.1');
      expect(json['location'], 'Tokyo');
      expect(json['is_current'], true);
    });
  });
}