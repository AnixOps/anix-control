import 'package:dio/dio.dart';
import 'package:anixops_mobile/core/models/notification_models.dart';
import 'package:anixops_mobile/core/models/api_response.dart';

/// Notifications API service
class NotificationsApi {
  final Dio _dio;

  NotificationsApi(this._dio);

  /// List notifications
  Future<NotificationListResponse> list({
    int limit = 50,
    int page = 1,
    bool? unreadOnly,
  }) async {
    final queryParams = <String, dynamic>{
      'limit': limit,
      'page': page,
    };
    if (unreadOnly == true) {
      queryParams['unread_only'] = true;
    }

    final response = await _dio.get('/notifications', queryParameters: queryParams);
    return NotificationListResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Get unread count
  Future<UnreadCountResponse> unreadCount() async {
    final response = await _dio.get('/notifications/unread-count');
    return UnreadCountResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Mark notification as read
  Future<ApiMessageResponse> markAsRead(int id) async {
    final response = await _dio.put('/notifications/$id/read');
    return ApiMessageResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Mark all as read
  Future<ApiMessageResponse> markAllAsRead() async {
    final response = await _dio.put('/notifications/read-all');
    return ApiMessageResponse.fromJson(response.data as Map<String, dynamic>);
  }

  /// Delete notification
  Future<ApiMessageResponse> delete(int id) async {
    final response = await _dio.delete('/notifications/$id');
    return ApiMessageResponse.fromJson(response.data as Map<String, dynamic>);
  }
}