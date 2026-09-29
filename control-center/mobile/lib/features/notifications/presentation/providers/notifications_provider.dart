import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../../../../core/models/notification_models.dart';
import '../../../../core/providers/api_providers.dart';

/// Notifications state
class NotificationsState {
  final List<Notification> notifications;
  final bool isLoading;
  final String? error;
  final int unreadCount;

  const NotificationsState({
    this.notifications = const [],
    this.isLoading = false,
    this.error,
    this.unreadCount = 0,
  });

  NotificationsState copyWith({
    List<Notification>? notifications,
    bool? isLoading,
    String? error,
    int? unreadCount,
  }) {
    return NotificationsState(
      notifications: notifications ?? this.notifications,
      isLoading: isLoading ?? this.isLoading,
      error: error,
      unreadCount: unreadCount ?? this.unreadCount,
    );
  }

  List<Notification> get unreadNotifications =>
      notifications.where((n) => !n.read).toList();

  List<Notification> get readNotifications =>
      notifications.where((n) => n.read).toList();
}

/// Provider for notifications
final notificationsProvider = NotifierProvider<NotificationsNotifier, NotificationsState>(NotificationsNotifier.new);

/// Provider for unread count
final unreadCountProvider = Provider<int>((ref) {
  return ref.watch(notificationsProvider).unreadCount;
});

/// Notifications notifier
class NotificationsNotifier extends Notifier<NotificationsState> {
  @override
  NotificationsState build() => const NotificationsState();

  Future<void> loadNotifications() async {
    state = state.copyWith(isLoading: true, error: null);
    try {
      final client = ref.read(apiClientProvider);
      final response = await client.notifications.list();
      state = state.copyWith(
        notifications: response.data.items,
        isLoading: false,
        unreadCount: response.data.unreadCount,
      );
    } catch (e) {
      state = state.copyWith(
        isLoading: false,
        error: e.toString(),
      );
    }
  }

  Future<void> markAsRead(int id) async {
    try {
      final client = ref.read(apiClientProvider);
      await client.notifications.markAsRead(id);
      final notifications = state.notifications.map((n) {
        if (n.id == id) {
          return Notification(
            id: n.id,
            userId: n.userId,
            type: n.type,
            title: n.title,
            message: n.message,
            resourceType: n.resourceType,
            resourceId: n.resourceId,
            read: true,
            actionUrl: n.actionUrl,
            createdAt: n.createdAt,
          );
        }
        return n;
      }).toList();
      state = state.copyWith(
        notifications: notifications,
        unreadCount: notifications.where((n) => !n.read).length,
      );
    } catch (e) {
      // Ignore error
    }
  }

  Future<void> markAllAsRead() async {
    try {
      final client = ref.read(apiClientProvider);
      await client.notifications.markAllAsRead();
      final notifications = state.notifications.map((n) {
        return Notification(
          id: n.id,
          userId: n.userId,
          type: n.type,
          title: n.title,
          message: n.message,
          resourceType: n.resourceType,
          resourceId: n.resourceId,
          read: true,
          actionUrl: n.actionUrl,
          createdAt: n.createdAt,
        );
      }).toList();
      state = state.copyWith(
        notifications: notifications,
        unreadCount: 0,
      );
    } catch (e) {
      // Ignore error
    }
  }

  Future<void> deleteNotification(int id) async {
    try {
      final client = ref.read(apiClientProvider);
      await client.notifications.delete(id);
      final notifications = state.notifications.where((n) => n.id != id).toList();
      state = state.copyWith(
        notifications: notifications,
        unreadCount: notifications.where((n) => !n.read).length,
      );
    } catch (e) {
      // Still update UI
      final notifications = state.notifications.where((n) => n.id != id).toList();
      state = state.copyWith(
        notifications: notifications,
        unreadCount: notifications.where((n) => !n.read).length,
      );
    }
  }

  Future<int> fetchUnreadCount() async {
    try {
      final client = ref.read(apiClientProvider);
      final response = await client.notifications.unreadCount();
      state = state.copyWith(unreadCount: response.data);
      return response.data;
    } catch (e) {
      return state.unreadCount;
    }
  }

  void clearError() {
    state = state.copyWith(error: null);
  }
}