import 'package:dio/dio.dart';
import 'package:anixops_mobile/core/services/auth_api.dart';
import 'package:anixops_mobile/core/services/nodes_api.dart';
import 'package:anixops_mobile/core/services/users_api.dart';
import 'package:anixops_mobile/core/services/plugins_api.dart';
import 'package:anixops_mobile/core/services/ssh_api.dart';
import 'package:anixops_mobile/core/services/playbooks_api.dart';
import 'package:anixops_mobile/core/services/tasks_api.dart';
import 'package:anixops_mobile/core/services/mfa_api.dart';
import 'package:anixops_mobile/core/services/schedules_api.dart';
import 'package:anixops_mobile/core/services/notifications_api.dart';
import 'package:anixops_mobile/core/services/backup_api.dart';

/// Central API client providing access to all API services
class ApiClient {
  late final Dio _dio;
  late final AuthApi auth;
  late final NodesApi nodes;
  late final UsersApi users;
  late final PluginsApi plugins;
  late final SshApi ssh;
  late final PlaybooksApi playbooks;
  late final TasksApi tasks;
  late final MFAApi mfa;
  late final SchedulesApi schedules;
  late final TokensApi tokens;
  late final SessionsApi sessions;
  late final NotificationsApi notifications;
  late final BackupApi backup;

  // Cloud API endpoint
  static const String defaultBaseUrl = 'https://api.anixops.com/api/v1';

  ApiClient({
    String baseUrl = defaultBaseUrl,
    Duration connectTimeout = const Duration(seconds: 30),
    Duration receiveTimeout = const Duration(seconds: 30),
  }) {
    _dio = Dio(BaseOptions(
      baseUrl: baseUrl,
      connectTimeout: connectTimeout,
      receiveTimeout: receiveTimeout,
      headers: {
        'Content-Type': 'application/json',
      },
    ));

    // Add interceptors
    _dio.interceptors.add(InterceptorsWrapper(
      onRequest: (options, handler) {
        // Add auth token if available
        // final token = _getToken();
        // if (token != null) {
        //   options.headers['Authorization'] = 'Bearer $token';
        // }
        return handler.next(options);
      },
      onResponse: (response, handler) {
        return handler.next(response);
      },
      onError: (error, handler) {
        if (error.response?.statusCode == 401) {
          // Handle unauthorized
        }
        return handler.next(error);
      },
    ));

    // Initialize API services
    auth = AuthApi(_dio);
    nodes = NodesApi(_dio);
    users = UsersApi(_dio);
    plugins = PluginsApi(_dio);
    ssh = SshApi(_dio);
    playbooks = PlaybooksApi(_dio);
    tasks = TasksApi(_dio);
    mfa = MFAApi(_dio);
    schedules = SchedulesApi(_dio);
    tokens = TokensApi(_dio);
    sessions = SessionsApi(_dio);
    notifications = NotificationsApi(_dio);
    backup = BackupApi(_dio);
  }

  /// Update base URL
  void setBaseUrl(String url) {
    _dio.options.baseUrl = url;
  }

  /// Set authentication token
  void setAuthToken(String token) {
    _dio.options.headers['Authorization'] = 'Bearer $token';
  }

  /// Clear authentication token
  void clearAuthToken() {
    _dio.options.headers.remove('Authorization');
  }

  /// Get raw Dio instance for custom requests
  Dio get dio => _dio;
}

/// Provider for global API client
final ApiClient apiClient = ApiClient();