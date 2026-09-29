// Notification models matching the backend contract
// See: control-center/workers/src/types.ts

import 'package:anixops_mobile/core/models/api_response.dart';

/// Notification type
enum NotificationType { info, success, warning, error, task, system }

/// Notification entity
class Notification {
  final int id;
  final int userId;
  final NotificationType type;
  final String title;
  final String? message;
  final String? resourceType;
  final String? resourceId;
  final bool read;
  final String? actionUrl;
  final String createdAt;

  Notification({
    required this.id,
    required this.userId,
    required this.type,
    required this.title,
    this.message,
    this.resourceType,
    this.resourceId,
    required this.read,
    this.actionUrl,
    required this.createdAt,
  });

  factory Notification.fromJson(Map<String, dynamic> json) {
    return Notification(
      id: json['id'] as int,
      userId: json['user_id'] as int,
      type: NotificationType.values.firstWhere(
        (e) => e.name == json['type'],
        orElse: () => NotificationType.info,
      ),
      title: json['title'] as String,
      message: json['message'] as String?,
      resourceType: json['resource_type'] as String?,
      resourceId: json['resource_id'] as String?,
      read: json['read'] as bool,
      actionUrl: json['action_url'] as String?,
      createdAt: json['created_at'] as String,
    );
  }
}

/// Notification list response data
class NotificationListResponseData {
  final List<Notification> items;
  final int total;
  final int page;
  final int perPage;
  final int totalPages;
  final int unreadCount;

  NotificationListResponseData({
    required this.items,
    required this.total,
    required this.page,
    required this.perPage,
    required this.totalPages,
    required this.unreadCount,
  });

  factory NotificationListResponseData.fromJson(Map<String, dynamic> json) {
    return NotificationListResponseData(
      items: (json['items'] as List)
          .map((e) => Notification.fromJson(e as Map<String, dynamic>))
          .toList(),
      total: json['total'] as int,
      page: json['page'] as int,
      perPage: json['per_page'] as int,
      totalPages: json['total_pages'] as int,
      unreadCount: json['unread_count'] as int,
    );
  }
}

/// Notification create request
class NotificationCreateRequest {
  final int userId;
  final NotificationType type;
  final String title;
  final String? message;
  final String? resourceType;
  final String? resourceId;
  final String? actionUrl;

  NotificationCreateRequest({
    required this.userId,
    required this.type,
    required this.title,
    this.message,
    this.resourceType,
    this.resourceId,
    this.actionUrl,
  });

  Map<String, dynamic> toJson() => {
        'user_id': userId,
        'type': type.name,
        'title': title,
        if (message != null) 'message': message,
        if (resourceType != null) 'resource_type': resourceType,
        if (resourceId != null) 'resource_id': resourceId,
        if (actionUrl != null) 'action_url': actionUrl,
      };
}

/// Response types
class NotificationListResponse
    extends ApiSuccessResponse<NotificationListResponseData> {
  NotificationListResponse({required super.data});

  factory NotificationListResponse.fromJson(Map<String, dynamic> json) {
    return NotificationListResponse(
      data: NotificationListResponseData.fromJson(
        json['data'] as Map<String, dynamic>,
      ),
    );
  }
}

class NotificationDetailResponse extends ApiSuccessResponse<Notification> {
  NotificationDetailResponse({required super.data});

  factory NotificationDetailResponse.fromJson(Map<String, dynamic> json) {
    return NotificationDetailResponse(
      data: Notification.fromJson(json['data'] as Map<String, dynamic>),
    );
  }
}

class UnreadCountResponse extends ApiSuccessResponse<int> {
  UnreadCountResponse({required super.data});

  factory UnreadCountResponse.fromJson(Map<String, dynamic> json) {
    return UnreadCountResponse(data: json['data'] as int);
  }
}