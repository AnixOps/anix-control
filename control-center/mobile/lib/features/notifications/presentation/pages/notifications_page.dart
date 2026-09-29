import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:anixops_mobile/core/theme/app_theme.dart';
import 'package:anixops_mobile/core/services/api_client.dart';

/// Notification center page
class NotificationsPage extends ConsumerStatefulWidget {
  const NotificationsPage({super.key});

  @override
  ConsumerState<NotificationsPage> createState() => _NotificationsPageState();
}

class _NotificationsPageState extends ConsumerState<NotificationsPage> {
  List<NotificationItem> _notifications = [];
  bool _isLoading = true;
  int _unreadCount = 0;

  @override
  void initState() {
    super.initState();
    _loadNotifications();
  }

  Future<void> _loadNotifications() async {
    setState(() => _isLoading = true);
    try {
      final response = await apiClient.dio.get('/notifications');
      if (response.data['success'] == true) {
        final data = response.data['data'];
        setState(() {
          _notifications = (data['items'] as List)
              .map((json) => NotificationItem.fromJson(json))
              .toList();
          _unreadCount = data['unread_count'] ?? 0;
        });
      }
    } catch (e) {
      // Load mock data on error
      setState(() {
        _notifications = _getMockNotifications();
        _unreadCount = _notifications.where((n) => !n.read).length;
      });
    } finally {
      setState(() => _isLoading = false);
    }
  }

  List<NotificationItem> _getMockNotifications() {
    return [
      NotificationItem(
        id: '1',
        title: 'Node Offline',
        message: 'Node "US-East-1" has gone offline',
        type: NotificationType.error,
        timestamp: DateTime.now().subtract(const Duration(minutes: 5)),
        read: false,
      ),
      NotificationItem(
        id: '2',
        title: 'High CPU Usage',
        message: 'Node "EU-West-2" CPU usage exceeded 90%',
        type: NotificationType.warning,
        timestamp: DateTime.now().subtract(const Duration(hours: 1)),
        read: false,
      ),
      NotificationItem(
        id: '3',
        title: 'New User Registered',
        message: 'user@example.com has registered',
        type: NotificationType.info,
        timestamp: DateTime.now().subtract(const Duration(hours: 3)),
        read: true,
      ),
      NotificationItem(
        id: '4',
        title: 'Backup Completed',
        message: 'Daily backup completed successfully',
        type: NotificationType.success,
        timestamp: DateTime.now().subtract(const Duration(hours: 6)),
        read: true,
      ),
    ];
  }

  Future<void> _markAllRead() async {
    try {
      await apiClient.dio.put('/notifications/read-all');
      setState(() {
        for (var notification in _notifications) {
          notification = notification.copyWith(read: true);
        }
        _notifications = _notifications.map((n) => n.copyWith(read: true)).toList();
        _unreadCount = 0;
      });
    } catch (e) {
      // Still update UI on error
      setState(() {
        _notifications = _notifications.map((n) => n.copyWith(read: true)).toList();
        _unreadCount = 0;
      });
    }
  }

  Future<void> _markAsRead(NotificationItem notification) async {
    try {
      await apiClient.dio.put('/notifications/${notification.id}/read');
      setState(() {
        final index = _notifications.indexWhere((n) => n.id == notification.id);
        if (index != -1) {
          _notifications[index] = notification.copyWith(read: true);
          _unreadCount = _notifications.where((n) => !n.read).length;
        }
      });
    } catch (e) {
      // Still update UI on error
      setState(() {
        final index = _notifications.indexWhere((n) => n.id == notification.id);
        if (index != -1) {
          _notifications[index] = notification.copyWith(read: true);
          _unreadCount = _notifications.where((n) => !n.read).length;
        }
      });
    }
  }

  Future<void> _removeNotification(String id) async {
    try {
      await apiClient.dio.delete('/notifications/$id');
      setState(() {
        _notifications.removeWhere((n) => n.id == id);
        _unreadCount = _notifications.where((n) => !n.read).length;
      });
    } catch (e) {
      // Still update UI on error
      setState(() {
        _notifications.removeWhere((n) => n.id == id);
        _unreadCount = _notifications.where((n) => !n.read).length;
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: Text('Notifications${_unreadCount > 0 ? ' ($_unreadCount)' : ''}'),
        actions: [
          if (_notifications.any((n) => !n.read))
            TextButton(
              onPressed: _markAllRead,
              child: const Text('Mark all read'),
            ),
        ],
      ),
      body: _isLoading
          ? const Center(child: CircularProgressIndicator())
          : _notifications.isEmpty
              ? const Center(child: Text('No notifications'))
              : RefreshIndicator(
                  onRefresh: _loadNotifications,
                  child: ListView.builder(
                    itemCount: _notifications.length,
                    itemBuilder: (context, index) {
                      final notification = _notifications[index];
                      return _NotificationTile(
                        notification: notification,
                        onTap: () => _markAsRead(notification),
                        onDismiss: () => _removeNotification(notification.id),
                      );
                    },
                  ),
                ),
    );
  }
}

class _NotificationTile extends StatelessWidget {
  final NotificationItem notification;
  final VoidCallback? onTap;
  final VoidCallback? onDismiss;

