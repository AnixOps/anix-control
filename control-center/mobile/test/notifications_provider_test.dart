import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:anixops_mobile/features/notifications/presentation/providers/notifications_provider.dart';
import 'package:anixops_mobile/core/models/notification_models.dart';

void main() {
  group('NotificationsProvider', () {
    test('initial state is correct', () {
      final container = ProviderContainer();
      final state = container.read(notificationsProvider);

      expect(state.notifications, isEmpty);
      expect(state.isLoading, false);
      expect(state.error, isNull);
      expect(state.unreadCount, 0);
    });

    group('Notification model', () {
      test('is created correctly from JSON', () {
        final json = {
          'id': 123,
          'user_id': 1,
          'title': 'Test Notification',
          'message': 'This is a test message',
          'type': 'warning',
          'read': false,
          'created_at': '2026-03-20T10:00:00Z',
          'resource_type': 'node',
          'resource_id': 'node-1',
        };

        final notification = Notification.fromJson(json);

        expect(notification.id, 123);
        expect(notification.userId, 1);
        expect(notification.title, 'Test Notification');
        expect(notification.message, 'This is a test message');
        expect(notification.type, NotificationType.warning);
        expect(notification.read, false);
        expect(notification.createdAt, isNotNull);
        expect(notification.resourceType, 'node');
        expect(notification.resourceId, 'node-1');
      });

      test('handles missing optional fields', () {
        final json = <String, dynamic>{
          'id': 2,
          'user_id': 1,
          'title': 'Simple',
          'message': 'Message',
          'type': 'info',
          'read': false,
          'created_at': '2026-03-20T10:00:00Z',
        };

        final notification = Notification.fromJson(json);

        expect(notification.id, 2);
        expect(notification.title, 'Simple');
        expect(notification.message, 'Message');
        expect(notification.type, NotificationType.info);
        expect(notification.read, false);
        expect(notification.resourceType, isNull);
        expect(notification.resourceId, isNull);
      });
    });

    group('NotificationsState', () {
      test('copyWith works correctly', () {
        const original = NotificationsState();
        final updated = original.copyWith(
          isLoading: true,
          error: 'Test error',
          unreadCount: 5,
        );

        expect(updated.isLoading, true);
        expect(updated.error, 'Test error');
        expect(updated.unreadCount, 5);
        expect(updated.notifications, isEmpty);
      });

      test('unreadNotifications filters correctly', () {
        final state = NotificationsState(
          notifications: [
            Notification(id: 1, userId: 1, title: '1', message: 'm1', type: NotificationType.info, read: false, createdAt: '2026-03-20T10:00:00Z'),
            Notification(id: 2, userId: 1, title: '2', message: 'm2', type: NotificationType.info, read: true, createdAt: '2026-03-20T10:00:00Z'),
            Notification(id: 3, userId: 1, title: '3', message: 'm3', type: NotificationType.info, read: false, createdAt: '2026-03-20T10:00:00Z'),
          ],
        );

        expect(state.unreadNotifications.length, 2);
        expect(state.readNotifications.length, 1);
      });
    });
  });
}