  const _NotificationTile({
    required this.notification,
    this.onTap,
    this.onDismiss,
  });

  @override
  Widget build(BuildContext context) {
    return Dismissible(
      key: Key(notification.id),
      direction: DismissDirection.endToStart,
      background: Container(
        alignment: Alignment.centerRight,
        padding: const EdgeInsets.only(right: 16),
        color: Colors.red,
        child: const Icon(Icons.delete, color: Colors.white),
      ),
      onDismissed: (_) => onDismiss?.call(),
      child: ListTile(
        leading: _buildIcon(),
        title: Text(
          notification.title,
          style: TextStyle(
            fontWeight: notification.read ? FontWeight.normal : FontWeight.bold,
          ),
        ),
        subtitle: Text(notification.message),
        trailing: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.end,
          children: [
            Text(
              _formatTime(notification.timestamp),
              style: const TextStyle(fontSize: 12, color: AppTheme.darkTextSecondary),
            ),
            if (!notification.read)
              Container(
                width: 8,
                height: 8,
                margin: const EdgeInsets.only(top: 4),
                decoration: const BoxDecoration(
                  color: AppTheme.primaryColor,
                  shape: BoxShape.circle,
                ),
              ),
          ],
        ),
        onTap: onTap,
      ),
    );
  }

  Widget _buildIcon() {
    IconData iconData;
    Color color;

    switch (notification.type) {
      case NotificationType.error:
        iconData = Icons.error;
        color = Colors.red;
        break;
      case NotificationType.warning:
        iconData = Icons.warning;
        color = Colors.orange;
        break;
      case NotificationType.success:
        iconData = Icons.check_circle;
        color = Colors.green;
        break;
      case NotificationType.info:
        iconData = Icons.info;
        color = Colors.blue;
        break;
    }

    return Container(
      width: 40,
      height: 40,
      decoration: BoxDecoration(
        color: color.withOpacity(0.1),
        shape: BoxShape.circle,
      ),
      child: Icon(iconData, color: color),
    );
  }

  String _formatTime(DateTime time) {
    final diff = DateTime.now().difference(time);
    if (diff.inMinutes < 60) {
      return '${diff.inMinutes}m ago';
    } else if (diff.inHours < 24) {
      return '${diff.inHours}h ago';
    } else {
      return '${diff.inDays}d ago';
    }
  }
}

enum NotificationType { error, warning, success, info }

class NotificationItem {
  final String id;
  final String title;
  final String message;
  final NotificationType type;
  final DateTime timestamp;
  final bool read;

  NotificationItem({
    required this.id,
    required this.title,
    required this.message,
    required this.type,
    required this.timestamp,
    this.read = false,
  });

  factory NotificationItem.fromJson(Map<String, dynamic> json) {
    return NotificationItem(
      id: json['id']?.toString() ?? '',
      title: json['title'] ?? '',
      message: json['message'] ?? '',
      type: _parseType(json['type'] ?? 'info'),
      timestamp: json['created_at'] != null
          ? DateTime.tryParse(json['created_at']) ?? DateTime.now()
          : DateTime.now(),
      read: json['read'] ?? false,
    );
  }

  static NotificationType _parseType(String type) {
    switch (type.toLowerCase()) {
      case 'error':
        return NotificationType.error;
      case 'warning':
        return NotificationType.warning;
      case 'success':
        return NotificationType.success;
      default:
        return NotificationType.info;
    }
  }

  NotificationItem copyWith({
    String? id,
    String? title,
    String? message,
    NotificationType? type,
    DateTime? timestamp,
    bool? read,
  }) {
    return NotificationItem(
      id: id ?? this.id,
      title: title ?? this.title,
      message: message ?? this.message,
      type: type ?? this.type,
      timestamp: timestamp ?? this.timestamp,
      read: read ?? this.read,
    );
  }
}